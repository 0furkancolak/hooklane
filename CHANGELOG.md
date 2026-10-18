# Changelog

All notable changes are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/). Before 1.0, minor versions may
include breaking changes, which are called out.

## [Unreleased]

### Added

- `make dev` starts PostgreSQL, migrations, the API, the worker and the dashboard with one command, and bootstraps a development API key on an empty database.
- Dashboard API keys page: generate a key for a project (shown once, with a copy button), see each key's prefix and status, and revoke it.
- Sign-in screen with a feature overview and API status. With `APP_ENV=development` it can generate a development key (loopback requests only, `POST /v1/dev/api-keys`), so a database that was bootstrapped earlier no longer needs the admin CLI.

## [0.1.0] - 2026-10-17

First public release.

### Added
- Organizations, projects and hashed API keys; `hooklane-admin bootstrap`
  creates the first set.
- Endpoints with encrypted signing secrets; enable, disable, change URL.
- Event ingestion with idempotency keys; event and deliveries are written in
  one transaction (deliveries are the outbox).
- Worker with `FOR UPDATE SKIP LOCKED` claiming, leases, graceful shutdown and
  Standard Webhooks signatures.
- Retries with jittered exponential backoff and `Retry-After`, dead letters
  and manual replay.
- SSRF protection on every connection (private, loopback, link-local,
  metadata and reserved ranges; DNS rebinding; no redirects or proxies).
- Query API for deliveries, attempts and delivery stats; filters and bounded
  pagination.
- Health and readiness probes, Prometheus metrics, JSON logs with request IDs.
- React dashboard served by the API.
- Retention: finished events older than `RETENTION_PERIOD` (30 days by
  default) are deleted; `hooklane-admin prune` prunes on demand.
- `hooklane-admin migrate` with embedded migrations; one container image for
  all processes; OpenAPI description at `/openapi.yaml`.
- Example receivers in Go and Node.js.

[Unreleased]: https://github.com/0furkancolak/hooklane/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/0furkancolak/hooklane/releases/tag/v0.1.0
