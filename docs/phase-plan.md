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
- [ ] Phase 5 complete and verified
- [ ] Phase 6 complete and verified
- [ ] Phase 7 complete and verified
- [ ] Phase 8 complete and verified
- [ ] Phase 9 complete and verified

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
