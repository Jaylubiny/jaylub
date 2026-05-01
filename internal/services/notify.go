package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const filePath = "users.txt"
const workDir = "onUser" // change this to your actual folder path

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
			fmt.Println("Detected 1 -> uploading Arduino sketch")

			cmd := exec.Command(
				"arduino-cli",
				"upload",
				"-p", "/dev/ttyACM0",
				"--fqbn", "arduino:avr:uno",
				".",
			)

			// run inside onUser directory
			cmd.Dir = workDir

			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			err := cmd.Run()
			if err != nil {
				fmt.Println("Error running arduino-cli upload:", err)
			}

			// reset file back to 0
			err = os.WriteFile(filePath, []byte("0"), 0644)
			if err != nil {
				fmt.Println("Error writing file:", err)
			} else {
				fmt.Println("Reset users.txt to 0")
			}
		}

		time.Sleep(1 * time.Second)
	}
}
