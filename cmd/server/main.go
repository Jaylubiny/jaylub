package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"jaylub/internal/auth"
	"jaylub/internal/bot"
	router "jaylub/internal/routers"
)

type server struct {
	addr    string
	handler http.Handler
}

func main() {
	// Create context listening for system termination signals (SIGINT, SIGTERM)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
		if err := discordBot.Start(ctx); err != nil {
			log.Fatalf("Discord bot runtime failure: %v", err)
		}
	}()
	defer discordBot.Stop()

	// 2. Start Your Existing Web Servers
	servers := []server{
		{":8080", router.Basic(authService)},
		{":8090", router.Company(authService)},
	}

	for _, srv := range servers {
		go startServer(srv)
	}

	// Wait for OS shutdown signal so defers can execute properly
	<-ctx.Done()
	fmt.Println("Shutting down gracefully...")
}

func startServer(srv server) {
	fmt.Println("Server running on " + srv.addr)
	if err := http.ListenAndServe(srv.addr, srv.handler); err != nil {
		log.Println(err)
	}
}
