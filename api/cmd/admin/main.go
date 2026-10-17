package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database"
)

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

func usage() {
	fmt.Fprintln(os.Stderr, "usage: admin bootstrap -org-name NAME -org-slug SLUG [-project-name NAME] [-key-name NAME]")
}
