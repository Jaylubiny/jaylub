package email

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestValidateResendWebhook(t *testing.T) {
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("test-secret-key"))
	body := []byte(`{"type":"email.received"}`)
	headers := signedResendHeaders(secret, body, time.Now())
	service := &Service{webhookSecret: secret}

	if !service.ValidateResendWebhook(body, headers) {
		t.Fatal("valid signed Resend webhook was rejected")
	}
	if service.ValidateResendWebhook([]byte(`{"type":"email.sent"}`), headers) {
		t.Fatal("webhook with a changed body was accepted")
	}
	if service.ValidateResendWebhook(body, signedResendHeaders(secret, body, time.Now().Add(-6*time.Minute))) {
		t.Fatal("stale Resend webhook was accepted")
	}
}

func TestSaveResendReceivedEmail(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Errorf("Authorization header = %q, want Resend API key", got)
		}
		if r.URL.Path != "/emails/receiving/message-123" {
			t.Errorf("request path = %q, want received email endpoint", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"from":"sender@example.net",
			"to":["alice@jaylub.com"],
			"received_for":["ALICE@jaylub.com","outside@example.net"],
			"subject":"Received through Resend",
			"text":"Hello from inbound mail"
		}`))
	}))
	defer server.Close()

	service, err := New(filepath.Join(t.TempDir(), "emails.db"), "test-api-key", "", "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.resendAPIBase = server.URL
	service.resendClient = server.Client()

	for range 2 {
		if err := service.SaveResendReceivedEmail(context.Background(), "message-123"); err != nil {
			t.Fatalf("SaveResendReceivedEmail() error = %v", err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("Resend API requests = %d, want one after duplicate webhook", got)
	}
	mailbox, err := service.GetMailboxEmails("alice@jaylub.com", "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailbox) != 1 {
		t.Fatalf("inbox contains %d messages, want one", len(mailbox))
	}
	if mailbox[0].Sender != "sender@example.net" ||
		mailbox[0].Recipient != "alice@jaylub.com" ||
		mailbox[0].Subject != "Received through Resend" ||
		mailbox[0].Body != "Hello from inbound mail" {
		t.Fatalf("unexpected received message: %#v", mailbox[0])
	}
}

func signedResendHeaders(secret string, body []byte, timestamp time.Time) http.Header {
	encodedSecret := strings.TrimPrefix(secret, "whsec_")
	key, err := base64.StdEncoding.DecodeString(encodedSecret)
	if err != nil {
		key = []byte(encodedSecret)
	}
	id := "msg_test-id"
	unixTimestamp := fmt.Sprint(timestamp.Unix())
	mac := hmac.New(sha256.New, key)
	_, _ = fmt.Fprintf(mac, "%s.%s.%s", id, unixTimestamp, body)

	headers := make(http.Header)
	headers.Set("svix-id", id)
	headers.Set("svix-timestamp", unixTimestamp)
	headers.Set("svix-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return headers
}
