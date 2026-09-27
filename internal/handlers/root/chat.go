package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"jaylub/internal/auth"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"crypto/rand"
)

const (
	chatMaxMessageLength = 500
	chatMaxGiftAmount    = 1000000
	chatMaxFileSize      = 25 << 20
	chatMultipartMemory  = 1 << 20
	chatRecentLimit      = 100
	chatRetention        = 30 * 24 * time.Hour
	chatCleanupInterval  = 10 * time.Minute
	chatRateLimit        = 2 * time.Second
	chatOnlineWindow     = 60 * time.Second
	chatFilesDir         = "internal/database/files"
)

var errChatFileTooLarge = errors.New("Files must be 25 MB or smaller.")
var (
	errInsufficientGiftCoins = errors.New("insufficient Jaylive coins")
	errInvalidGiftRecipient  = errors.New("invalid gift recipient")
	errGiftAlreadyClaimed    = errors.New("gift card already claimed")
	errGiftNotFound          = errors.New("gift card not found")
	errOwnGiftCard           = errors.New("cannot claim own gift card")
	errGiftExpired           = errors.New("gift card expired")
)

type ChatService struct {
	db          *sql.DB
	mu          sync.Mutex
	giftMu      sync.Mutex
	cleanupMu   sync.Mutex
	lastPost    map[string]time.Time
	lastActive  map[string]time.Time
	lastCleanup time.Time
	filesDir    string
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
	settings.ShowImagePreviews = r.FormValue("show_image_previews") == "on"

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
	Gift        *ChatCoinGift    `json:"gift,omitempty"`
}

type ChatCoinGift struct {
	ID                int64  `json:"id"`
	Type              string `json:"type"`
	Amount            int    `json:"amount"`
	SenderUsername    string `json:"senderUsername"`
	RecipientUsername string `json:"recipientUsername,omitempty"`
	ClaimedByUsername string `json:"claimedByUsername,omitempty"`
	Claimed           bool   `json:"claimed"`
	Expired           bool   `json:"expired"`
}

type chatRecipient struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
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
		filesDir:   chatFilesDir,
	}
}

func (s *ChatService) Page(w http.ResponseWriter, r *http.Request) {
	s.markActive(r)
	if err := s.cleanupExpired(); err != nil {
		http.Error(w, "Could not load chat.", http.StatusInternalServerError)
		return
	}
	s.markRead(r)
	renderer.Render(w, r, "chat")
}

func (s *ChatService) GiftRecipients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := s.db.Query(`SELECT id, username FROM users WHERE id != ? ORDER BY username COLLATE NOCASE`, user.ID)
	if err != nil {
		http.Error(w, "Could not load gift recipients.", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	recipients := make([]chatRecipient, 0)
	for rows.Next() {
		var recipient chatRecipient
		if err := rows.Scan(&recipient.ID, &recipient.Username); err != nil {
			http.Error(w, "Could not load gift recipients.", http.StatusInternalServerError)
			return
		}
		recipients = append(recipients, recipient)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Could not load gift recipients.", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, map[string]any{"recipients": recipients})
}

func (s *ChatService) GiftCoins(w http.ResponseWriter, r *http.Request) {
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

	var payload struct {
		Type        string `json:"type"`
		Amount      int    `json:"amount"`
		RecipientID int64  `json:"recipientId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&payload); err != nil {
		http.Error(w, "Invalid gift request.", http.StatusBadRequest)
		return
	}
	if payload.Amount <= 0 || payload.Amount > chatMaxGiftAmount {
		http.Error(w, "Gift amount must be between 1 and 1,000,000 coins.", http.StatusBadRequest)
		return
	}
	if payload.Type != "direct" && payload.Type != "card" {
		http.Error(w, "Invalid gift type.", http.StatusBadRequest)
		return
	}
	if payload.Type == "direct" && (payload.RecipientID <= 0 || payload.RecipientID == user.ID) {
		http.Error(w, "Choose another user to receive this gift.", http.StatusBadRequest)
		return
	}

	if !s.allowPost(strconv.FormatInt(user.ID, 10) + ":gift") {
		http.Error(w, "Please wait before sending another gift.", http.StatusTooManyRequests)
		return
	}
	response, err := s.createCoinGift(user, payload.Type, payload.Amount, payload.RecipientID)
	if err != nil {
		s.releasePost(strconv.FormatInt(user.ID, 10) + ":gift")
		switch {
		case errors.Is(err, errInsufficientGiftCoins):
			http.Error(w, "You do not have enough Jaylive coins.", http.StatusBadRequest)
		case errors.Is(err, errInvalidGiftRecipient):
			http.Error(w, "That gift recipient is not available.", http.StatusBadRequest)
		default:
			http.Error(w, "Could not create coin gift.", http.StatusInternalServerError)
		}
		return
	}
	s.writeJSON(w, response)
}

func (s *ChatService) ClaimGiftCard(w http.ResponseWriter, r *http.Request) {
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

	var payload struct {
		GiftID int64 `json:"giftId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&payload); err != nil || payload.GiftID <= 0 {
		http.Error(w, "Invalid gift card.", http.StatusBadRequest)
		return
	}
	result, err := s.claimCoinGift(user, payload.GiftID)
	if err != nil {
		switch {
		case errors.Is(err, errOwnGiftCard):
			http.Error(w, "You cannot claim your own gift card.", http.StatusForbidden)
		case errors.Is(err, errGiftAlreadyClaimed):
			http.Error(w, "This gift card has already been claimed.", http.StatusConflict)
		case errors.Is(err, errGiftExpired):
			http.Error(w, "This gift card has expired.", http.StatusGone)
		case errors.Is(err, errGiftNotFound):
			http.Error(w, "Gift card not found.", http.StatusNotFound)
		default:
			http.Error(w, "Could not claim gift card.", http.StatusInternalServerError)
		}
		return
	}
	s.writeJSON(w, result)
}

func (s *ChatService) createCoinGift(user auth.User, giftType string, amount int, recipientID int64) (map[string]any, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO game_profiles (user_id, username)
		VALUES (?, ?)
		ON CONFLICT(user_id) DO NOTHING
	`, user.ID, user.Username); err != nil {
		return nil, err
	}
	result, err := tx.Exec(`
		UPDATE game_profiles SET gold = gold - ?
		WHERE user_id = ? AND gold >= ?
	`, amount, user.ID, amount)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected != 1 {
		return nil, errInsufficientGiftCoins
	}

	var recipientUsername string
	if giftType == "direct" {
		err := tx.QueryRow(`SELECT username FROM users WHERE id = ? AND id != ?`, recipientID, user.ID).
			Scan(&recipientUsername)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errInvalidGiftRecipient
		}
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`
			INSERT INTO game_profiles (user_id, username)
			VALUES (?, ?)
			ON CONFLICT(user_id) DO NOTHING
		`, recipientID, recipientUsername); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`
			UPDATE game_profiles SET gold = gold + ?
			WHERE user_id = ?
		`, amount, recipientID); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	messageText := fmt.Sprintf("%s created a Jaylive coin gift card.", user.Username)
	if giftType == "direct" {
		messageText = fmt.Sprintf("%s sent %d Jaylive coins to %s.", user.Username, amount, recipientUsername)
	}
	messageResult, err := tx.Exec(`
		INSERT INTO chat_messages (username, message, timestamp) VALUES (?, ?, ?)
	`, user.Username, messageText, now)
	if err != nil {
		return nil, err
	}
	messageID, err := messageResult.LastInsertId()
	if err != nil {
		return nil, err
	}

	var recipientUserID any
	var recipientName any
	if giftType == "direct" {
		recipientUserID = recipientID
		recipientName = recipientUsername
	}
	giftResult, err := tx.Exec(`
		INSERT INTO chat_coin_gifts (
			message_id, gift_type, amount, sender_user_id, sender_username,
			recipient_user_id, recipient_username, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, messageID, giftType, amount, user.ID, user.Username, recipientUserID, recipientName, now)
	if err != nil {
		return nil, err
	}
	giftID, err := giftResult.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	gift := &ChatCoinGift{
		ID:                giftID,
		Type:              giftType,
		Amount:            amount,
		SenderUsername:    user.Username,
		RecipientUsername: recipientUsername,
	}
	message := ChatMessage{
		ID:        messageID,
		Username:  user.Username,
		Message:   messageText,
		Timestamp: now,
		Gift:      gift,
	}
	return map[string]any{"message": message}, nil
}

func (s *ChatService) claimCoinGift(user auth.User, giftID int64) (map[string]any, error) {
	s.giftMu.Lock()
	defer s.giftMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE chat_coin_gifts
		SET claimed_by_user_id = ?, claimed_by_username = ?
		WHERE id = ? AND gift_type = 'card' AND claimed_by_user_id IS NULL
			AND sender_user_id != ? AND created_at >= ?
	`, user.ID, user.Username, giftID, user.ID, time.Now().UTC().Add(-chatRetention).Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected != 1 {
		var giftType string
		var senderID int64
		var claimedBy sql.NullInt64
		var createdAt string
		err := tx.QueryRow(`
			SELECT gift_type, sender_user_id, claimed_by_user_id, created_at
			FROM chat_coin_gifts
			WHERE id = ?
		`, giftID).Scan(&giftType, &senderID, &claimedBy, &createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errGiftNotFound
		}
		if err != nil {
			return nil, err
		}
		if giftType == "card" && senderID == user.ID && !claimedBy.Valid {
			return nil, errOwnGiftCard
		}
		if giftType == "card" && !claimedBy.Valid {
			expiresAt, err := time.Parse(time.RFC3339, createdAt)
			if err != nil {
				return nil, err
			}
			if time.Now().UTC().After(expiresAt.Add(chatRetention)) {
				return nil, errGiftExpired
			}
		}
		return nil, errGiftAlreadyClaimed
	}

	var amount int
	var senderUsername string
	if err := tx.QueryRow(`
		SELECT amount, sender_username
		FROM chat_coin_gifts
		WHERE id = ?
	`, giftID).Scan(&amount, &senderUsername); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`
		INSERT INTO game_profiles (user_id, username)
		VALUES (?, ?)
		ON CONFLICT(user_id) DO NOTHING
	`, user.ID, user.Username); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE game_profiles SET gold = gold + ? WHERE user_id = ?`, amount, user.ID); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	messageText := fmt.Sprintf("%s claimed a %d-coin Jaylive gift card from %s.", user.Username, amount, senderUsername)
	messageResult, err := tx.Exec(`
		INSERT INTO chat_messages (username, message, timestamp) VALUES (?, ?, ?)
	`, user.Username, messageText, now)
	if err != nil {
		return nil, err
	}
	claimMessageID, err := messageResult.LastInsertId()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE chat_coin_gifts SET claim_message_id = ? WHERE id = ?`, claimMessageID, giftID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	gift := &ChatCoinGift{
		ID:                giftID,
		Type:              "claim",
		Amount:            amount,
		SenderUsername:    senderUsername,
		RecipientUsername: user.Username,
		Claimed:           true,
		ClaimedByUsername: user.Username,
	}
	return map[string]any{
		"message": ChatMessage{
			ID:        claimMessageID,
			Username:  user.Username,
			Message:   messageText,
			Timestamp: now,
			Gift:      gift,
		},
	}, nil
}

func (s *ChatService) Messages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	channel, err := requestedChatChannel(r)
	if err != nil {
		http.Error(w, "Invalid chat channel.", http.StatusBadRequest)
		return
	}

	s.markActive(r)
	if err := s.cleanupExpired(); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

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
	var messages []ChatMessage
	if channel == "self" {
		user, ok := auth.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		messages, err = s.recentSelfMessages(user.ID, afterID)
	} else {
		messages, err = s.recentMessages(afterID)
	}
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	onlineUsers := s.onlineUsers()
	s.writeJSON(w, map[string]any{
		"messages":    messages,
		"onlineCount": len(onlineUsers),
		"onlineUsers": onlineUsers,
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
	channel, err := requestedChatChannel(r)
	if err != nil {
		http.Error(w, "Invalid chat channel.", http.StatusBadRequest)
		return
	}

	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	s.markActive(r)
	if err := s.cleanupExpired(); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	userKey := strconv.FormatInt(user.ID, 10)
	if !s.allowPost(userKey) {
		http.Error(w, "Please wait before sending another message.", http.StatusTooManyRequests)
		return
	}

	messageText, upload, err := parseChatSubmission(w, r)
	if err != nil {
		s.releasePost(userKey)
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
		s.releasePost(userKey)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if channel == "self" {
		s.sendSelfMessage(w, user, message, upload, userKey)
		return
	}

	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		s.releasePost(userKey)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	result, err := tx.Exec(
		`INSERT INTO chat_messages (username, message, timestamp) VALUES (?, ?, ?)`,
		user.Username, message, now.Format(time.RFC3339),
	)
	if err != nil {
		_ = tx.Rollback()
		s.releasePost(userKey)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		s.releasePost(userKey)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if upload != nil {
		if err := s.saveChatUpload(tx, id, upload, false); err != nil {
			_ = tx.Rollback()
			s.releasePost(userKey)
			if errors.Is(err, errChatFileTooLarge) {
				http.Error(w, err.Error(), http.StatusBadRequest)
			} else {
				http.Error(w, "Could not save the attachment.", http.StatusInternalServerError)
			}
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.releasePost(userKey)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	sentMessages := []ChatMessage{{ID: id}}
	if err := s.loadAttachments(sentMessages); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	onlineUsers := s.onlineUsers()
	s.writeJSON(w, map[string]any{
		"message": ChatMessage{
			ID:          id,
			Username:    user.Username,
			Message:     message,
			Timestamp:   now.Format(time.RFC3339),
			Attachments: sentMessages[0].Attachments,
		},
		"onlineCount": len(onlineUsers),
		"onlineUsers": onlineUsers,
	})
}

func (s *ChatService) sendSelfMessage(w http.ResponseWriter, user auth.User, message string, upload *chatUpload, userKey string) {
	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		s.releasePost(userKey)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	result, err := tx.Exec(
		`INSERT INTO self_chat_messages (user_id, username, message, timestamp) VALUES (?, ?, ?, ?)`,
		user.ID, user.Username, message, now.Format(time.RFC3339),
	)
	if err != nil {
		_ = tx.Rollback()
		s.releasePost(userKey)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		s.releasePost(userKey)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if upload != nil {
		if err := s.saveChatUpload(tx, id, upload, true); err != nil {
			_ = tx.Rollback()
			s.releasePost(userKey)
			if errors.Is(err, errChatFileTooLarge) {
				http.Error(w, err.Error(), http.StatusBadRequest)
			} else {
				http.Error(w, "Could not save the attachment.", http.StatusInternalServerError)
			}
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.releasePost(userKey)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	messages := []ChatMessage{{ID: id}}
	if err := s.loadSelfAttachments(messages, user.ID); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	onlineUsers := s.onlineUsers()
	s.writeJSON(w, map[string]any{
		"message": ChatMessage{
			ID:          id,
			Username:    user.Username,
			Message:     message,
			Timestamp:   now.Format(time.RFC3339),
			Attachments: messages[0].Attachments,
		},
		"onlineCount": len(onlineUsers),
		"onlineUsers": onlineUsers,
	})
}

func requestedChatChannel(r *http.Request) (string, error) {
	switch channel := r.URL.Query().Get("channel"); channel {
	case "", "global":
		return "global", nil
	case "self":
		return "self", nil
	default:
		return "", errors.New("unknown chat channel")
	}
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
	messages := make([]ChatMessage, 0, chatRecentLimit)
	for rows.Next() {
		var message ChatMessage
		if err := rows.Scan(&message.ID, &message.Username, &message.Message, &message.Timestamp); err != nil {
			_ = rows.Close()
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := s.loadAttachments(messages); err != nil {
		return nil, err
	}
	return messages, s.loadGiftEvents(messages)
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
	messages := make([]ChatMessage, 0)
	for rows.Next() {
		var message ChatMessage
		if err := rows.Scan(&message.ID, &message.Username, &message.Message, &message.Timestamp); err != nil {
			_ = rows.Close()
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := s.loadAttachments(messages); err != nil {
		return nil, err
	}
	return messages, s.loadGiftEvents(messages)
}

func (s *ChatService) loadGiftEvents(messages []ChatMessage) error {
	if len(messages) == 0 {
		return nil
	}

	messageIndexes := make(map[int64]int, len(messages))
	placeholders := make([]string, len(messages))
	args := make([]any, 0, len(messages)*2)
	for index := range messages {
		messageIndexes[messages[index].ID] = index
		placeholders[index] = "?"
		args = append(args, messages[index].ID)
	}
	args = append(args, args[:len(messages)]...)

	rows, err := s.db.Query(`
		SELECT message_id, id, gift_type, amount, sender_username,
			COALESCE(recipient_username, ''), COALESCE(claimed_by_username, ''),
			claimed_by_user_id IS NOT NULL, created_at, 'gift'
		FROM chat_coin_gifts
		WHERE message_id IN (`+strings.Join(placeholders, ",")+`)
		UNION ALL
		SELECT claim_message_id, id, gift_type, amount, sender_username,
			COALESCE(recipient_username, ''), COALESCE(claimed_by_username, ''),
			claimed_by_user_id IS NOT NULL, created_at, 'claim'
		FROM chat_coin_gifts
		WHERE claim_message_id IN (`+strings.Join(placeholders, ",")+`)
	`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var messageID int64
		var gift ChatCoinGift
		var createdAt string
		var eventType string
		if err := rows.Scan(
			&messageID, &gift.ID, &gift.Type, &gift.Amount, &gift.SenderUsername,
			&gift.RecipientUsername, &gift.ClaimedByUsername, &gift.Claimed, &createdAt, &eventType,
		); err != nil {
			return err
		}
		if gift.Type == "card" && !gift.Claimed {
			created, err := time.Parse(time.RFC3339, createdAt)
			if err != nil {
				return err
			}
			gift.Expired = time.Now().UTC().After(created.Add(chatRetention))
		}
		if eventType == "claim" {
			gift.Type = "claim"
			gift.RecipientUsername = gift.ClaimedByUsername
		}
		if index, exists := messageIndexes[messageID]; exists {
			messages[index].Gift = &gift
		}
	}
	return rows.Err()
}

func (s *ChatService) recentSelfMessages(userID, afterID int64) ([]ChatMessage, error) {
	cutoff := time.Now().UTC().Add(-chatRetention).Format(time.RFC3339)
	var rows *sql.Rows
	var err error
	if afterID > 0 {
		rows, err = s.db.Query(`
			SELECT id, username, message, timestamp
			FROM self_chat_messages
			WHERE user_id = ? AND id > ? AND timestamp >= ?
			ORDER BY id ASC
			LIMIT ?
		`, userID, afterID, cutoff, chatRecentLimit)
	} else {
		rows, err = s.db.Query(`
			SELECT id, username, message, timestamp
			FROM (
				SELECT id, username, message, timestamp
				FROM self_chat_messages
				WHERE user_id = ? AND timestamp >= ?
				ORDER BY id DESC
				LIMIT ?
			)
			ORDER BY id ASC
		`, userID, cutoff, chatRecentLimit)
	}
	if err != nil {
		return nil, err
	}

	messages := make([]ChatMessage, 0, chatRecentLimit)
	for rows.Next() {
		var message ChatMessage
		if err := rows.Scan(&message.ID, &message.Username, &message.Message, &message.Timestamp); err != nil {
			_ = rows.Close()
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return messages, s.loadSelfAttachments(messages, userID)
}

func (s *ChatService) loadAttachments(messages []ChatMessage) error {
	if len(messages) == 0 {
		return nil
	}

	messageIndexes := make(map[int64]int, len(messages))
	placeholders := make([]string, len(messages))
	args := make([]any, len(messages))
	for index := range messages {
		messageIndexes[messages[index].ID] = index
		placeholders[index] = "?"
		args[index] = messages[index].ID
	}

	rows, err := s.db.Query(`
		SELECT message_id, id, original_name, content_type, size
		FROM chat_attachments
		WHERE message_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY message_id, id
	`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var messageID int64
		var attachment ChatAttachment
		if err := rows.Scan(&messageID, &attachment.ID, &attachment.Name, &attachment.ContentType, &attachment.Size); err != nil {
			return err
		}
		attachment.URL = fmt.Sprintf("/chat/files/%d", attachment.ID)
		index, exists := messageIndexes[messageID]
		if exists {
			messages[index].Attachments = append(messages[index].Attachments, attachment)
		}
	}
	return rows.Err()
}

func (s *ChatService) loadSelfAttachments(messages []ChatMessage, userID int64) error {
	if len(messages) == 0 {
		return nil
	}

	messageIndexes := make(map[int64]int, len(messages))
	placeholders := make([]string, len(messages))
	args := make([]any, 0, len(messages)+1)
	args = append(args, userID)
	for index := range messages {
		messageIndexes[messages[index].ID] = index
		placeholders[index] = "?"
		args = append(args, messages[index].ID)
	}

	rows, err := s.db.Query(`
		SELECT a.message_id, a.id, a.original_name, a.content_type, a.size
		FROM self_chat_attachments a
		JOIN self_chat_messages m ON m.id = a.message_id
		WHERE m.user_id = ? AND a.message_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY a.message_id, a.id
	`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var messageID int64
		var attachment ChatAttachment
		if err := rows.Scan(&messageID, &attachment.ID, &attachment.Name, &attachment.ContentType, &attachment.Size); err != nil {
			return err
		}
		attachment.URL = fmt.Sprintf("/chat/files/%d?channel=self", attachment.ID)
		if index, exists := messageIndexes[messageID]; exists {
			messages[index].Attachments = append(messages[index].Attachments, attachment)
		}
	}
	return rows.Err()
}

func (s *ChatService) cleanupExpired() error {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()

	now := time.Now().UTC()
	if !s.lastCleanup.IsZero() && now.Sub(s.lastCleanup) < chatCleanupInterval {
		return nil
	}
	cutoff := now.Add(-chatRetention).Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT stored_name
		FROM chat_attachments
		WHERE message_id IN (SELECT id FROM chat_messages WHERE timestamp < ?)
	`, cutoff)
	if err != nil {
		return err
	}
	var expiredFiles []string
	for rows.Next() {
		var storedName string
		if err := rows.Scan(&storedName); err != nil {
			_ = rows.Close()
			return err
		}
		expiredFiles = append(expiredFiles, storedName)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = tx.Query(`
		SELECT stored_name
		FROM self_chat_attachments
		WHERE message_id IN (SELECT id FROM self_chat_messages WHERE timestamp < ?)
	`, cutoff)
	if err != nil {
		return err
	}
	for rows.Next() {
		var storedName string
		if err := rows.Scan(&storedName); err != nil {
			_ = rows.Close()
			return err
		}
		expiredFiles = append(expiredFiles, storedName)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		DELETE FROM chat_attachments
		WHERE message_id IN (SELECT id FROM chat_messages WHERE timestamp < ?)
	`, cutoff); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE game_profiles
		SET gold = gold + (
			SELECT COALESCE(SUM(g.amount), 0)
			FROM chat_coin_gifts g
			JOIN chat_messages m ON m.id = g.message_id
			WHERE g.sender_user_id = game_profiles.user_id
				AND g.gift_type = 'card'
				AND g.claimed_by_user_id IS NULL
				AND m.timestamp < ?
		)
		WHERE user_id IN (
			SELECT g.sender_user_id
			FROM chat_coin_gifts g
			JOIN chat_messages m ON m.id = g.message_id
			WHERE g.gift_type = 'card'
				AND g.claimed_by_user_id IS NULL
				AND m.timestamp < ?
		)
	`, cutoff, cutoff); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM chat_coin_gifts
		WHERE message_id IN (SELECT id FROM chat_messages WHERE timestamp < ?)
	`, cutoff); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM chat_messages WHERE timestamp < ?`, cutoff); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM self_chat_attachments
		WHERE message_id IN (SELECT id FROM self_chat_messages WHERE timestamp < ?)
	`, cutoff); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM self_chat_messages WHERE timestamp < ?`, cutoff); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.lastCleanup = now
	var cleanupErr error
	for _, storedName := range expiredFiles {
		path := filepath.Join(s.filesDir, filepath.Base(storedName))
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove expired chat attachment: %w", err))
		}
	}
	if err := s.removeOrphanedExpiredFiles(cutoff); err != nil {
		cleanupErr = errors.Join(cleanupErr, err)
	}
	return cleanupErr
}

func (s *ChatService) removeOrphanedExpiredFiles(cutoff string) error {
	entries, err := os.ReadDir(s.filesDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	rows, err := s.db.Query(`
		SELECT stored_name FROM chat_attachments
		UNION
		SELECT stored_name FROM self_chat_attachments
	`)
	if err != nil {
		return err
	}
	referenced := make(map[string]struct{})
	for rows.Next() {
		var storedName string
		if err := rows.Scan(&storedName); err != nil {
			_ = rows.Close()
			return err
		}
		referenced[filepath.Base(storedName)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	cutoffTime, err := time.Parse(time.RFC3339, cutoff)
	if err != nil {
		return err
	}
	var cleanupErr error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, exists := referenced[entry.Name()]; exists {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("inspect chat attachment %q: %w", entry.Name(), err))
			continue
		}
		if info.ModTime().After(cutoffTime) {
			continue
		}
		if err := os.Remove(filepath.Join(s.filesDir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove orphaned chat attachment %q: %w", entry.Name(), err))
		}
	}
	return cleanupErr
}

func (s *ChatService) File(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/chat/files/"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}

	var storedName, contentType, originalName string
	if r.URL.Query().Get("channel") == "self" {
		user, ok := auth.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		err = s.db.QueryRow(`
			SELECT a.stored_name, a.content_type, a.original_name
			FROM self_chat_attachments a
			JOIN self_chat_messages m ON m.id = a.message_id
			WHERE a.id = ? AND m.user_id = ?
		`, id, user.ID).Scan(&storedName, &contentType, &originalName)
	} else {
		err = s.db.QueryRow(`SELECT stored_name, content_type, original_name FROM chat_attachments WHERE id = ?`, id).
			Scan(&storedName, &contentType, &originalName)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}

	path := filepath.Join(s.filesDir, filepath.Base(storedName))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	disposition := "attachment"
	if strings.HasPrefix(contentType, "image/") {
		disposition = "inline"
	}
	if contentType == "image/svg+xml" {
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, safeDownloadName(originalName)))
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

	r.Body = http.MaxBytesReader(w, r.Body, chatMaxFileSize+(1<<20))
	if err := r.ParseMultipartForm(chatMultipartMemory); err != nil {
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

	contentTypeBuffer := make([]byte, 4096)
	n, err := file.Read(contentTypeBuffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", nil, errors.New("Could not read the attachment.")
	}
	if _, err := file.Seek(0, 0); err != nil {
		return "", nil, errors.New("Could not read the attachment.")
	}
	detectedType := detectChatContentType(contentTypeBuffer[:n])
	if header.Size > chatMaxFileSize {
		return "", nil, errChatFileTooLarge
	}
	return message, &chatUpload{header: header, contentType: detectedType}, nil
}

func detectChatContentType(sample []byte) string {
	if isTIFFImage(sample) {
		return "image/tiff"
	}
	if isAVIFImage(sample) {
		return "image/avif"
	}
	if isSVGImage(sample) {
		return "image/svg+xml"
	}
	return http.DetectContentType(sample)
}

func isTIFFImage(data []byte) bool {
	return len(data) >= 4 && (bytes.Equal(data[:4], []byte{'I', 'I', 42, 0}) ||
		bytes.Equal(data[:4], []byte{'M', 'M', 0, 42}) ||
		bytes.Equal(data[:4], []byte{'I', 'I', 43, 0}) ||
		bytes.Equal(data[:4], []byte{'M', 'M', 0, 43}))
}

func isAVIFImage(data []byte) bool {
	return len(data) >= 12 &&
		string(data[4:8]) == "ftyp" &&
		(string(data[8:12]) == "avif" || string(data[8:12]) == "avis")
}

func isSVGImage(data []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		switch token := token.(type) {
		case xml.StartElement:
			return token.Name.Local == "svg"
		case xml.CharData:
			if len(bytes.TrimSpace(token)) != 0 {
				return false
			}
		case xml.Directive:
			return false
		}
	}
}

func (s *ChatService) saveChatUpload(tx *sql.Tx, messageID int64, upload *chatUpload, selfChat bool) error {
	if err := os.MkdirAll(s.filesDir, 0750); err != nil {
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
	path := filepath.Join(s.filesDir, storedName)
	target, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	written, err := io.Copy(target, io.LimitReader(source, chatMaxFileSize+1))
	if err != nil {
		_ = target.Close()
		_ = os.Remove(path)
		return err
	}
	if written > chatMaxFileSize {
		_ = target.Close()
		_ = os.Remove(path)
		return errChatFileTooLarge
	}
	if err := target.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	attachmentName := safeDownloadName(upload.header.Filename)
	if selfChat {
		_, err = tx.Exec(`
			INSERT INTO self_chat_attachments (message_id, original_name, stored_name, content_type, size)
			VALUES (?, ?, ?, ?, ?)
		`, messageID, attachmentName, storedName, upload.contentType, written)
	} else {
		_, err = tx.Exec(`
			INSERT INTO chat_attachments (message_id, original_name, stored_name, content_type, size)
			VALUES (?, ?, ?, ?, ?)
		`, messageID, attachmentName, storedName, upload.contentType, written)
	}
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

func (s *ChatService) onlineUsers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	users := make([]string, 0, len(s.lastActive))
	for username, lastSeen := range s.lastActive {
		if now.Sub(lastSeen) <= chatOnlineWindow {
			users = append(users, username)
			continue
		}
		delete(s.lastActive, username)
	}
	sort.Strings(users)
	return users
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
