# Hooklane Architecture Plan

## 1. Product Overview

Hooklane is an open-source, self-hostable webhook delivery platform

Core capabilities:
- Multi-tenant organization and project management
- Webhook endpoint registration
- Event ingestion through an API
- Reliable asynchronous webhook delivery
- HMAC-SHA256 webhook signatures compatible with Standard Webhooks conventions
- Idempotent event ingestion
- Automatic retries with exponential backoff and jitter
- Dead-letter handling and manual replay
- Delivery attempts and event history
- API keys and authenticated management APIs
- Web dashboard
- CLI-based initial administrator bootstrap

## 2. Technology Stack

### Backend
- Go
- Fiber v3
- PostgreSQL
- pgx
- sqlc
- Goose migrations
- Structured logging

### Frontend
- React
- TypeScript
- Vite
- Feature-based architecture

### Infrastructure
- Docker Compose for local development
- PostgreSQL as the primary persistent store
- GitHub Actions for CI
- OpenAPI for API documentation

## 3. Repository Structure

api/
  cmd/
    api/
    worker/
    admin/
  internal/
    config/
    domain/
    application/
    infrastructure/
      database/
      webhook/
    transport/
      http/
  migrations/
  queries/

web/

docs/
.github/
  workflows/

The Go module lives in api/ and follows the standard cmd/ + internal/ layout.
Repository interfaces are declared in application/, next to their consumers.

## 4. Architecture Principles

- Pragmatic Clean Architecture
- CQRS separation between commands and queries where useful
- Explicit domain boundaries
- Dependency inversion
- Small interfaces defined near their consumers
- PostgreSQL as the source of truth
- Transactional outbox for reliable asynchronous processing: delivery rows are
  written with the event and consumed by workers (ADR 0001)
- Separate API and worker processes
- Feature-based React architecture
- No unnecessary abstractions or speculative infrastructure

## 5. Core Data Model

### organizations
Tenant and ownership boundary

### projects
Projects belonging to an organization

### api_keys
Hashed API key material, scopes, status and lifecycle metadata

### endpoints
Webhook destination configuration and endpoint status

### events
Accepted events and their associated project

### deliveries
Delivery state, scheduling, lease information and retry metadata.
Pending deliveries double as the transactional outbox (ADR 0001).

### delivery_attempts
Immutable historical records of delivery attempts


## 6. Event Processing Flow

1. Authenticate the caller
2. Validate request and tenant ownership
3. Validate idempotency requirements
4. Persist the event and associated delivery work in one transaction
5. The pending delivery rows are the transactional outbox (ADR 0001)
6. Return an acceptance response
7. Worker claims eligible work using PostgreSQL coordination
8. Worker sends the HTTP request with a signed payload
9. Worker records the attempt and response metadata
10. Successful deliveries are marked completed
11. Retryable failures are scheduled according to retry policy
12. Exhausted deliveries enter a dead-letter state

## 7. Delivery Guarantees

- At-least-once delivery attempts
- No promise of exactly-once delivery at the receiver
- Stable event identifiers
- Idempotent ingestion within the documented idempotency scope
- Independent historical records for each attempt
- Replay creates new deliveries and preserves existing history
- Database transactions protect event and delivery consistency

## 8. Retry Policy

- Configurable maximum attempts
- Exponential backoff
- Random jitter
- Retry classification based on network failures and HTTP status
- Scheduled retries persisted in PostgreSQL
- Dead-letter state after retry exhaustion
- Manual replay with auditability

## 9. Security Requirements

### Authentication
- Store only cryptographic hashes of API keys
- Never log raw API keys
- Support key revocation and appropriate authorization scopes
- Use constant-time comparison where applicable

### Webhook Signatures
- HMAC-SHA256
- Standard Webhooks-compatible signing conventions
- Timestamp included in signed material
- Documented receiver-side verification examples

### SSRF Protection
- HTTPS required by default
- HTTP localhost allowed only in an explicit development profile
- Reject loopback, private, link-local and reserved destinations in production
- Reject cloud metadata endpoints
- Validate DNS results and protect against DNS rebinding
- Validate every redirect destination or disable redirects
- Prevent validation-to-connection bypasses

### Multi-Tenant Isolation
- Derive tenant context from authenticated credentials
- Enforce organization and project ownership in application operations
- Avoid trusting tenant identifiers supplied by clients without authorization
- Add tests for cross-tenant access attempts

### Operational Security
- Redact secrets and sensitive payload data from logs
- Bound request sizes and response-body reads
- Apply timeouts to outbound requests
- Validate configuration on startup
- Use safe defaults and explicit development overrides

## 10. Retention and Data Management

- Default event retention: 30 days
- Document retention behavior and deletion semantics
- Preserve delivery-attempt history according to the documented retention policy
- Provide a future path for scheduled cleanup without requiring a separate infrastructure service

## 11. API and Operational Requirements

- Versioned management API
- Consistent error response format
- OpenAPI specification
- Health endpoint
- Readiness checks
- Structured logs
- Delivery and worker metrics
- Graceful shutdown
- Bounded concurrency
- Database connection pooling
- Request and worker timeouts

## 12. Explicit MVP Non-Goals

- Redis
- Kafka
- Kubernetes
- Event sourcing
- Separate read database
- Premature microservice decomposition
- Distributed infrastructure without a demonstrated requirement

## 13. Quality Gates

Every phase must:
- Build successfully
- Pass relevant automated tests
- Preserve tenant isolation
- Avoid logging secrets
- Update documentation when behavior changes
- Include migration changes when the database changes
- Be reviewed against its acceptance criteria before the next phase begins
