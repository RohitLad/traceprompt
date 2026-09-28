package httpapi

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/models"
)

// ---- Datasets (UI API, JWT + membership) ----

type datasetOut struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	ItemCount   int64     `json:"itemCount"`
	RunCount    int64     `json:"runCount"`
}

// POST /api/v1/projects/:id/datasets
func (h *Handler) CreateDataset(c *fiber.Ctx) error {
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
	ds := models.Dataset{ProjectID: p.ID, Name: req.Name, Description: req.Description}
	if err := h.db.Create(&ds).Error; err != nil {
		if isUniqueViolation(err) {
			return fiber.NewError(fiber.StatusConflict, "dataset name already exists")
		}
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(datasetOut{ID: ds.ID, Name: ds.Name, Description: ds.Description})
}

// GET /api/v1/projects/:id/datasets
func (h *Handler) ListDatasets(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	var sets []models.Dataset
	if err := h.db.Where("project_id = ?", p.ID).Order("name ASC").Find(&sets).Error; err != nil {
		return err
	}
	out := make([]datasetOut, 0, len(sets))
	for _, ds := range sets {
		var items, runs int64
		h.db.Model(&models.DatasetItem{}).Where("dataset_id = ?", ds.ID).Count(&items)
		h.db.Model(&models.DatasetRun{}).Where("dataset_id = ?", ds.ID).Count(&runs)
		out = append(out, datasetOut{ID: ds.ID, Name: ds.Name, Description: ds.Description, ItemCount: items, RunCount: runs})
	}
	return c.JSON(fiber.Map{"data": out})
}

func (h *Handler) findDataset(projectID uuid.UUID, did string) (*models.Dataset, error) {
	id, err := uuid.Parse(did)
	if err != nil {
		// Fall back to name lookup for friendlier URLs.
		var ds models.Dataset
		if err := h.db.First(&ds, "project_id = ? AND name = ?", projectID, did).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fiber.NewError(fiber.StatusNotFound, "dataset not found")
			}
			return nil, err
		}
		return &ds, nil
	}
	var ds models.Dataset
	if err := h.db.First(&ds, "id = ? AND project_id = ?", id, projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fiber.NewError(fiber.StatusNotFound, "dataset not found")
		}
		return nil, err
	}
	return &ds, nil
}

type itemOut struct {
	ID             uuid.UUID      `json:"id"`
	Input          *string        `json:"input"`
	ExpectedOutput *string        `json:"expectedOutput"`
	Metadata       map[string]any `json:"metadata"`
}

// GET /api/v1/projects/:id/datasets/:did (detail with items)
func (h *Handler) GetDataset(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	ds, err := h.findDataset(p.ID, c.Params("did"))
	if err != nil {
		return err
	}
	var items []models.DatasetItem
	if err := h.db.Where("dataset_id = ?", ds.ID).Order("created_at ASC").Find(&items).Error; err != nil {
		return err
	}
	iout := make([]itemOut, 0, len(items))
	for _, it := range items {
		iout = append(iout, itemOut{ID: it.ID, Input: it.Input, ExpectedOutput: it.ExpectedOutput, Metadata: it.Metadata})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"id": ds.ID, "name": ds.Name, "description": ds.Description, "items": iout,
	}})
}

// POST /api/v1/projects/:id/datasets/:did/items
func (h *Handler) CreateDatasetItem(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	ds, err := h.findDataset(p.ID, c.Params("did"))
	if err != nil {
		return err
	}
	var req struct {
		Input          *string        `json:"input"`
		ExpectedOutput *string        `json:"expectedOutput"`
		Metadata       map[string]any `json:"metadata"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.Input == nil && req.ExpectedOutput == nil {
		return fiber.NewError(fiber.StatusBadRequest, "input or expectedOutput is required")
	}
	it := models.DatasetItem{
		DatasetID: ds.ID, Input: req.Input, ExpectedOutput: req.ExpectedOutput,
		Metadata: orEmptyMapAny(req.Metadata),
	}
	if err := h.db.Create(&it).Error; err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(itemOut{ID: it.ID, Input: it.Input, ExpectedOutput: it.ExpectedOutput, Metadata: it.Metadata})
}

// ---- Runs ----

type runOut struct {
	ID          uuid.UUID      `json:"id"`
	DatasetID   uuid.UUID      `json:"datasetId"`
	Name        string         `json:"name"`
	Description *string        `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata"`
	ItemCount   int64          `json:"itemCount"`
}

// POST /api/v1/projects/:id/datasets/:did/runs
func (h *Handler) CreateDatasetRun(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	ds, err := h.findDataset(p.ID, c.Params("did"))
	if err != nil {
		return err
	}
	var req struct {
		Name        string         `json:"name"`
		Description *string        `json:"description"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if strings.TrimSpace(req.Name) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}
	run := models.DatasetRun{
		DatasetID: ds.ID, Name: strings.TrimSpace(req.Name),
		Description: req.Description, Metadata: orEmptyMapAny(req.Metadata),
	}
	if err := h.db.Create(&run).Error; err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(runOut{ID: run.ID, DatasetID: run.DatasetID, Name: run.Name, Description: run.Description, Metadata: run.Metadata})
}

// GET /api/v1/projects/:id/datasets/:did/runs
func (h *Handler) ListDatasetRuns(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	ds, err := h.findDataset(p.ID, c.Params("did"))
	if err != nil {
		return err
	}
	var runs []models.DatasetRun
	if err := h.db.Where("dataset_id = ?", ds.ID).Order("created_at DESC").Find(&runs).Error; err != nil {
		return err
	}
	out := make([]runOut, 0, len(runs))
	for _, r := range runs {
		var n int64
		h.db.Model(&models.DatasetRunItem{}).Where("run_id = ?", r.ID).Count(&n)
		out = append(out, runOut{ID: r.ID, DatasetID: r.DatasetID, Name: r.Name, Description: r.Description, Metadata: r.Metadata, ItemCount: n})
	}
	return c.JSON(fiber.Map{"data": out})
}

type runItemOut struct {
	ItemID  uuid.UUID `json:"itemId"`
	TraceID *string   `json:"traceId"`
}

// POST /api/v1/projects/:id/runs/:runId/items — link item → trace.
func (h *Handler) LinkRunItem(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	runID, err := uuid.Parse(c.Params("runId"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid run id")
	}
	var run models.DatasetRun
	if err := h.db.First(&run, "id = ?", runID).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "run not found")
	}
	// Ensure the run belongs to this project via its dataset.
	var ds models.Dataset
	if err := h.db.First(&ds, "id = ? AND project_id = ?", run.DatasetID, p.ID).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "run not found")
	}
	var req struct {
		ItemID  uuid.UUID `json:"itemId"`
		TraceID *string   `json:"traceId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.ItemID == uuid.Nil {
		return fiber.NewError(fiber.StatusBadRequest, "itemId is required")
	}
	var item models.DatasetItem
	if err := h.db.First(&item, "id = ? AND dataset_id = ?", req.ItemID, run.DatasetID).Error; err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "item does not belong to this run's dataset")
	}
	ri := models.DatasetRunItem{RunID: run.ID, ItemID: req.ItemID, TraceID: req.TraceID}
	// Re-running an item updates its trace link (upsert on run+item).
	if err := h.db.Where("run_id = ? AND item_id = ?", run.ID, req.ItemID).
		Assign(models.DatasetRunItem{TraceID: req.TraceID}).
		FirstOrCreate(&ri).Error; err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(runItemOut{ItemID: ri.ItemID, TraceID: ri.TraceID})
}

// GET /api/v1/projects/:id/runs/:runId — run with linked items + trace names.
func (h *Handler) GetDatasetRun(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	runID, err := uuid.Parse(c.Params("runId"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid run id")
	}
	var run models.DatasetRun
	if err := h.db.First(&run, "id = ?", runID).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "run not found")
	}
	if err := h.db.First(&models.Dataset{}, "id = ? AND project_id = ?", run.DatasetID, p.ID).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "run not found")
	}
	var links []models.DatasetRunItem
	if err := h.db.Where("run_id = ?", run.ID).Find(&links).Error; err != nil {
		return err
	}
	type detail struct {
		runItemOut
		TraceName *string `json:"traceName"`
	}
	items := make([]detail, 0, len(links))
	for _, l := range links {
		d := detail{runItemOut: runItemOut{ItemID: l.ItemID, TraceID: l.TraceID}}
		if l.TraceID != nil {
			var tr models.Trace
			if err := h.db.First(&tr, "project_id = ? AND trace_id = ?", p.ID, *l.TraceID).Error; err == nil {
				d.TraceName = &tr.Name
			}
		}
		items = append(items, d)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"id": run.ID, "datasetId": run.DatasetID, "name": run.Name,
		"description": run.Description, "items": items,
	}})
}

// ---- Public dataset API (BasicAuth, experiment runners) ----

// GET /api/public/datasets
func (h *Handler) ListPublicDatasets(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	var sets []models.Dataset
	if err := h.db.Where("project_id = ?", p.ID).Order("name ASC").Find(&sets).Error; err != nil {
		return err
	}
	names := make([]fiber.Map, 0, len(sets))
	for _, ds := range sets {
		names = append(names, fiber.Map{"id": ds.ID, "name": ds.Name, "description": ds.Description})
	}
	return c.JSON(fiber.Map{"data": names})
}

// GET /api/public/datasets/:name — dataset with items (bounded 200).
func (h *Handler) GetPublicDataset(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	ds, err := h.findDataset(p.ID, c.Params("name"))
	if err != nil {
		return err
	}
	var items []models.DatasetItem
	if err := h.db.Where("dataset_id = ?", ds.ID).Order("created_at ASC").Limit(200).Find(&items).Error; err != nil {
		return err
	}
	iout := make([]itemOut, 0, len(items))
	for _, it := range items {
		iout = append(iout, itemOut{ID: it.ID, Input: it.Input, ExpectedOutput: it.ExpectedOutput, Metadata: it.Metadata})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"id": ds.ID, "name": ds.Name, "description": ds.Description, "items": iout,
	}})
}
