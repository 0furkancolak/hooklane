package config

import (
	"testing"
	"time"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("APP_ENV", "development")
	t.Setenv("PORT", "8080")

	if _, err := Load(); err == nil {
		t.Fatal("expected DATABASE_URL validation error")
	}
}

func TestLoadRejectsUnknownAppEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://hooklane:dev@localhost:5432/hooklane?sslmode=disable")
	t.Setenv("APP_ENV", "staging")

	if _, err := Load(); err == nil {
		t.Fatal("expected APP_ENV validation error")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://hooklane:dev@localhost:5432/hooklane?sslmode=disable")
	t.Setenv("APP_ENV", "")
	t.Setenv("PORT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppEnv != AppEnvDevelopment {
		t.Fatalf("app env = %q", cfg.AppEnv)
	}
	if cfg.Port != "8080" {
		t.Fatalf("port = %q", cfg.Port)
	}
}

func TestLoadEndpointSecretKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://hooklane:dev@localhost:5432/hooklane?sslmode=disable")
	t.Setenv("APP_ENV", "")

	t.Setenv("ENDPOINT_SECRET_KEY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EndpointSecretKey != nil {
		t.Fatal("expected no key when unset")
	}

	t.Setenv("ENDPOINT_SECRET_KEY", "c2hvcnQ=")
	if _, err := Load(); err == nil {
		t.Fatal("expected short key to be rejected")
	}

	t.Setenv("ENDPOINT_SECRET_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.EndpointSecretKey) != 32 {
		t.Fatalf("key length = %d", len(cfg.EndpointSecretKey))
	}
}

func TestLoadWorkerConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://hooklane:dev@localhost:5432/hooklane?sslmode=disable")
	t.Setenv("APP_ENV", "")
	t.Setenv("WORKER_CONCURRENCY", "")
	t.Setenv("WORKER_REQUEST_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worker.Concurrency != 10 || cfg.Worker.RequestTimeout != 15*time.Second {
		t.Fatalf("defaults = %+v", cfg.Worker)
	}
	if cfg.Worker.Lease() <= cfg.Worker.RequestTimeout {
		t.Fatal("lease must exceed request timeout")
	}

	t.Setenv("WORKER_REQUEST_TIMEOUT", "5s")
	if cfg, err = Load(); err != nil || cfg.Worker.RequestTimeout != 5*time.Second {
		t.Fatalf("override: %v %v", cfg.Worker.RequestTimeout, err)
	}
	for key, value := range map[string]string{
		"WORKER_CONCURRENCY":     "0",
		"WORKER_REQUEST_TIMEOUT": "2h",
		"WORKER_POLL_INTERVAL":   "soon",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			if _, err := Load(); err == nil {
				t.Fatalf("%s=%s should be rejected", key, value)
			}
		})
	}
}

func TestLoadRateLimit(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://hooklane:dev@localhost:5432/hooklane?sslmode=disable")

	t.Setenv("RATE_LIMIT_RPS", "")
	t.Setenv("RATE_LIMIT_BURST", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RateLimit.RequestsPerSecond != 50 || cfg.RateLimit.Burst != 200 {
		t.Fatalf("defaults = %+v", cfg.RateLimit)
	}

	t.Setenv("RATE_LIMIT_RPS", "0")
	if cfg, err = Load(); err != nil || cfg.RateLimit.RequestsPerSecond != 0 {
		t.Fatalf("0 should disable limiting: %+v, %v", cfg.RateLimit, err)
	}

	for _, bad := range []string{"-1", "fast", "1e9"} {
		t.Setenv("RATE_LIMIT_RPS", bad)
		if _, err := Load(); err == nil {
			t.Fatalf("RATE_LIMIT_RPS=%q was accepted", bad)
		}
	}

	t.Setenv("RATE_LIMIT_RPS", "10")
	t.Setenv("RATE_LIMIT_BURST", "0")
	if _, err := Load(); err == nil {
		t.Fatal("RATE_LIMIT_BURST=0 was accepted")
	}
}
