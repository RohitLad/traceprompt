package httpapi

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/models"
)

// ProductionLabel is the default label SDKs fetch (Langfuse-compatible).
const ProductionLabel = "production"

type promptMessageIn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type createPromptReq struct {
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	Template      *string           `json:"template"`
	Messages      []promptMessageIn `json:"messages"`
	Config        map[string]any    `json:"config"`
	Labels        []string          `json:"labels"`
	CommitMessage *string           `json:"commitMessage"`
}

type createVersionReq struct {
	Template      *string           `json:"template"`
	Messages      []promptMessageIn `json:"messages"`
	Config        map[string]any    `json:"config"`
	CommitMessage *string           `json:"commitMessage"`
}

type setLabelsReq struct {
	Version int      `json:"version"`
	Labels  []string `json:"labels"`
}

type versionOut struct {
	Version       int                    `json:"version"`
	Template      *string                `json:"template,omitempty"`
	Messages      []models.PromptMessage `json:"messages,omitempty"`
	Config        map[string]any         `json:"config"`
	Labels        []string               `json:"labels"`
	CommitMessage *string                `json:"commitMessage,omitempty"`
}

type promptOut struct {
	Name     string       `json:"name"`
	Type     string       `json:"type"`
	Versions []versionOut `json:"versions"`
}

func toVersionOut(v models.PromptVersion) versionOut {
	return versionOut{
		Version: v.Version, Template: v.Template, Messages: v.Messages,
		Config: v.Config, Labels: v.Labels, CommitMessage: v.CommitMessage,
	}
}

func validatePromptPayload(promptType string, template *string, messages []promptMessageIn) (string, error) {
	t := strings.ToLower(strings.TrimSpace(promptType))
	if t == "" {
		t = "text"
	}
	switch t {
	case "text":
		if template == nil || strings.TrimSpace(*template) == "" {
			return "", fiber.NewError(fiber.StatusBadRequest, "template is required for text prompts")
		}
	case "chat":
		if len(messages) == 0 {
			return "", fiber.NewError(fiber.StatusBadRequest, "messages are required for chat prompts")
		}
		for _, m := range messages {
			if m.Role == "" || m.Content == "" {
				return "", fiber.NewError(fiber.StatusBadRequest, "each message needs role and content")
			}
		}
	default:
		return "", fiber.NewError(fiber.StatusBadRequest, "type must be text or chat")
	}
	return t, nil
}

// POST /api/v1/projects/:id/prompts — create prompt with version 1.
func (h *Handler) CreatePrompt(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	var req createPromptReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}
	t, err := validatePromptPayload(req.Type, req.Template, req.Messages)
	if err != nil {
		return err
	}
	labels := req.Labels
	if len(labels) == 0 {
		labels = []string{ProductionLabel, "latest"}
	}
	var out promptOut
	err = h.db.Transaction(func(tx *gorm.DB) error {
		pr := models.Prompt{ProjectID: p.ID, Name: req.Name, Type: t}
		if err := tx.Create(&pr).Error; err != nil {
			return err
		}
		ver := models.PromptVersion{
			PromptID: pr.ID, Version: 1, Template: req.Template,
			Config: orEmptyMapAny(req.Config), Labels: labels, CommitMessage: req.CommitMessage,
		}
		for _, m := range req.Messages {
			ver.Messages = append(ver.Messages, models.PromptMessage{Role: m.Role, Content: m.Content})
		}
		if err := tx.Create(&ver).Error; err != nil {
			return err
		}
		out = promptOut{Name: pr.Name, Type: pr.Type, Versions: []versionOut{toVersionOut(ver)}}
		return nil
	})
	if err != nil {
		if isUniqueViolation(err) {
			return fiber.NewError(fiber.StatusConflict, "prompt name already exists")
		}
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// POST /api/v1/projects/:id/prompts/:name/versions — append immutable version.
func (h *Handler) CreatePromptVersion(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	pr, err := h.findPrompt(p.ID, c.Params("name"))
	if err != nil {
		return err
	}
	var req createVersionReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if _, err := validatePromptPayload(pr.Type, req.Template, req.Messages); err != nil {
		// Allow partial updates: fall back to previous version's content.
		if req.Template == nil && len(req.Messages) == 0 {
			prev, err := h.latestVersion(pr.ID)
			if err != nil {
				return err
			}
			req.Template = prev.Template
			for _, m := range prev.Messages {
				req.Messages = append(req.Messages, promptMessageIn{Role: m.Role, Content: m.Content})
			}
			if req.Config == nil {
				req.Config = prev.Config
			}
		} else {
			return err
		}
	}
	var maxV int
	if err := h.db.Model(&models.PromptVersion{}).Select("COALESCE(MAX(version),0)").
		Where("prompt_id = ?", pr.ID).Scan(&maxV).Error; err != nil {
		return err
	}
	ver := models.PromptVersion{
		PromptID: pr.ID, Version: maxV + 1, Template: req.Template,
		Config: orEmptyMapAny(req.Config), Labels: []string{"latest"}, CommitMessage: req.CommitMessage,
	}
	for _, m := range req.Messages {
		ver.Messages = append(ver.Messages, models.PromptMessage{Role: m.Role, Content: m.Content})
	}
	if err := h.db.Create(&ver).Error; err != nil {
		return err
	}
	// "latest" always follows the newest version.
	if err := h.moveLabel(pr.ID, ver.Version, "latest"); err != nil {
		return err
	}
	ver.Labels = []string{"latest"}
	return c.Status(fiber.StatusCreated).JSON(toVersionOut(ver))
}

// GET /api/v1/projects/:id/prompts — list with latest version summary.
func (h *Handler) ListPrompts(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	var prompts []models.Prompt
	if err := h.db.Where("project_id = ?", p.ID).Order("name ASC").Find(&prompts).Error; err != nil {
		return err
	}
	type summary struct {
		Name              string   `json:"name"`
		Type              string   `json:"type"`
		Versions          int      `json:"versions"`
		ProductionVersion *int     `json:"productionVersion"`
		Labels            []string `json:"labels"`
	}
	out := make([]summary, 0, len(prompts))
	for _, pr := range prompts {
		var versions []models.PromptVersion
		h.db.Where("prompt_id = ?", pr.ID).Find(&versions)
		s := summary{Name: pr.Name, Type: pr.Type, Versions: len(versions)}
		seen := map[string]bool{}
		for _, v := range versions {
			for _, l := range v.Labels {
				if !seen[l] {
					seen[l] = true
					s.Labels = append(s.Labels, l)
				}
				if l == ProductionLabel {
					vv := v.Version
					s.ProductionVersion = &vv
				}
			}
		}
		out = append(out, s)
	}
	return c.JSON(fiber.Map{"data": out})
}

// GET /api/v1/projects/:id/prompts/:name — full detail with versions.
func (h *Handler) GetPrompt(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	pr, err := h.findPrompt(p.ID, c.Params("name"))
	if err != nil {
		return err
	}
	var versions []models.PromptVersion
	if err := h.db.Where("prompt_id = ?", pr.ID).Order("version ASC").Find(&versions).Error; err != nil {
		return err
	}
	vout := make([]versionOut, 0, len(versions))
	for _, v := range versions {
		vout = append(vout, toVersionOut(v))
	}
	return c.JSON(promptOut{Name: pr.Name, Type: pr.Type, Versions: vout})
}

// POST /api/v1/projects/:id/prompts/:name/labels — move labels to a version.
func (h *Handler) SetPromptLabels(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	pr, err := h.findPrompt(p.ID, c.Params("name"))
	if err != nil {
		return err
	}
	var req setLabelsReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.Version < 1 || len(req.Labels) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "version and labels are required")
	}
	var target models.PromptVersion
	if err := h.db.First(&target, "prompt_id = ? AND version = ?", pr.ID, req.Version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "version not found")
		}
		return err
	}
	for _, l := range req.Labels {
		if strings.TrimSpace(l) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "labels must not be blank")
		}
		if err := h.moveLabel(pr.ID, req.Version, l); err != nil {
			return err
		}
	}
	return h.GetPrompt(c)
}

// moveLabel removes a label from all versions of a prompt and sets it on one.
// NOTE: writes go through Save (not Update) so GORM's json serializer
// encodes the slice; single-column Update would store driver-stringified text.
func (h *Handler) moveLabel(promptID uuid.UUID, version int, label string) error {
	var versions []models.PromptVersion
	if err := h.db.Where("prompt_id = ?", promptID).Find(&versions).Error; err != nil {
		return err
	}
	for _, v := range versions {
		var kept []string
		for _, l := range v.Labels {
			if l != label {
				kept = append(kept, l)
			}
		}
		if version == v.Version && !contains(kept, label) {
			kept = append(kept, label)
		}
		if len(kept) != len(v.Labels) || version == v.Version {
			v.Labels = kept
			if err := h.db.Save(&v).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) findPrompt(projectID uuid.UUID, name string) (*models.Prompt, error) {
	var pr models.Prompt
	if err := h.db.First(&pr, "project_id = ? AND name = ?", projectID, name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fiber.NewError(fiber.StatusNotFound, "prompt not found")
		}
		return nil, err
	}
	return &pr, nil
}

func (h *Handler) latestVersion(promptID uuid.UUID) (*models.PromptVersion, error) {
	var v models.PromptVersion
	if err := h.db.Where("prompt_id = ?", promptID).Order("version DESC").First(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

// GET /api/public/prompts/:name — SDK fetch (?version=N wins, else ?label=, default production).
func (h *Handler) GetPublicPrompt(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	pr, err := h.findPrompt(p.ID, c.Params("name"))
	if err != nil {
		return err
	}
	var versions []models.PromptVersion
	if err := h.db.Where("prompt_id = ?", pr.ID).Order("version ASC").Find(&versions).Error; err != nil {
		return err
	}
	if len(versions) == 0 {
		return fiber.NewError(fiber.StatusNotFound, "no versions")
	}
	label := c.Query("label", ProductionLabel)
	if v := c.Query("version"); v != "" {
		n, err := parsePositiveInt(v)
		if err != nil {
			return err
		}
		for _, ver := range versions {
			if ver.Version == n {
				return c.JSON(publicPromptOut(pr, ver))
			}
		}
		return fiber.NewError(fiber.StatusNotFound, "version not found")
	}
	for _, ver := range versions {
		if contains(ver.Labels, label) {
			return c.JSON(publicPromptOut(pr, ver))
		}
	}
	return fiber.NewError(fiber.StatusNotFound, "no version with label "+label)
}

func publicPromptOut(pr *models.Prompt, v models.PromptVersion) fiber.Map {
	out := fiber.Map{
		"name": pr.Name, "type": pr.Type, "version": v.Version,
		"config": v.Config, "labels": v.Labels,
	}
	if v.Template != nil {
		out["prompt"] = *v.Template
	}
	if len(v.Messages) > 0 {
		out["prompt"] = v.Messages
	}
	return out
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

func orEmptyMapAny(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func toMessageIn(ms []models.PromptMessage) []promptMessageIn {
	out := make([]promptMessageIn, 0, len(ms))
	for _, m := range ms {
		out = append(out, promptMessageIn{Role: m.Role, Content: m.Content})
	}
	return out
}

func toModelMessages(ms []promptMessageIn) []models.PromptMessage {
	out := make([]models.PromptMessage, 0, len(ms))
	for _, m := range ms {
		out = append(out, models.PromptMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

func parsePositiveInt(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 {
		return 0, fiber.NewError(fiber.StatusBadRequest, "invalid version")
	}
	return n, nil
}
