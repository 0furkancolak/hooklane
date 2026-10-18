package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/logging"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/metrics"
	httpserver "github.com/0furkancolak/hooklane/api/internal/transport/http"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if len(cfg.EndpointSecretKey) == 0 {
		return errors.New("ENDPOINT_SECRET_KEY is required; generate one with: openssl rand -base64 32")
	}
	secrets, err := application.NewSecretBox(cfg.EndpointSecretKey)
	if err != nil {
		return fmt.Errorf("ENDPOINT_SECRET_KEY: %w", err)
	}

	logger := logging.New(os.Stdout, logging.Format(cfg.LogFormat), logging.ColorEnabled(os.Stdout))
	slog.SetDefault(logger)
	ready := func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			logger.Warn("database ping failed", "error", err)
			return errors.New("database unavailable")
		}
		return nil
	}
	registry := metrics.NewRegistry()

	store := database.NewStore(pool)
	projects := application.NewProjectService(store.Projects(), store.Keys())
	var devKeys *application.DevelopmentKeyService
	if cfg.AppEnv == config.AppEnvDevelopment {
		bootstrap := application.NewBootstrapService(store.Orgs(), store.Projects(), store.Keys(), store)
		devKeys = application.NewDevelopmentKeyService(bootstrap, store, projects)
	}
	server := httpserver.NewServer(cfg, httpserver.Services{
		Auth:     application.NewAuthenticator(store.Keys()).WithPortalTokens(store.PortalTokens()),
		Portal:   application.NewPortalService(store.Projects(), store.PortalTokens()),
		Projects: projects,
		Endpoints: application.NewEndpointService(store.Projects(), store.Endpoints(), secrets, application.EndpointRules{
			AllowInsecureHTTP: cfg.AllowInsecureHTTP(),
			Destinations:      application.DestinationPolicy{AllowPrivateNetworks: cfg.AllowPrivateNetworks()},
		}),
		Events:     application.NewEventService(store.Projects(), store.Events(), store.Deliveries(), store),
		Deliveries: application.NewDeliveryService(store.Projects(), store.Events(), store.Deliveries()),
		DevKeys:    devKeys,
	},
		httpserver.WithLogger(logger),
		httpserver.WithRequestObserver(metrics.NewHTTP(registry)),
		httpserver.WithReadiness(ready),
		httpserver.WithDashboard(cfg.WebDir),
	)

	var ops *metrics.OpsServer
	if cfg.MetricsAddr != "" {
		ops = metrics.NewOpsServer(cfg.MetricsAddr, registry, ready, logger)
		ops.Start()
		logger.Info("ops listener started", "addr", cfg.MetricsAddr)
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api listening", "port", cfg.Port, "env", string(cfg.AppEnv), "version", version)
		errCh <- server.Listen()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		logger.Info("api shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown", "error", err)
		}
		if ops != nil {
			_ = ops.Shutdown(shutdownCtx)
		}
	}
	return nil
}
