# Hooklane

Open-source, self-hostable webhook delivery platform.

## Repository layout

```
api/                                Go module (github.com/0furkancolak/hooklane/api)
  cmd/api                           HTTP API process
  cmd/worker                        Delivery worker process
  cmd/admin                         Administrator CLI
  internal/config                   Environment loading
  internal/domain                   Domain types
  internal/application              Use cases, worker, repository interfaces
  internal/infrastructure/database  PostgreSQL repositories and sqlc output (db/)
  internal/infrastructure/webhook   Webhook signing and outbound HTTP
  internal/infrastructure/metrics   Prometheus metrics and the ops listener
  internal/transport/http           Fiber HTTP API
  migrations/                       Goose SQL migrations
  queries/                          sqlc queries
web/                                React + TypeScript + Vite dashboard
  src/app                           Shell, layout and routing
  src/features                      One folder per area: auth, projects, endpoints, events, deliveries
  src/shared                        API client, session, router, UI primitives
docs/                               Architecture and phase plans
```

Go commands run inside `api/`; npm commands run inside `web/`. The Makefile wraps both.

## Local development

Copy `.env.example` to `.env`, set `ENDPOINT_SECRET_KEY` (`openssl rand -base64 32`), and load it into your shell with `set -a; . ./.env; set +a`.

PostgreSQL listens on `127.0.0.1:5433` by default; override with `POSTGRES_PORT`.

```sh
docker compose up -d postgres
make migrate-up
make sqlc-up
(cd web && npm install)
make test
make test-integration   # uses a separate hooklane_test database
make dev-api
make dev-worker
```

In another terminal:

```sh
make dev-web
```

The dev server proxies `/v1`, `/healthz` and `/readyz` to `http://127.0.0.1:8080`; set `HOOKLANE_API_URL` when the API runs elsewhere (for example `HOOKLANE_API_URL=http://127.0.0.1:18080 make dev-web`).

Bootstrap the first organization, project, and API key:

```sh
export DATABASE_URL=postgres://hooklane:local_dev_only_change_me@localhost:5433/hooklane?sslmode=disable
cd api && go run ./cmd/admin bootstrap -org-name Acme -org-slug acme -project-name default
```

The CLI prints the raw API key once. Use it as `Authorization: Bearer <key>` against `/v1`.

## API overview

All `/v1` routes require `Authorization: Bearer <key>`. Access is scoped to the key's organization.

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/v1/projects/{id}/endpoints` | Register an endpoint. The response includes the signing `secret`. |
| `GET` | `/v1/projects/{id}/endpoints` | List endpoints (`?enabled=true\|false`) |
| `GET` / `PATCH` | `/v1/endpoints/{id}` | Read, change `url`, or toggle `enabled` |
| `GET` | `/v1/endpoints/{id}/secret` | Retrieve the signing secret |
| `POST` | `/v1/projects/{id}/events` | Accept an event |
| `GET` | `/v1/projects/{id}/events` | List events (`?event_type=`) |
| `GET` | `/v1/events/{id}` | Read an event |
| `GET` | `/v1/events/{id}/deliveries` | List an event's deliveries, including replays |
| `GET` | `/v1/projects/{id}/deliveries` | List deliveries (`?status=`, `?endpoint_id=`) |
| `GET` | `/v1/projects/{id}/delivery-stats` | Delivery counts by status and the oldest overdue delivery |
| `GET` | `/v1/deliveries/{id}` | Read a delivery |
| `GET` | `/v1/deliveries/{id}/attempts` | Attempt history: status code, stored response body (first 4 KiB), error, duration |
| `POST` | `/v1/deliveries/{id}/replay` | Queue a new delivery of a succeeded or dead-lettered delivery |

List routes are newest first and take `limit` (1–100, default 50) and `offset`. Responses look like `{"events": [...], "limit": 50, "offset": 0, "next_offset": 50}`; `next_offset` is `null` on the last page. Every response carries an `X-Request-ID` header; a valid incoming `X-Request-ID` (up to 64 characters from `A-Z a-z 0-9 . _ -`) is kept, otherwise one is generated.

Endpoint URLs must use `https`; `http` is accepted only when `APP_ENV=development`. URLs that point at blocked addresses are rejected with `400 destination_not_allowed` (see [Security](#security)).

### Accepting events

```sh
curl -X POST localhost:8080/v1/projects/$PROJECT_ID/events \
  -H "Authorization: Bearer $KEY" \
  -H "Idempotency-Key: order-42-created" \
  -H "Content-Type: application/json" \
  -d '{"event_type":"order.created","payload":{"id":42}}'
```

- `payload` must be a JSON object of at most 256 KiB. Request bodies are capped at 1 MiB.
- `event_type` matches `^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`.
- The event and one `pending` delivery per enabled endpoint are written in one transaction; the deliveries are the queue the worker consumes ([ADR 0001](docs/adr/0001-deliveries-as-outbox.md)). The response is `202 Accepted` with `delivery_count`.

### Idempotency

`Idempotency-Key` is optional: up to 255 printable ASCII characters, scoped per project.

- Same key, same `event_type` and payload: `200 OK` with the original event and `Idempotent-Replayed: true`. Nothing new is written. Payloads compare as JSON, so key order and whitespace do not matter.
- Same key, different `event_type` or payload: `409` with code `idempotency_conflict`.
- Concurrent requests with the same key create exactly one event.
- Keys stay reserved for as long as the event is retained.

### Delivery, retries and replay

The worker POSTs `{"id", "type", "timestamp", "data"}` signed with Standard Webhooks headers (`webhook-id` is the event ID, so receivers can deduplicate). Delivery is at-least-once.

| Outcome | Result |
| --- | --- |
| `2xx` | `succeeded` |
| Network error, timeout, `408`, `425`, `429`, `5xx` | Retried |
| Blocked destination address | `dead_letter` immediately |
| Any other status, including `3xx` (redirects are not followed) | `dead_letter` immediately |

After the n-th failed attempt the next attempt waits a random duration in `[d/2, d]`, where `d = min(WORKER_RETRY_MAX_DELAY, WORKER_RETRY_BASE_DELAY × 2^(n-1))`. A `Retry-After` header can lengthen the wait up to the maximum. After `WORKER_MAX_ATTEMPTS` failed attempts the delivery becomes `dead_letter`. With the defaults (10 attempts, 30s base, 4h cap) the last attempt happens roughly 2 to 4.3 hours after the first.

Replaying creates a new `pending` delivery with `replay_of` set and a fresh retry budget. The original delivery and its attempts are never modified. Only one unfinished replay per delivery is allowed (`409 replay_in_progress`). Deliveries for disabled endpoints wait until the endpoint is re-enabled.

| Worker variable | Default | Meaning |
| --- | --- | --- |
| `WORKER_CONCURRENCY` | `10` | Deliveries in flight |
| `WORKER_POLL_INTERVAL` | `1s` | Idle wait between claims |
| `WORKER_REQUEST_TIMEOUT` | `15s` | Whole request; the lease is this plus 30s |
| `WORKER_SHUTDOWN_TIMEOUT` | `20s` | Drain time before in-flight requests are cancelled |
| `WORKER_MAX_ATTEMPTS` | `10` | Attempts before dead-lettering |
| `WORKER_RETRY_BASE_DELAY` | `30s` | First retry delay upper bound |
| `WORKER_RETRY_MAX_DELAY` | `4h` | Cap for any single delay |

## Dashboard

The dashboard signs in with an API key and only calls the documented `/v1` API, so it can see exactly what the key's organization can see.

- **Projects and overview**: delivery counts by status, with links to filtered lists, and a warning when due deliveries are waiting for a worker.
- **Endpoints**: add an endpoint (the signing secret is shown once, with a copy button), edit its URL, enable or disable it, and reveal the secret on request. Blocked destinations are explained.
- **Events**: filter by type, then open an event to see its payload and every delivery, including replays.
- **Deliveries**: filter by status or endpoint (filters live in the URL, so views can be shared), page through history, and open a delivery to see each attempt's status code, response excerpt, error and duration.
- **Replay**: available only for succeeded or dead-lettered deliveries, asks for confirmation, and opens the new delivery. A `409` (for example, a replay already in progress) is shown inline.

The key is kept in the tab's `sessionStorage`. It is dropped when the tab closes, on sign-out, or as soon as the API answers `401` (for example, after the key is revoked). It is never put in `localStorage`, cookies or URLs. Any script running on the dashboard's origin could read it, so serve the dashboard only from an origin you control. API keys have no per-action scopes yet: any key of an organization can replay that organization's deliveries.

`npm test` runs the component tests (Vitest, Testing Library, jsdom); `npm run build` type-checks and builds `web/dist`.

## Operations

### Health and readiness

| Path | Meaning |
| --- | --- |
| `GET /healthz` | The process is running. Never checks dependencies, so it is safe for liveness probes. |
| `GET /readyz` | The process can do useful work: the database answers a ping within 2s. Returns `503 {"status":"unavailable"}` otherwise. A worker also reports unavailable once shutdown has begun. |

The API serves both on its main port. With `METRICS_ADDR` set (for example `127.0.0.1:9101`), the API and the worker also serve `/healthz`, `/readyz` and `/metrics` on that separate listener. The worker has no other HTTP port, so set `METRICS_ADDR` to probe it. Metrics are never served on the public API port.

### Metrics

Prometheus text format at `/metrics` on the `METRICS_ADDR` listener.

| Metric | Type | Meaning |
| --- | --- | --- |
| `hooklane_http_requests_total{method,route,status}` | counter | API requests, labelled by route pattern (`/v1/events/:id`), never by ID |
| `hooklane_http_request_duration_seconds{method,route}` | histogram | API latency |
| `hooklane_delivery_attempts_total{result,response}` | counter | Recorded attempts. `result` is `succeeded`, `retry_scheduled` or `dead_letter`; `response` is `2xx`…`5xx` or `transport_error` |
| `hooklane_delivery_attempt_duration_seconds{result}` | histogram | Outbound request time |
| `hooklane_worker_deliveries_in_flight` | gauge | Deliveries being sent by this worker |
| `hooklane_worker_claimed_deliveries_total` | counter | Deliveries claimed by this worker |
| `hooklane_worker_claim_errors_total`, `hooklane_worker_record_errors_total` | counter | Database failures while claiming or recording |
| `hooklane_worker_lease_lost_total` | counter | Results discarded because the lease expired (raise `WORKER_REQUEST_TIMEOUT` margin or check database latency) |
| `hooklane_queue_due_deliveries` | gauge | Pending deliveries due now (instance-wide, read from the database at scrape time) |
| `hooklane_queue_scheduled_deliveries` | gauge | Pending deliveries waiting for a retry |
| `hooklane_queue_held_deliveries` | gauge | Pending deliveries of disabled endpoints |
| `hooklane_queue_processing_deliveries` | gauge | Claimed deliveries |
| `hooklane_queue_oldest_due_seconds` | gauge | How long the oldest due delivery has waited; alert when it keeps growing |
| `hooklane_queue_backlog_query_up` | gauge | `0` when the backlog query failed |

Go runtime and process metrics are included. Queue gauges are global, so scrape them from one worker or aggregate with `max`.

Suggested alerts: `hooklane_queue_oldest_due_seconds > 300` for 10 minutes (workers are behind or down); a rising rate of `result="dead_letter"`; any increase in `hooklane_worker_lease_lost_total`.

### Logs

Both processes write JSON logs to stdout. The API writes one `request` line per request with `request_id`, `method`, `route`, `path`, `status`, `duration_ms` and, once authenticated, `api_key_id` and `organization_id`. It does not log headers, query strings or bodies. The worker logs each attempt with `delivery_id`, `event_id`, `endpoint_id`, `attempt`, `status`, `status_code` and `duration_ms`; endpoint URLs, secrets and payloads are never logged.

### Diagnosing a delivery

1. `GET /v1/projects/{id}/delivery-stats` shows whether deliveries are piling up (`pending`, `oldest_due_at`) or failing (`dead_letter`).
2. `GET /v1/projects/{id}/deliveries?status=dead_letter` lists the failures.
3. `GET /v1/deliveries/{id}/attempts` shows each attempt's status code, response excerpt and error.
4. Fix the receiver, then `POST /v1/deliveries/{id}/replay`.

## Security

Endpoint URLs are chosen by tenants, so the worker treats every destination as untrusted.

**Destination addresses.** Outside development, deliveries never connect to:

- loopback, private (RFC 1918) and unique local IPv6 addresses
- link-local addresses, which include the AWS, GCP and Azure metadata service (`169.254.169.254`, `fd00:ec2::254`)
- carrier-grade NAT (`100.64.0.0/10`, which includes Alibaba Cloud metadata), "this network", multicast, broadcast, documentation, benchmarking and other reserved ranges
- IPv6 forms that embed one of the above IPv4 addresses (IPv4-mapped, NAT64 `64:ff9b::/96`, 6to4 `2002::/16`)

`APP_ENV=development` additionally allows loopback and private networks, so local receivers work. Link-local, metadata and reserved ranges stay blocked there too.

**Where it is enforced.** The check runs on the address of every outbound socket, after DNS resolution and immediately before connecting (`net.Dialer.Control`). A hostname that resolves to a blocked address, or that rebinds after the URL was saved, is therefore still refused, and so are endpoints saved before the check existed. Saving an endpoint also rejects literal blocked addresses and `localhost` names early, with a clear error. A blocked delivery is dead-lettered at once, with the reason recorded on the attempt.

**Redirects and proxies.** Redirects are never followed; a `3xx` is a permanent failure. `HTTP_PROXY` and similar variables are ignored for webhook traffic.

**Limits.** Request bodies are capped at 1 MiB and event payloads at 256 KiB. Only the first 4 KiB of a response body is read and stored. Outbound requests have connect, TLS, header and total timeouts. The API applies read, write and idle timeouts.

**Credentials and logs.** API keys are stored as SHA-256 hashes, and malformed keys are rejected before any database lookup. Endpoint secrets are encrypted at rest with AES-256-GCM and returned only by the create and `/secret` routes. Request logs do not include headers, payloads or secrets. Attempt errors store the destination without its query string, because receivers sometimes put tokens there. Every API response carries `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.

**Tenant isolation.** The tenant is always taken from the API key, never from the request. Every route that takes a resource ID returns `403` for resources owned by another organization. A test exercises each of these routes.

**Assumptions and limits.**
- Run the worker with egress that does not depend on Hooklane's checks alone where possible, for example a network policy or firewall that also blocks internal ranges.
- Ports are not restricted; receivers often use non-standard ports, and internal services are already unreachable through the address checks.
- A public address that forwards to internal systems (such as an open reverse proxy you operate) is outside what address filtering can detect.
