package httpapi

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
	"github.com/traceprompt/traceprompt/backend/internal/models"
	"github.com/traceprompt/traceprompt/backend/internal/playground"
)

type playgroundReq struct {
	// Template sources (exactly one of inline template or stored prompt).
	Template   *string `json:"template"`
	PromptName *string `json:"promptName"`
	Version    *int    `json:"version"`
	Label      *string `json:"label"`

	Variables map[string]string `json:"variables"`
	Provider  struct {
		BaseURL string `json:"baseUrl"`
		APIKey  string `json:"apiKey"`
		Model   string `json:"model"`
	} `json:"provider"`
	SaveAsTrace *bool   `json:"saveAsTrace"`
	TraceName   *string `json:"traceName"`
}

// POST /api/v1/projects/:id/playground/run — render + call + optionally log.
// Provider credentials are request-scoped only and never persisted.
func (h *Handler) PlaygroundRun(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	var req playgroundReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	template, promptName, promptVersion, err := h.resolveTemplate(p.ID, &req)
	if err != nil {
		return err
	}
	vars := req.Variables
	if vars == nil {
		vars = map[string]string{}
	}
	if missing := playground.Missing(template, vars); len(missing) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, "missing variables: "+strings.Join(missing, ", "))
	}
	rendered := playground.Render(template, vars)

	res, err := playground.NewClient().Run(c.Context(),
		req.Provider.BaseURL, req.Provider.APIKey, req.Provider.Model,
		[]playground.Message{{Role: "user", Content: rendered}})
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}

	var traceID *string
	if req.SaveAsTrace != nil && *req.SaveAsTrace {
		tid := "pg-" + uuid.NewString()[:8] + "-" + time.Now().UTC().Format("150405")
		name := "playground"
		if req.TraceName != nil && *req.TraceName != "" {
			name = *req.TraceName
		}
		if err := h.logPlaygroundTrace(c, p.ID, tid, name, rendered, &res, promptName, promptVersion); err != nil {
			return err
		}
		traceID = &tid
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"output": res.Output, "model": res.Model,
		"inputTokens": res.InputTokens, "outputTokens": res.OutputTokens,
		"totalTokens": res.TotalTokens, "latencyMs": res.LatencyMs,
		"traceId": traceID,
	}})
}

// resolveTemplate returns the runnable template plus prompt linkage (if any).
func (h *Handler) resolveTemplate(projectID uuid.UUID, req *playgroundReq) (template string, name *string, version *int, err error) {
	hasInline := req.Template != nil && *req.Template != ""
	hasStored := req.PromptName != nil && *req.PromptName != ""
	if hasInline == hasStored {
		return "", nil, nil, fiber.NewError(fiber.StatusBadRequest, "exactly one of template or promptName is required")
	}
	if hasInline {
		return *req.Template, nil, nil, nil
	}
	pr, ferr := h.findPrompt(projectID, *req.PromptName)
	if ferr != nil {
		return "", nil, nil, ferr
	}
	var versions []models.PromptVersion
	if err := h.db.Where("prompt_id = ?", pr.ID).Order("version ASC").Find(&versions).Error; err != nil {
		return "", nil, nil, err
	}
	label := "production"
	if req.Label != nil && *req.Label != "" {
		label = *req.Label
	}
	var chosen *models.PromptVersion
	if req.Version != nil {
		for i := range versions {
			if versions[i].Version == *req.Version {
				chosen = &versions[i]
			}
		}
	} else {
		for i := range versions {
			for _, l := range versions[i].Labels {
				if l == label {
					chosen = &versions[i]
				}
			}
		}
	}
	if chosen == nil {
		return "", nil, nil, fiber.NewError(fiber.StatusNotFound, "prompt version not found")
	}
	if pr.Type == "chat" {
		// Chat prompts compile to a single user message for the proxy call.
		var sb strings.Builder
		for _, m := range chosen.Messages {
			sb.WriteString(m.Role + ": " + m.Content + "\n")
		}
		v := chosen.Version
		return strings.TrimSpace(sb.String()), &pr.Name, &v, nil
	}
	if chosen.Template == nil {
		return "", nil, nil, fiber.NewError(fiber.StatusNotFound, "prompt version has no template")
	}
	v := chosen.Version
	return *chosen.Template, &pr.Name, &v, nil
}

// logPlaygroundTrace persists the run as trace + generation for comparison.
func (h *Handler) logPlaygroundTrace(c *fiber.Ctx, projectID uuid.UUID, traceID, name, input string, res *playground.Result, promptName *string, promptVersion *int) error {
	store := ingest.NewStore(h.db)
	ctx := c.Context()
	tbody, _ := json.Marshal(map[string]any{"id": traceID, "name": name, "tags": []string{"playground"}})
	if err := store.Apply(ctx, projectID, ingest.Parsed{
		EventID: "pg-" + traceID, Type: ingest.TypeTraceCreate, Timestamp: time.Now().UTC(), Body: tbody,
	}); err != nil {
		return err
	}
	obody, _ := json.Marshal(map[string]any{
		"id": "pg-" + traceID + "-gen", "traceId": traceID, "type": "GENERATION",
		"name": "playground-run", "model": res.Model,
		"input": input, "output": res.Output,
		"usage":      map[string]any{"input": res.InputTokens, "output": res.OutputTokens, "total": res.TotalTokens},
		"promptName": promptName, "promptVersion": promptVersion,
	})
	return store.Apply(ctx, projectID, ingest.Parsed{
		EventID: "pg-" + traceID + "-gen", Type: ingest.TypeObservationCreate,
		Timestamp: time.Now().UTC(), Body: obody,
	})
}

// ---- Sessions (UI API; traces already carry session_id) ----

type sessionOut struct {
	SessionID   string `json:"sessionId"`
	TraceCount  int    `json:"traceCount"`
	LastTraceAt string `json:"lastTraceAt"`
}

// GET /api/v1/projects/:id/sessions — recent sessions by activity.
func (h *Handler) ListSessionsUI(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	limit := clampInt(queryInt(c, "limit", 25), 1, 100)
	var traces []models.Trace
	if err := h.db.Select("session_id", "created_at").
		Where("project_id = ? AND session_id IS NOT NULL AND session_id <> ''", p.ID).
		Order("created_at DESC").Limit(5000).Find(&traces).Error; err != nil {
		return err
	}
	type agg struct {
		count int
		last  string
	}
	groups := map[string]*agg{}
	order := []string{}
	for _, t := range traces {
		sid := ""
		if t.SessionID != nil {
			sid = *t.SessionID
		}
		if sid == "" {
			continue
		}
		a, ok := groups[sid]
		if !ok {
			a = &agg{}
			groups[sid] = a
			order = append(order, sid)
		}
		a.count++
		if a.last == "" {
			a.last = t.CreatedAt.UTC().Format(time.RFC3339)
		}
	}
	out := make([]sessionOut, 0, len(order))
	for _, sid := range order {
		a := groups[sid]
		out = append(out, sessionOut{SessionID: sid, TraceCount: a.count, LastTraceAt: a.last})
		if len(out) >= limit {
			break
		}
	}
	return c.JSON(fiber.Map{"data": out})
}

// GET /api/v1/projects/:id/sessions/:sid/traces — traces in a session.
func (h *Handler) ListSessionTracesUI(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	sid := c.Params("sid")
	limit := clampInt(queryInt(c, "limit", 25), 1, 100)
	var traces []models.Trace
	if err := h.db.Where("project_id = ? AND session_id = ?", p.ID, sid).
		Order("created_at DESC").Limit(limit).Find(&traces).Error; err != nil {
		return err
	}
	out := make([]traceListItem, 0, len(traces))
	for _, t := range traces {
		var n int64
		h.db.Model(&models.Observation{}).
			Where("project_id = ? AND trace_id = ?", p.ID, t.TraceID).Count(&n)
		out = append(out, traceListItem{
			TraceID: t.TraceID, Name: t.Name, UserID: t.UserID,
			SessionID: t.SessionID, Tags: t.Tags, Metadata: t.Metadata,
			ObservationCount: n,
		})
	}
	return c.JSON(fiber.Map{"data": out})
}

// ---- Scores (UI API) ----

// GET /api/v1/projects/:id/scores?traceId&name&limit&cursor
func (h *Handler) ListScoresUI(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	q := h.db.Model(&models.Score{}).Where("project_id = ?", p.ID)
	if v := c.Query("traceId"); v != "" {
		q = q.Where("trace_id = ?", v)
	}
	if v := c.Query("name"); v != "" {
		q = q.Where("name = ?", v)
	}
	limit := clampInt(queryInt(c, "limit", 25), 1, 100)
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
