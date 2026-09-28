// Package worker drains queues into Postgres.
//
// Two modes share the same store logic:
//   - DrainMemory: in-process loop for single-binary dev (no Redis).
//   - RunRedis:    Redis Streams consumer-group loop for production.
//
// Both are at-least-once: store.Apply is idempotent on external IDs,
// so redelivery converges. Poison events go to the DLQ after MaxAttempts.
package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
	"github.com/traceprompt/traceprompt/backend/internal/queue"
)

// DrainMemory polls a memory queue and applies items until ctx ends.
// Used by cmd/api as a fallback when Redis is unreachable.
func DrainMemory(ctx context.Context, mem *queue.Memory, store *ingest.Store, poll time.Duration) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, it := range mem.Drain(100) {
				pid, err := uuid.Parse(it.ProjectID)
				if err != nil {
					slog.Warn("worker: bad project id, dropping", "event", it.EventID)
					continue
				}
				if err := store.Apply(ctx, pid, it.ToParsed()); err != nil {
					slog.Warn("worker: apply failed", "event", it.EventID, "err", err)
				}
			}
		}
	}
}

// RunRedis consumes the stream via consumer group until ctx ends.
func RunRedis(ctx context.Context, r *queue.Redis, store *ingest.Store, consumer string) error {
	if err := r.EnsureGroup(ctx); err != nil {
		return err
	}
	client := r.Client()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		res, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    queue.Group,
			Consumer: consumer,
			Streams:  []string{queue.Stream, ">"},
			Count:    100,
			Block:    5 * time.Second,
		}).Result()
		if err != nil && err != redis.Nil {
			slog.Warn("worker: xreadgroup failed", "err", err)
			time.Sleep(time.Second)
			continue
		}
		for _, stream := range res {
			for _, msg := range stream.Messages {
				handleMessage(ctx, client, store, msg)
			}
		}
	}
}

func handleMessage(ctx context.Context, client *redis.Client, store *ingest.Store, msg redis.XMessage) {
	raw, _ := msg.Values["data"].(string)
	var it queue.Item
	if err := json.Unmarshal([]byte(raw), &it); err != nil {
		deadLetter(ctx, client, msg.ID, raw, "unparseable envelope")
		ack(ctx, client, msg.ID)
		return
	}
	pid, err := uuid.Parse(it.ProjectID)
	if err != nil {
		deadLetter(ctx, client, msg.ID, raw, "invalid project id")
		ack(ctx, client, msg.ID)
		return
	}
	if err := store.Apply(ctx, pid, it.ToParsed()); err != nil {
		// Retry via redelivery; dead-letter after MaxAttempts.
		pending, perr := client.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: queue.Stream, Group: queue.Group,
			Start: msg.ID, End: msg.ID, Count: 1,
		}).Result()
		attempts := 0
		if perr == nil && len(pending) == 1 {
			attempts = int(pending[0].RetryCount)
		}
		if attempts+1 >= queue.MaxAttempts {
			deadLetter(ctx, client, msg.ID, raw, err.Error())
			ack(ctx, client, msg.ID)
			return
		}
		slog.Warn("worker: apply failed, will retry", "event", it.EventID, "err", err)
		return // no ack → redelivered
	}
	ack(ctx, client, msg.ID)
}

func ack(ctx context.Context, client *redis.Client, ids ...string) {
	if err := client.XAck(ctx, queue.Stream, queue.Group, ids...).Err(); err != nil {
		slog.Warn("worker: ack failed", "err", err)
	}
}

func deadLetter(ctx context.Context, client *redis.Client, id, raw, reason string) {
	slog.Warn("worker: dead-lettering event", "id", id, "reason", reason)
	_ = client.XAdd(ctx, &redis.XAddArgs{
		Stream: queue.DeadLetter,
		Values: map[string]any{"originalId": id, "reason": reason, "data": raw},
	}).Err()
}
