package auth

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
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

func TestNewRestrictsDatabaseFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permission bits are not supported on Windows")
	}

	dbPath := filepath.Join(t.TempDir(), "users.db")
	service, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer service.Close()

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("database permissions = %04o, want 0600", got)
	}
}

func TestSQLiteConnectionsEnableForeignKeysAndWAL(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer service.Close()

	conn, err := service.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var foreignKeys int
	if err := conn.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want enabled", foreignKeys)
	}

	var journalMode string
	if err := conn.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("journal_mode = %q, want WAL", journalMode)
	}
}

func TestNewAcceptsRelativeDatabasePath(t *testing.T) {
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	temporaryDirectory := t.TempDir()
	if err := os.Chdir(temporaryDirectory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalWorkingDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	service, err := New("internal/database/users.db")
	if err != nil {
		t.Fatalf("New() with relative path error = %v", err)
	}
	defer service.Close()

	var userCount int
	if err := service.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		t.Fatal(err)
	}
	if userCount != 0 {
		t.Fatalf("relative-path database contains %d users, want 0", userCount)
	}
}

func TestMiddlewareRedirectsUnauthenticatedHomeAndProtectedPagesToLogin(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer service.Close()

	handler := service.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("full landing page"))
	}))

	for _, path := range []string{"/", "/chat"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
				t.Fatalf("response = (%d, %q), want redirect to login",
					response.Code, response.Header().Get("Location"))
			}
			if got := response.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("redirect Cache-Control = %q, want no-store", got)
			}
		})
	}

	staticResponse := httptest.NewRecorder()
	handler.ServeHTTP(staticResponse, httptest.NewRequest(http.MethodGet, "/static/css/site.css", nil))
	if got := staticResponse.Header().Get("Cache-Control"); got != "" {
		t.Errorf("static asset Cache-Control = %q, want unchanged", got)
	}
}

func TestSessionCookieSecureForHTTPSAndProductionDomain(t *testing.T) {
	service := &Service{}

	tests := []struct {
		name       string
		requestURL string
		host       string
		forwarded  string
		wantSecure bool
	}{
		{
			name:       "local HTTP",
			requestURL: "http://jaylub.local/login",
			host:       "jaylub.local",
			wantSecure: false,
		},
		{
			name:       "production host behind TLS proxy",
			requestURL: "http://jaylub.com/login",
			host:       "jaylub.com",
			wantSecure: true,
		},
		{
			name:       "direct HTTPS",
			requestURL: "https://jaylub.local/login",
			host:       "jaylub.local",
			wantSecure: true,
		},
		{
			name:       "does not trust forwarded protocol from arbitrary clients",
			requestURL: "http://jaylub.local/login",
			host:       "jaylub.local",
			forwarded:  "https",
			wantSecure: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.requestURL, nil)
			request.Host = test.host
			if test.forwarded != "" {
				request.Header.Set("X-Forwarded-Proto", test.forwarded)
			}
			cookie := service.sessionCookie(request, "session-token", time.Now().Add(time.Hour))
			if cookie.Secure != test.wantSecure {
				t.Errorf("cookie Secure = %v, want %v", cookie.Secure, test.wantSecure)
			}
		})
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
	if err := service.AcceptTerms(userID); err != nil {
		t.Fatalf("accept terms for test user: %v", err)
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

func TestMiddlewareRequiresCurrentTermsBeforeProtectedPages(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	if _, err := service.db.Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		"terms-user",
		"unused-hash",
	); err != nil {
		t.Fatal(err)
	}
	const token = "terms-test-session"
	if _, err := service.db.Exec(
		`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (
			(SELECT id FROM users WHERE username = ?), ?, ?
		)`,
		"terms-user",
		hashToken(token),
		time.Now().Add(time.Hour),
	); err != nil {
		t.Fatal(err)
	}

	handler := service.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFromContext(r.Context()); !ok {
			t.Error("authenticated user missing from request context")
		}
		w.WriteHeader(http.StatusOK)
	}))
	protectedRequest := httptest.NewRequest(http.MethodGet, "https://jaylub.com/chat", nil)
	protectedRequest.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	protectedResponse := httptest.NewRecorder()
	handler.ServeHTTP(protectedResponse, protectedRequest)
	if protectedResponse.Code != http.StatusSeeOther || protectedResponse.Header().Get("Location") != "/terms" {
		t.Fatalf("unaccepted protected response = (%d, %q), want redirect to /terms",
			protectedResponse.Code, protectedResponse.Header().Get("Location"))
	}

	termsRequest := httptest.NewRequest(http.MethodGet, "https://jaylub.com/terms", nil)
	termsRequest.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	termsResponse := httptest.NewRecorder()
	handler.ServeHTTP(termsResponse, termsRequest)
	if termsResponse.Code != http.StatusOK {
		t.Fatalf("terms page status = %d, want %d", termsResponse.Code, http.StatusOK)
	}

	if err := service.AcceptTerms(1); err != nil {
		t.Fatal(err)
	}
	acceptedResponse := httptest.NewRecorder()
	handler.ServeHTTP(acceptedResponse, protectedRequest)
	if acceptedResponse.Code != http.StatusOK {
		t.Fatalf("accepted protected status = %d, want %d", acceptedResponse.Code, http.StatusOK)
	}
}

func TestLoginRedirectsToTermsUntilAccepted(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if _, err := service.db.Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		"terms-user",
		"unused-hash",
	); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Exec(`UPDATE users SET password_hash = ? WHERE username = ?`, string(hash), "terms-user"); err != nil {
		t.Fatal(err)
	}

	form := url.Values{"username": {"terms-user"}, "password": {"correct-password"}}
	request := httptest.NewRequest(
		http.MethodPost,
		"https://jaylub.com/login",
		strings.NewReader(form.Encode()),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test source")
	}
	projectRoot := filepath.Join(filepath.Dir(sourceFile), "..", "..")
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(workingDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	service.LoginPage().ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/terms" {
		t.Fatalf("login response = (%d, %q), want redirect to /terms",
			response.Code, response.Header().Get("Location"))
	}
}
