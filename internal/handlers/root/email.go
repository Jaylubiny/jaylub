package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/mail"
	"strings"

	"jaylub/internal/auth"
	"jaylub/internal/email"
)

type EmailHandler struct {
	emailService *email.Service
}

func NewEmailHandler(service *email.Service) *EmailHandler {
	return &EmailHandler{emailService: service}
}

type IncomingPayload struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

func (h *EmailHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if !requireEmailMethod(w, r, http.MethodPost) {
		return
	}
	if !h.emailService.ValidateSecret(r.Header.Get("X-Secret-Auth")) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var payload IncomingPayload
	if err := decodeJSON(w, r, &payload, 2<<20); err != nil {
		http.Error(w, "Invalid email payload", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.From) == "" || strings.TrimSpace(payload.To) == "" {
		http.Error(w, "Sender and recipient are required", http.StatusBadRequest)
		return
	}
	if err := h.emailService.SaveIncoming(payload.From, payload.To, payload.Subject, payload.Text); err != nil {
		log.Printf("save incoming email: %v", err)
		http.Error(w, "Unable to save incoming email", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *EmailHandler) GetEmails(w http.ResponseWriter, r *http.Request) {
	if !requireEmailMethod(w, r, http.MethodGet) {
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "inbox"
	}
	emails, err := h.emailService.GetMailboxEmails(email.MailboxAddress(user.Username), folder)
	if err != nil {
		if errors.Is(err, email.ErrInvalidFolder) {
			http.Error(w, "Invalid mailbox folder", http.StatusBadRequest)
			return
		}
		log.Printf("load email mailbox: %v", err)
		http.Error(w, "Unable to load mailbox", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, emails)
}

type SendPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (h *EmailHandler) SendEmail(w http.ResponseWriter, r *http.Request) {
	if !requireEmailMethod(w, r, http.MethodPost) {
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var payload SendPayload
	if err := decodeJSON(w, r, &payload, 1<<20); err != nil {
		http.Error(w, "Invalid email payload", http.StatusBadRequest)
		return
	}
	payload.To = strings.TrimSpace(payload.To)
	payload.Subject = strings.TrimSpace(payload.Subject)
	if len(payload.To) > 320 || len(payload.Subject) > 998 || len(payload.Body) > 1<<20 {
		http.Error(w, "Email exceeds the allowed size", http.StatusBadRequest)
		return
	}
	to, err := mail.ParseAddress(payload.To)
	if err != nil || to.Address != payload.To || strings.ContainsAny(payload.To, "\r\n") {
		http.Error(w, "A valid recipient address is required", http.StatusBadRequest)
		return
	}
	if payload.Subject == "" || strings.ContainsAny(payload.Subject, "\r\n") || strings.TrimSpace(payload.Body) == "" {
		http.Error(w, "Subject and message body are required", http.StatusBadRequest)
		return
	}
	from := email.MailboxAddress(user.Username)
	if err := h.emailService.SendEmail(from, to.Address, payload.Subject, payload.Body); err != nil {
		log.Printf("send email for user %d: %v", user.ID, err)
		http.Error(w, "Unable to send email. Please try again later.", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type emailActionPayload struct {
	ID   int64 `json:"id"`
	Read *bool `json:"read,omitempty"`
}

func (h *EmailHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	if !requireEmailMethod(w, r, http.MethodPost) {
		return
	}
	user, payload, ok := h.actionPayload(w, r)
	if !ok {
		return
	}
	if payload.Read == nil {
		http.Error(w, "Read state is required", http.StatusBadRequest)
		return
	}
	if err := h.emailService.MarkRead(email.MailboxAddress(user.Username), payload.ID, *payload.Read); err != nil {
		h.writeActionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *EmailHandler) MoveToTrash(w http.ResponseWriter, r *http.Request) {
	if !requireEmailMethod(w, r, http.MethodPost) {
		return
	}
	user, payload, ok := h.actionPayload(w, r)
	if !ok {
		return
	}
	if err := h.emailService.MoveToTrash(email.MailboxAddress(user.Username), payload.ID); err != nil {
		h.writeActionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *EmailHandler) Restore(w http.ResponseWriter, r *http.Request) {
	if !requireEmailMethod(w, r, http.MethodPost) {
		return
	}
	user, payload, ok := h.actionPayload(w, r)
	if !ok {
		return
	}
	if err := h.emailService.Restore(email.MailboxAddress(user.Username), payload.ID); err != nil {
		h.writeActionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *EmailHandler) DeleteFromTrash(w http.ResponseWriter, r *http.Request) {
	if !requireEmailMethod(w, r, http.MethodPost) {
		return
	}
	user, payload, ok := h.actionPayload(w, r)
	if !ok {
		return
	}
	if err := h.emailService.DeleteFromTrash(email.MailboxAddress(user.Username), payload.ID); err != nil {
		h.writeActionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *EmailHandler) actionPayload(w http.ResponseWriter, r *http.Request) (auth.User, emailActionPayload, bool) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return auth.User{}, emailActionPayload{}, false
	}
	var payload emailActionPayload
	if err := decodeJSON(w, r, &payload, 4<<10); err != nil || payload.ID < 1 {
		http.Error(w, "A valid email ID is required", http.StatusBadRequest)
		return auth.User{}, emailActionPayload{}, false
	}
	return user, payload, true
}

func (h *EmailHandler) writeActionError(w http.ResponseWriter, err error) {
	if errors.Is(err, email.ErrNotFound) {
		http.Error(w, "Email not found in this mailbox", http.StatusNotFound)
		return
	}
	log.Printf("update email mailbox: %v", err)
	http.Error(w, "Unable to update email", http.StatusInternalServerError)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("content type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write email API response: %v", err)
	}
}

func requireEmailMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	return false
}
