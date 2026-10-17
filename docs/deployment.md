# Deploying Hooklane

Hooklane runs as two long-lived processes that share one PostgreSQL database:

- **API** (`hooklane-api`): the management API, event ingestion and, when
  `WEB_DIR` is set, the dashboard. Stateless; run as many as you need behind
  a load balancer.
- **Worker** (`hooklane-worker`): claims due deliveries, sends them, records
  attempts and prunes old events. Run one or more; workers coordinate through
  row locks (`FOR UPDATE SKIP LOCKED`) and leases, so adding workers adds
  throughput without double delivery.

`hooklane-admin` applies migrations, creates the first organization and
prunes on demand. All three ship in one image,
`ghcr.io/0furkancolak/hooklane:<version>`, and as release binaries.

## Requirements

- PostgreSQL. Version 17 is the one CI and development test against; the
  schema uses only features available since PostgreSQL 13
  (`gen_random_uuid()`, partial indexes, `FOR UPDATE SKIP LOCKED`).
- A 32-byte key for encrypting endpoint secrets:
  `openssl rand -base64 32`. Keep it with your other secrets and back it up:
  without it, stored endpoint secrets cannot be decrypted and deliveries to
  those endpoints dead-letter.
- TLS termination in front of the API (a reverse proxy or load balancer).
  The API speaks plain HTTP.

## First deployment

1. Create a database and a role that owns it.
2. Apply migrations:
   `DATABASE_URL=... hooklane-admin migrate up`
3. Create the first organization, project and API key:
   `DATABASE_URL=... hooklane-admin bootstrap -org-name Acme -org-slug acme`
   The API key is printed once.
4. Start the API and at least one worker with the variables below.
5. Point your probes at `/healthz` (liveness) and `/readyz` (readiness).

The Compose file's `app` profile does exactly this for a single host:
`docker compose --profile app up -d --build`.

## Upgrading

1. Read the CHANGELOG entry for the release.
2. Run `hooklane-admin migrate up` with the new version. Migrations are
   additive and the previous version keeps working against the new schema
   unless the CHANGELOG says otherwise.
3. Roll the API and the workers. Workers drain in-flight deliveries for up to
   `WORKER_SHUTDOWN_TIMEOUT` on `SIGTERM`; anything still running is
   cancelled and retried by another worker after its lease expires.

`hooklane-admin migrate status` lists applied and pending migrations.

## Configuration

Every setting is an environment variable. Invalid values stop the process at
startup with a message naming the variable.

### Common

| Variable | Default | Used by | Meaning |
| --- | --- | --- | --- |
| `DATABASE_URL` | (required) | all | PostgreSQL connection URL. Use `sslmode=require` or stricter across a network. |
| `APP_ENV` | `development` | all | `production`, `development` or `test`. `development` allows `http://` endpoints and loopback/private destinations; never use it in production. |
| `ENDPOINT_SECRET_KEY` | (required by API and worker) | api, worker | Base64 of 32 random bytes; encrypts endpoint signing secrets (AES-256-GCM). |
| `METRICS_ADDR` | (off) | api, worker | Separate listener for `/metrics`, `/healthz`, `/readyz`, e.g. `0.0.0.0:9101`. Do not expose it publicly. |
| `RETENTION_PERIOD` | `720h` | worker, admin | Delete finished events older than this; `0` disables. Minimum `1h`. |

### API

| Variable | Default | Meaning |
| --- | --- | --- |
| `PORT` | `8080` | HTTP port. |
| `WEB_DIR` | (off; `/app/web` in the image) | Directory with the built dashboard to serve at `/`. |

### Worker

| Variable | Default | Range | Meaning |
| --- | --- | --- | --- |
| `WORKER_CONCURRENCY` | `10` | 1–1000 | Deliveries in flight per worker. |
| `WORKER_POLL_INTERVAL` | `1s` | 10ms–1m | Wait between claims when the queue is empty. |
| `WORKER_REQUEST_TIMEOUT` | `15s` | 1s–1m | Limit for one outbound request. The lease is this plus 30s. |
| `WORKER_SHUTDOWN_TIMEOUT` | `20s` | 1s–5m | Drain time after `SIGTERM`. |
| `WORKER_MAX_ATTEMPTS` | `10` | 1–50 | Attempts before a delivery is dead-lettered. |
| `WORKER_RETRY_BASE_DELAY` | `30s` | 100ms–1h | Upper bound of the first retry delay. |
| `WORKER_RETRY_MAX_DELAY` | `4h` | 100ms–7d | Cap for any retry delay, including `Retry-After`. |

### Development only

| Variable | Meaning |
| --- | --- |
| `HOOKLANE_API_URL` | Where the dashboard's Vite dev server proxies API calls (default `http://127.0.0.1:8080`). |
| `HOOKLANE_PORT`, `HOOKLANE_APP_ENV` | Host port and `APP_ENV` for the Compose `app` profile. |
| `POSTGRES_PORT`, `POSTGRES_PASSWORD` | Compose PostgreSQL settings. |

## Network and security checklist

- Terminate TLS in front of the API; it does not redirect HTTP to HTTPS.
- Keep `METRICS_ADDR` on a private interface; it is unauthenticated.
- Give the worker outbound internet access but, where your platform allows,
  also block internal ranges at the network level. Hooklane refuses private,
  loopback, link-local (including cloud metadata) and reserved addresses on
  every connection; a firewall adds a second layer.
- Do not route webhook traffic through a proxy: the worker ignores
  `HTTP_PROXY` on purpose so the address checks see the real destination.
- Serve the dashboard only from an origin you control. It keeps the API key
  in `sessionStorage`, and the API sends a strict Content Security Policy
  with it.

## Sizing and operations

- Throughput is roughly `workers × WORKER_CONCURRENCY / average request time`.
  Watch `hooklane_queue_oldest_due_seconds`: if it keeps growing, add workers
  or concurrency.
- Each process opens a pgx pool (default size: max(4, CPUs)). Make sure
  PostgreSQL's `max_connections` covers all processes.
- Back up PostgreSQL as you would any primary database. Everything Hooklane
  knows lives there, except `ENDPOINT_SECRET_KEY`.
- Logs are JSON on stdout. They never contain API keys, endpoint secrets or
  payloads.
