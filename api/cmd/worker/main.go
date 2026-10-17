package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/webhook"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if len(cfg.EndpointSecretKey) == 0 {
		log.Fatal("ENDPOINT_SECRET_KEY is required; generate one with: openssl rand -base64 32")
	}
	secrets, err := application.NewSecretBox(cfg.EndpointSecretKey)
	if err != nil {
		log.Fatalf("ENDPOINT_SECRET_KEY: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	sender := webhook.NewSender(webhook.SenderConfig{RequestTimeout: cfg.Worker.RequestTimeout})
	worker, err := application.NewDeliveryWorker(database.NewStore(pool).Queue(), sender, secrets, application.WorkerConfig{
		Concurrency:     cfg.Worker.Concurrency,
		PollInterval:    cfg.Worker.PollInterval,
		Lease:           cfg.Worker.Lease(),
		ShutdownTimeout: cfg.Worker.ShutdownTimeout,
	}, logger)
	if err != nil {
		log.Fatal(err)
	}

	logger.Info("worker started", "concurrency", cfg.Worker.Concurrency, "lease", cfg.Worker.Lease().String())
	if err := worker.Run(ctx); err != nil {
		log.Fatal(err)
	}
	logger.Info("worker stopped")
}
