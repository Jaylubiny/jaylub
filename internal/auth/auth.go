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
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName      = "jaylub_session"
	sessionDuration = 30 * 24 * time.Hour
)

type contextKey string

const userContextKey contextKey = "authUser"

type User struct {
	ID       int64
	Username string
}

type Service struct {
	db *sql.DB
}

func New(dbPath string) (*Service, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	service := &Service{db: db}
	if err := service.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := service.ensureExampleUser(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return service, nil
}

func (s *Service) Close() error {
	return s.db.Close()
}

func (s *Service) DB() *sql.DB {
	return s.db
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

		CREATE TABLE IF NOT EXISTS chat_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			message TEXT NOT NULL,
			timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_chat_messages_timestamp ON chat_messages(timestamp);
		CREATE INDEX IF NOT EXISTS idx_chat_messages_id ON chat_messages(id);

		CREATE TABLE IF NOT EXISTS chat_reads (
			user_id INTEGER PRIMARY KEY,
			last_read_at DATETIME NOT NULL,
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

func (s *Service) ensureExampleUser() error {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)`, "preclik").Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("jaykluk"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`INSERT INTO users (username, password_hash) VALUES (?, ?)`, "preclik", string(hash))
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

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		user, ok := s.AuthenticatedUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Service) LoginPage() http.HandlerFunc {
	tmpl := template.Must(template.ParseFiles("web/templates/login.html"))

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if _, ok := s.AuthenticatedUser(r); ok {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			_ = tmpl.Execute(w, map[string]string{"Title": "Login"})
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}

			username := strings.TrimSpace(r.FormValue("username"))
			password := r.FormValue("password")
			if err := s.Login(w, r, username, password); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_ = tmpl.Execute(w, map[string]string{
					"Title": "Login",
					"Error": "Invalid username or password.",
				})
				return
			}

			http.Redirect(w, r, "/", http.StatusSeeOther)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
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
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return User{}, false
	}

	var user User
	var expiresAt time.Time
	err = s.db.QueryRow(`
		SELECT users.id, users.username, sessions.expires_at
		FROM sessions
		JOIN users ON users.id = sessions.user_id
		WHERE sessions.token_hash = ?
	`, hashToken(cookie.Value)).Scan(&user.ID, &user.Username, &expiresAt)
	if err != nil {
		return User{}, false
	}
	if time.Now().After(expiresAt) {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(cookie.Value))
		return User{}, false
	}

	return user, true
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
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	}
}

func (s *Service) isPublicPath(path string) bool {
	return path == "/login" ||
		path == "/.well-known/discord" ||
		strings.HasPrefix(path, "/web/static/") ||
		strings.HasPrefix(path, "/static/")
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
