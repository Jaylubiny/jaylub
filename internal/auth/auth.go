package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName          = "jaylub_session"
	termsDeviceCookie   = "jaylub_terms_device"
	sessionDuration     = 30 * 24 * time.Hour
	termsDeviceDuration = 365 * 24 * time.Hour
	CurrentTermsVersion = "2026-10-06-device"
)

type contextKey string

const userContextKey contextKey = "authUser"

type User struct {
	ID       int64
	Username string
}

type loginPageData struct {
	Title           string
	MetaDescription string
	CanonicalURL    string
	OpenGraphTitle  string
	OpenGraphDesc   string
	OpenGraphType   string
	OpenGraphURL    string
	Robots          string
	Error           string
}

type ChatSettings struct {
	TimeFormat        string
	MessageDensity    string
	ShowTimestamps    bool
	ShowImagePreviews bool
}

type Service struct {
	db *sql.DB
}

func New(dbPath string) (*Service, error) {
	absoluteDBPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	dbPath = absoluteDBPath

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	dbURL := url.URL{
		Scheme: "file",
		Path:   dbPath,
		RawQuery: url.Values{
			"_foreign_keys": {"on"},
			"_busy_timeout": {"5000"},
			"_journal_mode": {"WAL"},
		}.Encode(),
	}
	db, err := sql.Open("sqlite3", dbURL.String())
	if err != nil {
		return nil, err
	}

	service := &Service{db: db}
	if err := service.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(dbPath, 0600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("secure database permissions: %w", err)
	}

	return service, nil
}

func (s *Service) Close() error {
	return s.db.Close()
}

func (s *Service) DB() *sql.DB {
	return s.db
}

func DefaultChatSettings() ChatSettings {
	return ChatSettings{
		TimeFormat:        "24h",
		MessageDensity:    "comfortable",
		ShowTimestamps:    true,
		ShowImagePreviews: true,
	}
}

func (s *Service) ChatSettings(userID int64) (ChatSettings, error) {
	settings := DefaultChatSettings()
	var showTimestamps, showImagePreviews int
	err := s.db.QueryRow(`
		SELECT time_format, message_density, show_timestamps, show_image_previews
		FROM chat_settings
		WHERE user_id = ?
	`, userID).Scan(
		&settings.TimeFormat,
		&settings.MessageDensity,
		&showTimestamps,
		&showImagePreviews,
	)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db.Exec(`INSERT INTO chat_settings (user_id) VALUES (?)`, userID)
		return settings, err
	}
	if err != nil {
		return settings, err
	}
	settings.ShowTimestamps = showTimestamps != 0
	settings.ShowImagePreviews = showImagePreviews != 0
	return settings, nil
}

func (s *Service) SaveChatSettings(userID int64, settings ChatSettings) error {
	_, err := s.db.Exec(`
		INSERT INTO chat_settings (user_id, time_format, message_density, show_timestamps, show_image_previews)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			time_format = excluded.time_format,
			message_density = excluded.message_density,
			show_timestamps = excluded.show_timestamps,
			show_image_previews = excluded.show_image_previews
	`, userID, settings.TimeFormat, settings.MessageDensity, settings.ShowTimestamps, settings.ShowImagePreviews)
	return err
}

func (s *Service) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS terms_acceptances (
			user_id INTEGER NOT NULL,
			terms_version TEXT NOT NULL,
			accepted_at DATETIME NOT NULL,
			PRIMARY KEY (user_id, terms_version),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS terms_device_acceptances (
			token_hash TEXT PRIMARY KEY,
			terms_version TEXT NOT NULL,
			accepted_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_terms_device_acceptances_expires
			ON terms_device_acceptances(expires_at);

		CREATE TABLE IF NOT EXISTS chat_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			message TEXT NOT NULL,
			timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_chat_messages_timestamp ON chat_messages(timestamp);
		CREATE INDEX IF NOT EXISTS idx_chat_messages_id ON chat_messages(id);

		CREATE TABLE IF NOT EXISTS chat_attachments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id INTEGER NOT NULL,
			original_name TEXT NOT NULL,
			stored_name TEXT NOT NULL UNIQUE,
			content_type TEXT NOT NULL,
			size INTEGER NOT NULL,
			FOREIGN KEY (message_id) REFERENCES chat_messages(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_chat_attachments_message_id ON chat_attachments(message_id);

		CREATE TABLE IF NOT EXISTS chat_coin_gifts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id INTEGER NOT NULL UNIQUE,
			gift_type TEXT NOT NULL CHECK (gift_type IN ('direct', 'card')),
			amount INTEGER NOT NULL CHECK (amount > 0),
			sender_user_id INTEGER NOT NULL,
			sender_username TEXT NOT NULL,
			recipient_user_id INTEGER,
			recipient_username TEXT,
			claimed_by_user_id INTEGER,
			claimed_by_username TEXT,
			claim_message_id INTEGER,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (message_id) REFERENCES chat_messages(id) ON DELETE CASCADE,
			FOREIGN KEY (sender_user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (recipient_user_id) REFERENCES users(id) ON DELETE SET NULL,
			FOREIGN KEY (claimed_by_user_id) REFERENCES users(id) ON DELETE SET NULL,
			FOREIGN KEY (claim_message_id) REFERENCES chat_messages(id) ON DELETE SET NULL
		);

		CREATE INDEX IF NOT EXISTS idx_chat_coin_gifts_claimable ON chat_coin_gifts(gift_type, claimed_by_user_id);
		CREATE INDEX IF NOT EXISTS idx_chat_coin_gifts_claim_message ON chat_coin_gifts(claim_message_id);

		CREATE TABLE IF NOT EXISTS self_chat_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			username TEXT NOT NULL,
			message TEXT NOT NULL,
			timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_self_chat_messages_user_id_id ON self_chat_messages(user_id, id);
		CREATE INDEX IF NOT EXISTS idx_self_chat_messages_timestamp ON self_chat_messages(timestamp);

		CREATE TABLE IF NOT EXISTS self_chat_attachments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id INTEGER NOT NULL,
			original_name TEXT NOT NULL,
			stored_name TEXT NOT NULL UNIQUE,
			content_type TEXT NOT NULL,
			size INTEGER NOT NULL,
			FOREIGN KEY (message_id) REFERENCES self_chat_messages(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_self_chat_attachments_message_id ON self_chat_attachments(message_id);

		CREATE TABLE IF NOT EXISTS chat_reads (
			user_id INTEGER PRIMARY KEY,
			last_read_at DATETIME NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS chat_settings (
			user_id INTEGER PRIMARY KEY,
			time_format TEXT NOT NULL DEFAULT '24h',
			message_density TEXT NOT NULL DEFAULT 'comfortable',
			show_timestamps INTEGER NOT NULL DEFAULT 1,
			show_image_previews INTEGER NOT NULL DEFAULT 1,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS game_profiles (
			user_id INTEGER PRIMARY KEY,
			username TEXT NOT NULL,
			gold INTEGER NOT NULL DEFAULT 0,
			lifetime_kills INTEGER NOT NULL DEFAULT 0,
			selected_character TEXT NOT NULL DEFAULT 'jaylub',
			goblin_jaylub_unlocked INTEGER NOT NULL DEFAULT 0,
			vampire_jaylub_unlocked INTEGER NOT NULL DEFAULT 0,
			bulvy_jaylub_unlocked INTEGER NOT NULL DEFAULT 0,
			bulvy_damage_level INTEGER NOT NULL DEFAULT 0,
			damage_level INTEGER NOT NULL DEFAULT 0,
			max_hp_level INTEGER NOT NULL DEFAULT 0,
			attack_speed_level INTEGER NOT NULL DEFAULT 0,
			move_speed_level INTEGER NOT NULL DEFAULT 0,
			piercing_level INTEGER NOT NULL DEFAULT 0,
			ability_damage_level INTEGER NOT NULL DEFAULT 0,
			aura_damage_level INTEGER NOT NULL DEFAULT 0,
			football_damage_level INTEGER NOT NULL DEFAULT 0,
			bike_damage_level INTEGER NOT NULL DEFAULT 0,
			pigeon_damage_level INTEGER NOT NULL DEFAULT 0,
			game_level INTEGER NOT NULL DEFAULT 0,
			game_xp INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS game_leaderboard (
			user_id INTEGER PRIMARY KEY,
			username TEXT NOT NULL,
			total_kills INTEGER NOT NULL DEFAULT 0,
			best_run_kills INTEGER NOT NULL DEFAULT 0,
			best_run_seconds INTEGER NOT NULL DEFAULT 0,
			level INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_game_leaderboard_total_kills ON game_leaderboard(total_kills DESC);
	`)
	if err != nil {
		return err
	}
	if err := s.addColumnIfMissing("chat_settings", "show_image_previews", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_leaderboard", "best_run_kills", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_leaderboard", "best_run_seconds", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "goblin_jaylub_unlocked", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "vampire_jaylub_unlocked", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "bulvy_jaylub_unlocked", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "bulvy_damage_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "piercing_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "ability_damage_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "aura_damage_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "football_damage_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "bike_damage_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "pigeon_damage_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "game_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_profiles", "game_xp", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("game_leaderboard", "level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_game_leaderboard_best_run_kills ON game_leaderboard(best_run_kills DESC);
		CREATE INDEX IF NOT EXISTS idx_game_leaderboard_best_run_seconds ON game_leaderboard(best_run_seconds DESC);
	`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_game_leaderboard_level ON game_leaderboard(level DESC)`)
	return err
}

func (s *Service) addColumnIfMissing(table, column, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

func (s *Service) Login(w http.ResponseWriter, r *http.Request, username, password string) error {
	user, passwordHash, err := s.userWithPasswordHash(username)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return errors.New("invalid credentials")
	}

	token, err := randomToken()
	if err != nil {
		return err
	}

	expiresAt := time.Now().Add(sessionDuration)
	_, err = s.db.Exec(
		`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		user.ID,
		hashToken(token),
		expiresAt.UTC(),
	)
	if err != nil {
		return err
	}

	// The raw session token is sent only in an HttpOnly cookie; the database
	// stores a SHA-256 hash so a leaked database cannot be used as active login cookies.
	http.SetCookie(w, s.sessionCookie(r, token, expiresAt))
	return nil
}

func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(cookie.Value))
	}

	expired := s.sessionCookie(r, "", time.Unix(0, 0))
	expired.MaxAge = -1
	http.SetCookie(w, expired)
}

func (s *Service) DeviceTermsAccepted(r *http.Request) (bool, error) {
	cookie, err := r.Cookie(termsDeviceCookie)
	if err != nil || cookie.Value == "" {
		return false, nil
	}
	var accepted bool
	err = s.db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM terms_device_acceptances
			WHERE token_hash = ? AND terms_version = ? AND julianday(expires_at) > julianday(?)
		)
	`, hashToken(cookie.Value), CurrentTermsVersion, time.Now().UTC()).Scan(&accepted)
	return accepted, err
}

func (s *Service) AcceptTermsOnDevice(w http.ResponseWriter, r *http.Request) error {
	token, err := randomToken()
	if err != nil {
		return fmt.Errorf("create terms acceptance token: %w", err)
	}
	now := time.Now().UTC()
	expiresAt := now.Add(termsDeviceDuration)
	if _, err := s.db.Exec(
		`DELETE FROM terms_device_acceptances WHERE julianday(expires_at) <= julianday(?) OR terms_version <> ?`,
		now, CurrentTermsVersion,
	); err != nil {
		return fmt.Errorf("remove expired device terms acceptances: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO terms_device_acceptances (token_hash, terms_version, accepted_at, expires_at)
		VALUES (?, ?, ?, ?)
	`, hashToken(token), CurrentTermsVersion, now, expiresAt)
	if err != nil {
		return fmt.Errorf("save device terms acceptance: %w", err)
	}
	http.SetCookie(w, s.deviceTermsCookie(r, token, expiresAt))
	return nil
}

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if !strings.HasPrefix(r.URL.Path, "/web/static/") &&
			!strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Set("Cache-Control", "no-store")
		}

		if r.URL.Path == "/login" ||
			r.URL.Path == "/.well-known/discord" ||
			strings.HasPrefix(r.URL.Path, "/web/static/") ||
			strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		user, ok := s.AuthenticatedUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		request := r.WithContext(ctx)
		if r.URL.Path != "/terms" && r.URL.Path != "/logout" {
			accepted, err := s.DeviceTermsAccepted(r)
			if err != nil {
				http.Error(w, "Could not verify terms acceptance.", http.StatusInternalServerError)
				return
			}
			if !accepted {
				http.Redirect(w, r, "/terms", http.StatusSeeOther)
				return
			}
		}
		next.ServeHTTP(w, request)
	})
}

func (s *Service) LoginPage() http.HandlerFunc {
	tmpl := template.Must(template.ParseFiles("web/templates/login.html"))
	pageData := defaultLoginPageData()

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if _, ok := s.AuthenticatedUser(r); ok {
				accepted, err := s.DeviceTermsAccepted(r)
				if err != nil {
					http.Error(w, "Could not verify terms acceptance.", http.StatusInternalServerError)
					return
				}
				if accepted {
					http.Redirect(w, r, "/", http.StatusSeeOther)
				} else {
					http.Redirect(w, r, "/terms", http.StatusSeeOther)
				}
				return
			}
			if err := tmpl.Execute(w, pageData); err != nil {
				http.Error(w, "Could not render login page.", http.StatusInternalServerError)
			}
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}

			username := strings.TrimSpace(r.FormValue("username"))
			password := r.FormValue("password")
			if err := s.Login(w, r, username, password); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				errorData := pageData
				errorData.Error = "Invalid username or password."
				if executeErr := tmpl.Execute(w, errorData); executeErr != nil {
					http.Error(w, "Could not render login page.", http.StatusInternalServerError)
				}
				return
			}

			accepted, err := s.DeviceTermsAccepted(r)
			if err != nil {
				http.Error(w, "Could not verify terms acceptance.", http.StatusInternalServerError)
				return
			}
			if accepted {
				http.Redirect(w, r, "/", http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/terms", http.StatusSeeOther)
			}
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	}
}

func defaultLoginPageData() loginPageData {
	return loginPageData{
		Title:           "Sign in to Jaylub",
		MetaDescription: "Sign in to your Jaylub account to access the community chat, games, and member features.",
		CanonicalURL:    "https://jaylub.com/login",
		OpenGraphTitle:  "Sign in to Jaylub",
		OpenGraphDesc:   "Sign in to your Jaylub account to access the community chat, games, and member features.",
		OpenGraphType:   "website",
		OpenGraphURL:    "https://jaylub.com/login",
		Robots:          "noindex, nofollow",
	}
}

func (s *Service) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		s.Logout(w, r)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func (s *Service) AuthenticatedUser(r *http.Request) (User, bool) {
	for _, cookie := range r.Cookies() {
		if cookie.Name != cookieName || cookie.Value == "" {
			continue
		}

		var user User
		var expiresAt time.Time
		tokenHash := hashToken(cookie.Value)
		err := s.db.QueryRow(`
			SELECT users.id, users.username, sessions.expires_at
			FROM sessions
			JOIN users ON users.id = sessions.user_id
			WHERE sessions.token_hash = ?
		`, tokenHash).Scan(&user.ID, &user.Username, &expiresAt)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return User{}, false
		}
		if time.Now().After(expiresAt) {
			_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
			continue
		}

		return user, true
	}

	return User{}, false
}

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey).(User)
	return user, ok
}

func (s *Service) userWithPasswordHash(username string) (User, string, error) {
	var user User
	var passwordHash string
	err := s.db.QueryRow(
		`SELECT id, username, password_hash FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &passwordHash)
	if err != nil {
		return User{}, "", errors.New("invalid credentials")
	}
	return user, passwordHash, nil
}

func (s *Service) sessionCookie(r *http.Request, value string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		Domain:   cookieDomain(r.Host),
		Expires:  expires,
		HttpOnly: true,
		Secure:   r.TLS != nil || cookieDomain(r.Host) != "",
		SameSite: http.SameSiteLaxMode,
	}
}

func (s *Service) deviceTermsCookie(r *http.Request, value string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     termsDeviceCookie,
		Value:    value,
		Path:     "/",
		Domain:   cookieDomain(r.Host),
		Expires:  expires,
		MaxAge:   int(termsDeviceDuration.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil || cookieDomain(r.Host) != "",
		SameSite: http.SameSiteLaxMode,
	}
}

func randomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum[:])
}

func cookieDomain(host string) string {
	hostWithoutPort, _, err := net.SplitHostPort(host)
	if err == nil {
		host = hostWithoutPort
	}
	host = strings.ToLower(host)
	if host == "jaylub.com" || strings.HasSuffix(host, ".jaylub.com") {
		return ".jaylub.com"
	}
	return ""
}
