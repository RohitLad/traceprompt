package main

import (
	"context"
	"log"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/config"
	"github.com/traceprompt/traceprompt/backend/internal/db"
	"github.com/traceprompt/traceprompt/backend/internal/httpapi"
	"github.com/traceprompt/traceprompt/backend/internal/ingest"
	"github.com/traceprompt/traceprompt/backend/internal/queue"
	"github.com/traceprompt/traceprompt/backend/internal/worker"
)

func main() {
	cfg := config.Load()

	// Connect with retries so `docker compose up` ordering races resolve.
	gdb, err := connectWithRetry(cfg.DatabaseURL, cfg.Env == "development")
	if err != nil {
		log.Printf("warning: database unavailable, serving health only: %v", err)
		gdb = nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var deps *httpapi.Deps
	if gdb != nil {
		sqlDB, err := gdb.DB()
		if err != nil {
			log.Fatalf("database handle: %v", err)
		}
		// Versioned migrations are the schema source of truth in every env
		// with Postgres (dev included). SQLite tests use AutoMigrate.
		if err := db.Migrate(sqlDB); err != nil {
			log.Fatalf("migrations: %v", err)
		}
		q := resolveQueue(ctx, cfg.RedisURL, gdb)
		deps = &httpapi.Deps{DB: gdb, JWTSecret: cfg.JWTSecret, Queue: q, PublicRateLimit: cfg.PublicRateLimit}
	}
	app := httpapi.New(deps)

	go func() {
		<-ctx.Done()
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

// resolveQueue prefers Redis; without it (local dev) it falls back to an
// in-memory queue drained inline so single-binary runs still ingest.
func resolveQueue(ctx context.Context, redisURL string, gdb *gorm.DB) queue.Queue {
	rq, err := queue.NewRedis(redisURL)
	if err != nil {
		return queue.NewMemory()
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := rq.Client().Ping(pingCtx).Err(); err != nil {
		slog.Warn("redis unreachable, using in-memory queue (dev fallback)", "err", err)
		mem := queue.NewMemory()
		go worker.DrainMemory(context.Background(), mem, ingest.NewStore(gdb), 500*time.Millisecond)
		return mem
	}
	return rq
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
