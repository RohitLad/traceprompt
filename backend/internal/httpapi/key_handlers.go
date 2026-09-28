package httpapi

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/traceprompt/traceprompt/backend/internal/auth"
	"github.com/traceprompt/traceprompt/backend/internal/models"
)

type createKeyReq struct {
	Name string `json:"name"`
}

type keyOut struct {
	ID        uuid.UUID  `json:"id"`
	ProjectID uuid.UUID  `json:"projectId"`
	Name      string     `json:"name"`
	PublicKey string     `json:"publicKey"`
	CreatedAt time.Time  `json:"createdAt"`
	RevokedAt *time.Time `json:"revokedAt"`
}

type createdKeyOut struct {
	keyOut
	// Secret is returned ONCE at creation; it is never stored raw
	// and never returned again (Langfuse behavior).
	Secret string `json:"secret"`
}

func toKeyOut(k models.ApiKey) keyOut {
	return keyOut{
		ID: k.ID, ProjectID: k.ProjectID, Name: k.Name,
		PublicKey: k.PublicKey, CreatedAt: k.CreatedAt, RevokedAt: k.RevokedAt,
	}
}

// CreateKey generates a pk-lf-/sk-lf- pair scoped to the project.
// The raw secret appears only in this response; only its hash is stored.
func (h *Handler) CreateKey(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	uid, err := h.currentUserID(c)
	if err != nil {
		return err
	}
	var req createKeyReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if strings.TrimSpace(req.Name) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}
	gen, err := auth.GenerateKey()
	if err != nil {
		return err
	}
	key := models.ApiKey{
		ProjectID: p.ID, Name: strings.TrimSpace(req.Name),
		PublicKey: gen.PublicKey, SecretHash: gen.Hash, CreatedBy: &uid,
	}
	if err := h.db.Create(&key).Error; err != nil {
		if isUniqueViolation(err) {
			return fiber.NewError(fiber.StatusConflict, "key collision, retry")
		}
		return err
	}
	out := createdKeyOut{keyOut: toKeyOut(key), Secret: gen.Secret}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// ListKeys returns key metadata (never secrets), newest last.
func (h *Handler) ListKeys(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	var keys []models.ApiKey
	if err := h.db.Where("project_id = ?", p.ID).Order("created_at ASC").Find(&keys).Error; err != nil {
		return err
	}
	out := make([]keyOut, 0, len(keys))
	for _, k := range keys {
		out = append(out, toKeyOut(k))
	}
	return c.JSON(fiber.Map{"data": out})
}

// RevokeKey soft-revokes a key; in-flight SDK batches fail closed on next call.
func (h *Handler) RevokeKey(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	keyID, err := uuid.Parse(c.Params("keyId"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid key id")
	}
	var key models.ApiKey
	if err := h.db.First(&key, "id = ? AND project_id = ?", keyID, p.ID).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "key not found")
	}
	now := time.Now().UTC()
	key.RevokedAt = &now
	if err := h.db.Save(&key).Error; err != nil {
		return err
	}
	return c.JSON(toKeyOut(key))
}
