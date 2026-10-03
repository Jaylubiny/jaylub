package music

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func OpenStore(path string) (*Store, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve music database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0750); err != nil {
		return nil, fmt.Errorf("create music database directory: %w", err)
	}

	databaseURL := url.URL{
		Scheme: "file",
		Path:   absolutePath,
		RawQuery: url.Values{
			"_foreign_keys": {"on"},
			"_busy_timeout": {"5000"},
			"_journal_mode": {"WAL"},
		}.Encode(),
	}
	db, err := sql.Open("sqlite3", databaseURL.String())
	if err != nil {
		return nil, fmt.Errorf("open music database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to music database: %w", err)
	}
	if err := os.Chmod(absolutePath, 0600); err != nil {
		db.Close()
		return nil, fmt.Errorf("secure music database permissions: %w", err)
	}

	store := &Store{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			username TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS songs (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			artist TEXT NOT NULL,
			album TEXT NOT NULL,
			duration_seconds INTEGER NOT NULL CHECK (duration_seconds >= 0),
			file_path TEXT NOT NULL UNIQUE,
			uploaded_by INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (uploaded_by) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_songs_created_at ON songs(created_at DESC);

		CREATE TABLE IF NOT EXISTS user_favorites (
			user_id INTEGER NOT NULL,
			song_id TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, song_id),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_user_favorites_song_id ON user_favorites(song_id);
	`)
	if err != nil {
		return fmt.Errorf("initialize music database schema: %w", err)
	}
	return nil
}

func (s *Store) ensureUser(userID int64, username string) error {
	_, err := s.db.Exec(`
		INSERT INTO users (id, username) VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET username = excluded.username
	`, userID, username)
	if err != nil {
		return fmt.Errorf("synchronize music account: %w", err)
	}
	return nil
}
