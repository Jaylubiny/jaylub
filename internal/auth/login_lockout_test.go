package auth

import (
	"database/sql"
	"errors"
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

func TestLoginLockoutEscalatesAndPersistsByUsername(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "users.db")
	service, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if service != nil {
			_ = service.Close()
		}
	})

	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		"lock-user", string(hash),
	); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	response := httptest.NewRecorder()
	for attempt := 1; attempt <= initialLoginFailureLimit; attempt++ {
		err := service.Login(response, request, "lock-user", "wrong-password")
		if attempt < initialLoginFailureLimit && !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d error = %v, want invalid credentials", attempt, err)
		}
		if attempt == initialLoginFailureLimit {
			assertLockoutDuration(t, err, initialLoginLockoutDuration)
		}
	}

	var failedAttempts, lockoutStage int
	var lockoutUntil sql.NullTime
	if err := service.db.QueryRow(
		`SELECT failed_attempts, lockout_stage, lockout_until FROM login_attempts WHERE username = ?`,
		"lock-user",
	).Scan(&failedAttempts, &lockoutStage, &lockoutUntil); err != nil {
		t.Fatal(err)
	}
	if failedAttempts != 0 || lockoutStage != 1 || !lockoutUntil.Valid {
		t.Fatalf("initial lockout state = (%d, %d, %v)", failedAttempts, lockoutStage, lockoutUntil)
	}
	var activeLockout *LoginLockedError
	if err := service.Login(response, request, "lock-user", "wrong-password"); !errors.As(err, &activeLockout) ||
		activeLockout.RetryAfter <= 0 {
		t.Fatalf("login during active first lockout error = %v, want remaining lockout duration", err)
	}

	expireTestLockout(t, service)
	for attempt := 1; attempt <= repeatedLoginFailureLimit; attempt++ {
		err := service.Login(response, request, "lock-user", "wrong-password")
		if attempt < repeatedLoginFailureLimit && !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("post-expiry attempt %d error = %v, want invalid credentials", attempt, err)
		}
		if attempt == repeatedLoginFailureLimit {
			assertLockoutDuration(t, err, repeatedLoginLockoutDuration)
		}
	}

	expireTestLockout(t, service)
	for attempt := 1; attempt <= repeatedLoginFailureLimit; attempt++ {
		err := service.Login(response, request, "lock-user", "wrong-password")
		if attempt == repeatedLoginFailureLimit {
			assertLockoutDuration(t, err, repeatedLoginLockoutDuration)
		}
	}

	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	service = nil
	service, err = New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.db.QueryRow(
		`SELECT failed_attempts, lockout_stage FROM login_attempts WHERE username = ?`,
		"lock-user",
	).Scan(&failedAttempts, &lockoutStage); err != nil {
		t.Fatal(err)
	}
	if failedAttempts != 0 || lockoutStage != 3 {
		t.Fatalf("subsequent lockout state after restart = (%d, %d), want (0, 3)", failedAttempts, lockoutStage)
	}
	var persistedLockout *LoginLockedError
	if err := service.Login(response, request, "lock-user", "wrong-password"); !errors.As(err, &persistedLockout) ||
		persistedLockout.RetryAfter <= 0 {
		t.Fatalf("login during persisted lockout error = %v, want remaining lockout duration", err)
	}
}

func TestSuccessfulLoginClearsFailedAttempts(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		"reset-user", string(hash),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Exec(
		`INSERT INTO login_attempts (username, failed_attempts, lockout_stage) VALUES (?, ?, ?)`,
		"reset-user", 3, 0,
	); err != nil {
		t.Fatal(err)
	}
	if err := service.Login(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/login", nil),
		"reset-user",
		"correct-password",
	); err != nil {
		t.Fatalf("correct password login error = %v", err)
	}
	var attempts int
	err = service.db.QueryRow(`SELECT failed_attempts FROM login_attempts WHERE username = ?`, "reset-user").Scan(&attempts)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("login attempt record remains after successful login: attempts=%d, err=%v", attempts, err)
	}
}

func TestUnknownUsernameAttemptsAreAlsoLockedOut(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	response := httptest.NewRecorder()
	for attempt := 1; attempt <= initialLoginFailureLimit; attempt++ {
		username := "missing-user"
		if attempt%2 == 0 {
			username = "MISSING-USER"
		}
		err := service.Login(response, request, username, "wrong-password")
		if attempt < initialLoginFailureLimit && !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("unknown-user attempt %d error = %v, want invalid credentials", attempt, err)
		}
		if attempt == initialLoginFailureLimit {
			assertLockoutDuration(t, err, initialLoginLockoutDuration)
		}
	}
	var locked *LoginLockedError
	if err := service.Login(response, request, "missing-user", "another-password"); !errors.As(err, &locked) {
		t.Fatalf("unknown username was not locked after %d attempts: %v", initialLoginFailureLimit, err)
	}
}

func TestLoginPageReportsLockoutWithRetryAfter(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		"page-lock-user", string(hash),
	); err != nil {
		t.Fatal(err)
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test source")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(filepath.Dir(sourceFile), "..", "..")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(workingDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	handler := service.LoginPage()

	form := url.Values{"username": {"page-lock-user"}, "password": {"wrong-password"}}
	for attempt := 1; attempt <= initialLoginFailureLimit; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		wantStatus := http.StatusUnauthorized
		if attempt == initialLoginFailureLimit {
			wantStatus = http.StatusTooManyRequests
		}
		if response.Code != wantStatus {
			t.Fatalf("attempt %d status = %d, want %d", attempt, response.Code, wantStatus)
		}
		if attempt == initialLoginFailureLimit {
			if response.Header().Get("Retry-After") != "60" {
				t.Fatalf("Retry-After = %q, want 60", response.Header().Get("Retry-After"))
			}
			if !strings.Contains(response.Body.String(), "Sign-in temporarily locked") ||
				!strings.Contains(response.Body.String(), `data-lockout-countdown data-seconds="60"`) {
				t.Fatalf("lockout response missing retry message: %s", response.Body.String())
			}
		}
	}
}

func expireTestLockout(t *testing.T, service *Service) {
	t.Helper()
	if _, err := service.db.Exec(
		`UPDATE login_attempts SET lockout_until = datetime('now', '-1 second') WHERE username = ?`,
		"lock-user",
	); err != nil {
		t.Fatal(err)
	}
}

func assertLockoutDuration(t *testing.T, err error, want time.Duration) {
	t.Helper()
	var locked *LoginLockedError
	if !errors.As(err, &locked) || locked.RetryAfter != want {
		t.Fatalf("lockout error = %v, want duration %s", err, want)
	}
}
