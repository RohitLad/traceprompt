package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/traceprompt/traceprompt/backend/internal/config"
	"github.com/traceprompt/traceprompt/backend/internal/httpapi"
)

func main() {
	cfg := config.Load()
	app := httpapi.New()

	// Graceful shutdown so in-flight ingestion batches complete.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("shutting down...")
		if err := app.Shutdown(); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Printf("traceprompt api listening on %s (%s)", cfg.Addr(), cfg.Env)
	if err := app.Listen(cfg.Addr()); err != nil {
		log.Printf("server closed: %v", err)
	}
}
