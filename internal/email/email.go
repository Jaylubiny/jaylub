package email

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const MailboxDomain = "jaylub.com"

var (
	ErrNotFound      = errors.New("email not found")
	ErrInvalidFolder = errors.New("invalid mailbox folder")
)

type Email struct {
	ID             int64  `json:"id"`
	Sender         string `json:"sender"`
	Recipient      string `json:"recipient"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
	Timestamp      string `json:"timestamp"`
	Folder         string `json:"folder"`
	Read           bool   `json:"read"`
	PreviousFolder string `json:"previous_folder,omitempty"`
}

func MailboxAddress(username string) string {
	return strings.ToLower(strings.TrimSpace(username)) + "@" + MailboxDomain
}

type Sender interface {
	Send(from, to, subject, body string) error
}

type Service struct {
	db            *sql.DB
	sender        Sender
	webhookSecret string
}

func New(dbPath, resendAPIKey, _ string, webhookSecret string) (*Service, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	service := &Service{
		db:            db,
		sender:        resendSender{apiKey: resendAPIKey, client: &http.Client{Timeout: 20 * time.Second}},
		webhookSecret: webhookSecret,
	}
	if err := service.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return service, nil
}

func NewWithSender(dbPath string, sender Sender, webhookSecret string) (*Service, error) {
	service, err := New(dbPath, "", "", webhookSecret)
	if err != nil {
		return nil, err
	}
	service.sender = sender
	return service, nil
}

func (s *Service) Close() error {
	return s.db.Close()
}

func (s *Service) initSchema() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS emails (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sender TEXT NOT NULL,
			recipient TEXT NOT NULL,
			subject TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '',
			timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			folder TEXT NOT NULL DEFAULT 'inbox',
			is_read INTEGER NOT NULL DEFAULT 0,
			previous_folder TEXT
		)
	`); err != nil {
		return err
	}

	for _, column := range []struct {
		name       string
		definition string
	}{
		{"folder", "TEXT NOT NULL DEFAULT 'inbox'"},
		{"is_read", "INTEGER NOT NULL DEFAULT 0"},
		{"previous_folder", "TEXT"},
	} {
		if err := s.addColumnIfMissing(column.name, column.definition); err != nil {
			return err
		}
	}

	_, err := s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_emails_recipient_folder ON emails(recipient, folder, timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_emails_sender_folder ON emails(sender, folder, timestamp DESC)
	`)
	return err
}

func (s *Service) addColumnIfMissing(name, definition string) error {
	rows, err := s.db.Query("PRAGMA table_info(emails)")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var columnName, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		if columnName == name {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = s.db.Exec(fmt.Sprintf("ALTER TABLE emails ADD COLUMN %s %s", name, definition))
	return err
}

func (s *Service) SaveIncoming(sender, recipient, subject, body string) error {
	_, err := s.db.Exec(`
		INSERT INTO emails (sender, recipient, subject, body, folder, is_read)
		VALUES (?, ?, ?, ?, 'inbox', 0)
	`, strings.TrimSpace(sender), strings.TrimSpace(recipient), subject, body)
	return err
}

func (s *Service) GetMailboxEmails(owner, folder string) ([]Email, error) {
	if folder != "inbox" && folder != "sent" && folder != "trash" {
		return nil, ErrInvalidFolder
	}

	rows, err := s.db.Query(`
		SELECT id, sender, recipient, subject, body, timestamp, folder, is_read, COALESCE(previous_folder, '')
		FROM emails
		WHERE folder = ?
			AND (
				(CASE WHEN folder = 'sent' OR (folder = 'trash' AND previous_folder = 'sent')
					THEN lower(sender) ELSE lower(recipient) END) = lower(?)
			)
		ORDER BY timestamp DESC, id DESC
	`, folder, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	emails := make([]Email, 0)
	for rows.Next() {
		var e Email
		var read int
		if err := rows.Scan(&e.ID, &e.Sender, &e.Recipient, &e.Subject, &e.Body, &e.Timestamp, &e.Folder, &read, &e.PreviousFolder); err != nil {
			return nil, err
		}
		e.Read = read != 0
		emails = append(emails, e)
	}
	return emails, rows.Err()
}

func (s *Service) SendEmail(from, to, subject, body string) error {
	if s.sender == nil {
		return errors.New("email sender is not configured")
	}
	if err := s.sender.Send(from, to, subject, body); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO emails (sender, recipient, subject, body, folder, is_read)
		VALUES (?, ?, ?, ?, 'sent', 1)
	`, from, to, subject, body)
	return err
}

func (s *Service) MarkRead(owner string, id int64, read bool) error {
	result, err := s.db.Exec(`
		UPDATE emails SET is_read = ?
		WHERE id = ? AND (
			(CASE WHEN folder = 'sent' OR (folder = 'trash' AND previous_folder = 'sent')
				THEN lower(sender) ELSE lower(recipient) END) = lower(?)
		)
	`, read, id, owner)
	return affected(result, err)
}

func (s *Service) MoveToTrash(owner string, id int64) error {
	result, err := s.db.Exec(`
		UPDATE emails
		SET previous_folder = folder, folder = 'trash'
		WHERE id = ? AND folder IN ('inbox', 'sent')
			AND (CASE WHEN folder = 'sent' THEN lower(sender) ELSE lower(recipient) END) = lower(?)
	`, id, owner)
	return affected(result, err)
}

func (s *Service) Restore(owner string, id int64) error {
	result, err := s.db.Exec(`
		UPDATE emails
		SET folder = COALESCE(NULLIF(previous_folder, ''), 'inbox'), previous_folder = NULL
		WHERE id = ? AND folder = 'trash'
			AND (CASE WHEN previous_folder = 'sent' THEN lower(sender) ELSE lower(recipient) END) = lower(?)
	`, id, owner)
	return affected(result, err)
}

func (s *Service) DeleteFromTrash(owner string, id int64) error {
	result, err := s.db.Exec(`
		DELETE FROM emails
		WHERE id = ? AND folder = 'trash'
			AND (CASE WHEN previous_folder = 'sent' THEN lower(sender) ELSE lower(recipient) END) = lower(?)
	`, id, owner)
	return affected(result, err)
}

func affected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ValidateSecret(secret string) bool {
	return s.webhookSecret != "" && s.webhookSecret == secret
}

type resendSender struct {
	apiKey string
	client *http.Client
}

func (s resendSender) Send(from, to, subject, body string) error {
	if strings.TrimSpace(s.apiKey) == "" {
		return errors.New("email sending is not configured")
	}
	fromAddress, err := mail.ParseAddress(from)
	if err != nil || !strings.HasSuffix(strings.ToLower(fromAddress.Address), "@"+MailboxDomain) {
		return errors.New("sender address is invalid")
	}
	toAddress, err := mail.ParseAddress(to)
	if err != nil {
		return errors.New("recipient address is invalid")
	}

	payload, err := json.Marshal(struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text"`
	}{
		From: from, To: []string{toAddress.Address}, Subject: subject, Text: body,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("email provider request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("email provider returned HTTP %d", response.StatusCode)
	}
	return nil
}
