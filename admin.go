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
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := initSchema(db); err != nil {
		log.Fatal(err)
	}

	printMenu()
	choice := prompt(reader, "Choose action: ")

	switch choice {
	case "1":
		addUser(db, reader)
	case "2":
		listUsers(db)
	case "3":
		removeUser(db, reader)
	case "4":
		deleteChatHistory(db, reader)
	case "5":
		resetLeaderboards(db, reader)
	default:
		log.Fatal("unknown action")
	}
}

func printMenu() {
	fmt.Println("Jaylub admin")
	fmt.Println()
	fmt.Println("1. Add user")
	fmt.Println("2. List users")
	fmt.Println("3. Remove user")
	fmt.Println("4. Delete chat history")
	fmt.Println("5. Reset Jaylive leaderboards")
	fmt.Println()
}

func addUser(db *sql.DB, reader *bufio.Reader) {
	username := prompt(reader, "Username: ")
	password := prompt(reader, "Password: ")
	if username == "" || password == "" {
		log.Fatal("username and password are required")
	}

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

func listUsers(db *sql.DB) {
	rows, err := db.Query(`SELECT username, created_at FROM users ORDER BY username`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		found = true

		var username string
		var createdAt string
		if err := rows.Scan(&username, &createdAt); err != nil {
			log.Fatal(err)
		}

		fmt.Printf("%s\t%s\n", username, createdAt)
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
	if !found {
		fmt.Println("No users found.")
	}
}

func removeUser(db *sql.DB, reader *bufio.Reader) {
	username := prompt(reader, "Username to remove: ")
	if username == "" {
		log.Fatal("username is required")
	}

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

func deleteChatHistory(db *sql.DB, reader *bufio.Reader) {
	confirmation := prompt(reader, `Type "DELETE" to clear all chat history: `)
	if confirmation != "DELETE" {
		log.Fatal("chat history deletion cancelled")
	}

	result, err := db.Exec(`DELETE FROM chat_messages`)
	if err != nil {
		log.Fatal(err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Deleted %d chat messages.\n", deleted)
}

func resetLeaderboards(db *sql.DB, reader *bufio.Reader) {
	confirmation := prompt(reader, `Type "RESET" to reset all Jaylive leaderboards: `)
	if confirmation != "RESET" {
		log.Fatal("leaderboard reset cancelled")
	}

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(`DELETE FROM game_leaderboard`)
	if err != nil {
		log.Fatal(err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		log.Fatal(err)
	}

	if _, err := tx.Exec(`UPDATE game_profiles SET lifetime_kills = 0`); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Reset leaderboards and deleted %d leaderboard rows.\n", deleted)
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
