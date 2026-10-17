package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/metrics"
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

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
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
	server := httpserver.NewServer(cfg, httpserver.Services{
		Auth:     application.NewAuthenticator(store.Keys()),
		Projects: application.NewProjectService(store.Projects(), store.Keys()),
		Endpoints: application.NewEndpointService(store.Projects(), store.Endpoints(), secrets, application.EndpointRules{
			AllowInsecureHTTP: cfg.AllowInsecureHTTP(),
			Destinations:      application.DestinationPolicy{AllowPrivateNetworks: cfg.AllowPrivateNetworks()},
		}),
		Events:     application.NewEventService(store.Projects(), store.Events(), store.Deliveries(), store),
		Deliveries: application.NewDeliveryService(store.Projects(), store.Events(), store.Deliveries()),
	},
		httpserver.WithLogger(logger),
		httpserver.WithRequestObserver(metrics.NewHTTP(registry)),
		httpserver.WithReadiness(ready),
	)

	var ops *metrics.OpsServer
	if cfg.MetricsAddr != "" {
		ops = metrics.NewOpsServer(cfg.MetricsAddr, registry, ready, logger)
		ops.Start()
		logger.Info("ops listener started", "addr", cfg.MetricsAddr)
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api listening", "port", cfg.Port, "env", string(cfg.AppEnv))
		errCh <- server.Listen()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatal(err)
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
}
