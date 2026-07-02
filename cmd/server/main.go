package main

import (
	"fmt"
	"jaylub/internal/auth"
	"jaylub/internal/bot" // Imported your new bot package
	"log"
	"net/http"
	"os" // Added to read environment variables

	router "jaylub/internal/routers"
)

type server struct {
	addr    string
	handler http.Handler
}

func main() {
	authService, err := auth.New("internal/database/users.db")
	if err != nil {
		log.Fatal(err)
	}
	defer authService.Close()

	// 1. Initialize and Start the Discord Bot
	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		log.Fatal("DISCORD_BOT_TOKEN environment variable is missing")
	}

	discordBot, err := bot.NewBot(token)
	if err != nil {
		log.Fatalf("Failed to initialize Discord bot: %v", err)
	}

	go func() {
		if err := discordBot.Start(); err != nil {
			log.Fatalf("Discord bot runtime failure: %v", err)
		}
	}()
	defer discordBot.Stop()

	// 2. Start Your Existing Web Servers
	servers := []server{
		{":8080", router.Basic(authService)},
		{":8090", router.Company(authService)},
	}

	for _, srv := range servers[:len(servers)-1] {
		go startServer(srv)
	}

	go startServer(servers[len(servers)-1])
	select {}
}

func startServer(srv server) {
	fmt.Println("Server running on " + srv.addr)
	if err := http.ListenAndServe(srv.addr, srv.handler); err != nil {
		log.Println(err)
	}
}
