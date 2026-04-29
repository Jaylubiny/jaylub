package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const filePath = "users.txt"

func main() {
	fmt.Println("Watching:", filePath)

	for {
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Println("Error reading file:", err)
			time.Sleep(1 * time.Second)
			continue
		}

		content := strings.TrimSpace(string(data))

		if content == "1" {
			fmt.Println("Detected 1 -> executing a.out")

			// execute ./a.out
			cmd := exec.Command("./a.out")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			err := cmd.Run()
			if err != nil {
				fmt.Println("Error running a.out:", err)
			}

			// reset file back to 0
			err = os.WriteFile(filePath, []byte("0"), 0644)
			if err != nil {
				fmt.Println("Error writing file:", err)
			} else {
				fmt.Println("Reset users.txt to 0")
			}
		}

		// check every second
		time.Sleep(1 * time.Second)
	}
}
