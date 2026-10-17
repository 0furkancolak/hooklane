# Hooklane Phase Plan

## Working Rules

- Work strictly in the Hooklane repository
- Complete one phase at a time
- Inspect existing files before modifying them
- Prefer small, verifiable changes over broad rewrites
- Run relevant tests and builds before declaring a phase complete
- Never silently overwrite existing work
- Never delete database volumes or production data as a troubleshooting shortcut
- Update this document when phase status changes
- Do not begin the next phase until the current phase passes its acceptance criteria

## Phase 0: Repository and Development Infrastructure

### Scope
- Establish repository structure
- Configure Go module and dependencies
- Add API, worker and admin entrypoints
- Configure Fiber v3
- Add PostgreSQL Docker Compose configuration
- Add environment configuration and validation foundations
- Add Goose migration setup
- Add sqlc configuration and initial generated code
- Create React, TypeScript and Vite application under web/
- Add Makefile and GitHub Actions CI
- Document local development setup

### Acceptance Criteria
- PostgreSQL starts through Docker Compose
- Database readiness can be checked
- Go API, worker and admin packages build
- Frontend production build succeeds
- sqlc generation succeeds
- CI configuration is present
- Secrets are not committed

## Phase 1: Domain Model and Database Foundation

### Scope
- Define organization, project, endpoint, event and delivery domain types
- Implement initial PostgreSQL schema
- Add appropriate constraints and indexes
- Add Goose migration up/down paths
- Generate typed database code with sqlc
- Define repository boundaries
- Validate transaction and tenant ownership assumptions

### Acceptance Criteria
- Migrations apply to a clean development database
- Migration status is inspectable
- Schema matches the architecture plan
- sqlc generation succeeds
- Go tests and builds pass
- Existing database data is preserved during troubleshooting

## Phase 2: Organizations, Projects, API Keys and Bootstrap

### Scope
- Implement CLI-based first administrator bootstrap
- Implement organization and project persistence
- Generate API keys securely
- Store API key hashes only
- Implement API key authentication
- Implement revocation and authorization checks
- Add project management API
- Add unit and integration tests

### Acceptance Criteria
- First administrator can be initialized through the CLI
- Raw API keys are shown only at creation when appropriate
- Raw API keys are never stored or logged
- Invalid and revoked keys are rejected
- Cross-tenant access tests pass

## Phase 3: Endpoints, Events, Idempotency and Transactional Outbox

### Scope
- Create and manage webhook endpoints
- Accept events through the API
- Validate event payloads and request sizes
- Implement idempotency semantics
- Persist events and delivery records transactionally; deliveries act as the
  outbox (ADR 0001)
- Add event retrieval and query paths

### Acceptance Criteria
- Duplicate idempotency keys behave deterministically
- Event and delivery persistence are atomic
- Partial transaction failures do not leave inconsistent records
- Endpoint ownership is enforced
- Tests cover duplicate and concurrent ingestion

## Phase 4: Worker and HTTP Delivery

### Scope
- Implement worker polling and claiming
- Coordinate workers safely through PostgreSQL
- Use leases or equivalent bounded claims
- Send outbound HTTP requests
- Implement signed webhook requests
- Apply connection, request and response timeouts
- Record delivery attempts and results
- Support graceful shutdown

### Acceptance Criteria
- Multiple workers do not concurrently own the same active lease
- Signatures can be verified by a compatible receiver
- Timeouts and network failures are recorded safely
- Worker crashes do not permanently strand recoverable work
- Payload and response handling are bounded

## Phase 5: Retries, Dead Letters and Replay

### Scope
- Implement retry classification
- Add exponential backoff with jitter
- Persist next-attempt scheduling
- Move exhausted deliveries to dead-letter state
- Implement manual replay
- Preserve original delivery and attempt history

### Acceptance Criteria
- Retry schedules are deterministic within documented bounds
- Non-retryable failures follow documented policy
- Exhausted deliveries reach dead-letter state
- Replay creates new delivery records
- Historical attempts remain unchanged

## Phase 6: Security Hardening

### Scope
- Implement production SSRF defenses
- Restrict destination IP ranges
- Defend against DNS rebinding and redirects
- Restrict local HTTP to explicit development mode
- Harden authentication and authorization
- Redact secrets and sensitive data
- Validate payload and response limits
- Add security-focused integration tests

### Acceptance Criteria
- Production requests cannot reach prohibited address ranges
- Redirects cannot bypass destination validation
- Tenant isolation tests pass
- API keys and signature secrets do not appear in logs
- Security assumptions are documented

## Phase 7: Query API and Observability

### Scope
- Add endpoint, event, delivery and attempt query APIs
- Add filtering and pagination
- Add health and readiness checks
- Add structured logs and request correlation
- Add worker and delivery metrics
- Add operational diagnostics

### Acceptance Criteria
- Query responses are tenant-scoped
- Pagination is bounded
- Health and readiness behavior is documented
- Metrics expose meaningful delivery and worker signals
- Logs avoid sensitive payloads and credentials

## Phase 8: Dashboard, Resilience and Quality

### Scope
- Build feature-based React dashboard
- Implement authentication and project navigation
- Add endpoint management
- Add event and delivery history views
- Add attempt inspection and replay controls
- Add loading, error and empty states
- Add frontend and end-to-end tests where practical
- Test failure recovery and concurrency behavior

### Acceptance Criteria
- Dashboard flows use the documented API
- Tenant-scoped views cannot expose other tenants' data
- Delivery history and attempt status are understandable
- Replay actions require appropriate authorization
- Production frontend build and automated checks pass

## Phase 9: Release and Open Source Readiness

### Scope
- Complete README and quick-start guide
- Document environment variables and deployment
- Publish OpenAPI documentation
- Add example webhook receiver and signature verification
- Document retries, retention, delivery guarantees and security
- Add license and contribution guidelines
- Verify clean installation from documented instructions
- Prepare versioning and release workflow

### Acceptance Criteria
- A new developer can run the application using the documentation
- Migrations and initial setup are reproducible
- CI passes
- Security limitations and operational requirements are documented
- Release checklist is complete

## Phase Status

- [x] Phase plan approved
- [x] Phase 0 complete and verified
- [x] Phase 1 complete and verified
- [x] Phase 2 complete and verified
- [x] Phase 3 complete and verified
- [x] Phase 4 complete and verified
- [x] Phase 5 complete and verified
- [x] Phase 6 complete and verified
- [x] Phase 7 complete and verified
- [x] Phase 8 complete and verified
- [x] Phase 9 complete and verified

## Phase 2 Verification Notes (2026-10-17)

Verified against a clean local PostgreSQL (compose, port 5433):
- Migrations applied from empty database; `make migrate-status` shows both applied
- `admin bootstrap` creates organization, project and key; a second run fails with "already bootstrapped"
- Raw key printed once by the CLI; `api_keys` holds only prefix and 32-byte SHA-256 hash
- Key listings do not return raw keys or hashes; API logs contain no raw keys
- Missing, malformed (non-Bearer), unknown and revoked keys return 401
- Project read, key list and key create on another organization's project return 403

Known gaps carried forward:
- No automated Postgres integration tests yet; tenant tests use in-memory repositories
- `/readyz` does not check database connectivity (Phase 7)

## Phase 3 Verification Notes (2026-10-17)

No schema change was needed; the initial schema already carried endpoints,
events (partial unique index on project_id + idempotency_key), deliveries and
outbox_events.

Decisions:
- Endpoint secrets are `whsec_` + base64(24 random bytes), encrypted with
  AES-256-GCM using `ENDPOINT_SECRET_KEY`. The API refuses to start without it;
  the admin CLI does not need it
- Endpoints are disabled through `PATCH`, not deleted, because deleting an
  endpoint cascades to its delivery history
- An event fans out to every enabled endpoint of its project; per-endpoint
  event-type filtering is not implemented
- Idempotency uses `INSERT ... ON CONFLICT DO NOTHING` and then reads the
  existing row in the same transaction; replay matching compares `jsonb` values
- `outbox_events` has no foreign key to `events` (generic aggregate columns), so
  outbox rows do not cascade on event or tenant deletion and need their own
  cleanup once retention is implemented

Verified:
- Unit tests (in-memory store with rollback) cover validation, fan-out,
  idempotent replay and conflict, outbox-failure rollback and tenant isolation
- `make test-integration` runs against a separate `hooklane_test` database and
  covers atomic writes, rollback on outbox failure, jsonb-semantic replay,
  per-project key scope, 24 concurrent same-key requests creating one event and
  concurrent keyless ingestion; stable across 10 repeated runs with `-race`
- Manual smoke test against the running API in production mode: http endpoints
  rejected, replay returns `200` with `Idempotent-Replayed: true`, conflicting
  reuse returns `409`, 1.1 MB body returns JSON `413`, no secrets in logs
- CI gains an `integration` job with a PostgreSQL service

## Repository Restructure (2026-10-17)

The npm-workspace monorepo (apps/, packages/, Go module at the root) was
replaced with the layout used by sibling projects: a Go module in api/ with
cmd/ and internal/, and a standalone web/ application. The ports package was
merged into application/; Goose and sqlc are unchanged. Unit and integration
tests, sqlc output, web lint/build and the api Docker image were verified after
the move.

## Phase 4 Verification Notes (2026-10-17)

Implementation:
- Migration 20261017120000 adds `deliveries.lease_token` and a check that
  `processing` rows always carry a lease token and expiry
- Workers claim with `FOR UPDATE SKIP LOCKED`, reclaim expired leases, skip
  disabled endpoints and future `next_attempt_at`; completion requires the lease
  token, so a worker that lost its lease cannot record a result
- Lease length is `WORKER_REQUEST_TIMEOUT + 30s`, so a live worker never loses a
  lease mid-request; all lease times use the database clock
- Standard Webhooks headers (`webhook-id` = event ID, `webhook-timestamp`,
  `webhook-signature: v1,...`); the body depends only on the event, so retries
  send identical bytes
- Sender: 5s connect and TLS timeouts, overall `WORKER_REQUEST_TIMEOUT`, no
  redirects, no proxy, response body capped at 4 KiB and sanitized for TEXT
- Shutdown stops claiming, lets in-flight deliveries finish for up to
  `WORKER_SHUTDOWN_TIMEOUT`, then cancels them and still records the attempt
- Phase 4 policy: 2xx succeeds, anything else is `failed`; retries are Phase 5

Acceptance evidence:
- Exclusive leases: 12 concurrent claimers over 100 deliveries claimed each
  exactly once; an active lease is not claimable; a stale owner gets
  `ErrLeaseLost` and records no attempt (integration tests, stable over 5 runs
  with `-race`)
- Verifiable signatures: matches the Standard Webhooks specification test
  vector; an independent Python receiver on the development database verified
  every signature
- Timeouts and network failures: unit tests cover header timeout, slow body,
  connection refused and redirects; a 503 is stored with status, body and error
- Crash recovery: `kill -9` during delivery left rows `processing`; a restarted
  worker left them alone until the lease expired, then delivered them
  (`attempt_count = 2`, receiver saw the same `webhook-id` twice, as expected
  for at-least-once delivery)
- Graceful shutdown: SIGTERM with three in-flight requests recorded all three
  as succeeded before the process exited
- No API keys or endpoint secrets appeared in API or worker logs

Open decision:
- [ADR 0001](adr/0001-deliveries-as-outbox.md) was accepted; see below

## ADR 0001 Applied (2026-10-17)

`deliveries` is the transactional outbox. Ingestion no longer writes
`outbox_events`, and migration 20261017140000 drops the table (its down path
recreates it). The Phase 3 notes above describe the state before this change;
the rollback tests now inject a failure while creating deliveries, after the
event row was inserted, and still prove that nothing is left behind.

Verified: unit and integration tests pass with `-race`; the drop migration was
applied, rolled back and reapplied on the test database; the development
database was migrated (its 11 never-read outbox rows were removed) and event
ingestion still returns `202` with the expected delivery.

## Phase 5 Verification Notes (2026-10-17)

Policy (documented in the README):
- Retryable: transport errors, 408, 425, 429 and 5xx. Permanent: other
  statuses, including 3xx because redirects are not followed, and local
  failures such as an undecryptable endpoint secret
- A 2xx counts as success even when reading the response body then fails
- Delay after the n-th failure: uniform in [d/2, d], d = min(max, base·2^(n-1));
  `Retry-After` raises it up to the cap; delays are applied with the database
  clock (`now() + interval` in `CompleteDelivery`)
- Dead letter after `WORKER_MAX_ATTEMPTS` failures or a permanent failure

Schema (migration 20261017150000):
- The `failed` status is removed; Phase 4 `failed` rows become `dead_letter`
  because they were final under the Phase 4 policy
- `deliveries.replay_of` links a replay to the delivery it repeats; the
  `(event_id, endpoint_id)` uniqueness now applies to originals only, and a
  partial unique index allows one unfinished replay per delivery
- Down path restores the old constraints and deletes replay rows

Acceptance evidence:
- Deterministic bounds: unit tests check nominal delays for attempts 1..10, the
  cap, overflow safety, that 2000 samples per attempt stay in [d/2, d] and span
  it, and exact bounds for fixed random values
- Non-retryable policy: classification table test; a 410 receiver is hit
  exactly once in integration and on the development database
- Dead letter: integration test and dev run reach `dead_letter` after exactly
  `MaxAttempts` hits
- Replay: creates a new delivery; eight concurrent replay requests create one;
  a snapshot of the original row and its attempts is identical before and after
  the replay is delivered
- Dev run (base 1s, cap 4s, 3 attempts): retries at 0.81s/1.02s then 1.63s/2.02s
  after the previous attempt, a 429 with `Retry-After: 3` waited 3.04s, and an
  API replay of the dead-lettered delivery succeeded while a duplicate request
  got `409 replay_in_progress`
- Migration verified on the test database: `failed` rows convert, the new check
  rejects `failed`, down and up both apply

## Phase 6 Verification Notes (2026-10-17)

No schema change was needed.

Decisions:
- One `DestinationPolicy` in the application layer is enforced in two places:
  on save (literal addresses and `localhost` names, `400
  destination_not_allowed`) and on every outbound socket through
  `net.Dialer.Control`, which sees the resolved address right before connect.
  The connect-time check is authoritative: it covers DNS rebinding, resolver
  quirks (such as decimal IPv4 hostnames) and endpoints saved before Phase 6
- Development (`APP_ENV=development`) allows loopback and private ranges only;
  link-local, metadata, CGNAT and reserved ranges are blocked in every
  environment. Embedded IPv4 in mapped, NAT64 and 6to4 addresses is judged as
  IPv4
- Redirects stay disabled rather than validated: a redirect is a permanent
  failure, so there is no second destination to check
- A blocked destination is non-retryable and dead-letters on the first attempt
- Attempt errors drop the URL query string; API responses get `no-store` and
  `nosniff`; the API has read/write/idle timeouts; malformed API keys are
  rejected before a database lookup

Acceptance evidence:
- Prohibited ranges: table test of 26 addresses under production and
  development policies; endpoint create and update reject blocked literals and
  localhost names
- Connect-time enforcement: sender tests show a production sender refuses a
  loopback literal and `localhost` (resolved) without the server being hit, and
  a development sender still refuses `169.254.169.254`; an integration test
  runs a production worker against a `localhost` endpoint in Postgres and gets
  one attempt, `dead_letter`, zero receiver hits
- Redirects: a 307 to a second server is not followed and the second server is
  never contacted
- Tenant isolation: one HTTP test calls all 14 ID-taking routes with another
  organization's resources and gets `403` with no writes; earlier memstore and
  Postgres cross-tenant tests still pass
- Secrets in logs: a test captures logs during endpoint creation, secret
  reads, ingestion and a bad key and finds no key, secret or payload value; a
  transport error test confirms the query string is dropped
- Dev run with `APP_ENV=production` on the development database: the API
  returned `400 destination_not_allowed` for loopback, metadata, mapped-private
  and localhost URLs; the worker dead-lettered all four deliveries to
  `127.0.0.1` endpoints after one attempt with `destination address is not
  allowed`, the receiver logged zero hits, and neither log contained the API key

Known gaps carried forward:
- No port restrictions (documented)
- `/readyz` still does not check the database (Phase 7)

## Phase 7 Verification Notes (2026-10-17)

No schema change was needed; the new project-wide delivery queries go through
`endpoints` so the existing `deliveries_endpoint_created_idx` serves them.

Implementation:
- Query API: project deliveries with `status` and `endpoint_id` filters, single
  delivery, attempt history, project delivery stats; `enabled` filter on
  endpoints and `event_type` filter on events
- Pagination: `limit` clamped to 1..100 (default 50), `offset` ≥ 0; lists fetch
  one extra row to return `next_offset` without a count query
- Readiness: `/readyz` pings the database (2s timeout); `/healthz` never checks
  dependencies. Optional `METRICS_ADDR` listener with `/metrics`, `/healthz`
  and `/readyz` for both processes; the worker reports not ready once shutdown
  starts
- Logs: JSON via slog in both processes; an `X-Request-ID` is accepted or
  generated, echoed, and attached to the access log line and to error logs
- Authentication moved from the `/v1` group to each route so rejected requests
  are labelled with their real route pattern; unknown `/v1` paths now return
  `404` instead of `401`
- Metrics: Prometheus client with route-pattern labels, worker attempt
  counters by result and response class, in-flight gauge, lease-lost and
  database error counters, and queue gauges read from the database at scrape
  time. Pending deliveries of disabled endpoints are reported as `held`, not
  due, so a disabled endpoint does not look like a stalled worker

Acceptance evidence:
- Tenant-scoped: the cross-tenant route test now covers 18 routes, including
  the four new delivery routes; service tests check `403` for each new query;
  the Postgres test shows project lists exclude another tenant's deliveries
- Bounded pagination: tests for first, middle, exact-end and last pages,
  clamping of `limit=10000` and negative offsets, and HTTP `next_offset`
- Health and readiness: HTTP tests flip readiness to `503` while `/healthz`
  stays `200`; ops server test covers all three paths; documented in the
  README Operations section
- Metrics: unit tests assert the exported series and labels; the backlog
  collector reports `up=0` and no stale gauges when the query fails; an
  integration test checks due and held counts against Postgres
- Logs: tests assert the access log contains the request ID, route pattern,
  status and key ID, and contains no query string, API key, endpoint secret or
  payload value
- Dev run: an event sent with `X-Request-ID: phase7-check` was traced through
  the access log; the worker's `/metrics` showed attempts split into
  succeeded, retry_scheduled and dead_letter by response class; the dead-letter
  filter, attempt history and stats matched the receiver's behaviour; metrics
  are not served on the public port (`404`); neither log contained the API key.
  The run also exposed that pending deliveries of a disabled endpoint inflated
  `oldest_due_seconds`; they are now counted as held

## Phase 8 Verification Notes (2026-10-17)

Dashboard (`web/`):
- Feature folders (`auth`, `projects`, `endpoints`, `events`, `deliveries`) on
  shared primitives: a typed API client, a hash router (static hosting, no
  server rewrites), an abortable `useAsync` hook, and loading, error and empty
  states with retry
- Sign-in with an API key, which is checked locally against the key format
  and then with one API call; kept in `sessionStorage` and cleared on
  sign-out or any `401`
- Views: projects, overview stats, endpoint management (create with secret
  shown once, edit URL, enable or disable, reveal secret), events with type
  filter and detail (payload and deliveries), deliveries with status and
  endpoint filters in the URL, delivery detail with attempt history and replay
  with confirmation
- No new runtime dependencies (React only); Vitest, Testing Library and jsdom
  are dev dependencies; CI runs `npm test`

Resilience (backend):
- Worker keeps polling through claim failures (simulated database outage) and
  delivers once the queue recovers
- Three concurrent workers over 60 deliveries send each event exactly once
- A claim abandoned by a crashed worker is not taken over while its lease is
  valid and is delivered by another worker after it expires (two claims, one
  request)

Acceptance evidence:
- Documented API only: the client calls only routes listed in the README;
  tests assert paths, query strings and the bearer header
- Tenant scoping: the dashboard has no tenant selector; every view is limited
  to the key's organization by the API, and `403` responses render as "this
  key's organization does not own that resource" (tested)
- Understandable history: status badges, HTTP status colouring, "no response"
  for transport errors, attempt error and response excerpt (tested)
- Replay authorization: replay requires an authenticated key whose
  organization owns the delivery (API-enforced, cross-tenant test); the UI
  offers it only for terminal deliveries, requires confirmation, and shows
  `409` conflicts (tested)
- Checks: 31 component and unit tests, `oxlint`, `tsc -b` and `vite build`
  pass; backend tests pass with `-race`; resilience tests stable over 3 runs
- Browser run against the development API and worker: signed in with a
  temporary key, followed overview to dead-letter list to delivery detail,
  replayed a dead-lettered delivery after bringing its receiver back (the new
  delivery shows `replay of` and succeeded with `200`), saw the destination
  error for `169.254.169.254`, checked a 375px viewport (no horizontal page
  scroll), then revoked the key and saw the dashboard return to sign-in with
  storage cleared. The key never appeared in API or worker logs

Known limits:
- API keys have no scopes, so any key of an organization can replay
- The dashboard is served by Vite in development; production serving is
  part of Phase 9

## Phase 9 Verification Notes (2026-10-17)

Added:
- README rewritten around a Docker quick start, receiving and verifying
  webhooks, delivery guarantees, retention, API, dashboard, operations and
  security; [deployment.md](deployment.md) documents every environment
  variable with defaults and ranges, upgrades, sizing and a security checklist
- OpenAPI 3.1 description (`api/openapi/openapi.yaml`), embedded and served at
  `/openapi.yaml`; tests parse it, resolve every `$ref` and require it to
  match the registered routes exactly
- Example receivers in Go and Node.js without dependencies, tested against the
  Standard Webhooks test vector and run in CI
- Retention (`RETENTION_PERIOD`, default 30 days): the worker deletes finished
  events hourly in batches; events with pending or in-flight deliveries are
  kept; `hooklane-admin prune` prunes on demand; migration
  20261017170000 adds `events_created_idx`
- `hooklane-admin migrate up|status` with embedded migrations (CI now migrates
  with it) and `hooklane-admin version`
- One container image (`Dockerfile` at the root) with all three binaries and
  the dashboard; the API serves the dashboard from `WEB_DIR` with a strict
  CSP; Compose `app` profile runs migrate, API and worker
- MIT license, CONTRIBUTING, SECURITY, CHANGELOG (0.1.0), release workflow
  (binaries, dashboard archive, multi-arch GHCR image, GitHub release from the
  CHANGELOG section) and [release.md](release.md) with the checklist
- Project and API key names are capped at 200 characters

Acceptance evidence:
- A new developer can run it: from a fresh clone in an empty directory, the
  Quick start commands brought up postgres, applied all six migrations,
  started the API and worker (`/readyz` ready, dashboard served with CSP),
  bootstrapped a key, and delivered an event to the Node example receiver
  (`succeeded`, `204`); a replay to a receiver with the wrong secret got
  `401` and was dead-lettered
- Reproducible setup: migrations run from the embedded set in CI, in the
  image and in the clean install; bootstrap is a single command
- CI: api (including the openapi tests), web, examples and image jobs
- Security limitations and operational requirements: README Security and
  Operations sections, deployment checklist, SECURITY.md
- Release checklist: docs/release.md. The release workflow has not run yet;
  it runs on the first `v*` tag

Decisions to revisit:
- License: MIT was chosen as a common permissive default; change `LICENSE`,
  `openapi.yaml` and CONTRIBUTING if another license is preferred
- The check stack's volume `hooklane-check_hooklane_postgres` was left in
  place; remove it with `docker volume rm hooklane-check_hooklane_postgres`

## Migration Naming Convention

Use UTC timestamps in the format:

YYYYMMDDHHMMSS_description.sql

Example:

20261015142219_create_initial_schema.sql

Rules:
- Timestamp must be unique
- Applied migrations are immutable
- Schema changes are added through new migrations
- Never edit an applied migration to repair an existing database
