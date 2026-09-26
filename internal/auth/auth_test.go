package auth

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewDoesNotCreateOrResetUsers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "users.db")

	service, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var userCount int
	if err := service.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("New() created %d users; want no seeded users", userCount)
	}

	const existingHash = "existing-password-hash"
	if _, err := service.db.Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		"existing-user",
		existingHash,
	); err != nil {
		t.Fatalf("insert existing user: %v", err)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("close service: %v", err)
	}

	service, err = New(dbPath)
	if err != nil {
		t.Fatalf("New() on existing database error = %v", err)
	}
	defer service.Close()

	var username, passwordHash string
	if err := service.db.QueryRow(`SELECT username, password_hash FROM users`).Scan(&username, &passwordHash); err != nil {
		t.Fatalf("read existing user: %v", err)
	}
	if username != "existing-user" || passwordHash != existingHash {
		t.Fatalf("existing account changed on startup: username=%q passwordHash=%q", username, passwordHash)
	}
}

func TestMiddlewareAllowsPublicHomeButKeepsApplicationProtected(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer service.Close()

	handler := service.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("full landing page"))
	}))

	homeResponse := httptest.NewRecorder()
	handler.ServeHTTP(homeResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if homeResponse.Code != http.StatusOK || homeResponse.Body.String() != "full landing page" {
		t.Fatalf("home response = (%d, %q), want full unauthenticated response", homeResponse.Code, homeResponse.Body.String())
	}

	protectedResponse := httptest.NewRecorder()
	handler.ServeHTTP(protectedResponse, httptest.NewRequest(http.MethodGet, "/chat", nil))
	if protectedResponse.Code != http.StatusSeeOther || protectedResponse.Header().Get("Location") != "/login" {
		t.Fatalf("protected route response = (%d, %q), want redirect to login", protectedResponse.Code, protectedResponse.Header().Get("Location"))
	}
}

func TestMiddlewareAddsUserContextOnPublicHomeWhenSessionIsValid(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer service.Close()

	result, err := service.db.Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		"homepage-user",
		"unused-hash",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get user ID: %v", err)
	}

	const sessionToken = "valid-test-session"
	if _, err := service.db.Exec(
		`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		userID,
		hashToken(sessionToken),
		time.Now().Add(time.Hour),
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	handler := service.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok {
			t.Error("authenticated homepage request has no user context")
			http.Error(w, "missing user", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(user.Username))
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: cookieName, Value: sessionToken})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != "homepage-user" {
		t.Fatalf("homepage response = (%d, %q), want authenticated user context", response.Code, response.Body.String())
	}
}

func TestLoginTemplateRendersMetadataAndNoIndex(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test source")
	}
	templatePath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "web", "templates", "login.html")
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		t.Fatalf("parse login template: %v", err)
	}

	data := defaultLoginPageData()
	response := httptest.NewRecorder()
	if err := tmpl.Execute(response, data); err != nil {
		t.Fatalf("render login template: %v", err)
	}
	html := response.Body.String()
	for _, fragment := range []string{
		"<title>Sign in to Jaylub</title>",
		`<meta name="description" content="Sign in to your Jaylub account to access the community chat, games, and member features.">`,
		`<link rel="canonical" href="https://jaylub.com/login">`,
		`<meta name="robots" content="noindex, nofollow">`,
		`<meta property="og:type" content="website">`,
		`<meta property="og:url" content="https://jaylub.com/login">`,
	} {
		if !strings.Contains(html, fragment) {
			t.Errorf("rendered login page is missing %q", fragment)
		}
	}
}
