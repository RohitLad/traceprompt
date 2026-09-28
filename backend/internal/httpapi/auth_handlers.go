package httpapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/auth"
	"github.com/traceprompt/traceprompt/backend/internal/models"
)

type registerReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	OrgName  string `json:"orgName"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResp struct {
	Token string  `json:"token"`
	User  userOut `json:"user"`
	Org   orgOut  `json:"org"`
}

type userOut struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

type orgOut struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Register creates user + organization + owner membership and returns a JWT.
// First-user bootstrap is intentionally open; gate it behind invite codes later.
func (h *Handler) Register(c *fiber.Ctx) error {
	var req registerReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(req.Email, "@") {
		return fiber.NewError(fiber.StatusBadRequest, "invalid email")
	}
	if strings.TrimSpace(req.Name) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}
	orgName := strings.TrimSpace(req.OrgName)
	if orgName == "" {
		orgName = req.Name + "'s org"
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	var out authResp
	err = h.db.Transaction(func(tx *gorm.DB) error {
		user := models.User{Email: req.Email, Name: strings.TrimSpace(req.Name), PasswordHash: hash}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		org := models.Organization{Name: orgName}
		if err := tx.Create(&org).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.Membership{
			UserID:         user.ID,
			OrganizationID: org.ID,
			Role:           "owner",
		}).Error; err != nil {
			return err
		}
		tok, err := auth.IssueToken(h.jwtSecret, user.ID, auth.TokenTTL)
		if err != nil {
			return err
		}
		out = authResp{
			Token: tok,
			User:  userOut{ID: user.ID, Email: user.Email, Name: user.Name},
			Org:   orgOut{ID: org.ID, Name: org.Name},
		}
		return nil
	})
	if err != nil {
		if isUniqueViolation(err) {
			return fiber.NewError(fiber.StatusConflict, "email already registered")
		}
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// Login verifies credentials and returns a fresh JWT.
func (h *Handler) Login(c *fiber.Ctx) error {
	var req loginReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	var user models.User
	if err := h.db.First(&user, "email = ?", email).Error; err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	var membership models.Membership
	if err := h.db.First(&membership, "user_id = ?", user.ID).Error; err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "no organization")
	}
	var org models.Organization
	if err := h.db.First(&org, "id = ?", membership.OrganizationID).Error; err != nil {
		return err
	}
	tok, err := auth.IssueToken(h.jwtSecret, user.ID, auth.TokenTTL)
	if err != nil {
		return err
	}
	return c.JSON(authResp{
		Token: tok,
		User:  userOut{ID: user.ID, Email: user.Email, Name: user.Name},
		Org:   orgOut{ID: org.ID, Name: org.Name},
	})
}

// Me returns the JWT-authenticated user.
func (h *Handler) Me(c *fiber.Ctx) error {
	uid, err := h.currentUserID(c)
	if err != nil {
		return err
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", uid).Error; err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "user not found")
	}
	return c.JSON(userOut{ID: user.ID, Email: user.Email, Name: user.Name})
}
