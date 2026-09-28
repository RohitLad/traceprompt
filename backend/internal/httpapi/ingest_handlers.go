package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
	"github.com/traceprompt/traceprompt/backend/internal/models"
	"github.com/traceprompt/traceprompt/backend/internal/queue"
)

// POST /api/public/ingestion — Langfuse-compatible batch intake.
// Always 207 with per-event results; transport/auth failures are the
// only non-2xx cases (SDKs rely on this for batching).
func (h *Handler) Ingestion(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	raw := json.RawMessage(append([]byte(nil), c.Body()...))
	parsed, errs := ingest.Parse(raw)

	items := make([]queue.Item, 0, len(parsed))
	successes := make([]ingest.EventSuccess, 0, len(parsed))
	for _, ev := range parsed {
		items = append(items, queue.FromParsed(p.ID, ev))
		successes = append(successes, ingest.EventSuccess{ID: ev.EventID, Status: 201})
	}
	if len(items) > 0 {
		if err := h.queue.Enqueue(c.Context(), items); err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "ingestion queue unavailable")
		}
	}
	return c.Status(fiber.StatusMultiStatus).JSON(ingest.Result{
		Successes: successes,
		Errors:    orEmptyErrors(errs),
	})
}

func orEmptyErrors(errs []ingest.EventError) []ingest.EventError {
	if errs == nil {
		return []ingest.EventError{}
	}
	return errs
}

// observationOut mirrors Langfuse v2 observation rows (core+basic+usage+io).
type observationOut struct {
	ID              uuid.UUID      `json:"id"`
	TraceID         string         `json:"traceId"`
	ProjectID       uuid.UUID      `json:"projectId"`
	ParentID        *string        `json:"parentObservationId"`
	Type            string         `json:"type"`
	Name            string         `json:"name"`
	StartTime       time.Time      `json:"startTime"`
	EndTime         *time.Time     `json:"endTime"`
	Input           *string        `json:"input"`
	Output          *string        `json:"output"`
	Metadata        map[string]any `json:"metadata"`
	Model           *string        `json:"model"`
	ModelParameters map[string]any `json:"modelParameters"`
	UsageInput      *int           `json:"inputUsage"`
	UsageOutput     *int           `json:"outputUsage"`
	UsageTotal      *int           `json:"totalUsage"`
	Level           string         `json:"level"`
	StatusMessage   *string        `json:"statusMessage"`
	Environment     string         `json:"environment"`
	UserID          *string        `json:"userId,omitempty"`
	SessionID       *string        `json:"sessionId,omitempty"`
	PromptName      *string        `json:"promptName"`
	PromptVersion   *int           `json:"promptVersion"`
}

// GET /api/public/v2/observations — cursor-paginated observation rows.
// NOTE: `fields` is accepted for SDK compat but full rows are returned;
// selective projection lands with ClickHouse-scale optimization later.
func (h *Handler) ListObservationsV2(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	q := h.db.Model(&models.Observation{}).Where("project_id = ?", p.ID)
	if v := c.Query("traceId"); v != "" {
		q = q.Where("trace_id = ?", v)
	}
	if v := c.Query("name"); v != "" {
		q = q.Where("name = ?", v)
	}
	if v := c.Query("type"); v != "" {
		q = q.Where("type = ?", strings.ToUpper(v))
	}
	if v := c.Query("userId"); v != "" {
		q = q.Where("trace_id IN (?)",
			h.db.Model(&models.Trace{}).Select("trace_id").Where("project_id = ? AND user_id = ?", p.ID, v))
	}
	if v := c.Query("sessionId"); v != "" {
		q = q.Where("trace_id IN (?)",
			h.db.Model(&models.Trace{}).Select("trace_id").Where("project_id = ? AND session_id = ?", p.ID, v))
	}
	if v := c.Query("environment"); v != "" {
		q = q.Where("environment = ?", v)
	}
	if v := c.Query("fromStartTime"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q = q.Where("start_time >= ?", t)
		}
	}
	if v := c.Query("toStartTime"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q = q.Where("start_time < ?", t)
		}
	}
	limit := clampInt(queryInt(c, "limit", 50), 1, 1000)
	offset := decodeCursor(c.Query("cursor"))

	var rows []models.Observation
	if err := q.Order("start_time DESC").Limit(limit + 1).Offset(offset).Find(&rows).Error; err != nil {
		return err
	}
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		next = encodeCursor(offset + limit)
	}
	out := make([]observationOut, 0, len(rows))
	for _, o := range rows {
		out = append(out, toObservationOut(o))
	}
	// Trace-level attrs (user/session) live on traces; join-lite per row.
	fillTraceAttrs(h, p.ID, out)
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"cursor": next}})
}

func toObservationOut(o models.Observation) observationOut {
	return observationOut{
		ID: o.ID, TraceID: o.TraceID, ProjectID: o.ProjectID,
		ParentID: o.ParentID, Type: string(o.Type), Name: o.Name,
		StartTime: o.StartTime, EndTime: o.EndTime,
		Input: o.Input, Output: o.Output, Metadata: o.Metadata,
		Model: o.Model, ModelParameters: o.ModelParameters,
		UsageInput: o.UsageInput, UsageOutput: o.UsageOutput, UsageTotal: o.UsageTotal,
		Level: o.Level, StatusMessage: o.StatusMessage, Environment: o.Environment,
		PromptName: o.PromptName, PromptVersion: o.PromptVersion,
	}
}

// fillTraceAttrs copies user/session from parent traces (N+1 bounded by
// page size ≤1000; acceptable until a denormalized column lands).
func fillTraceAttrs(h *Handler, pid uuid.UUID, out []observationOut) {
	seen := map[string]*models.Trace{}
	for i := range out {
		tid := out[i].TraceID
		tr, ok := seen[tid]
		if !ok {
			var t models.Trace
			if err := h.db.First(&t, "project_id = ? AND trace_id = ?", pid, tid).Error; err != nil {
				continue
			}
			tr = &t
			seen[tid] = tr
		}
		out[i].UserID = tr.UserID
		out[i].SessionID = tr.SessionID
	}
}

// scoreOut mirrors Scores API v3 with a typed value field.
type scoreOut struct {
	ID            uuid.UUID `json:"id"`
	ProjectID     uuid.UUID `json:"projectId"`
	TraceID       string    `json:"traceId"`
	ObservationID *string   `json:"observationId"`
	SessionID     *string   `json:"sessionId"`
	Name          string    `json:"name"`
	Value         any       `json:"value"`
	DataType      string    `json:"dataType"`
	Source        string    `json:"source"`
	Comment       *string   `json:"comment"`
	CreatedAt     time.Time `json:"createdAt"`
}

func toScoreOut(s models.Score) scoreOut {
	var v any
	switch s.DataType {
	case "NUMERIC", "BOOLEAN":
		if s.ValueNum != nil {
			if s.DataType == "BOOLEAN" {
				v = *s.ValueNum == 1
			} else {
				v = *s.ValueNum
			}
		}
	default:
		if s.ValueStr != nil {
			v = *s.ValueStr
		}
	}
	return scoreOut{
		ID: s.ID, ProjectID: s.ProjectID, TraceID: s.TraceID,
		ObservationID: s.ObservationID, SessionID: s.SessionID,
		Name: s.Name, Value: v, DataType: s.DataType, Source: s.Source,
		Comment: s.Comment, CreatedAt: s.CreatedAt,
	}
}

// GET /api/public/v3/scores — minimal filter set (traceId, name), cursor pages.
func (h *Handler) ListScoresV3(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	q := h.db.Model(&models.Score{}).Where("project_id = ?", p.ID)
	if v := c.Query("traceId"); v != "" {
		q = q.Where("trace_id IN ?", strings.Split(v, ","))
	}
	if v := c.Query("name"); v != "" {
		q = q.Where("name IN ?", strings.Split(v, ","))
	}
	limit := clampInt(queryInt(c, "limit", 50), 1, 100)
	offset := decodeCursor(c.Query("cursor"))
	var rows []models.Score
	if err := q.Order("created_at DESC").Limit(limit + 1).Offset(offset).Find(&rows).Error; err != nil {
		return err
	}
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		next = encodeCursor(offset + limit)
	}
	out := make([]scoreOut, 0, len(rows))
	for _, s := range rows {
		out = append(out, toScoreOut(s))
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"cursor": next}})
}

// POST /api/public/scores — single score, synchronous (bulk goes via ingestion).
func (h *Handler) CreateScore(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	var body map[string]any
	if err := json.Unmarshal(append([]byte(nil), c.Body()...), &body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid json")
	}
	if _, ok := body["id"]; !ok {
		body["id"] = "api-" + uuid.NewString()
	}
	wrapped, _ := json.Marshal(map[string]any{
		"batch": []map[string]any{{
			"id": "single", "type": "score-create",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"body":      body,
		}},
	})
	parsed, errs := ingest.Parse(wrapped)
	if len(errs) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, errs[0].Message)
	}
	store := ingest.NewStore(h.db)
	if err := store.Apply(c.Context(), p.ID, parsed[0]); err != nil {
		return err
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"id": body["id"]})
}

// cursor helpers: opaque base64 offset (v2/v3 pagination).

func encodeCursor(offset int) *string {
	s := base64.URLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
	return &s
}

func decodeCursor(raw string) int {
	if raw == "" {
		return 0
	}
	b, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(string(b))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func queryInt(c *fiber.Ctx, key string, def int) int {
	if v := c.Query(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
