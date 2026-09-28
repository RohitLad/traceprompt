package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/config"
	"github.com/traceprompt/traceprompt/backend/internal/db"
	"github.com/traceprompt/traceprompt/backend/internal/httpapi"
	"github.com/traceprompt/traceprompt/backend/internal/models"
)

func main() {
	cfg := config.Load()

	// Connect with retries so `docker compose up` ordering races resolve.
	var gdb, err = connectWithRetry(cfg.DatabaseURL, cfg.Env == "development")
	if err != nil {
		log.Printf("warning: database unavailable, serving health only: %v", err)
		gdb = nil
	} else if cfg.Env != "production" {
		// Dev/test convenience. Production must use versioned SQL migrations.
		if err := gdb.AutoMigrate(models.AllModels()...); err != nil {
			log.Printf("warning: automigrate failed: %v", err)
		}
	}

	var deps *httpapi.Deps
	if gdb != nil {
		deps = &httpapi.Deps{DB: gdb, JWTSecret: cfg.JWTSecret}
	}
	app := httpapi.New(deps)

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

func connectWithRetry(dsn string, debug bool) (*gorm.DB, error) {
	var lastErr error
	for i := 0; i < 10; i++ {
		gdb, err := db.Open(dsn, debug)
		if err == nil {
			return gdb, nil
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	return nil, lastErr
}
