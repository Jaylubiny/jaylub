//go:build ignore

package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
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
	case "6":
		giveJayliveGold(db, reader)
	case "7":
		removeUserFromLeaderboardSection(db, reader)
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
	fmt.Println("6. Give Jaylive gold")
	fmt.Println("7. Remove user from one Jaylive leaderboard section")
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

func giveJayliveGold(db *sql.DB, reader *bufio.Reader) {
	username := prompt(reader, "Username: ")
	if username == "" {
		log.Fatal("username is required")
	}

	amountText := prompt(reader, "Gold amount to add: ")
	amount, err := strconv.Atoi(amountText)
	if err != nil || amount <= 0 {
		log.Fatal("gold amount must be a positive whole number")
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

	_, err = tx.Exec(`
		INSERT INTO game_profiles (user_id, username, gold)
		VALUES (?, ?, 0)
		ON CONFLICT(user_id) DO NOTHING
	`, userID, username)
	if err != nil {
		log.Fatal(err)
	}

	_, err = tx.Exec(`
		UPDATE game_profiles
		SET gold = gold + ?, username = ?, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ?
	`, amount, username, userID)
	if err != nil {
		log.Fatal(err)
	}

	var balance int
	if err := tx.QueryRow(`SELECT gold FROM game_profiles WHERE user_id = ?`, userID).Scan(&balance); err != nil {
		log.Fatal(err)
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Added %d gold to %q. New balance: %d\n", amount, username, balance)
}

func removeUserFromLeaderboardSection(db *sql.DB, reader *bufio.Reader) {
	sections := []struct {
		name   string
		column string
	}{
		{name: "Total kills", column: "total_kills"},
		{name: "Kills in one run", column: "best_run_kills"},
		{name: "Most time in one run", column: "best_run_seconds"},
	}

	fmt.Println("Leaderboard sections")
	for i, section := range sections {
		fmt.Printf("%d. %s\n", i+1, section.name)
	}

	choiceText := prompt(reader, "Choose section: ")
	choice, err := strconv.Atoi(choiceText)
	if err != nil || choice < 1 || choice > len(sections) {
		log.Fatal("unknown leaderboard section")
	}
	section := sections[choice-1]

	username := prompt(reader, "Username to remove from this section: ")
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

	result, err := tx.Exec(`
		UPDATE game_leaderboard
		SET `+section.column+` = 0, username = ?, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ?
	`, username, userID)
	if err != nil {
		log.Fatal(err)
	}

	updated, err := result.RowsAffected()
	if err != nil {
		log.Fatal(err)
	}
	if updated == 0 {
		log.Fatalf("user %q does not have a leaderboard row", username)
	}

	if section.column == "total_kills" {
		if _, err := tx.Exec(`UPDATE game_profiles SET lifetime_kills = 0, username = ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`, username, userID); err != nil {
			log.Fatal(err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Removed %q from Jaylive leaderboard section: %s\n", username, section.name)
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

		CREATE TABLE IF NOT EXISTS game_profiles (
			user_id INTEGER PRIMARY KEY,
			username TEXT NOT NULL,
			gold INTEGER NOT NULL DEFAULT 0,
			lifetime_kills INTEGER NOT NULL DEFAULT 0,
			selected_character TEXT NOT NULL DEFAULT 'jaylub',
			goblin_jaylub_unlocked INTEGER NOT NULL DEFAULT 0,
			damage_level INTEGER NOT NULL DEFAULT 0,
			max_hp_level INTEGER NOT NULL DEFAULT 0,
			attack_speed_level INTEGER NOT NULL DEFAULT 0,
			move_speed_level INTEGER NOT NULL DEFAULT 0,
			piercing_level INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS game_leaderboard (
			user_id INTEGER PRIMARY KEY,
			username TEXT NOT NULL,
			total_kills INTEGER NOT NULL DEFAULT 0,
			best_run_kills INTEGER NOT NULL DEFAULT 0,
			best_run_seconds INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_game_leaderboard_total_kills ON game_leaderboard(total_kills DESC);
		CREATE INDEX IF NOT EXISTS idx_game_leaderboard_best_run_kills ON game_leaderboard(best_run_kills DESC);
		CREATE INDEX IF NOT EXISTS idx_game_leaderboard_best_run_seconds ON game_leaderboard(best_run_seconds DESC);
	`)
	return err
}
