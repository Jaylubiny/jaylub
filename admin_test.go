//go:build ignore

package main

import (
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPrintMenuIncludesPasswordResetOption(t *testing.T) {
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	printMenu()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = oldStdout

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "8. Reset a user's password") {
		t.Fatalf("admin menu does not show password reset option:\n%s", output)
	}
	if !strings.Contains(string(output), "9. Remove a music track by ID") {
		t.Fatalf("admin menu does not show music track removal option:\n%s", output)
	}
	if !strings.Contains(string(output), "10. Remove Jaylive gold") {
		t.Fatalf("admin menu does not show Jaylive gold removal option:\n%s", output)
	}
	if !strings.Contains(string(output), "11. Rename a music track by ID") {
		t.Fatalf("admin menu does not show music track rename option:\n%s", output)
	}
}

func TestRenameMusicTrackByIDUpdatesTitle(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "music.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE songs (id TEXT PRIMARY KEY, title TEXT NOT NULL);
		INSERT INTO songs (id, title) VALUES ('track-1', 'Old title');
	`); err != nil {
		t.Fatal(err)
	}

	if err := renameMusicTrackByID(db, "track-1", "  New title  "); err != nil {
		t.Fatalf("rename track: %v", err)
	}
	var title string
	if err := db.QueryRow(`SELECT title FROM songs WHERE id = 'track-1'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "New title" {
		t.Errorf("track title = %q, want New title", title)
	}
}

func TestRenameMusicTrackByIDRejectsMissingTrackAndBlankTitle(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "music.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE songs (id TEXT PRIMARY KEY, title TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}

	if err := renameMusicTrackByID(db, "missing", "Title"); err == nil {
		t.Fatal("renaming missing track succeeded")
	}
	if err := renameMusicTrackByID(db, "track-1", "  "); err == nil {
		t.Fatal("renaming to blank title succeeded")
	}
}

func TestDeductJayliveGoldRemovesRequestedAmount(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT UNIQUE);
		CREATE TABLE game_profiles (
			user_id INTEGER PRIMARY KEY,
			gold INTEGER NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO users (id, username) VALUES (1, 'alice');
		INSERT INTO game_profiles (user_id, gold) VALUES (1, 100);
	`); err != nil {
		t.Fatal(err)
	}

	balance, err := deductJayliveGold(db, "alice", 35)
	if err != nil {
		t.Fatalf("deduct gold: %v", err)
	}
	if balance != 65 {
		t.Errorf("remaining balance = %d, want 65", balance)
	}
}

func TestDeductJayliveGoldRejectsInsufficientBalance(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT UNIQUE);
		CREATE TABLE game_profiles (
			user_id INTEGER PRIMARY KEY,
			gold INTEGER NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO users (id, username) VALUES (1, 'alice');
		INSERT INTO game_profiles (user_id, gold) VALUES (1, 10);
	`); err != nil {
		t.Fatal(err)
	}

	if _, err := deductJayliveGold(db, "alice", 11); err == nil {
		t.Fatal("deduction beyond available gold succeeded")
	}
	var balance int
	if err := db.QueryRow(`SELECT gold FROM game_profiles WHERE user_id = 1`).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 10 {
		t.Errorf("balance after rejected deduction = %d, want 10", balance)
	}
}

func TestRemoveMusicTrackByIDRemovesTrackFileAndFavorites(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "music.db")+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
		CREATE TABLE songs (id TEXT PRIMARY KEY, title TEXT NOT NULL, file_path TEXT NOT NULL);
		CREATE TABLE user_favorites (
			user_id INTEGER NOT NULL,
			song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
			PRIMARY KEY (user_id, song_id)
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	uploadDir := t.TempDir()
	const songID = "7ca6802d-e997-4f85-b169-9185df172c1a"
	fileName := songID + ".mp3"
	filePath := filepath.Join(uploadDir, fileName)
	if err := os.WriteFile(filePath, []byte("mp3 data"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO songs (id, title, file_path) VALUES (?, 'Track', ?)`, songID, fileName); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_favorites (user_id, song_id) VALUES (1, ?)`, songID); err != nil {
		t.Fatal(err)
	}

	if err := removeMusicTrackByID(db, uploadDir, songID); err != nil {
		t.Fatalf("remove music track: %v", err)
	}
	if _, err := os.Stat(filePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("track file still exists or cannot be checked: %v", err)
	}
	var songs, favorites int
	if err := db.QueryRow(`SELECT COUNT(*) FROM songs WHERE id = ?`, songID).Scan(&songs); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_favorites WHERE song_id = ?`, songID).Scan(&favorites); err != nil {
		t.Fatal(err)
	}
	if songs != 0 || favorites != 0 {
		t.Errorf("remaining track rows = %d, favorite rows = %d; want both zero", songs, favorites)
	}
}

func TestRemoveMusicTrackByIDRejectsMissingAndUnsafePaths(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "music.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE songs (id TEXT PRIMARY KEY, title TEXT NOT NULL, file_path TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	uploadDir := t.TempDir()
	const songID = "7ca6802d-e997-4f85-b169-9185df172c1a"

	if err := removeMusicTrackByID(db, uploadDir, songID); err == nil {
		t.Fatal("removing missing track succeeded")
	}

	if _, err := db.Exec(`INSERT INTO songs (id, title, file_path) VALUES (?, 'Track', ?)`, songID, "../outside.mp3"); err != nil {
		t.Fatal(err)
	}
	if err := removeMusicTrackByID(db, uploadDir, songID); err == nil {
		t.Fatal("removing track with unsafe path succeeded")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM songs WHERE id = ?`, songID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("unsafe track rows = %d, want 1", count)
	}
}

func TestResetPasswordSetsChosenPasswordAndRevokesOnlySelectedSessions(t *testing.T) {
	db, err := sql.Open("sqlite3", t.TempDir()+"/users.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT UNIQUE, password_hash TEXT);
		CREATE TABLE sessions (user_id INTEGER, token_hash TEXT);
		INSERT INTO users (id, username, password_hash) VALUES (1, 'alice', 'old-hash');
		INSERT INTO sessions (user_id, token_hash) VALUES (1, 'alice-session');
		INSERT INTO users (id, username, password_hash) VALUES (2, 'bob', 'bob-hash');
		INSERT INTO sessions (user_id, token_hash) VALUES (2, 'bob-session');
	`)
	if err != nil {
		t.Fatal(err)
	}

	chosenPassword := "a password chosen by the admin"
	if err := resetPassword(db, "alice", chosenPassword); err != nil {
		t.Fatal(err)
	}

	var storedHash string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE username = 'alice'`).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(chosenPassword)); err != nil {
		t.Fatal("chosen password does not match the saved hash")
	}

	var aliceSessions, bobSessions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = 1`).Scan(&aliceSessions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = 2`).Scan(&bobSessions); err != nil {
		t.Fatal(err)
	}
	if aliceSessions != 0 || bobSessions != 1 {
		t.Fatalf("sessions after reset: alice=%d, bob=%d; want alice=0, bob=1", aliceSessions, bobSessions)
	}
}

func TestResetPasswordRejectsUnknownUser(t *testing.T) {
	db, err := sql.Open("sqlite3", t.TempDir()+"/users.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT UNIQUE, password_hash TEXT);
		CREATE TABLE sessions (user_id INTEGER, token_hash TEXT);
	`); err != nil {
		t.Fatal(err)
	}

	if err := resetPassword(db, "missing", "some new password"); err == nil {
		t.Fatal("expected an error for an unknown username")
	}
}

func TestResetPasswordRejectsEmptyPassword(t *testing.T) {
	db, err := sql.Open("sqlite3", t.TempDir()+"/users.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := resetPassword(db, "alice", ""); err == nil {
		t.Fatal("expected empty password to be rejected")
	}
}
