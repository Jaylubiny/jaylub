package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"jaylub/internal/auth"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	chatMaxMessageLength = 500
	chatRecentLimit      = 100
	chatRetention        = 30 * 24 * time.Hour
	chatRateLimit        = 2 * time.Second
	chatOnlineWindow     = 60 * time.Second
)

type ChatService struct {
	db         *sql.DB
	mu         sync.Mutex
	lastPost   map[string]time.Time
	lastActive map[string]time.Time
}

type ChatMessage struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
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

	afterID, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
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

	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	s.markActive(r)
	s.cleanupExpired()

	var payload struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&payload); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	message, err := validateChatMessage(payload.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.allowPost(user.Username) {
		http.Error(w, "Please wait before sending another message.", http.StatusTooManyRequests)
		return
	}

	now := time.Now().UTC()
	result, err := s.db.Exec(
		`INSERT INTO chat_messages (username, message, timestamp) VALUES (?, ?, ?)`,
		user.Username,
		message,
		now.Format(time.RFC3339),
	)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()
	s.writeJSON(w, map[string]any{
		"message": ChatMessage{
			ID:        id,
			Username:  user.Username,
			Message:   message,
			Timestamp: now.Format(time.RFC3339),
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
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *ChatService) cleanupExpired() {
	_, _ = s.db.Exec(`DELETE FROM chat_messages WHERE timestamp < ?`, time.Now().UTC().Add(-chatRetention).Format(time.RFC3339))
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

func (s *ChatService) allowPost(username string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if last, ok := s.lastPost[username]; ok && now.Sub(last) < chatRateLimit {
		return false
	}
	s.lastPost[username] = now
	return true
}

func (s *ChatService) writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
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
