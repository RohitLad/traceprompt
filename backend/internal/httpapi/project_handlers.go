package httpapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/traceprompt/traceprompt/backend/internal/models"
)

// isUniqueViolation detects duplicate-key errors across Postgres and SQLite
// so handlers can return 409 instead of 500 without importing drivers.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "23505")
}

type createProjectReq struct {
	Name           string    `json:"name"`
	OrganizationID uuid.UUID `json:"organizationId"`
}

type projectOut struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	Name           string    `json:"name"`
}

// CreateProject creates a project in an org the user belongs to.
func (h *Handler) CreateProject(c *fiber.Ctx) error {
	uid, err := h.currentUserID(c)
	if err != nil {
		return err
	}
	var req createProjectReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if strings.TrimSpace(req.Name) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}
	if req.OrganizationID == uuid.Nil {
		return fiber.NewError(fiber.StatusBadRequest, "organizationId is required")
	}
	var count int64
	if err := h.db.Model(&models.Membership{}).
		Where("user_id = ? AND organization_id = ?", uid, req.OrganizationID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fiber.NewError(fiber.StatusForbidden, "not a member of this organization")
	}
	project := models.Project{OrganizationID: req.OrganizationID, Name: strings.TrimSpace(req.Name)}
	if err := h.db.Create(&project).Error; err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(projectOut{
		ID: project.ID, OrganizationID: project.OrganizationID, Name: project.Name,
	})
}

// ListProjects returns all projects in orgs the user belongs to.
func (h *Handler) ListProjects(c *fiber.Ctx) error {
	uid, err := h.currentUserID(c)
	if err != nil {
		return err
	}
	var memberships []models.Membership
	if err := h.db.Find(&memberships, "user_id = ?", uid).Error; err != nil {
		return err
	}
	orgIDs := make([]uuid.UUID, 0, len(memberships))
	for _, m := range memberships {
		orgIDs = append(orgIDs, m.OrganizationID)
	}
	var projects []models.Project
	if len(orgIDs) > 0 {
		if err := h.db.Where("organization_id IN ?", orgIDs).Order("created_at ASC").Find(&projects).Error; err != nil {
			return err
		}
	}
	out := make([]projectOut, 0, len(projects))
	for _, p := range projects {
		out = append(out, projectOut{ID: p.ID, OrganizationID: p.OrganizationID, Name: p.Name})
	}
	return c.JSON(fiber.Map{"data": out})
}

// GetProject returns one project (membership already checked by middleware).
func (h *Handler) GetProject(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	var org models.Organization
	if err := h.db.First(&org, "id = ?", p.OrganizationID).Error; err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"id":           p.ID,
			"name":         p.Name,
			"organization": fiber.Map{"id": org.ID, "name": org.Name},
		},
	})
}

// PublicProjects returns the calling key's project (Langfuse GET /api/public/projects shape).
func (h *Handler) PublicProjects(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	var org models.Organization
	if err := h.db.First(&org, "id = ?", p.OrganizationID).Error; err != nil {
		return err
	}
	// Langfuse returns {data:[...]}; single-project keys get a one-element list.
	return c.JSON(fiber.Map{
		"data": []fiber.Map{{
			"id":           p.ID,
			"name":         p.Name,
			"organization": fiber.Map{"id": org.ID, "name": org.Name},
		}},
	})
}
