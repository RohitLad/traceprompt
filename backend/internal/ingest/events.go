// Package ingest parses and validates Langfuse-compatible ingestion batches.
//
// Upstream contract (reference/langfuse, legacy ingestion API):
// POST /api/public/ingestion {batch:[{id, type, timestamp, body}]}
// responds 207 with per-event {successes, errors} — never 4xx for
// per-event input errors. This package implements the parse/validate
// half; persistence lives in store.go; transport in queue.go.
package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Supported event types. span/generation/event aliases exist because
// official SDKs emit them; they map onto observation rows.
const (
	TypeTraceCreate       = "trace-create"
	TypeObservationCreate = "observation-create"
	TypeObservationUpdate = "observation-update"
	TypeSpanCreate        = "span-create"
	TypeSpanUpdate        = "span-update"
	TypeGenerationCreate  = "generation-create"
	TypeGenerationUpdate  = "generation-update"
	TypeEventCreate       = "event-create"
	TypeScoreCreate       = "score-create"
)

// MaxBatchEvents caps batch size to bound memory and request time.
// (Upstream caps total bytes at ~3.5MB; count cap is simpler and stricter.)
const MaxBatchEvents = 100

// Envelope is one event in the batch.
type Envelope struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Body      json.RawMessage `json:"body"`
}

// Batch is the request payload.
type Batch struct {
	Batch []Envelope `json:"batch"`
}

// EventError is one per-event failure for the 207 response.
type EventError struct {
	ID      string `json:"id"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// EventSuccess marks one accepted event.
type EventSuccess struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
}

// Result is the 207 response body.
type Result struct {
	Successes []EventSuccess `json:"successes"`
	Errors    []EventError   `json:"errors"`
}

// PermanentError marks failures that retry cannot fix (schema violations).
// Workers dead-letter these immediately instead of redelivering.
type PermanentError struct {
	Msg string
}

// Error implements error.
func (e *PermanentError) Error() string {
	return e.Msg
}

// Permanent wraps msg as a *PermanentError.
func Permanent(msg string) *PermanentError {
	return &PermanentError{Msg: msg}
}

// IsPermanent reports whether err is a *PermanentError (unwrapping).
func IsPermanent(err error) bool {
	if err == nil {
		return false
	}
	var target *PermanentError
	return errors.As(err, &target)
}

// Parsed is a validated event ready for enqueue/persist.
type Parsed struct {
	EventID   string
	Type      string // normalized: trace-create|observation-create|observation-update|score-create
	ObsType   string // default observation type for alias events (span-create→SPAN...); "" otherwise
	Timestamp time.Time
	Body      json.RawMessage
}

// Parse validates a raw batch, returning parsed events plus per-event errors.
// Unknown types are rejected per-event (4xx would break SDK batching).
func Parse(raw json.RawMessage) ([]Parsed, []EventError) {
	var batch Batch
	if err := json.Unmarshal(raw, &batch); err != nil {
		return nil, []EventError{{ID: "", Status: 400, Message: "invalid json body"}}
	}
	if len(batch.Batch) > MaxBatchEvents {
		return nil, []EventError{{
			ID:      "",
			Status:  413,
			Message: fmt.Sprintf("batch too large: max %d events", MaxBatchEvents),
		}}
	}
	var out []Parsed
	var errs []EventError
	for i, env := range batch.Batch {
		p, err := parseOne(env)
		if err != nil {
			id := env.ID
			if id == "" {
				id = fmt.Sprintf("index-%d", i)
			}
			errs = append(errs, EventError{ID: id, Status: 400, Message: err.Error()})
			continue
		}
		out = append(out, p)
	}
	return out, errs
}

func parseOne(env Envelope) (Parsed, error) {
	if strings.TrimSpace(env.ID) == "" {
		return Parsed{}, fmt.Errorf("event id is required")
	}
	typ, obsType, err := normalizeType(env.Type)
	if err != nil {
		return Parsed{}, err
	}
	if len(env.Body) == 0 {
		return Parsed{}, fmt.Errorf("event body is required")
	}
	ts := time.Now().UTC()
	if env.Timestamp != "" {
		t, err := parseTime(env.Timestamp)
		if err != nil {
			return Parsed{}, fmt.Errorf("invalid timestamp: %w", err)
		}
		ts = t
	}
	if err := validateBody(typ, env.Body); err != nil {
		return Parsed{}, err
	}
	return Parsed{EventID: env.ID, Type: typ, ObsType: obsType, Timestamp: ts, Body: env.Body}, nil
}

func normalizeType(t string) (normalized, obsType string, err error) {
	switch t {
	case TypeTraceCreate:
		return TypeTraceCreate, "", nil
	case TypeObservationCreate:
		return TypeObservationCreate, "", nil
	case TypeSpanCreate:
		return TypeObservationCreate, "SPAN", nil
	case TypeGenerationCreate:
		return TypeObservationCreate, "GENERATION", nil
	case TypeEventCreate:
		return TypeObservationCreate, "EVENT", nil
	case TypeObservationUpdate:
		return TypeObservationUpdate, "", nil
	case TypeSpanUpdate, TypeGenerationUpdate:
		return TypeObservationUpdate, "", nil
	case TypeScoreCreate:
		return TypeScoreCreate, "", nil
	default:
		return "", "", fmt.Errorf("unsupported event type %q", t)
	}
}

// validateBody checks required fields per normalized type.
func validateBody(typ string, body json.RawMessage) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return fmt.Errorf("body must be a json object")
	}
	requireID := func() error {
		var id string
		if err := json.Unmarshal(m["id"], &id); err != nil || strings.TrimSpace(id) == "" {
			return fmt.Errorf("body.id is required")
		}
		return nil
	}
	switch typ {
	case TypeTraceCreate, TypeObservationCreate, TypeObservationUpdate:
		return requireID()
	case TypeScoreCreate:
		var s struct {
			TraceID       *string          `json:"traceId"`
			ObservationID *string          `json:"observationId"`
			SessionID     *string          `json:"sessionId"`
			Name          *string          `json:"name"`
			Value         *json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(body, &s); err != nil {
			return fmt.Errorf("invalid score body")
		}
		if s.TraceID == nil && s.ObservationID == nil && s.SessionID == nil {
			return fmt.Errorf("score requires traceId, observationId, or sessionId")
		}
		if s.Name == nil || strings.TrimSpace(*s.Name) == "" {
			return fmt.Errorf("score name is required")
		}
		if s.Value == nil {
			return fmt.Errorf("score value is required")
		}
		return nil
	default:
		return fmt.Errorf("unsupported event type %q", typ)
	}
}

// parseTime accepts RFC3339 (+nanoseconds) and plain datetime fallbacks.
func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339Nano, time.RFC3339,
		"2006-01-02T15:04:05.000000", "2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable time %q", s)
}
