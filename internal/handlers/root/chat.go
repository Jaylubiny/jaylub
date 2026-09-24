package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"jaylub/internal/auth"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"crypto/rand"
)

const (
	chatMaxMessageLength = 500
	chatMaxFileSize      = 10 << 20
	chatRecentLimit      = 100
	chatRetention        = 30 * 24 * time.Hour
	chatRateLimit        = 2 * time.Second
	chatOnlineWindow     = 60 * time.Second
	chatFilesDir         = "internal/database/files"
)

type ChatService struct {
	db         *sql.DB
	mu         sync.Mutex
	lastPost   map[string]time.Time
	lastActive map[string]time.Time
}

func Settings(authService *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			renderer.Render(w, r, "settings")
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}

			settings, err := parseChatSettings(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := authService.SaveChatSettings(user.ID, settings); err != nil {
				http.Error(w, "Could not save settings.", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	}
}

func parseChatSettings(r *http.Request) (auth.ChatSettings, error) {
	settings := auth.DefaultChatSettings()
	settings.TimeFormat = r.FormValue("time_format")
	settings.MessageDensity = r.FormValue("message_density")
	settings.ShowTimestamps = r.FormValue("show_timestamps") == "on"

	if settings.TimeFormat != "24h" && settings.TimeFormat != "12h" && settings.TimeFormat != "relative" {
		return settings, errors.New("Invalid time format.")
	}
	if settings.MessageDensity != "comfortable" && settings.MessageDensity != "compact" {
		return settings, errors.New("Invalid message density.")
	}
	return settings, nil
}

type ChatMessage struct {
	ID          int64            `json:"id"`
	Username    string           `json:"username"`
	Message     string           `json:"message"`
	Timestamp   string           `json:"timestamp"`
	Attachments []ChatAttachment `json:"attachments,omitempty"`
}

type ChatAttachment struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

func NewChatService(db *sql.DB) *ChatService {
	return &ChatService{
		db:         db,
		lastPost:   make(map[string]time.Time),
		lastActive: make(map[string]time.Time),
	}
}

func (s *ChatService) Page(w http.ResponseWriter, r *http.Request) {
	s.markActive(r)
	s.cleanupExpired()
	s.markRead(r)
	renderer.Render(w, r, "chat")
}

func (s *ChatService) Messages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	s.markActive(r)
	s.cleanupExpired()

	afterValue := r.URL.Query().Get("after")
	afterID := int64(0)
	if afterValue != "" {
		var err error
		afterID, err = strconv.ParseInt(afterValue, 10, 64)
		if err != nil || afterID < 0 {
			http.Error(w, "Invalid message cursor.", http.StatusBadRequest)
			return
		}
	}
	messages, err := s.recentMessages(afterID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, map[string]any{
		"messages":    messages,
		"onlineCount": s.onlineCount(),
	})
}

func (s *ChatService) SendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOriginRequest(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	s.markActive(r)
	s.cleanupExpired()

	messageText, upload, err := parseChatSubmission(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	message := strings.TrimSpace(messageText)
	if message != "" {
		message, err = validateChatMessage(message)
	} else if upload == nil {
		err = errors.New("Message or file is required.")
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.allowPost(strconv.FormatInt(user.ID, 10)) {
		http.Error(w, "Please wait before sending another message.", http.StatusTooManyRequests)
		return
	}

	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		s.releasePost(strconv.FormatInt(user.ID, 10))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	result, err := tx.Exec(
		`INSERT INTO chat_messages (username, message, timestamp) VALUES (?, ?, ?)`,
		user.Username, message, now.Format(time.RFC3339),
	)
	if err != nil {
		_ = tx.Rollback()
		s.releasePost(strconv.FormatInt(user.ID, 10))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	id, err := result.LastInsertId()
	if upload != nil {
		if err := saveChatUpload(tx, id, upload); err != nil {
			_ = tx.Rollback()
			s.releasePost(strconv.FormatInt(user.ID, 10))
			http.Error(w, "Could not save the attachment.", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.releasePost(strconv.FormatInt(user.ID, 10))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	attachments, err := s.attachments(id)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, map[string]any{
		"message": ChatMessage{
			ID:          id,
			Username:    user.Username,
			Message:     message,
			Timestamp:   now.Format(time.RFC3339),
			Attachments: attachments,
		},
		"onlineCount": s.onlineCount(),
	})
}

func (s *ChatService) recentMessages(afterID int64) ([]ChatMessage, error) {
	if afterID > 0 {
		return s.messagesAfter(afterID)
	}

	rows, err := s.db.Query(`
		SELECT id, username, message, timestamp
		FROM (
			SELECT id, username, message, timestamp
			FROM chat_messages
			WHERE timestamp >= ?
			ORDER BY id DESC
			LIMIT ?
		)
		ORDER BY id ASC
	`, time.Now().UTC().Add(-chatRetention).Format(time.RFC3339), chatRecentLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]ChatMessage, 0, chatRecentLimit)
	for rows.Next() {
		var message ChatMessage
		if err := rows.Scan(&message.ID, &message.Username, &message.Message, &message.Timestamp); err != nil {
			return nil, err
		}
		message.Attachments, err = s.attachments(message.ID)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *ChatService) messagesAfter(afterID int64) ([]ChatMessage, error) {
	rows, err := s.db.Query(`
		SELECT id, username, message, timestamp
		FROM chat_messages
		WHERE id > ? AND timestamp >= ?
		ORDER BY id ASC
		LIMIT ?
	`, afterID, time.Now().UTC().Add(-chatRetention).Format(time.RFC3339), chatRecentLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]ChatMessage, 0)
	for rows.Next() {
		var message ChatMessage
		if err := rows.Scan(&message.ID, &message.Username, &message.Message, &message.Timestamp); err != nil {
			return nil, err
		}
		message.Attachments, err = s.attachments(message.ID)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *ChatService) cleanupExpired() {
	_, _ = s.db.Exec(`DELETE FROM chat_messages WHERE timestamp < ?`, time.Now().UTC().Add(-chatRetention).Format(time.RFC3339))
}

func (s *ChatService) File(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/chat/files/"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}

	var storedName, contentType, originalName string
	err = s.db.QueryRow(`SELECT stored_name, content_type, original_name FROM chat_attachments WHERE id = ?`, id).
		Scan(&storedName, &contentType, &originalName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	path := filepath.Join(chatFilesDir, filepath.Base(storedName))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, safeDownloadName(originalName)))
	http.ServeFile(w, r, path)
}

type chatUpload struct {
	header      *multipart.FileHeader
	contentType string
}

func parseChatSubmission(w http.ResponseWriter, r *http.Request) (string, *chatUpload, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		var payload struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&payload); err != nil {
			return "", nil, errors.New("Bad Request.")
		}
		return payload.Message, nil, nil
	}

	r.Body = http.MaxBytesReader(w, r.Body, chatMaxFileSize+2048)
	if err := r.ParseMultipartForm(chatMaxFileSize + 2048); err != nil {
		return "", nil, errors.New("The message or file is too large.")
	}
	message := r.FormValue("message")
	file, header, err := r.FormFile("file")
	if err == http.ErrMissingFile {
		return message, nil, nil
	}
	if err != nil {
		return "", nil, errors.New("Could not read the attachment.")
	}
	defer file.Close()

	contentTypeBuffer := make([]byte, 512)
	n, err := file.Read(contentTypeBuffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", nil, errors.New("Could not read the attachment.")
	}
	if _, err := file.Seek(0, 0); err != nil {
		return "", nil, errors.New("Could not read the attachment.")
	}
	detectedType := http.DetectContentType(contentTypeBuffer[:n])
	if detectedType == "application/octet-stream" {
		detectedType = "application/octet-stream"
	}
	if header.Size > chatMaxFileSize {
		return "", nil, errors.New("Files must be 10 MB or smaller.")
	}
	return message, &chatUpload{header: header, contentType: detectedType}, nil
}

func saveChatUpload(tx *sql.Tx, messageID int64, upload *chatUpload) error {
	if err := os.MkdirAll(chatFilesDir, 0750); err != nil {
		return err
	}
	source, err := upload.header.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	randomName := make([]byte, 16)
	if _, err := rand.Read(randomName); err != nil {
		return err
	}
	storedName := fmt.Sprintf("%x", randomName)
	path := filepath.Join(chatFilesDir, storedName)
	target, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, io.LimitReader(source, chatMaxFileSize+1)); err != nil {
		_ = target.Close()
		_ = os.Remove(path)
		return err
	}
	if err := target.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	_, err = tx.Exec(`INSERT INTO chat_attachments (message_id, original_name, stored_name, content_type, size) VALUES (?, ?, ?, ?, ?)`,
		messageID, safeDownloadName(upload.header.Filename), storedName, upload.contentType, upload.header.Size)
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}

func (s *ChatService) attachments(messageID int64) ([]ChatAttachment, error) {
	rows, err := s.db.Query(`SELECT id, original_name, content_type, size FROM chat_attachments WHERE message_id = ? ORDER BY id`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var attachments []ChatAttachment
	for rows.Next() {
		var attachment ChatAttachment
		if err := rows.Scan(&attachment.ID, &attachment.Name, &attachment.ContentType, &attachment.Size); err != nil {
			return nil, err
		}
		attachment.URL = fmt.Sprintf("/chat/files/%d", attachment.ID)
		attachments = append(attachments, attachment)
	}
	return attachments, rows.Err()
}

func safeDownloadName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." {
		return "download"
	}
	name = strings.NewReplacer(`"`, "", "\r", "", "\n", "").Replace(name)
	if name == "" || name == "." {
		return "download"
	}
	return name
}

func (s *ChatService) markActive(r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActive[user.Username] = time.Now()
}

func (s *ChatService) markRead(r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return
	}

	_, _ = s.db.Exec(`
		INSERT INTO chat_reads (user_id, last_read_at)
		VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET last_read_at = excluded.last_read_at
	`, user.ID, time.Now().UTC().Format(time.RFC3339))
}

func (s *ChatService) onlineCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := 0
	for username, lastSeen := range s.lastActive {
		if now.Sub(lastSeen) <= chatOnlineWindow {
			count++
			continue
		}
		delete(s.lastActive, username)
	}
	return count
}

func (s *ChatService) allowPost(userKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if last, ok := s.lastPost[userKey]; ok && now.Sub(last) < chatRateLimit {
		return false
	}
	s.lastPost[userKey] = now
	return true
}

func (s *ChatService) releasePost(userKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.lastPost, userKey)
}

func (s *ChatService) writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func sameOriginRequest(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return origin == r.URL.Scheme+"://"+r.Host || origin == "http://"+r.Host || origin == "https://"+r.Host
}

func validateChatMessage(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", errors.New("Message cannot be empty.")
	}
	if len([]rune(message)) > chatMaxMessageLength {
		return "", errors.New("Message must be 500 characters or fewer.")
	}
	return message, nil
}
