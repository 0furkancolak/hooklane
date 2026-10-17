package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type AppEnv string

const (
	AppEnvDevelopment AppEnv = "development"
	AppEnvProduction  AppEnv = "production"
	AppEnvTest        AppEnv = "test"
)

type Config struct {
	AppEnv      AppEnv
	Port        string
	DatabaseURL string
	// EndpointSecretKey encrypts endpoint signing secrets at rest. It is
	// optional here so the admin CLI can run without it; the API and worker
	// refuse to start when it is missing.
	EndpointSecretKey []byte

	Worker WorkerConfig
}

type WorkerConfig struct {
	Concurrency     int
	PollInterval    time.Duration
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	MaxAttempts     int
	RetryBaseDelay  time.Duration
	RetryMaxDelay   time.Duration
}

// Lease leaves room after the request timeout to record the result, so a live
// worker never loses its claim mid-delivery.
func (w WorkerConfig) Lease() time.Duration {
	return w.RequestTimeout + 30*time.Second
}

func (c Config) AllowInsecureHTTP() bool {
	return c.AppEnv == AppEnvDevelopment
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:      AppEnv(getenv("APP_ENV", string(AppEnvDevelopment))),
		Port:        getenv("PORT", "8080"),
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
	}

	switch cfg.AppEnv {
	case AppEnvDevelopment, AppEnvProduction, AppEnvTest:
	default:
		return Config{}, fmt.Errorf("APP_ENV must be development, production, or test")
	}

	if cfg.Port == "" {
		return Config{}, fmt.Errorf("PORT is required")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	if raw := strings.TrimSpace(os.Getenv("ENDPOINT_SECRET_KEY")); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil || len(key) != 32 {
			return Config{}, fmt.Errorf("ENDPOINT_SECRET_KEY must be base64 encoding of 32 bytes")
		}
		cfg.EndpointSecretKey = key
	}

	worker, err := loadWorker()
	if err != nil {
		return Config{}, err
	}
	cfg.Worker = worker

	return cfg, nil
}

func loadWorker() (WorkerConfig, error) {
	var w WorkerConfig
	var err error
	if w.Concurrency, err = getInt("WORKER_CONCURRENCY", 10, 1, 1000); err != nil {
		return w, err
	}
	if w.PollInterval, err = getDuration("WORKER_POLL_INTERVAL", time.Second, 10*time.Millisecond, time.Minute); err != nil {
		return w, err
	}
	if w.RequestTimeout, err = getDuration("WORKER_REQUEST_TIMEOUT", 15*time.Second, time.Second, time.Minute); err != nil {
		return w, err
	}
	if w.ShutdownTimeout, err = getDuration("WORKER_SHUTDOWN_TIMEOUT", 20*time.Second, time.Second, 5*time.Minute); err != nil {
		return w, err
	}
	if w.MaxAttempts, err = getInt("WORKER_MAX_ATTEMPTS", 10, 1, 50); err != nil {
		return w, err
	}
	if w.RetryBaseDelay, err = getDuration("WORKER_RETRY_BASE_DELAY", 30*time.Second, 100*time.Millisecond, time.Hour); err != nil {
		return w, err
	}
	if w.RetryMaxDelay, err = getDuration("WORKER_RETRY_MAX_DELAY", 4*time.Hour, 100*time.Millisecond, 7*24*time.Hour); err != nil {
		return w, err
	}
	if w.RetryMaxDelay < w.RetryBaseDelay {
		return w, fmt.Errorf("WORKER_RETRY_MAX_DELAY must not be smaller than WORKER_RETRY_BASE_DELAY")
	}
	return w, nil
}

func getInt(key string, fallback, lo, hi int) (int, error) {
	raw := getenv(key, "")
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < lo || v > hi {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, lo, hi)
	}
	return v, nil
}

func getDuration(key string, fallback, lo, hi time.Duration) (time.Duration, error) {
	raw := getenv(key, "")
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v < lo || v > hi {
		return 0, fmt.Errorf("%s must be a duration between %s and %s", key, lo, hi)
	}
	return v, nil
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
