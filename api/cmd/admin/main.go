package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database"
	"github.com/0furkancolak/hooklane/api/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "bootstrap":
		if err := runBootstrap(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "migrate":
		if err := runMigrate(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "prune":
		if err := runPrune(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "version", "-version", "--version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func runBootstrap(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	orgName := fs.String("org-name", "", "organization name")
	orgSlug := fs.String("org-slug", "", "organization slug")
	projectName := fs.String("project-name", "default", "first project name")
	keyName := fs.String("key-name", "bootstrap", "first API key name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	store := database.NewStore(pool)
	svc := application.NewBootstrapService(store.Orgs(), store.Projects(), store.Keys(), store)
	result, err := svc.Bootstrap(ctx, application.BootstrapInput{
		OrganizationName: *orgName,
		OrganizationSlug: *orgSlug,
		ProjectName:      *projectName,
		KeyName:          *keyName,
	})
	if err != nil {
		return err
	}

	fmt.Printf("organization_id=%s\n", result.Organization.ID)
	fmt.Printf("project_id=%s\n", result.Project.ID)
	fmt.Printf("api_key_id=%s\n", result.APIKey.ID)
	fmt.Printf("api_key_prefix=%s\n", result.APIKey.Prefix)
	fmt.Printf("api_key=%s\n", result.RawAPIKey)
	fmt.Println("Store this API key now. It will not be shown again.")
	return nil
}

// runMigrate applies or reports the embedded migrations. Only DATABASE_URL
// is required, so it can run before the other settings exist.
func runMigrate(args []string) error {
	if len(args) != 1 || (args[0] != "up" && args[0] != "status") {
		return fmt.Errorf("usage: admin migrate up|status")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if args[0] == "status" {
		statuses, err := provider.Status(ctx)
		if err != nil {
			return err
		}
		for _, s := range statuses {
			applied := "pending"
			if s.State == goose.StateApplied {
				applied = "applied " + s.AppliedAt.UTC().Format(time.RFC3339)
			}
			fmt.Printf("%-50s %s\n", s.Source.Path, applied)
		}
		return nil
	}
	results, err := provider.Up(ctx)
	for _, r := range results {
		fmt.Printf("applied %s (%s)\n", r.Source.Path, r.Duration.Round(time.Millisecond))
	}
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Println("no migrations to apply")
	}
	return nil
}

func runPrune(args []string) error {
	fs := flag.NewFlagSet("prune", flag.ExitOnError)
	olderThan := fs.Duration("older-than", 0, "delete finished events older than this (default: RETENTION_PERIOD)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	period := *olderThan
	if period == 0 {
		period = cfg.RetentionPeriod
	}
	if period == 0 {
		return fmt.Errorf("retention is disabled; pass -older-than")
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	retention, err := application.NewRetention(database.NewStore(pool), period, nil)
	if err != nil {
		return err
	}
	n, err := retention.PruneOnce(ctx)
	fmt.Printf("deleted %d events older than %s\n", n, period)
	return err
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  admin migrate up|status
  admin bootstrap -org-name NAME -org-slug SLUG [-project-name NAME] [-key-name NAME]
  admin prune [-older-than DURATION]
  admin version
`)
}
