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
  internal/transport/http           Fiber HTTP API
  migrations/                       Goose SQL migrations
  queries/                          sqlc queries
web/                                React + TypeScript + Vite dashboard
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
| `GET` | `/v1/projects/{id}/endpoints` | List endpoints |
| `GET` / `PATCH` | `/v1/endpoints/{id}` | Read, change `url`, or toggle `enabled` |
| `GET` | `/v1/endpoints/{id}/secret` | Retrieve the signing secret |
| `POST` | `/v1/projects/{id}/events` | Accept an event |
| `GET` | `/v1/projects/{id}/events` | List events (newest first, `limit` ≤ 100) |
| `GET` | `/v1/events/{id}` | Read an event |
| `GET` | `/v1/events/{id}/deliveries` | List an event's deliveries, including replays |
| `POST` | `/v1/deliveries/{id}/replay` | Queue a new delivery of a succeeded or dead-lettered delivery |

Endpoint URLs must use `https`; `http` is accepted only when `APP_ENV=development`. Destination address filtering (SSRF) arrives in Phase 6.

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
