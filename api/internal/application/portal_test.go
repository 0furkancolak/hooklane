package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/testutil/memstore"
	"github.com/google/uuid"
)

func TestPortalTokenExpires(t *testing.T) {
	ctx := context.Background()
	store := memstore.New()
	org := uuid.New()
	project, err := store.Projects().Create(ctx, domain.Project{OrganizationID: org, Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	admin := application.Principal{OrganizationID: org, Scopes: []domain.Scope{domain.ScopeAdmin}}
	portal := application.NewPortalService(store.Projects(), store.PortalTokens())
	link, err := portal.CreateLink(ctx, admin, project.ID, "acme", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	auth := application.NewAuthenticator(store.Keys()).WithPortalTokens(store.PortalTokens())
	principal, _, err := auth.AuthenticatePortal(ctx, link.Token)
	if err != nil || principal.Customer == nil || *principal.Customer != "acme" || principal.ProjectID != project.ID {
		t.Fatalf("fresh token: %+v, %v", principal, err)
	}

	auth.SetClock(func() time.Time { return time.Now().Add(2 * time.Hour) })
	if _, _, err := auth.AuthenticatePortal(ctx, link.Token); !errors.Is(err, application.ErrUnauthorized) {
		t.Fatalf("expired token: %v, want ErrUnauthorized", err)
	}
}
