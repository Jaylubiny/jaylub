package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jaylub/internal/auth"
)

func TestSelfChatIsIsolatedAndLeavesGlobalMessagesUntouched(t *testing.T) {
	service, err := auth.New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer service.Close()

	userIDs := make(map[string]int64)
	tokens := map[string]string{"owner": "owner-session", "other": "other-session"}
	for username, token := range tokens {
		result, err := service.DB().Exec(
			`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
			username, "unused",
		)
		if err != nil {
			t.Fatalf("insert user %q: %v", username, err)
		}
		userID, err := result.LastInsertId()
		if err != nil {
			t.Fatalf("get user ID for %q: %v", username, err)
		}
		userIDs[username] = userID

		tokenHash := sha256.Sum256([]byte(token))
		if _, err := service.DB().Exec(
			`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
			userID, hex.EncodeToString(tokenHash[:]), time.Now().Add(time.Hour),
		); err != nil {
			t.Fatalf("insert session for %q: %v", username, err)
		}
	}

	if _, err := service.DB().Exec(
		`INSERT INTO chat_messages (username, message) VALUES (?, ?)`,
		"other", "existing global message",
	); err != nil {
		t.Fatalf("insert existing global message: %v", err)
	}

	chat := NewChatService(service.DB())
	chat.lastCleanup = time.Now()
	chat.filesDir = filepath.Join(t.TempDir(), "files")
	middleware := service.Middleware

	doRequest := func(method, target, body, token string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, target, strings.NewReader(body))
		request.AddCookie(&http.Cookie{Name: "jaylub_session", Value: token})
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		middleware(handler).ServeHTTP(response, request)
		return response
	}

	sendResponse := doRequest(
		http.MethodPost,
		"/chat/send?channel=self",
		`{"message":"private note"}`,
		tokens["owner"],
		chat.SendMessage,
	)
	if sendResponse.Code != http.StatusOK {
		t.Fatalf("self-chat send status = %d, body = %q", sendResponse.Code, sendResponse.Body.String())
	}
	var selfMessageID int64
	if err := service.DB().QueryRow(
		`SELECT id FROM self_chat_messages WHERE user_id = ?`,
		userIDs["owner"],
	).Scan(&selfMessageID); err != nil {
		t.Fatalf("get self-chat message ID: %v", err)
	}
	if err := os.MkdirAll(chat.filesDir, 0700); err != nil {
		t.Fatalf("create self-chat files directory: %v", err)
	}
	const selfAttachmentName = "private-self-chat-file"
	if err := os.WriteFile(filepath.Join(chat.filesDir, selfAttachmentName), []byte("private file"), 0600); err != nil {
		t.Fatalf("write self-chat attachment: %v", err)
	}
	if _, err := service.DB().Exec(`
		INSERT INTO self_chat_attachments (message_id, original_name, stored_name, content_type, size)
		VALUES (?, ?, ?, ?, ?)
	`, selfMessageID, "note.txt", selfAttachmentName, "text/plain", 12); err != nil {
		t.Fatalf("insert self-chat attachment: %v", err)
	}

	readMessages := func(token, channel string) []ChatMessage {
		response := doRequest(http.MethodGet, "/chat/messages?channel="+channel, "", token, chat.Messages)
		if response.Code != http.StatusOK {
			t.Fatalf("read %s channel status = %d, body = %q", channel, response.Code, response.Body.String())
		}
		var payload struct {
			Messages []ChatMessage `json:"messages"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("decode %s messages: %v", channel, err)
		}
		return payload.Messages
	}

	ownerSelfMessages := readMessages(tokens["owner"], "self")
	if len(ownerSelfMessages) != 1 || ownerSelfMessages[0].Message != "private note" {
		t.Fatalf("owner self messages = %#v, want only private note", ownerSelfMessages)
	}
	if len(ownerSelfMessages[0].Attachments) != 1 || ownerSelfMessages[0].Attachments[0].URL != "/chat/files/1?channel=self" {
		t.Fatalf("owner self-message attachments = %#v, want private-channel file URL", ownerSelfMessages[0].Attachments)
	}
	if otherSelfMessages := readMessages(tokens["other"], "self"); len(otherSelfMessages) != 0 {
		t.Fatalf("another user's self messages = %#v, want none", otherSelfMessages)
	}
	otherFileResponse := doRequest(http.MethodGet, "/chat/files/1?channel=self", "", tokens["other"], chat.File)
	if otherFileResponse.Code != http.StatusNotFound {
		t.Fatalf("another user's self attachment status = %d, want 404", otherFileResponse.Code)
	}
	ownerFileResponse := doRequest(http.MethodGet, "/chat/files/1?channel=self", "", tokens["owner"], chat.File)
	if ownerFileResponse.Code != http.StatusOK || ownerFileResponse.Body.String() != "private file" {
		t.Fatalf("owner self attachment response = (%d, %q), want private file", ownerFileResponse.Code, ownerFileResponse.Body.String())
	}
	globalMessages := readMessages(tokens["other"], "global")
	if len(globalMessages) != 1 || globalMessages[0].Message != "existing global message" {
		t.Fatalf("global messages = %#v, want original message unchanged", globalMessages)
	}

	var globalCount, selfCount int
	if err := service.DB().QueryRow(`SELECT COUNT(*) FROM chat_messages`).Scan(&globalCount); err != nil {
		t.Fatal(err)
	}
	if err := service.DB().QueryRow(`SELECT COUNT(*) FROM self_chat_messages WHERE user_id = ?`, userIDs["owner"]).Scan(&selfCount); err != nil {
		t.Fatal(err)
	}
	if globalCount != 1 || selfCount != 1 {
		t.Fatalf("stored message counts: global=%d self=%d; want 1 each", globalCount, selfCount)
	}
}

func TestParseChatSubmissionSpillsLargeUploadToDisk(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("message", "attachment"); err != nil {
		t.Fatalf("write message field: %v", err)
	}
	fileWriter, err := writer.CreateFormFile("file", "large.bin")
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := fileWriter.Write(make([]byte, chatMultipartMemory+1)); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/chat/send", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	message, upload, err := parseChatSubmission(response, request)
	if err != nil {
		t.Fatalf("parse multipart submission: %v", err)
	}
	if message != "attachment" || upload == nil {
		t.Fatalf("parsed submission = (%q, %#v), want message and attachment", message, upload)
	}
	defer request.MultipartForm.RemoveAll()

	file, err := upload.header.Open()
	if err != nil {
		t.Fatalf("open parsed attachment: %v", err)
	}
	defer file.Close()
	if _, ok := file.(*os.File); !ok {
		t.Fatalf("large attachment is held in memory (%T); want a temporary file", file)
	}
}

func TestDetectChatContentTypeSupportsCommonImageFormats(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "PNG", data: []byte("\x89PNG\r\n\x1a\n"), want: "image/png"},
		{name: "JPEG", data: []byte("\xff\xd8\xff\xe0\x00\x10JFIF"), want: "image/jpeg"},
		{name: "GIF", data: []byte("GIF89a"), want: "image/gif"},
		{name: "BMP", data: []byte("BM\x00\x00\x00\x00"), want: "image/bmp"},
		{name: "WebP", data: []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), want: "image/webp"},
		{name: "TIFF little endian", data: []byte{'I', 'I', 42, 0}, want: "image/tiff"},
		{name: "AVIF", data: []byte("\x00\x00\x00\x18ftypavif"), want: "image/avif"},
		{name: "SVG", data: []byte(`<?xml version="1.0"?><!-- icon --><svg xmlns="http://www.w3.org/2000/svg"></svg>`), want: "image/svg+xml"},
		{name: "non-image XML", data: []byte(`<root></root>`), want: "text/plain; charset=utf-8"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := detectChatContentType(test.data); got != test.want {
				t.Errorf("detectChatContentType() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSVGChatAttachmentIsInlineWithSandboxPolicy(t *testing.T) {
	service, err := auth.New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer service.Close()

	result, err := service.DB().Exec(
		`INSERT INTO chat_messages (username, message) VALUES (?, ?)`,
		"user", "",
	)
	if err != nil {
		t.Fatalf("insert chat message: %v", err)
	}
	messageID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get message ID: %v", err)
	}

	filesDir := filepath.Join(t.TempDir(), "files")
	if err := os.MkdirAll(filesDir, 0700); err != nil {
		t.Fatalf("create files directory: %v", err)
	}
	const storedName = "vector-image"
	if err := os.WriteFile(filepath.Join(filesDir, storedName), []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0600); err != nil {
		t.Fatalf("write SVG attachment: %v", err)
	}
	if _, err := service.DB().Exec(`
		INSERT INTO chat_attachments (message_id, original_name, stored_name, content_type, size)
		VALUES (?, ?, ?, ?, ?)
	`, messageID, "vector.svg", storedName, "image/svg+xml", 46); err != nil {
		t.Fatalf("insert SVG attachment: %v", err)
	}

	chat := NewChatService(service.DB())
	chat.filesDir = filesDir
	response := httptest.NewRecorder()
	chat.File(response, httptest.NewRequest(http.MethodGet, "/chat/files/1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("SVG file response status = %d", response.Code)
	}
	if got := response.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") {
		t.Errorf("SVG Content-Disposition = %q, want inline", got)
	}
	if got := response.Header().Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") {
		t.Errorf("SVG Content-Security-Policy = %q, want sandbox policy", got)
	}
}

func TestValidateChatMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "trims whitespace", input: "  hello  ", want: "hello"},
		{name: "rejects empty", input: " \n\t ", wantErr: true},
		{name: "rejects long message", input: string(make([]rune, chatMaxMessageLength+1)), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := validateChatMessage(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateChatMessage() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("validateChatMessage() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestOnlineUsersReturnsActiveUsersAndExpiresInactiveUsers(t *testing.T) {
	chat := NewChatService(nil)
	now := time.Now()
	chat.lastActive = map[string]time.Time{
		"bravo": now.Add(-time.Second),
		"alpha": now.Add(-chatOnlineWindow + time.Second),
		"stale": now.Add(-chatOnlineWindow - time.Second),
	}

	users := chat.onlineUsers()
	if len(users) != 2 || users[0] != "alpha" || users[1] != "bravo" {
		t.Fatalf("onlineUsers() = %v, want sorted active users [alpha bravo]", users)
	}
	if _, ok := chat.lastActive["stale"]; ok {
		t.Fatal("onlineUsers() did not remove stale user")
	}
}

func TestCleanupExpiredRemovesAttachmentMetadataAndFiles(t *testing.T) {
	t.Parallel()

	service, err := auth.New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer service.Close()

	filesDir := filepath.Join(t.TempDir(), "files")
	if err := os.MkdirAll(filesDir, 0700); err != nil {
		t.Fatalf("create files directory: %v", err)
	}
	const storedName = "expired-attachment"
	filePath := filepath.Join(filesDir, storedName)
	if err := os.WriteFile(filePath, []byte("expired"), 0600); err != nil {
		t.Fatalf("create expired file: %v", err)
	}

	expiredTimestamp := time.Now().UTC().Add(-chatRetention - time.Hour).Format(time.RFC3339)
	result, err := service.DB().Exec(
		`INSERT INTO chat_messages (username, message, timestamp) VALUES (?, ?, ?)`,
		"user", "", expiredTimestamp,
	)
	if err != nil {
		t.Fatalf("insert expired message: %v", err)
	}
	messageID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get message ID: %v", err)
	}
	if _, err := service.DB().Exec(
		`INSERT INTO chat_attachments (message_id, original_name, stored_name, content_type, size) VALUES (?, ?, ?, ?, ?)`,
		messageID, "expired.png", storedName, "image/png", 7,
	); err != nil {
		t.Fatalf("insert expired attachment: %v", err)
	}

	chat := NewChatService(service.DB())
	chat.filesDir = filesDir
	if err := chat.cleanupExpired(); err != nil {
		t.Fatalf("cleanup expired chat: %v", err)
	}

	for _, table := range []string{"chat_messages", "chat_attachments"} {
		var count int
		if err := service.DB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count rows in %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("%s contains %d expired rows; want 0", table, count)
		}
	}
	if _, err := os.Stat(filePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expired attachment file still exists or could not be checked: %v", err)
	}
}

func TestRecentMessagesLoadsAttachmentsInBatch(t *testing.T) {
	t.Parallel()

	service, err := auth.New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer service.Close()

	result, err := service.DB().Exec(
		`INSERT INTO chat_messages (username, message) VALUES (?, ?)`,
		"user", "message",
	)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
	messageID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get message ID: %v", err)
	}
	if _, err := service.DB().Exec(
		`INSERT INTO chat_attachments (message_id, original_name, stored_name, content_type, size) VALUES (?, ?, ?, ?, ?)`,
		messageID, "image.png", "stored-image", "image/png", 42,
	); err != nil {
		t.Fatalf("insert attachment: %v", err)
	}

	messages, err := NewChatService(service.DB()).recentMessages(0)
	if err != nil {
		t.Fatalf("load recent messages: %v", err)
	}
	if len(messages) != 1 || len(messages[0].Attachments) != 1 {
		t.Fatalf("loaded messages = %#v, want one message with one attachment", messages)
	}
	if got := messages[0].Attachments[0].URL; got != "/chat/files/1" {
		t.Errorf("attachment URL = %q, want /chat/files/1", got)
	}
}
