package http

import (
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type accountHandler struct {
	accounts *application.AccountService
}

type signUpRequest struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
	OrganizationName string `json:"organization_name"`
}

type signInRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type createOrganizationRequest struct {
	Name string `json:"name"`
}

type switchOrganizationRequest struct {
	OrganizationID uuid.UUID `json:"organization_id"`
}

type userResponse struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
}

type organizationResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

type accountResponse struct {
	User          userResponse           `json:"user"`
	Organization  organizationResponse   `json:"organization"`
	Organizations []organizationResponse `json:"organizations"`
}

type sessionResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	accountResponse
}

func mapOrganization(o domain.Organization) organizationResponse {
	return organizationResponse{ID: o.ID, Name: o.Name, Slug: o.Slug}
}

func mapAccount(a application.Account) accountResponse {
	orgs := make([]organizationResponse, 0, len(a.Organizations))
	for _, o := range a.Organizations {
		orgs = append(orgs, mapOrganization(o))
	}
	return accountResponse{
		User:          userResponse{ID: a.User.ID, Email: a.User.Email},
		Organization:  mapOrganization(a.Organization),
		Organizations: orgs,
	}
}

func mapSession(s application.SignedIn) sessionResponse {
	return sessionResponse{Token: s.Token, ExpiresAt: s.ExpiresAt.UTC().Format(time.RFC3339), accountResponse: mapAccount(s.Account)}
}

func (h accountHandler) signUp(c fiber.Ctx) error {
	var req signUpRequest
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	signedIn, err := h.accounts.SignUp(c.Context(), req.Email, req.Password, req.OrganizationName)
	if err != nil {
		return writeError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(mapSession(signedIn))
}

func (h accountHandler) signIn(c fiber.Ctx) error {
	var req signInRequest
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	signedIn, err := h.accounts.SignIn(c.Context(), req.Email, req.Password)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(mapSession(signedIn))
}

func (h accountHandler) me(c fiber.Ctx) error {
	account, err := h.accounts.Me(c.Context(), principalFrom(c))
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(mapAccount(account))
}

func (h accountHandler) signOut(c fiber.Ctx) error {
	if err := h.accounts.SignOut(c.Context(), principalFrom(c)); err != nil {
		return writeError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h accountHandler) createOrganization(c fiber.Ctx) error {
	var req createOrganizationRequest
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	account, err := h.accounts.CreateOrganization(c.Context(), principalFrom(c), req.Name)
	if err != nil {
		return writeError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(mapAccount(account))
}

func (h accountHandler) switchOrganization(c fiber.Ctx) error {
	var req switchOrganizationRequest
	if err := c.Bind().JSON(&req); err != nil || req.OrganizationID == uuid.Nil {
		return writeError(c, application.ErrInvalidInput)
	}
	account, err := h.accounts.SwitchOrganization(c.Context(), principalFrom(c), req.OrganizationID)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(mapAccount(account))
}
