//go:build ignore

package main

import (
	"database/sql"
	"io"
	"os"
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
