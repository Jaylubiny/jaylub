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
)

const dbPath = "internal/database/users.db"

func main() {
	reader := bufio.NewReader(os.Stdin)
	username := prompt(reader, "Username to remove: ")
	if username == "" {
		log.Fatal("username is required")
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	var userID int64
	err = tx.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&userID)
	if err == sql.ErrNoRows {
		log.Fatalf("user %q does not exist", username)
	}
	if err != nil {
		log.Fatal(err)
	}

	// Delete sessions first so the removed user is logged out immediately.
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		log.Fatal(err)
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Removed user %q\n", username)
}

func prompt(reader *bufio.Reader, label string) string {
	fmt.Print(label)
	value, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	return strings.TrimSpace(value)
}
