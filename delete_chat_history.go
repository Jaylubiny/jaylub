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
	confirmation := prompt(reader, `Type "DELETE" to clear all chat history: `)
	if confirmation != "DELETE" {
		log.Fatal("chat history deletion cancelled")
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

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

func prompt(reader *bufio.Reader, label string) string {
	fmt.Print(label)
	value, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	return strings.TrimSpace(value)
}
