# Changelog

All notable changes are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/). Before 1.0, minor versions may
include breaking changes, which are called out.

## [Unreleased]

### Changed

- The dashboard uses Inter (bundled, no external requests) and a single neutral palette: the dark theme now mirrors the light one, there is no accent colour apart from state colours, and the logo mark is replaced by the plain "Hooklane" wordmark.

### Added

- Customers and the customer portal: endpoints and events can carry a `customer`; events reach only matching endpoints. `POST /v1/projects/{id}/portal-links` issues a time-limited link (`hlp_` token, stored hashed) that opens a page where that customer manages their own endpoints and sees their deliveries, through `/v1/portal/*` only.
- API key scopes: `admin`, `ingest` (send events only) and `read`, chosen when a key is created (dashboard and API). Existing keys stay `admin`. Revealing endpoint secrets now needs `admin`.
- `LOG_FORMAT=pretty|json`: coloured one-line logs by default in development (JSON elsewhere).
- The dashboard uses the full width of wide screens, and an empty list no longer draws a box inside a box.
- The dashboard is available in English and Turkish (switch in the sidebar or on the sign-in screen; it follows the browser language at first and remembers the choice).
- Lists share one filter bar and pagination: filters, page size and page are kept in the URL, with Previous/Next, the visible range and a rows-per-page selector. Deliveries can be filtered by status and endpoint, endpoints by state and events by type.
- Playground: a guided page in every project that creates an endpoint, sends a test event and shows its deliveries, attempts and the request the receiver got. With `APP_ENV=development` it has a built-in receiver (`/v1/dev/sink/{token}`, which can simulate failures with `?status=`), and the dashboard can create projects.
- A "What is Hooklane for?" section on the Projects page explains the purpose and benefits, and an empty project points to the playground.
- Rate limiting on `/v1`: a token bucket per client address (`RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`; `0` disables) that answers `429` with `Retry-After`.
- Redesigned dashboard: card layout, icon navigation, breadcrumbs, a delivery health overview with recent deliveries, and a light/dark theme switch.
- `make check` runs gofmt, golangci-lint, the TypeScript typecheck, oxlint, all tests and govulncheck; CI runs the same checks.
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
