package email

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

type testSender struct {
	from, to, subject, body string
	err                     error
}

func (s *testSender) Send(from, to, subject, body string) error {
	s.from, s.to, s.subject, s.body = from, to, subject, body
	return s.err
}

func TestOldDatabaseMigrationAndMailboxIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "emails.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE emails (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sender TEXT,
			recipient TEXT,
			subject TEXT,
			body TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO emails (sender, recipient, subject, body)
		VALUES ('outside@example.com', 'alice@jaylub.com', 'For Alice', 'private');
		INSERT INTO emails (sender, recipient, subject, body)
		VALUES ('outside@example.com', 'bob@jaylub.com', 'For Bob', 'private');
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	sender := &testSender{}
	service, err := NewWithSender(path, sender, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	aliceEmails, err := service.GetMailboxEmails("alice@jaylub.com", "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceEmails) != 1 || aliceEmails[0].Subject != "For Alice" || aliceEmails[0].Read {
		t.Fatalf("unexpected Alice inbox: %#v", aliceEmails)
	}

	if err := service.SendEmail("alice@jaylub.com", "recipient@example.com", "Hello", "Body"); err != nil {
		t.Fatal(err)
	}
	if sender.from != "alice@jaylub.com" || sender.to != "recipient@example.com" {
		t.Fatalf("sender received unexpected message: %#v", sender)
	}
	sent, err := service.GetMailboxEmails("alice@jaylub.com", "sent")
	if err != nil || len(sent) != 1 || sent[0].Folder != "sent" {
		t.Fatalf("unexpected sent mailbox: %#v, %v", sent, err)
	}
	if _, err := service.GetMailboxEmails("bob@jaylub.com", "sent"); err != nil {
		t.Fatal(err)
	}
	bobSent, err := service.GetMailboxEmails("bob@jaylub.com", "sent")
	if err != nil || len(bobSent) != 0 {
		t.Fatalf("sent mail leaked to another user: %#v, %v", bobSent, err)
	}
	if err := service.MoveToTrash("alice@jaylub.com", sent[0].ID); err != nil {
		t.Fatal(err)
	}
	sentTrash, err := service.GetMailboxEmails("alice@jaylub.com", "trash")
	if err != nil || len(sentTrash) != 1 || sentTrash[0].PreviousFolder != "sent" {
		t.Fatalf("sent message did not retain its folder for restore: %#v, %v", sentTrash, err)
	}
	if err := service.Restore("alice@jaylub.com", sentTrash[0].ID); err != nil {
		t.Fatal(err)
	}

	if err := service.MarkRead("bob@jaylub.com", aliceEmails[0].ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected cross-user read update to be rejected, got %v", err)
	}
	if err := service.MarkRead("alice@jaylub.com", aliceEmails[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if err := service.MoveToTrash("alice@jaylub.com", aliceEmails[0].ID); err != nil {
		t.Fatal(err)
	}
	trash, err := service.GetMailboxEmails("alice@jaylub.com", "trash")
	if err != nil || len(trash) != 1 || trash[0].Folder != "trash" || !trash[0].Read {
		t.Fatalf("unexpected trash mailbox: %#v, %v", trash, err)
	}
	if err := service.Restore("alice@jaylub.com", trash[0].ID); err != nil {
		t.Fatal(err)
	}
	inbox, err := service.GetMailboxEmails("alice@jaylub.com", "inbox")
	if err != nil || len(inbox) != 1 || !inbox[0].Read {
		t.Fatalf("unexpected restored inbox: %#v, %v", inbox, err)
	}
}

func TestTrashCanBePermanentlyDeletedOnlyByMailboxOwner(t *testing.T) {
	service, err := NewWithSender(filepath.Join(t.TempDir(), "emails.db"), &testSender{}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	if err := service.SaveIncoming("sender@example.com", "alice@jaylub.com", "Subject", "Body"); err != nil {
		t.Fatal(err)
	}
	emails, err := service.GetMailboxEmails("alice@jaylub.com", "inbox")
	if err != nil || len(emails) != 1 {
		t.Fatalf("unexpected inbox: %#v, %v", emails, err)
	}
	if err := service.MoveToTrash("alice@jaylub.com", emails[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteFromTrash("bob@jaylub.com", emails[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected cross-user delete to be rejected, got %v", err)
	}
	if err := service.DeleteFromTrash("alice@jaylub.com", emails[0].ID); err != nil {
		t.Fatal(err)
	}
	trash, err := service.GetMailboxEmails("alice@jaylub.com", "trash")
	if err != nil || len(trash) != 0 {
		t.Fatalf("expected empty trash after delete: %#v, %v", trash, err)
	}
}

func TestSendFailureDoesNotCreateSentRecord(t *testing.T) {
	sender := &testSender{err: errors.New("provider unavailable")}
	service, err := NewWithSender(filepath.Join(t.TempDir(), "emails.db"), sender, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	if err := service.SendEmail("alice@jaylub.com", "recipient@example.com", "Subject", "Body"); err == nil {
		t.Fatal("expected send error")
	}
	sent, err := service.GetMailboxEmails("alice@jaylub.com", "sent")
	if err != nil || len(sent) != 0 {
		t.Fatalf("failed send was recorded as sent: %#v, %v", sent, err)
	}
}
