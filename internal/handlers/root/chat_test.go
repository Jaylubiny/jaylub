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
	"sync"
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
	deviceTokens := make(map[string]*http.Cookie)
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
		termsRequest := httptest.NewRequest(http.MethodPost, "https://jaylub.com/terms", nil)
		termsResponse := httptest.NewRecorder()
		if err := service.AcceptTermsOnDevice(termsResponse, termsRequest); err != nil {
			t.Fatalf("accept terms on device for %q: %v", username, err)
		}
		deviceTokens[token] = termsResponse.Result().Cookies()[0]

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
		request.AddCookie(deviceTokens[token])
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

func TestCoinGiftsDebitCreditAndClaimCardsOnlyOnce(t *testing.T) {
	service, err := auth.New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer service.Close()

	accounts := []struct {
		user auth.User
		gold int
	}{
		{user: auth.User{Username: "giver"}, gold: 100},
		{user: auth.User{Username: "receiver"}, gold: 0},
		{user: auth.User{Username: "claimer"}, gold: 0},
	}
	for index := range accounts {
		result, err := service.DB().Exec(
			`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
			accounts[index].user.Username, "unused",
		)
		if err != nil {
			t.Fatalf("insert %s: %v", accounts[index].user.Username, err)
		}
		accounts[index].user.ID, err = result.LastInsertId()
		if err != nil {
			t.Fatalf("get %s ID: %v", accounts[index].user.Username, err)
		}
		if _, err := service.DB().Exec(
			`INSERT INTO game_profiles (user_id, username, gold) VALUES (?, ?, ?)`,
			accounts[index].user.ID, accounts[index].user.Username, accounts[index].gold,
		); err != nil {
			t.Fatalf("create %s game profile: %v", accounts[index].user.Username, err)
		}
	}

	chat := NewChatService(service.DB())
	if _, err := chat.createCoinGift(accounts[0].user, "direct", 30, accounts[1].user.ID); err != nil {
		t.Fatalf("create direct gift: %v", err)
	}
	if _, err := chat.createCoinGift(accounts[0].user, "card", 25, 0); err != nil {
		t.Fatalf("create gift card: %v", err)
	}

	var giverGold, receiverGold int
	if err := service.DB().QueryRow(`SELECT gold FROM game_profiles WHERE user_id = ?`, accounts[0].user.ID).Scan(&giverGold); err != nil {
		t.Fatal(err)
	}
	if err := service.DB().QueryRow(`SELECT gold FROM game_profiles WHERE user_id = ?`, accounts[1].user.ID).Scan(&receiverGold); err != nil {
		t.Fatal(err)
	}
	if giverGold != 45 || receiverGold != 30 {
		t.Fatalf("balances after gifts: giver=%d receiver=%d; want 45 and 30", giverGold, receiverGold)
	}

	var giftID int64
	if err := service.DB().QueryRow(`SELECT id FROM chat_coin_gifts WHERE gift_type = 'card'`).Scan(&giftID); err != nil {
		t.Fatalf("get card ID: %v", err)
	}
	if _, err := chat.claimCoinGift(accounts[0].user, giftID); !errors.Is(err, errOwnGiftCard) {
		t.Fatalf("sender self-claim error = %v, want cannot claim own card", err)
	}
	if _, err := chat.claimCoinGift(accounts[2].user, giftID); err != nil {
		t.Fatalf("claim gift card: %v", err)
	}
	if _, err := chat.claimCoinGift(accounts[1].user, giftID); !errors.Is(err, errGiftAlreadyClaimed) {
		t.Fatalf("second claim error = %v, want already claimed", err)
	}

	var claimerGold int
	if err := service.DB().QueryRow(`SELECT gold FROM game_profiles WHERE user_id = ?`, accounts[2].user.ID).Scan(&claimerGold); err != nil {
		t.Fatal(err)
	}
	if claimerGold != 25 {
		t.Fatalf("claimer balance = %d, want exactly 25", claimerGold)
	}

	messages, err := chat.recentMessages(0)
	if err != nil {
		t.Fatalf("load gift events in global chat: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("global chat message count = %d, want direct gift, card, and claim events", len(messages))
	}
	if messages[0].Gift == nil || messages[0].Gift.Type != "direct" || messages[0].Gift.RecipientUsername != "receiver" {
		t.Errorf("direct gift event = %#v", messages[0].Gift)
	}
	if messages[1].Gift == nil || messages[1].Gift.Type != "card" || !messages[1].Gift.Claimed || messages[1].Gift.ClaimedByUsername != "claimer" {
		t.Errorf("gift card event = %#v", messages[1].Gift)
	}
	if messages[2].Gift == nil || messages[2].Gift.Type != "claim" || messages[2].Gift.RecipientUsername != "claimer" {
		t.Errorf("claim event = %#v", messages[2].Gift)
	}

	if _, err := chat.createCoinGift(accounts[1].user, "direct", 31, accounts[0].user.ID); !errors.Is(err, errInsufficientGiftCoins) {
		t.Fatalf("overspend gift error = %v, want insufficient funds", err)
	}
	var messageCount int
	if err := service.DB().QueryRow(`SELECT COUNT(*) FROM chat_messages`).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if messageCount != 3 {
		t.Fatalf("failed gift created %d chat events, want no additional transaction", messageCount)
	}

	if _, err := chat.createCoinGift(accounts[0].user, "card", 10, 0); err != nil {
		t.Fatalf("create expiring gift card: %v", err)
	}
	var expiredGiftID int64
	if err := service.DB().QueryRow(`SELECT id FROM chat_coin_gifts WHERE amount = 10`).Scan(&expiredGiftID); err != nil {
		t.Fatalf("get expiring gift card ID: %v", err)
	}
	expiredAt := time.Now().UTC().Add(-chatRetention - time.Second).Format(time.RFC3339)
	if _, err := service.DB().Exec(`UPDATE chat_coin_gifts SET created_at = ? WHERE id = ?`, expiredAt, expiredGiftID); err != nil {
		t.Fatalf("expire gift card: %v", err)
	}
	if _, err := chat.claimCoinGift(accounts[1].user, expiredGiftID); !errors.Is(err, errGiftExpired) {
		t.Fatalf("expired gift claim error = %v, want expired", err)
	}
}

func TestConcurrentGiftCardClaimsCreditOnlyOneUser(t *testing.T) {
	service, err := auth.New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer service.Close()

	users := make([]auth.User, 3)
	for index, username := range []string{"sender", "first", "second"} {
		result, err := service.DB().Exec(`INSERT INTO users (username, password_hash) VALUES (?, ?)`, username, "unused")
		if err != nil {
			t.Fatalf("insert %s: %v", username, err)
		}
		users[index] = auth.User{Username: username}
		users[index].ID, err = result.LastInsertId()
		if err != nil {
			t.Fatalf("get %s ID: %v", username, err)
		}
		if _, err := service.DB().Exec(
			`INSERT INTO game_profiles (user_id, username, gold) VALUES (?, ?, ?)`,
			users[index].ID, username, 100,
		); err != nil {
			t.Fatalf("create %s game profile: %v", username, err)
		}
	}

	chat := NewChatService(service.DB())
	if _, err := chat.createCoinGift(users[0], "card", 40, 0); err != nil {
		t.Fatalf("create gift card: %v", err)
	}
	var giftID int64
	if err := service.DB().QueryRow(`SELECT id FROM chat_coin_gifts`).Scan(&giftID); err != nil {
		t.Fatalf("get gift card ID: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, user := range users[1:] {
		wait.Add(1)
		go func(user auth.User) {
			defer wait.Done()
			<-start
			_, err := chat.claimCoinGift(user, giftID)
			results <- err
		}(user)
	}
	close(start)
	wait.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, errGiftAlreadyClaimed) {
			t.Errorf("concurrent claim error = %v, want success or already claimed", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent claims = %d, want exactly one", successes)
	}

	var totalRecipientGold int
	if err := service.DB().QueryRow(
		`SELECT SUM(gold) FROM game_profiles WHERE user_id IN (?, ?)`,
		users[1].ID, users[2].ID,
	).Scan(&totalRecipientGold); err != nil {
		t.Fatalf("sum recipient balances: %v", err)
	}
	if totalRecipientGold != 240 {
		t.Fatalf("combined recipient gold = %d, want initial 200 plus one 40-coin claim", totalRecipientGold)
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

func TestCleanupExpiredRefundsAndDeletesUnclaimedGiftCardsOnce(t *testing.T) {
	t.Parallel()

	service, err := auth.New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer service.Close()

	result, err := service.DB().Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		"gift-sender", "unused",
	)
	if err != nil {
		t.Fatalf("insert gift sender: %v", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get gift sender ID: %v", err)
	}
	if _, err := service.DB().Exec(
		`INSERT INTO game_profiles (user_id, username, gold) VALUES (?, ?, ?)`,
		userID, "gift-sender", 15,
	); err != nil {
		t.Fatalf("create sender profile: %v", err)
	}
	oldTimestamp := time.Now().UTC().Add(-chatRetention - time.Hour).Format(time.RFC3339)
	result, err = service.DB().Exec(
		`INSERT INTO chat_messages (username, message, timestamp) VALUES (?, ?, ?)`,
		"gift-sender", "expired gift card", oldTimestamp,
	)
	if err != nil {
		t.Fatalf("insert expired card message: %v", err)
	}
	messageID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get card message ID: %v", err)
	}
	if _, err := service.DB().Exec(`
		INSERT INTO chat_coin_gifts (message_id, gift_type, amount, sender_user_id, sender_username, created_at)
		VALUES (?, 'card', ?, ?, ?, ?)
	`, messageID, 40, userID, "gift-sender", oldTimestamp); err != nil {
		t.Fatalf("insert expired gift card: %v", err)
	}

	chat := NewChatService(service.DB())
	chat.filesDir = filepath.Join(t.TempDir(), "files")
	if err := chat.cleanupExpired(); err != nil {
		t.Fatalf("cleanup expired chat: %v", err)
	}

	var gold, giftCount int
	if err := service.DB().QueryRow(`SELECT gold FROM game_profiles WHERE user_id = ?`, userID).Scan(&gold); err != nil {
		t.Fatalf("read refunded balance: %v", err)
	}
	if err := service.DB().QueryRow(`SELECT COUNT(*) FROM chat_coin_gifts`).Scan(&giftCount); err != nil {
		t.Fatalf("count remaining gift cards: %v", err)
	}
	if gold != 55 || giftCount != 0 {
		t.Fatalf("after cleanup: gold=%d gift records=%d; want 55 and 0", gold, giftCount)
	}

	chat.lastCleanup = time.Time{}
	if err := chat.cleanupExpired(); err != nil {
		t.Fatalf("repeat cleanup: %v", err)
	}
	if err := service.DB().QueryRow(`SELECT gold FROM game_profiles WHERE user_id = ?`, userID).Scan(&gold); err != nil {
		t.Fatalf("read balance after repeat cleanup: %v", err)
	}
	if gold != 55 {
		t.Fatalf("repeated cleanup refunded twice: gold=%d, want 55", gold)
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
