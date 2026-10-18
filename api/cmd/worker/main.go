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
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/webhook"
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
	if len(cfg.EndpointSecretKey) == 0 {
		return errors.New("ENDPOINT_SECRET_KEY is required; generate one with: openssl rand -base64 32")
	}
	secrets, err := application.NewSecretBox(cfg.EndpointSecretKey)
	if err != nil {
		return fmt.Errorf("ENDPOINT_SECRET_KEY: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	logger := logging.New(os.Stdout, logging.Format(cfg.LogFormat), logging.ColorEnabled(os.Stdout))
	slog.SetDefault(logger)
	store := database.NewStore(pool)
	registry := metrics.NewRegistry()
	observer := metrics.NewWorker(registry)
	metrics.RegisterBacklog(registry, store.QueueBacklog)
	sender := webhook.NewSender(webhook.SenderConfig{
		RequestTimeout: cfg.Worker.RequestTimeout,
		Destinations:   application.DestinationPolicy{AllowPrivateNetworks: cfg.AllowPrivateNetworks()},
	})
	worker, err := application.NewDeliveryWorker(store.Queue(), sender, secrets, application.WorkerConfig{
		Concurrency:     cfg.Worker.Concurrency,
		PollInterval:    cfg.Worker.PollInterval,
		Lease:           cfg.Worker.Lease(),
		ShutdownTimeout: cfg.Worker.ShutdownTimeout,
		Retry: application.RetryPolicy{
			MaxAttempts: int32(cfg.Worker.MaxAttempts),
			BaseDelay:   cfg.Worker.RetryBaseDelay,
			MaxDelay:    cfg.Worker.RetryMaxDelay,
		},
		Observer: observer,
	}, logger)
	if err != nil {
		return err
	}

	if cfg.MetricsAddr != "" {
		ops := metrics.NewOpsServer(cfg.MetricsAddr, registry, func(probeCtx context.Context) error {
			if ctx.Err() != nil {
				return errors.New("shutting down")
			}
			if err := pool.Ping(probeCtx); err != nil {
				logger.Warn("database ping failed", "error", err)
				return errors.New("database unavailable")
			}
			return nil
		}, logger)
		ops.Start()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = ops.Shutdown(shutdownCtx)
		}()
		logger.Info("ops listener started", "addr", cfg.MetricsAddr)
	}

	if cfg.RetentionPeriod > 0 {
		retention, err := application.NewRetention(store, cfg.RetentionPeriod, logger)
		if err != nil {
			return err
		}
		retention.Deleted = observer.EventsPruned
		go retention.Run(ctx, time.Hour)
	}

	logger.Info("worker started", "version", version, "retention", cfg.RetentionPeriod.String(), "concurrency", cfg.Worker.Concurrency, "lease", cfg.Worker.Lease().String(), "allow_private_networks", cfg.AllowPrivateNetworks())
	if err := worker.Run(ctx); err != nil {
		return err
	}
	logger.Info("worker stopped")
	return nil
}
