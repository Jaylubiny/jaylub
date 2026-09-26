package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jaylub/internal/auth"
	"jaylub/internal/bot"
	router "jaylub/internal/routers"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	authService, err := auth.New("internal/database/users.db")
	if err != nil {
		return err
	}
	defer authService.Close()

	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		return errors.New("DISCORD_BOT_TOKEN environment variable is missing")
	}

	discordBot, err := bot.NewBot(token)
	if err != nil {
		return fmt.Errorf("failed to initialize Discord bot: %w", err)
	}

	defer discordBot.Stop()

	servers := []*http.Server{
		newHTTPServer(":8080", router.Basic(authService)),
		newHTTPServer(":8090", router.Company(authService)),
	}
	errCh := make(chan error, len(servers)+1)

	for _, srv := range servers {
		go func(srv *http.Server) {
			if err := startServer(srv); err != nil {
				errCh <- err
			}
		}(srv)
	}

	go func() {
		if err := discordBot.Start(ctx); err != nil && ctx.Err() == nil {
			errCh <- fmt.Errorf("Discord bot runtime failure: %w", err)
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
	case err := <-errCh:
		runErr = fmt.Errorf("application component failed: %w", err)
		stop()
	}

	fmt.Println("Shutting down gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("HTTP server %s shutdown failed: %w", srv.Addr, err))
		}
	}
	return runErr
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

func startServer(srv *http.Server) error {
	fmt.Println("Server running on " + srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP server %s failed: %w", srv.Addr, err)
	}
	return nil
}
