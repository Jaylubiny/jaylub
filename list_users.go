//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

const dbPath = "internal/database/users.db"

func main() {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

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
