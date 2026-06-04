//go:build ignore

package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

const dbPath = "internal/database/users.db"

func main() {
	reader := bufio.NewReader(os.Stdin)

	username := prompt(reader, "Username: ")
	password := prompt(reader, "Password: ")
	if username == "" || password == "" {
		log.Fatal("username and password are required")
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := initSchema(db); err != nil {
		log.Fatal(err)
	}

	// Store only a bcrypt hash. The raw password is never written to the database.
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		username,
		string(hash),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Added user %q\n", username)
}

func prompt(reader *bufio.Reader, label string) string {
	fmt.Print(label)
	value, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	return strings.TrimSpace(value)
}

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
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
	`)
	return err
}
