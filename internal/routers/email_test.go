package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"jaylub/internal/auth"
	"jaylub/internal/email"
)

func TestEmailWebhookIsPublicOnlyWithSecret(t *testing.T) {
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(workingDir)

	dir := t.TempDir()
	authService, err := auth.New(filepath.Join(dir, "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer authService.Close()

	emailService, err := email.NewWithSender(filepath.Join(dir, "emails.db"), nil, "webhook-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer emailService.Close()
	handler := Email(emailService, authService)

	request := httptest.NewRequest(http.MethodPost, "/api/webhooks/incoming", strings.NewReader(
		`{"from":"sender@example.com","to":"alice@jaylub.com","subject":"Hello","text":"Body"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("webhook without secret returned %d, want %d", response.Code, http.StatusUnauthorized)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/webhooks/incoming", strings.NewReader(
		`{"from":"sender@example.com","to":"alice@jaylub.com","subject":"Hello","text":"Body"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Secret-Auth", "webhook-secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("webhook with secret returned %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if err := emailService.SaveIncoming("sender@example.com", "preclik@jaylub.com", "Private", "For preclik"); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/emails", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Fatalf("unauthenticated API request returned %d with location %q", response.Code, response.Header().Get("Location"))
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	_, err = authService.DB().Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		"preclik", string(passwordHash),
	)
	if err != nil {
		t.Fatal(err)
	}
	termsRequest := httptest.NewRequest(http.MethodPost, "http://email.jaylub.com/terms", nil)
	termsResponse := httptest.NewRecorder()
	if err := authService.AcceptTermsOnDevice(termsResponse, termsRequest); err != nil {
		t.Fatal(err)
	}
	termsCookies := termsResponse.Result().Cookies()
	if len(termsCookies) == 0 {
		t.Fatal("terms acceptance did not issue a device cookie")
	}
	loginRequest := httptest.NewRequest(http.MethodPost, "https://jaylub.com/login", nil)
	loginResponse := httptest.NewRecorder()
	if err := authService.Login(loginResponse, loginRequest, "preclik", "test-password"); err != nil {
		t.Fatal(err)
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login did not issue a session cookie")
	}
	if cookies[0].Name != "jaylub_session_v2" || cookies[0].Domain != "jaylub.com" || !cookies[0].Secure {
		t.Fatalf("main-site login issued an invalid cross-subdomain cookie: %+v", cookies[0])
	}

	request = httptest.NewRequest(http.MethodGet, "https://email.jaylub.com/api/emails", nil)
	request.AddCookie(&http.Cookie{Name: "jaylub_session", Value: "stale-host-only-session"})
	request.AddCookie(cookies[0])
	request.AddCookie(termsCookies[0])
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated mailbox request returned %d: %s", response.Code, response.Body.String())
	}
	var mailbox []email.Email
	if err := json.NewDecoder(response.Body).Decode(&mailbox); err != nil {
		t.Fatal(err)
	}
	if len(mailbox) != 1 || mailbox[0].Recipient != "preclik@jaylub.com" {
		t.Fatalf("authenticated mailbox returned another user's email: %#v", mailbox)
	}

	request = httptest.NewRequest(http.MethodGet, "https://email.jaylub.com/", nil)
	request.AddCookie(&http.Cookie{Name: "jaylub_session", Value: "stale-host-only-session"})
	request.AddCookie(cookies[0])
	request.AddCookie(termsCookies[0])
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "preclik@jaylub.com") {
		t.Fatalf("authenticated email page returned %d: %s", response.Code, response.Body.String())
	}
}
