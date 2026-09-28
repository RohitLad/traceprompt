// Package queue decouples ingestion intake from persistence.
//
// POST /api/public/ingestion enqueues; cmd/worker drains. The Queue
// interface keeps handlers unit-testable (memory impl) while production
// uses Redis Streams with a consumer group (at-least-once + DLQ).
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
)

// Item is one queued event plus routing info.
type Item struct {
	ProjectID string          `json:"projectId"`
	EventID   string          `json:"eventId"`
	Type      string          `json:"type"`
	ObsType   string          `json:"obsType"`
	Timestamp time.Time       `json:"timestamp"`
	Body      json.RawMessage `json:"body"`
}

// FromParsed builds an Item from a validated event.
func FromParsed(projectID uuid.UUID, p ingest.Parsed) Item {
	return Item{
		ProjectID: projectID.String(),
		EventID:   p.EventID,
		Type:      p.Type,
		ObsType:   p.ObsType,
		Timestamp: p.Timestamp,
		Body:      p.Body,
	}
}

// ToParsed converts back for the store layer.
func (i Item) ToParsed() ingest.Parsed {
	return ingest.Parsed{
		EventID: i.EventID, Type: i.Type, ObsType: i.ObsType,
		Timestamp: i.Timestamp, Body: i.Body,
	}
}

// Queue is the intake contract used by HTTP handlers.
type Queue interface {
	Enqueue(ctx context.Context, items []Item) error
}

// Consumer is the drain contract used by the worker.
type Consumer interface {
	// Next blocks until items arrive or ctx ends. Ack confirms
	// processing; Nack dead-letters (after caller-side retries).
	Next(ctx context.Context) ([]Envelope, error)
	Ack(ctx context.Context, ids ...string) error
	Nack(ctx context.Context, ids ...string) error
}

// Envelope wraps queued items with transport IDs for ack/nack.
type Envelope struct {
	TransportID string
	Item        Item
}

// ---- In-memory (tests, local dev without Redis) ----

// Memory is a goroutine-safe FIFO. No persistence — drops on restart,
// which is fine for tests and documented for local dev.
type Memory struct {
	mu    sync.Mutex
	items []Item
}

var _ Queue = (*Memory)(nil)

// NewMemory returns an empty in-memory queue.
func NewMemory() *Memory {
	return &Memory{}
}

// Enqueue appends items.
func (m *Memory) Enqueue(_ context.Context, items []Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, items...)
	return nil
}

// Drain removes and returns up to n items (test/worker helper).
func (m *Memory) Drain(n int) []Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n > len(m.items) {
		n = len(m.items)
	}
	out := make([]Item, n)
	copy(out, m.items[:n])
	m.items = m.items[n:]
	return out
}

// Len reports queued depth.
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

// ---- Redis Streams (production) ----

const (
	// Stream is the main ingestion stream; Group is its consumer group.
	Stream = "traceprompt:ingest"
	Group  = "traceprompt-workers"
	// DeadLetter collects poison events after MaxAttempts.
	DeadLetter = "traceprompt:ingest:dlq"
	// MaxAttempts bounds redelivery before dead-lettering.
	MaxAttempts = 5
)

// Redis implements Queue over a Redis Stream.
type Redis struct {
	client *redis.Client
	stream string
}

// NewRedis builds a Redis queue client. Stream/group creation happens
// in EnsureGroup (idempotent, called by worker at startup).
func NewRedis(url string) (*Redis, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis url: %w", err)
	}
	return &Redis{client: redis.NewClient(opts), stream: Stream}, nil
}

// Enqueue XADDs each item; event ID dedupes SDK retries downstream in store.
func (r *Redis) Enqueue(ctx context.Context, items []Item) error {
	pipe := r.client.Pipeline()
	for _, it := range items {
		raw, err := json.Marshal(it)
		if err != nil {
			return err
		}
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: r.stream,
			Values: map[string]any{"data": string(raw)},
		})
	}
	_, err := pipe.Exec(ctx)
	return err
}

// EnsureGroup creates the consumer group idempotently.
func (r *Redis) EnsureGroup(ctx context.Context) error {
	err := r.client.XGroupCreateMkStream(ctx, r.stream, Group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// Client exposes the underlying client for the worker loop.
func (r *Redis) Client() *redis.Client {
	return r.client
}

// Close releases connections.
func (r *Redis) Close() error {
	return r.client.Close()
}
