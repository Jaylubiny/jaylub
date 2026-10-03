//go:build ignore

package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

const dbPath = "internal/database/users.db"
const musicDBPath = "internal/database/music.db"
const musicUploadDir = "data/mp3s"

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
	case "8":
		resetUserPassword(db, reader)
	case "9":
		removeMusicTrack(reader)
	case "10":
		removeJayliveGold(db, reader)
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
	fmt.Println("8. Reset a user's password")
	fmt.Println("9. Remove a music track by ID")
	fmt.Println("10. Remove Jaylive gold")
	fmt.Println()
}

func removeJayliveGold(db *sql.DB, reader *bufio.Reader) {
	username := prompt(reader, "Username: ")
	if username == "" {
		log.Fatal("username is required")
	}

	amountText := prompt(reader, "Gold amount to remove: ")
	amount, err := strconv.Atoi(amountText)
	if err != nil || amount <= 0 {
		log.Fatal("gold amount must be a positive whole number")
	}

	balance, err := deductJayliveGold(db, username, amount)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Removed %d gold from %q. New balance: %d\n", amount, username, balance)
}

func deductJayliveGold(db *sql.DB, username string, amount int) (int, error) {
	if strings.TrimSpace(username) == "" {
		return 0, fmt.Errorf("username is required")
	}
	if amount <= 0 {
		return 0, fmt.Errorf("gold amount must be a positive whole number")
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin gold removal: %w", err)
	}
	defer tx.Rollback()

	var userID int64
	if err := tx.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("user %q does not exist", username)
		}
		return 0, fmt.Errorf("look up user: %w", err)
	}

	result, err := tx.Exec(`
		UPDATE game_profiles
		SET gold = gold - ?, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND gold >= ?
	`, amount, userID, amount)
	if err != nil {
		return 0, fmt.Errorf("remove Jaylive gold: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("verify Jaylive gold removal: %w", err)
	}
	if updated == 0 {
		var exists int
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM game_profiles WHERE user_id = ?)`, userID).Scan(&exists); err != nil {
			return 0, fmt.Errorf("check Jaylive profile: %w", err)
		}
		if exists == 0 {
			return 0, fmt.Errorf("user %q does not have a Jaylive profile", username)
		}
		return 0, fmt.Errorf("user %q does not have enough gold", username)
	}

	var balance int
	if err := tx.QueryRow(`SELECT gold FROM game_profiles WHERE user_id = ?`, userID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("read remaining Jaylive gold: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("save Jaylive gold removal: %w", err)
	}
	return balance, nil
}

func removeMusicTrack(reader *bufio.Reader) {
	songID := prompt(reader, "Track ID to remove: ")
	if songID == "" {
		log.Fatal("track ID is required")
	}

	musicDB, err := sql.Open("sqlite3", "file:"+musicDBPath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		log.Fatalf("open music database: %v", err)
	}
	defer musicDB.Close()
	if err := musicDB.Ping(); err != nil {
		log.Fatalf("connect to music database: %v", err)
	}

	var title string
	err = musicDB.QueryRow(`SELECT title FROM songs WHERE id = ?`, songID).Scan(&title)
	if errors.Is(err, sql.ErrNoRows) {
		log.Fatalf("track %q does not exist", songID)
	}
	if err != nil {
		log.Fatalf("look up music track: %v", err)
	}
	fmt.Printf("Track: %s (%s)\n", title, songID)
	if prompt(reader, `Type "DELETE" to remove this track: `) != "DELETE" {
		log.Fatal("track deletion cancelled")
	}

	if err := removeMusicTrackByID(musicDB, musicUploadDir, songID); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Removed music track %q (%s).\n", title, songID)
}

func removeMusicTrackByID(db *sql.DB, uploadDir, songID string) error {
	songID = strings.TrimSpace(songID)
	if songID == "" {
		return fmt.Errorf("track ID is required")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin music track removal: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var fileName string
	if err := tx.QueryRow(`SELECT file_path FROM songs WHERE id = ?`, songID).Scan(&fileName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("track %q does not exist", songID)
		}
		return fmt.Errorf("look up music track: %w", err)
	}
	if fileName == "" || filepath.Base(fileName) != fileName || strings.ToLower(filepath.Ext(fileName)) != ".mp3" {
		return fmt.Errorf("track %q has an invalid stored file path", songID)
	}

	filePath := filepath.Join(uploadDir, fileName)
	var stagedPath string
	if _, err := os.Stat(filePath); err == nil {
		temporary, err := os.CreateTemp(uploadDir, ".admin-delete-*.tmp")
		if err != nil {
			return fmt.Errorf("prepare track file removal: %w", err)
		}
		stagedPath = temporary.Name()
		if err := temporary.Close(); err != nil {
			_ = os.Remove(stagedPath)
			return fmt.Errorf("prepare track file removal: %w", err)
		}
		if err := os.Remove(stagedPath); err != nil {
			return fmt.Errorf("prepare track file removal: %w", err)
		}
		if err := os.Rename(filePath, stagedPath); err != nil {
			return fmt.Errorf("stage track file for removal: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect track file: %w", err)
	}

	restoreStagedFile := func() error {
		if stagedPath == "" {
			return nil
		}
		if err := os.Rename(stagedPath, filePath); err != nil {
			return fmt.Errorf("restore track file after failed removal: %w", err)
		}
		return nil
	}
	rollback := func(cause error) error {
		rollbackErr := tx.Rollback()
		restoreErr := restoreStagedFile()
		return errors.Join(cause, rollbackErr, restoreErr)
	}

	result, err := tx.Exec(`DELETE FROM songs WHERE id = ?`, songID)
	if err != nil {
		return rollback(fmt.Errorf("remove music track record: %w", err))
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return rollback(fmt.Errorf("verify music track removal: %w", err))
	}
	if deleted != 1 {
		return rollback(fmt.Errorf("track %q does not exist", songID))
	}
	if err := tx.Commit(); err != nil {
		restoreErr := restoreStagedFile()
		return errors.Join(fmt.Errorf("commit music track removal: %w", err), restoreErr)
	}
	committed = true

	if stagedPath != "" {
		if err := os.Remove(stagedPath); err != nil {
			return fmt.Errorf("track %q was removed from the library, but its MP3 file could not be deleted: %w", songID, err)
		}
	}
	return nil
}

func resetUserPassword(db *sql.DB, reader *bufio.Reader) {
	username := prompt(reader, "Username to reset: ")
	if username == "" {
		log.Fatal("username is required")
	}

	password := prompt(reader, "New password: ")
	if password == "" {
		log.Fatal("password is required")
	}
	confirmation := prompt(reader, "Confirm new password: ")
	if password != confirmation {
		log.Fatal("passwords do not match")
	}

	if err := resetPassword(db, username, password); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Password reset for %q. All existing sessions were revoked.\n", username)
}

func resetPassword(db *sql.DB, username, password string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin password reset: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE users SET password_hash = ? WHERE username = ?`,
		string(passwordHash),
		username,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check password update: %w", err)
	}
	if updated == 0 {
		return fmt.Errorf("user %q does not exist", username)
	}

	if _, err := tx.Exec(
		`DELETE FROM sessions WHERE user_id = (SELECT id FROM users WHERE username = ?)`,
		username,
	); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save password reset: %w", err)
	}

	return nil
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

	if _, err := tx.Exec(`UPDATE game_profiles SET lifetime_kills = 0, game_level = 0, game_xp = 0`); err != nil {
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
		{name: "Level", column: "level"},
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
	if section.column == "level" {
		if _, err := tx.Exec(`UPDATE game_profiles SET game_level = 0, game_xp = 0, username = ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`, username, userID); err != nil {
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
		CREATE INDEX IF NOT EXISTS idx_game_leaderboard_best_run_kills ON game_leaderboard(best_run_kills DESC);
		CREATE INDEX IF NOT EXISTS idx_game_leaderboard_best_run_seconds ON game_leaderboard(best_run_seconds DESC);
	`)
	if err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "game_profiles", "game_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "game_profiles", "vampire_jaylub_unlocked", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "game_profiles", "game_xp", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "game_profiles", "ability_damage_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "game_profiles", "aura_damage_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "game_profiles", "football_damage_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "game_profiles", "bike_damage_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "game_profiles", "pigeon_damage_level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "game_leaderboard", "level", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_game_leaderboard_level ON game_leaderboard(level DESC)`)
	return err
}

func addColumnIfMissing(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
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

	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}
