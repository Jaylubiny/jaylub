package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	From        string                      `json:"from"`
	To          string                      `json:"to"`
	Subject     string                      `json:"subject"`
	Text        string                      `json:"text"`
	Attachments []IncomingAttachmentPayload `json:"attachments"`
}

type IncomingAttachmentPayload struct {
	Filename string `json:"filename"`
	MIMEType string `json:"mimeType"`
	Size     int64  `json:"size"`
	Data     string `json:"data"`
}

func (h *EmailHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if !requireEmailMethod(w, r, http.MethodPost) {
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 36<<20))
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "Email payload exceeds the allowed size", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Invalid email payload", http.StatusBadRequest)
		return
	}
	if h.emailService.ValidateResendWebhook(body, r.Header) {
		var event struct {
			Type string `json:"type"`
			Data struct {
				EmailID string `json:"email_id"`
			} `json:"data"`
		}
		if err := decodeJSONPayload(body, &event, false); err != nil {
			http.Error(w, "Invalid email event", http.StatusBadRequest)
			return
		}
		if event.Type != "email.received" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if event.Data.EmailID == "" {
			http.Error(w, "Received email ID is required", http.StatusBadRequest)
			return
		}
		if err := h.emailService.SaveResendReceivedEmail(r.Context(), event.Data.EmailID); err != nil {
			log.Printf("process Resend inbound email: %v", err)
			http.Error(w, "Unable to save incoming email", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		return
	}
	if !h.emailService.ValidateSecret(r.Header.Get("X-Secret-Auth")) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var payload IncomingPayload
	if err := decodeJSONPayload(body, &payload, true); err != nil {
		http.Error(w, "Invalid email payload", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.From) == "" || strings.TrimSpace(payload.To) == "" {
		http.Error(w, "Sender and recipient are required", http.StatusBadRequest)
		return
	}
	if len(payload.Attachments) > email.MaxIncomingAttachments {
		http.Error(w, "Too many email attachments", http.StatusRequestEntityTooLarge)
		return
	}
	attachments := make([]email.IncomingAttachment, 0, len(payload.Attachments))
	totalSize := int64(0)
	for _, incoming := range payload.Attachments {
		filename, ok := safeAttachmentFilename(incoming.Filename)
		if !ok || incoming.Size < 0 {
			http.Error(w, "Invalid email attachment metadata", http.StatusBadRequest)
			return
		}
		if incoming.Size > email.MaxIncomingAttachmentBytes {
			http.Error(w, "Email attachments must be 10 MiB or smaller", http.StatusRequestEntityTooLarge)
			return
		}
		if incoming.Size > email.MaxIncomingAttachmentsTotalBytes-totalSize {
			http.Error(w, "Email attachments exceed the 25 MiB total limit", http.StatusRequestEntityTooLarge)
			return
		}
		data, err := base64.StdEncoding.Strict().DecodeString(incoming.Data)
		if err != nil || int64(len(data)) != incoming.Size {
			http.Error(w, "Invalid base64 data or attachment size", http.StatusBadRequest)
			return
		}
		mimeType := "application/octet-stream"
		if incoming.MIMEType != "" {
			parsedType, _, err := mime.ParseMediaType(incoming.MIMEType)
			if err != nil || parsedType == "" {
				http.Error(w, "Invalid attachment MIME type", http.StatusBadRequest)
				return
			}
			mimeType = parsedType
		}
		totalSize += incoming.Size
		attachments = append(attachments, email.IncomingAttachment{
			Filename: filename,
			MIMEType: mimeType,
			Size:     incoming.Size,
			Data:     data,
		})
	}
	if err := h.emailService.SaveIncomingWithAttachments(payload.From, payload.To, payload.Subject, payload.Text, attachments); err != nil {
		log.Printf("save incoming email: %v", err)
		http.Error(w, "Unable to save incoming email", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func safeAttachmentFilename(filename string) (string, bool) {
	filename = strings.TrimSpace(strings.ReplaceAll(filename, `\`, `/`))
	filename = strings.TrimPrefix(filename, "/")
	if index := strings.LastIndex(filename, "/"); index >= 0 {
		filename = filename[index+1:]
	}
	if filename == "" || len(filename) > 255 || !utf8.ValidString(filename) {
		return "", false
	}
	for _, char := range filename {
		if unicode.IsControl(char) {
			return "", false
		}
	}
	return filename, true
}

func (h *EmailHandler) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	if !requireEmailMethod(w, r, http.MethodGet) {
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	idText := strings.TrimPrefix(r.URL.Path, "/api/emails/attachments/")
	if idText == "" || strings.Contains(idText, "/") {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	attachment, path, err := h.emailService.AttachmentForMailbox(email.MailboxAddress(user.Username), id)
	if errors.Is(err, email.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("find email attachment %d for user %d: %v", id, user.ID, err)
		http.Error(w, "Unable to load attachment", http.StatusInternalServerError)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		log.Printf("open email attachment %d for user %d: %v", id, user.ID, err)
		http.Error(w, "Unable to load attachment", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": attachment.Filename})
	if disposition == "" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", attachment.MIMEType)
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, attachment.Filename, time.Time{}, file)
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	return decodeJSONPayload(body, target, true)
}

func decodeJSONPayload(body []byte, target any, disallowUnknownFields bool) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if disallowUnknownFields {
		decoder.DisallowUnknownFields()
	}
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
