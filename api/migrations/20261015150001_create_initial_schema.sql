-- +goose Up

CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    slug TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]*$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX projects_organization_created_idx
    ON projects (organization_id, created_at DESC);

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_hash BYTEA NOT NULL UNIQUE,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX api_keys_project_idx ON api_keys (project_id);

CREATE TABLE endpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    secret_encrypted BYTEA NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX endpoints_project_enabled_idx
    ON endpoints (project_id, enabled);

CREATE TABLE events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (length(trim(event_type)) > 0),
    payload JSONB NOT NULL,
    idempotency_key TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (jsonb_typeof(payload) IS NOT NULL)
);

CREATE UNIQUE INDEX events_project_idempotency_idx
    ON events (project_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX events_project_created_idx
    ON events (project_id, created_at DESC);

CREATE TABLE deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    endpoint_id UUID NOT NULL REFERENCES endpoints(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'dead_letter')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_until TIMESTAMPTZ,
    last_response_status INTEGER,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, endpoint_id)
);

CREATE INDEX deliveries_pending_idx
    ON deliveries (next_attempt_at, created_at)
    WHERE status = 'pending';

CREATE INDEX deliveries_expired_lease_idx
    ON deliveries (locked_until)
    WHERE status = 'processing';

CREATE INDEX deliveries_endpoint_created_idx
    ON deliveries (endpoint_id, created_at DESC);

CREATE TABLE delivery_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_id UUID NOT NULL REFERENCES deliveries(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    status_code INTEGER,
    response_body TEXT,
    error TEXT,
    duration_ms BIGINT CHECK (duration_ms IS NULL OR duration_ms >= 0),
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (delivery_id, attempt_number)
);

CREATE INDEX delivery_attempts_delivery_idx
    ON delivery_attempts (delivery_id, attempted_at DESC);

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX outbox_unprocessed_idx
    ON outbox_events (available_at, created_at)
    WHERE processed_at IS NULL;

-- +goose Down

DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS delivery_attempts;
DROP TABLE IF EXISTS deliveries;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS endpoints;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS organizations;
