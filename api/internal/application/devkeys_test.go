package application

import (
	"context"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/jackc/pgx/v5"
)

// oldest serves OldestProject from memProjects, which keeps insertion order.
type oldest struct{ projects *memProjects }

func (o oldest) OldestProject(context.Context) (domain.Project, error) {
	if len(o.projects.items) == 0 {
		return domain.Project{}, pgx.ErrNoRows
	}
	return o.projects.items[0], nil
}

func TestDevelopmentKeyServiceBootstrapsThenReusesProject(t *testing.T) {
	ctx := context.Background()
	orgs, projects, keys := &memOrgs{}, &memProjects{}, &memKeys{}
	svc := NewDevelopmentKeyService(
		NewBootstrapService(orgs, projects, keys, nopTx{}),
		oldest{projects},
		NewProjectService(projects, keys),
	)
	auth := NewAuthenticator(keys)

	first, firstRaw, err := svc.Issue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs.items) != 1 || len(projects.items) != 1 || orgs.items[0].Slug != "dev" {
		t.Fatalf("empty database was not bootstrapped: orgs=%v projects=%v", orgs.items, projects.items)
	}

	second, secondRaw, err := svc.Issue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs.items) != 1 || len(projects.items) != 1 {
		t.Fatal("a second key must reuse the existing project")
	}
	if first.ProjectID != second.ProjectID || firstRaw == secondRaw {
		t.Fatalf("keys = %v / %v", first, second)
	}
	for _, raw := range []string{firstRaw, secondRaw} {
		principal, err := auth.Authenticate(ctx, raw)
		if err != nil {
			t.Fatal(err)
		}
		if principal.OrganizationID != orgs.items[0].ID {
			t.Fatal("key belongs to the wrong organization")
		}
	}
}
