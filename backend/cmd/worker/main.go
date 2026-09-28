// Command worker drains the Redis ingestion stream and writes to Postgres.
// Phase 0 stub: blocks on signal so docker compose stays green before Phase 2 lands.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("traceprompt worker starting (stub, Phase 2 implements drain loop)")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("worker stopped")
}
