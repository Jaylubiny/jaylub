package email

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const MailboxDomain = "jaylub.com"

const (
	MaxIncomingAttachmentBytes       = 10 << 20
	MaxIncomingAttachmentsTotalBytes = 25 << 20
	MaxIncomingAttachments           = 10
)

var (
	ErrNotFound           = errors.New("email not found")
	ErrInvalidFolder      = errors.New("invalid mailbox folder")
	ErrInvalidAttachment  = errors.New("invalid email attachment")
	ErrAttachmentTooLarge = errors.New("email attachment exceeds the size limit")
	ErrTooManyAttachments = errors.New("too many email attachments")
)

type EmailAttachment struct {
	ID       int64  `json:"id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MIMEType string `json:"mimeType"`
	URL      string `json:"url"`
}

type IncomingAttachment struct {
	Filename string
	MIMEType string
	Size     int64
	Data     []byte
}

type Email struct {
	ID             int64             `json:"id"`
	Sender         string            `json:"sender"`
	Recipient      string            `json:"recipient"`
	Subject        string            `json:"subject"`
	Body           string            `json:"body"`
	Timestamp      string            `json:"timestamp"`
	Folder         string            `json:"folder"`
	Read           bool              `json:"read"`
	PreviousFolder string            `json:"previous_folder,omitempty"`
	Attachments    []EmailAttachment `json:"attachments,omitempty"`
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
	resendAPIKey  string
	resendClient  *http.Client
	resendAPIBase string
	inboundMu     sync.Mutex
	webhookSecret string
	attachmentDir string
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

	resendClient := &http.Client{Timeout: 20 * time.Second}
	service := &Service{
		db:            db,
		sender:        resendSender{apiKey: resendAPIKey, client: resendClient},
		resendAPIKey:  resendAPIKey,
		resendClient:  resendClient,
		resendAPIBase: "https://api.resend.com",
		webhookSecret: webhookSecret,
		attachmentDir: filepath.Join(filepath.Dir(dbPath), "email-attachments"),
	}
	if err := os.MkdirAll(service.attachmentDir, 0700); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create email attachment storage: %w", err)
	}
	if err := os.Chmod(service.attachmentDir, 0700); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("secure email attachment storage: %w", err)
	}
	if err := service.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	deleted, err := service.PurgeExpiredEmails()
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("purge expired email during startup: %w", err)
	}
	if deleted > 0 {
		log.Printf("email retention cleanup permanently deleted %d expired messages during startup", deleted)
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
		CREATE INDEX IF NOT EXISTS idx_emails_sender_folder ON emails(sender, folder, timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_emails_timestamp ON emails(timestamp);
		CREATE TABLE IF NOT EXISTS email_attachments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email_id INTEGER NOT NULL,
			filename TEXT NOT NULL,
			storage_path TEXT NOT NULL UNIQUE,
			file_size INTEGER NOT NULL,
			mime_type TEXT NOT NULL,
			FOREIGN KEY (email_id) REFERENCES emails(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_email_attachments_email_id ON email_attachments(email_id);
		CREATE TABLE IF NOT EXISTS inbound_email_receipts (
			provider_email_id TEXT PRIMARY KEY,
			processed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func (s *Service) PurgeExpiredEmails() (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin expired email cleanup: %w", err)
	}
	defer tx.Rollback()

	paths, err := attachmentPathsForExpiredEmails(tx)
	if err != nil {
		return 0, fmt.Errorf("list expired email attachments: %w", err)
	}
	if _, err := tx.Exec(`
		DELETE FROM email_attachments
		WHERE email_id IN (SELECT id FROM emails WHERE timestamp < datetime('now', '-29 days'))
	`); err != nil {
		return 0, fmt.Errorf("delete expired email attachments: %w", err)
	}
	result, err := tx.Exec(`
		DELETE FROM emails
		WHERE timestamp < datetime('now', '-29 days')
	`)
	if err != nil {
		return 0, fmt.Errorf("delete emails older than 29 days: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted expired emails: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit expired email cleanup: %w", err)
	}
	if err := s.removeStoredAttachments(paths); err != nil {
		return deleted, err
	}
	return deleted, nil
}

func attachmentPathsForExpiredEmails(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`
		SELECT storage_path
		FROM email_attachments
		WHERE email_id IN (SELECT id FROM emails WHERE timestamp < datetime('now', '-29 days'))
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

func (s *Service) removeStoredAttachments(paths []string) error {
	for _, path := range paths {
		if filepath.Base(path) != path || path == "." || path == "" {
			return errors.New("invalid attachment storage path during cleanup")
		}
		if err := os.Remove(filepath.Join(s.attachmentDir, path)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stored email attachment: %w", err)
		}
	}
	return nil
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
	return s.SaveIncomingWithAttachments(sender, recipient, subject, body, nil)
}

func (s *Service) SaveIncomingWithAttachments(sender, recipient, subject, body string, attachments []IncomingAttachment) error {
	if len(attachments) > MaxIncomingAttachments {
		return ErrTooManyAttachments
	}
	totalSize := int64(0)
	for _, attachment := range attachments {
		if attachment.Filename == "" || attachment.Size != int64(len(attachment.Data)) || attachment.Size < 0 {
			return ErrInvalidAttachment
		}
		if attachment.Size > MaxIncomingAttachmentBytes {
			return ErrAttachmentTooLarge
		}
		totalSize += attachment.Size
		if totalSize > MaxIncomingAttachmentsTotalBytes {
			return ErrAttachmentTooLarge
		}
	}

	storedPaths := make([]string, 0, len(attachments))
	cleanup := func() {
		for _, path := range storedPaths {
			if err := os.Remove(filepath.Join(s.attachmentDir, path)); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("remove incomplete email attachment %q: %v", path, err)
			}
		}
	}
	for _, attachment := range attachments {
		path, err := s.storeAttachment(attachment.Data)
		if err != nil {
			cleanup()
			return fmt.Errorf("store email attachment: %w", err)
		}
		storedPaths = append(storedPaths, path)
	}

	tx, err := s.db.Begin()
	if err != nil {
		cleanup()
		return fmt.Errorf("begin incoming email save: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		INSERT INTO emails (sender, recipient, subject, body, folder, is_read)
		VALUES (?, ?, ?, ?, 'inbox', 0)
	`, strings.TrimSpace(sender), strings.TrimSpace(recipient), subject, body)
	if err != nil {
		cleanup()
		return err
	}
	emailID, err := result.LastInsertId()
	if err != nil {
		cleanup()
		return err
	}
	for i, attachment := range attachments {
		if _, err := tx.Exec(`
			INSERT INTO email_attachments (email_id, filename, storage_path, file_size, mime_type)
			VALUES (?, ?, ?, ?, ?)
		`, emailID, attachment.Filename, storedPaths[i], attachment.Size, attachment.MIMEType); err != nil {
			cleanup()
			return fmt.Errorf("save email attachment metadata: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		cleanup()
		return fmt.Errorf("commit incoming email: %w", err)
	}
	return nil
}

func (s *Service) storeAttachment(data []byte) (string, error) {
	var randomName [16]byte
	if _, err := rand.Read(randomName[:]); err != nil {
		return "", fmt.Errorf("generate attachment storage name: %w", err)
	}
	storagePath := hex.EncodeToString(randomName[:])

	temp, err := os.CreateTemp(s.attachmentDir, ".incoming-*.tmp")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return "", err
	}
	if _, err := io.Copy(temp, bytes.NewReader(data)); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, filepath.Join(s.attachmentDir, storagePath)); err != nil {
		return "", err
	}
	return storagePath, nil
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range emails {
		attachments, err := s.emailAttachments(emails[i].ID)
		if err != nil {
			return nil, err
		}
		emails[i].Attachments = attachments
	}
	return emails, nil
}

func (s *Service) emailAttachments(emailID int64) ([]EmailAttachment, error) {
	rows, err := s.db.Query(`
		SELECT id, filename, file_size, mime_type
		FROM email_attachments
		WHERE email_id = ?
		ORDER BY id
	`, emailID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := make([]EmailAttachment, 0)
	for rows.Next() {
		var attachment EmailAttachment
		if err := rows.Scan(&attachment.ID, &attachment.Filename, &attachment.Size, &attachment.MIMEType); err != nil {
			return nil, err
		}
		attachment.URL = fmt.Sprintf("/api/emails/attachments/%d", attachment.ID)
		attachments = append(attachments, attachment)
	}
	return attachments, rows.Err()
}

func (s *Service) AttachmentForMailbox(owner string, id int64) (EmailAttachment, string, error) {
	var attachment EmailAttachment
	var storagePath string
	err := s.db.QueryRow(`
		SELECT a.id, a.filename, a.file_size, a.mime_type, a.storage_path
		FROM email_attachments a
		JOIN emails e ON e.id = a.email_id
		WHERE a.id = ? AND lower(e.recipient) = lower(?)
	`, id, owner).Scan(&attachment.ID, &attachment.Filename, &attachment.Size, &attachment.MIMEType, &storagePath)
	if errors.Is(err, sql.ErrNoRows) {
		return EmailAttachment{}, "", ErrNotFound
	}
	if err != nil {
		return EmailAttachment{}, "", err
	}
	if filepath.Base(storagePath) != storagePath || storagePath == "." || storagePath == "" {
		return EmailAttachment{}, "", errors.New("invalid attachment storage path")
	}
	return attachment, filepath.Join(s.attachmentDir, storagePath), nil
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
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var emailID int64
	err = tx.QueryRow(`
		SELECT id FROM emails
		WHERE id = ? AND folder = 'trash'
			AND (CASE WHEN previous_folder = 'sent' THEN lower(sender) ELSE lower(recipient) END) = lower(?)
	`, id, owner).Scan(&emailID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	paths, err := attachmentPathsForEmail(tx, emailID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM email_attachments WHERE email_id = ?`, emailID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM emails WHERE id = ?`, emailID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.removeStoredAttachments(paths)
}

func attachmentPathsForEmail(tx *sql.Tx, emailID int64) ([]string, error) {
	rows, err := tx.Query(`SELECT storage_path FROM email_attachments WHERE email_id = ?`, emailID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
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
	return s.webhookSecret != "" && hmac.Equal([]byte(s.webhookSecret), []byte(secret))
}

func (s *Service) ValidateResendWebhook(body []byte, headers http.Header) bool {
	secret := strings.TrimPrefix(s.webhookSecret, "whsec_")
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(secret)
	}
	if err != nil || len(key) == 0 {
		return false
	}

	id := headers.Get("svix-id")
	timestamp := headers.Get("svix-timestamp")
	signature := headers.Get("svix-signature")
	unixTimestamp, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || id == "" || signature == "" || time.Since(time.Unix(unixTimestamp, 0)) > 5*time.Minute ||
		time.Until(time.Unix(unixTimestamp, 0)) > 5*time.Minute {
		return false
	}

	mac := hmac.New(sha256.New, key)
	_, _ = fmt.Fprintf(mac, "%s.%s.%s", id, timestamp, body)
	expected := mac.Sum(nil)
	for _, candidate := range strings.Fields(signature) {
		version, encoded, ok := strings.Cut(candidate, ",")
		if !ok || version != "v1" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err == nil && hmac.Equal(decoded, expected) {
			return true
		}
	}
	return false
}

type resendReceivedEmail struct {
	From        string                     `json:"from"`
	To          []string                   `json:"to"`
	ReceivedFor []string                   `json:"received_for"`
	Subject     string                     `json:"subject"`
	Text        *string                    `json:"text"`
	HTML        string                     `json:"html"`
	Attachments []resendReceivedAttachment `json:"attachments"`
}

type resendReceivedAttachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

func (s *Service) SaveResendReceivedEmail(ctx context.Context, emailID string) error {
	if strings.TrimSpace(s.resendAPIKey) == "" {
		return errors.New("Resend API key is not configured for incoming email")
	}
	if emailID == "" || strings.ContainsAny(emailID, "/?#") {
		return errors.New("received email ID is invalid")
	}
	s.inboundMu.Lock()
	defer s.inboundMu.Unlock()

	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM inbound_email_receipts WHERE provider_email_id = ?`, emailID).Scan(&exists)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check whether received email was processed: %w", err)
	}

	var received resendReceivedEmail
	if err := s.resendGET(ctx, "/emails/receiving/"+url.PathEscape(emailID), &received); err != nil {
		return fmt.Errorf("retrieve received email: %w", err)
	}
	if received.From == "" {
		return errors.New("received email has no sender")
	}
	recipients := mailboxRecipients(received.ReceivedFor, received.To)
	if len(recipients) == 0 {
		return errors.New("received email has no Jaylub mailbox recipient")
	}

	body := ""
	if received.Text != nil {
		body = *received.Text
	} else {
		body = received.HTML
	}
	attachments, err := s.fetchResendAttachments(ctx, emailID, received.Attachments)
	if err != nil {
		return err
	}
	return s.saveResendReceivedEmail(emailID, received.From, received.Subject, body, recipients, attachments)
}

func (s *Service) saveResendReceivedEmail(providerID, sender, subject, body string, recipients []string, attachments []IncomingAttachment) error {
	storedPaths := make([][]string, len(recipients))
	cleanup := func() {
		for _, paths := range storedPaths {
			for _, path := range paths {
				if err := os.Remove(filepath.Join(s.attachmentDir, path)); err != nil && !errors.Is(err, os.ErrNotExist) {
					log.Printf("remove incomplete Resend attachment %q: %v", path, err)
				}
			}
		}
	}
	for i := range recipients {
		for _, attachment := range attachments {
			path, err := s.storeAttachment(attachment.Data)
			if err != nil {
				cleanup()
				return fmt.Errorf("store received email attachment: %w", err)
			}
			storedPaths[i] = append(storedPaths[i], path)
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		cleanup()
		return fmt.Errorf("begin Resend email save: %w", err)
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRow(`SELECT 1 FROM inbound_email_receipts WHERE provider_email_id = ?`, providerID).Scan(&exists)
	if err == nil {
		cleanup()
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		cleanup()
		return fmt.Errorf("check Resend email receipt: %w", err)
	}

	for recipientIndex, recipient := range recipients {
		result, err := tx.Exec(`
			INSERT INTO emails (sender, recipient, subject, body, folder, is_read)
			VALUES (?, ?, ?, ?, 'inbox', 0)
		`, sender, recipient, subject, body)
		if err != nil {
			cleanup()
			return fmt.Errorf("save received email for %s: %w", recipient, err)
		}
		emailID, err := result.LastInsertId()
		if err != nil {
			cleanup()
			return fmt.Errorf("get received email ID for %s: %w", recipient, err)
		}
		for attachmentIndex, attachment := range attachments {
			if _, err := tx.Exec(`
				INSERT INTO email_attachments (email_id, filename, storage_path, file_size, mime_type)
				VALUES (?, ?, ?, ?, ?)
			`, emailID, attachment.Filename, storedPaths[recipientIndex][attachmentIndex], attachment.Size, attachment.MIMEType); err != nil {
				cleanup()
				return fmt.Errorf("save received attachment for %s: %w", recipient, err)
			}
		}
	}
	if _, err := tx.Exec(`INSERT INTO inbound_email_receipts (provider_email_id) VALUES (?)`, providerID); err != nil {
		cleanup()
		return fmt.Errorf("record processed Resend email: %w", err)
	}
	if err := tx.Commit(); err != nil {
		cleanup()
		return fmt.Errorf("commit Resend email save: %w", err)
	}
	return nil
}

func mailboxRecipients(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var recipients []string
	for _, group := range groups {
		for _, rawAddress := range group {
			address, err := mail.ParseAddress(strings.TrimSpace(rawAddress))
			if err != nil {
				continue
			}
			addressValue := strings.ToLower(address.Address)
			if !strings.HasSuffix(addressValue, "@"+MailboxDomain) {
				continue
			}
			if _, ok := seen[addressValue]; ok {
				continue
			}
			seen[addressValue] = struct{}{}
			recipients = append(recipients, addressValue)
		}
	}
	return recipients
}

func (s *Service) fetchResendAttachments(ctx context.Context, emailID string, metadata []resendReceivedAttachment) ([]IncomingAttachment, error) {
	if len(metadata) > MaxIncomingAttachments {
		return nil, ErrTooManyAttachments
	}

	attachments := make([]IncomingAttachment, 0, len(metadata))
	totalSize := int64(0)
	for _, item := range metadata {
		if item.ID == "" || item.Filename == "" || item.Size < 0 {
			return nil, ErrInvalidAttachment
		}
		if item.Size > MaxIncomingAttachmentBytes || item.Size > MaxIncomingAttachmentsTotalBytes-totalSize {
			return nil, ErrAttachmentTooLarge
		}

		var details struct {
			DownloadURL string `json:"download_url"`
		}
		path := "/emails/receiving/" + url.PathEscape(emailID) + "/attachments/" + url.PathEscape(item.ID)
		if err := s.resendGET(ctx, path, &details); err != nil {
			return nil, fmt.Errorf("retrieve received email attachment: %w", err)
		}
		data, err := s.downloadResendAttachment(ctx, details.DownloadURL)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) != item.Size {
			return nil, ErrInvalidAttachment
		}
		attachments = append(attachments, IncomingAttachment{
			Filename: item.Filename,
			MIMEType: item.ContentType,
			Size:     int64(len(data)),
			Data:     data,
		})
		totalSize += int64(len(data))
	}
	return attachments, nil
}

func (s *Service) resendGET(ctx context.Context, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.resendAPIBase, "/")+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+s.resendAPIKey)
	response, err := s.resendClient.Do(request)
	if err != nil {
		return fmt.Errorf("Resend API request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Resend API returned HTTP %d", response.StatusCode)
	}
	const maxResendResponseBytes = 36 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResendResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read Resend API response: %w", err)
	}
	if len(data) > maxResendResponseBytes {
		return errors.New("Resend API response exceeds the size limit")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode Resend API response: %w", err)
	}
	return nil
}

func (s *Service) downloadResendAttachment(ctx context.Context, downloadURL string) ([]byte, error) {
	parsedURL, err := url.Parse(downloadURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Hostname() != "inbound-cdn.resend.com" {
		return nil, errors.New("Resend attachment URL is invalid")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := s.resendClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download received email attachment: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("attachment download returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxIncomingAttachmentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read received email attachment: %w", err)
	}
	if int64(len(data)) > MaxIncomingAttachmentBytes {
		return nil, ErrAttachmentTooLarge
	}
	return data, nil
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
