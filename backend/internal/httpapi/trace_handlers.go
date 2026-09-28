package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/traceprompt/traceprompt/backend/internal/models"
)

type traceListItem struct {
	TraceID          string         `json:"traceId"`
	Name             string         `json:"name"`
	UserID           *string        `json:"userId"`
	SessionID        *string        `json:"sessionId"`
	Tags             []string       `json:"tags"`
	Metadata         map[string]any `json:"metadata"`
	ObservationCount int64          `json:"observationCount"`
}

// GET /api/v1/projects/:id/traces — UI trace list (JWT, membership-checked).
func (h *Handler) ListTracesUI(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	limit := clampInt(queryInt(c, "limit", 25), 1, 100)
	offset := decodeCursor(c.Query("cursor"))

	var traces []models.Trace
	if err := h.db.Where("project_id = ?", p.ID).
		Order("created_at DESC").Limit(limit + 1).Offset(offset).
		Find(&traces).Error; err != nil {
		return err
	}
	var next *string
	if len(traces) > limit {
		traces = traces[:limit]
		next = encodeCursor(offset + limit)
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
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"cursor": next}})
}

// GET /api/v1/projects/:id/traces/:traceId — trace + full observation tree.
func (h *Handler) GetTraceUI(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	tid := c.Params("traceId")
	var t models.Trace
	if err := h.db.First(&t, "project_id = ? AND trace_id = ?", p.ID, tid).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "trace not found")
	}
	var obs []models.Observation
	if err := h.db.Where("project_id = ? AND trace_id = ?", p.ID, tid).
		Order("start_time ASC").Find(&obs).Error; err != nil {
		return err
	}
	obsOut := make([]observationOut, 0, len(obs))
	for _, o := range obs {
		oo := toObservationOut(o)
		oo.UserID = t.UserID
		oo.SessionID = t.SessionID
		obsOut = append(obsOut, oo)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"traceId": t.TraceID, "name": t.Name, "userId": t.UserID,
		"sessionId": t.SessionID, "tags": t.Tags, "metadata": t.Metadata,
		"observations": obsOut,
	}})
}
