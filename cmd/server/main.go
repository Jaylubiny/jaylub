package main

import (
	"fmt"
	"net/http"
	"os/exec"

	router "jaylub/internal/routers"
)

func startNotifyService() {
	cmd := exec.Command("go", "run", "internal/services/notify")

	cmd.Stdout = nil
	cmd.Stderr = nil

	err := cmd.Start()
	if err != nil {
		fmt.Println("Failed to start notify service:", err)
		return
	}

	fmt.Println("Notify service started with PID:", cmd.Process.Pid)
}

func main() {
	startNotifyService()

	r1 := router.Basic()
	r2 := router.Company()

	go func() {
		fmt.Println("Server running on :8080")
		err := http.ListenAndServe(":8080", r1)
		if err != nil {
			fmt.Println(err)
		}
	}()

	fmt.Println("Server running on :8090")
	err := http.ListenAndServe(":8090", r2)
	if err != nil {
		fmt.Println(err)
	}
}
