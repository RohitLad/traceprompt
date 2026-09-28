package httpapi

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
	"github.com/traceprompt/traceprompt/backend/internal/models"
)

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// wrapSingle wraps a score body in a single-event ingestion batch.
func wrapSingle(body []byte) json.RawMessage {
	wrapped, _ := json.Marshal(map[string]any{
		"batch": []map[string]any{{
			"id": "single", "type": "score-create",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"body":      json.RawMessage(body),
		}},
	})
	return wrapped
}

// ---- Score configs ----

type scoreConfigOut struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	DataType    string    `json:"dataType"`
	MinValue    *float64  `json:"minValue"`
	MaxValue    *float64  `json:"maxValue"`
	Categories  []string  `json:"categories"`
	Description *string   `json:"description,omitempty"`
}

func toScoreConfigOut(c models.ScoreConfig) scoreConfigOut {
	return scoreConfigOut{
		ID: c.ID, Name: c.Name, DataType: c.DataType,
		MinValue: c.MinValue, MaxValue: c.MaxValue,
		Categories: c.Categories, Description: c.Description,
	}
}

// POST /api/v1/projects/:id/score-configs
func (h *Handler) CreateScoreConfig(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	var req struct {
		Name        string   `json:"name"`
		DataType    string   `json:"dataType"`
		MinValue    *float64 `json:"minValue"`
		MaxValue    *float64 `json:"maxValue"`
		Categories  []string `json:"categories"`
		Description *string  `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	req.Name = strings.TrimSpace(req.Name)
	dt := strings.ToUpper(strings.TrimSpace(req.DataType))
	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}
	switch dt {
	case "NUMERIC", "CATEGORICAL", "BOOLEAN":
	default:
		return fiber.NewError(fiber.StatusBadRequest, "dataType must be NUMERIC, CATEGORICAL, or BOOLEAN")
	}
	if dt == "NUMERIC" && req.MinValue != nil && req.MaxValue != nil && *req.MinValue > *req.MaxValue {
		return fiber.NewError(fiber.StatusBadRequest, "minValue must not exceed maxValue")
	}
	if dt == "CATEGORICAL" && len(req.Categories) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "categorical configs need categories")
	}
	cfg := models.ScoreConfig{
		ProjectID: p.ID, Name: req.Name, DataType: dt,
		MinValue: req.MinValue, MaxValue: req.MaxValue,
		Categories: orEmptySliceStr(req.Categories), Description: req.Description,
	}
	if err := h.db.Create(&cfg).Error; err != nil {
		if isUniqueViolation(err) {
			return fiber.NewError(fiber.StatusConflict, "score config name already exists")
		}
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(toScoreConfigOut(cfg))
}

// GET /api/v1/projects/:id/score-configs
func (h *Handler) ListScoreConfigs(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	var cfgs []models.ScoreConfig
	if err := h.db.Where("project_id = ?", p.ID).Order("name ASC").Find(&cfgs).Error; err != nil {
		return err
	}
	out := make([]scoreConfigOut, 0, len(cfgs))
	for _, cfg := range cfgs {
		out = append(out, toScoreConfigOut(cfg))
	}
	return c.JSON(fiber.Map{"data": out})
}

// GET /api/public/score-configs — runners discover allowed schemas.
func (h *Handler) ListPublicScoreConfigs(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	var cfgs []models.ScoreConfig
	if err := h.db.Where("project_id = ?", p.ID).Order("name ASC").Find(&cfgs).Error; err != nil {
		return err
	}
	out := make([]scoreConfigOut, 0, len(cfgs))
	for _, cfg := range cfgs {
		out = append(out, toScoreConfigOut(cfg))
	}
	return c.JSON(fiber.Map{"data": out})
}

func orEmptySliceStr(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ---- Annotation queues ----

type queueOut struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Pending     int64     `json:"pending"`
	Total       int64     `json:"total"`
}

// POST /api/v1/projects/:id/annotation-queues
func (h *Handler) CreateQueue(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}
	q := models.AnnotationQueue{ProjectID: p.ID, Name: req.Name, Description: req.Description}
	if err := h.db.Create(&q).Error; err != nil {
		if isUniqueViolation(err) {
			return fiber.NewError(fiber.StatusConflict, "queue name already exists")
		}
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(queueOut{ID: q.ID, Name: q.Name, Description: q.Description})
}

// GET /api/v1/projects/:id/annotation-queues
func (h *Handler) ListQueues(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	var queues []models.AnnotationQueue
	if err := h.db.Where("project_id = ?", p.ID).Order("name ASC").Find(&queues).Error; err != nil {
		return err
	}
	out := make([]queueOut, 0, len(queues))
	for _, q := range queues {
		var total, pending int64
		h.db.Model(&models.AnnotationQueueItem{}).Where("queue_id = ?", q.ID).Count(&total)
		h.db.Model(&models.AnnotationQueueItem{}).Where("queue_id = ? AND status = ?", q.ID, "PENDING").Count(&pending)
		out = append(out, queueOut{ID: q.ID, Name: q.Name, Description: q.Description, Pending: pending, Total: total})
	}
	return c.JSON(fiber.Map{"data": out})
}

func (h *Handler) findQueue(projectID uuid.UUID, qid string) (*models.AnnotationQueue, error) {
	id, err := uuid.Parse(qid)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid queue id")
	}
	var q models.AnnotationQueue
	if err := h.db.First(&q, "id = ? AND project_id = ?", id, projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fiber.NewError(fiber.StatusNotFound, "queue not found")
		}
		return nil, err
	}
	return &q, nil
}

type queueItemOut struct {
	ID            uuid.UUID `json:"id"`
	TraceID       *string   `json:"traceId"`
	ObservationID *string   `json:"observationId"`
	Status        string    `json:"status"`
	TraceName     *string   `json:"traceName"`
}

// GET /api/v1/projects/:id/annotation-queues/:qid?status=PENDING
func (h *Handler) GetQueue(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	q, err := h.findQueue(p.ID, c.Params("qid"))
	if err != nil {
		return err
	}
	query := h.db.Where("queue_id = ?", q.ID)
	if v := c.Query("status"); v != "" {
		query = query.Where("status = ?", strings.ToUpper(v))
	}
	var items []models.AnnotationQueueItem
	if err := query.Order("created_at ASC").Limit(100).Find(&items).Error; err != nil {
		return err
	}
	out := make([]queueItemOut, 0, len(items))
	for _, it := range items {
		o := queueItemOut{ID: it.ID, TraceID: it.TraceID, ObservationID: it.ObservationID, Status: it.Status}
		if it.TraceID != nil {
			var tr models.Trace
			if err := h.db.First(&tr, "project_id = ? AND trace_id = ?", p.ID, *it.TraceID).Error; err == nil {
				o.TraceName = &tr.Name
			}
		}
		out = append(out, o)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"id": q.ID, "name": q.Name, "description": q.Description, "items": out,
	}})
}

// POST /api/v1/projects/:id/annotation-queues/:qid/items {traceId, observationId?}
func (h *Handler) AddQueueItem(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	q, err := h.findQueue(p.ID, c.Params("qid"))
	if err != nil {
		return err
	}
	var req struct {
		TraceID       *string `json:"traceId"`
		ObservationID *string `json:"observationId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.TraceID == nil || *req.TraceID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "traceId is required")
	}
	var tr models.Trace
	if err := h.db.First(&tr, "project_id = ? AND trace_id = ?", p.ID, *req.TraceID).Error; err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "trace not found in this project")
	}
	it := models.AnnotationQueueItem{QueueID: q.ID, TraceID: req.TraceID, ObservationID: req.ObservationID, Status: "PENDING"}
	if err := h.db.Create(&it).Error; err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(queueItemOut{ID: it.ID, TraceID: it.TraceID, ObservationID: it.ObservationID, Status: it.Status, TraceName: &tr.Name})
}

// POST /api/v1/projects/:id/annotation-queues/:qid/items/:itemId/complete
func (h *Handler) CompleteQueueItem(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	q, err := h.findQueue(p.ID, c.Params("qid"))
	if err != nil {
		return err
	}
	itemID, err := uuid.Parse(c.Params("itemId"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid item id")
	}
	var it models.AnnotationQueueItem
	if err := h.db.First(&it, "id = ? AND queue_id = ?", itemID, q.ID).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "item not found")
	}
	it.Status = "COMPLETED"
	if err := h.db.Save(&it).Error; err != nil {
		return err
	}
	return c.JSON(queueItemOut{ID: it.ID, TraceID: it.TraceID, ObservationID: it.ObservationID, Status: it.Status})
}

// POST /api/v1/projects/:id/annotation-queues/:qid/items/:itemId/scores
// Human review: attaches an ANNOTATION score to the item's trace/observation.
func (h *Handler) ScoreQueueItem(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	q, err := h.findQueue(p.ID, c.Params("qid"))
	if err != nil {
		return err
	}
	itemID, err := uuid.Parse(c.Params("itemId"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid item id")
	}
	var it models.AnnotationQueueItem
	if err := h.db.First(&it, "id = ? AND queue_id = ?", itemID, q.ID).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "item not found")
	}
	var req struct {
		Name    string  `json:"name"`
		Value   any     `json:"value"`
		Comment *string `json:"comment"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if strings.TrimSpace(req.Name) == "" || req.Value == nil {
		return fiber.NewError(fiber.StatusBadRequest, "name and value are required")
	}
	valueRaw, _ := jsonMarshal(req.Value)
	body, _ := jsonMarshal(map[string]any{
		"id": "anno-" + it.ID.String(), "traceId": it.TraceID,
		"observationId": it.ObservationID, "name": req.Name,
		"value": json.RawMessage(valueRaw), "comment": req.Comment,
	})
	parsed, errs := ingest.Parse(wrapSingle(body))
	if len(errs) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, errs[0].Message)
	}
	// Reviewer scores bypass schema strictness? No — same rules, but
	// attributed as human review for audit.
	store := ingest.NewStore(h.db)
	if err := store.Apply(c.Context(), p.ID, parsed[0]); err != nil {
		if ingest.IsPermanent(err) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return err
	}
	// Mark the stored score as ANNOTATION source for audit clarity.
	h.db.Model(&models.Score{}).
		Where("project_id = ? AND external_id = ?", p.ID, "anno-"+it.ID.String()).
		Update("source", "ANNOTATION")
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"traceId": it.TraceID, "name": req.Name})
}
