package handlers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"jaylub/internal/auth"
)

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
