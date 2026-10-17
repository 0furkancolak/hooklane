package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database"
	httpserver "github.com/0furkancolak/hooklane/api/internal/transport/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if len(cfg.EndpointSecretKey) == 0 {
		log.Fatal("ENDPOINT_SECRET_KEY is required; generate one with: openssl rand -base64 32")
	}
	secrets, err := application.NewSecretBox(cfg.EndpointSecretKey)
	if err != nil {
		log.Fatalf("ENDPOINT_SECRET_KEY: %v", err)
	}

	store := database.NewStore(pool)
	server := httpserver.NewServer(cfg, httpserver.Services{
		Auth:     application.NewAuthenticator(store.Keys()),
		Projects: application.NewProjectService(store.Projects(), store.Keys()),
		Endpoints: application.NewEndpointService(store.Projects(), store.Endpoints(), secrets, application.EndpointRules{
			AllowInsecureHTTP: cfg.AllowInsecureHTTP(),
			Destinations:      application.DestinationPolicy{AllowPrivateNetworks: cfg.AllowPrivateNetworks()},
		}),
		Events:     application.NewEventService(store.Projects(), store.Events(), store.Deliveries(), store),
		Deliveries: application.NewDeliveryService(store.Events(), store.Deliveries()),
	})

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Hooklane API listening on :%s", cfg.Port)
		errCh <- server.Listen()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatal(err)
		}
	case <-ctx.Done():
		log.Println("Hooklane API shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}
}
