// Command worker drains the Redis ingestion stream and writes to Postgres.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/traceprompt/traceprompt/backend/internal/config"
	"github.com/traceprompt/traceprompt/backend/internal/db"
	"github.com/traceprompt/traceprompt/backend/internal/ingest"
	"github.com/traceprompt/traceprompt/backend/internal/queue"
	"github.com/traceprompt/traceprompt/backend/internal/worker"
)

func main() {
	cfg := config.Load()

	gdb, err := db.Open(cfg.DatabaseURL, false)
	if err != nil {
		log.Fatalf("worker: database: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		log.Fatalf("worker: database handle: %v", err)
	}
	if err := db.Migrate(sqlDB); err != nil {
		log.Fatalf("worker: migrations: %v", err)
	}

	rq, err := queue.NewRedis(cfg.RedisURL)
	if err != nil {
		log.Fatalf("worker: redis: %v", err)
	}
	defer rq.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}
	consumer := hostname + "-" + time.Now().Format("150405")
	log.Printf("worker: consuming %s as %s", queue.Stream, consumer)
	if err := worker.RunRedis(ctx, rq, ingest.NewStore(gdb), consumer); err != nil {
		log.Printf("worker stopped: %v", err)
	}
}
