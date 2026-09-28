package httpapi

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/auth"
	"github.com/traceprompt/traceprompt/backend/internal/models"
	"github.com/traceprompt/traceprompt/backend/internal/queue"
)

// Handler holds request-scoped dependencies.
type Handler struct {
	db        *gorm.DB
	jwtSecret string
	queue     queue.Queue
}

const (
	ctxUserKey    = "tp_user_id"
	ctxProjectKey = "tp_project"
	ctxAPIKeyKey  = "tp_api_key"
)

// requireJWT validates `Authorization: Bearer <jwt>` and stores the user ID.
func (h *Handler) requireJWT(c *fiber.Ctx) error {
	header := c.Get("Authorization")
	raw, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || raw == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "missing bearer token")
	}
	uid, err := auth.ParseToken(h.jwtSecret, raw)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid token")
	}
	c.Locals(ctxUserKey, uid)
	return c.Next()
}

// currentUserID returns the JWT-authenticated user or an error.
func (h *Handler) currentUserID(c *fiber.Ctx) (uuid.UUID, error) {
	uid, ok := c.Locals(ctxUserKey).(uuid.UUID)
	if !ok || uid == uuid.Nil {
		return uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
	}
	return uid, nil
}

// requireProjectMember ensures the JWT user belongs to the project org.
// Stores the project for downstream handlers.
func (h *Handler) requireProjectMember(c *fiber.Ctx) error {
	uid, err := h.currentUserID(c)
	if err != nil {
		return err
	}
	pid, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid project id")
	}
	var project models.Project
	if err := h.db.First(&project, "id = ?", pid).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "project not found")
		}
		return err
	}
	var count int64
	if err := h.db.Model(&models.Membership{}).
		Where("user_id = ? AND organization_id = ?", uid, project.OrganizationID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fiber.NewError(fiber.StatusForbidden, "not a member of this project")
	}
	c.Locals(ctxProjectKey, &project)
	return c.Next()
}

// currentProject returns the project stored by requireProjectMember.
func currentProject(c *fiber.Ctx) *models.Project {
	p, _ := c.Locals(ctxProjectKey).(*models.Project)
	return p
}

// requireAPIKey validates BasicAuth `pk-lf-...:sk-lf-...` for /api/public.
// On success stores the ApiKey + Project. Revoked keys are rejected.
func (h *Handler) requireAPIKey(c *fiber.Ctx) error {
	pub, sec, ok := parseBasicAuth(c.Get("Authorization"))
	if !ok || pub == "" || sec == "" {
		c.Set("WWW-Authenticate", `Basic realm="traceprompt"`)
		return fiber.NewError(fiber.StatusUnauthorized, "missing api credentials")
	}
	var key models.ApiKey
	if err := h.db.First(&key, "public_key = ?", pub).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
		}
		return err
	}
	if key.Revoked() || !auth.CheckSecret(sec, key.SecretHash) {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	var project models.Project
	if err := h.db.First(&project, "id = ?", key.ProjectID).Error; err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	c.Locals(ctxAPIKeyKey, &key)
	c.Locals(ctxProjectKey, &project)
	return c.Next()
}

// parseBasicAuth decodes a `Basic base64(user:pass)` header.
func parseBasicAuth(header string) (user, pass string, ok bool) {
	const prefix = "Basic "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len(prefix):]))
	if err != nil {
		return "", "", false
	}
	user, pass, ok = strings.Cut(string(decoded), ":")
	return user, pass, ok
}
