-- +goose Up
-- A customer is an opaque identifier from the integrator's own system. An
-- endpoint with a customer only receives events sent for that customer, and
-- events without a customer only reach endpoints without one.
ALTER TABLE endpoints
    ADD COLUMN customer TEXT,
    ADD CONSTRAINT endpoints_customer_length CHECK (customer IS NULL OR char_length(customer) BETWEEN 1 AND 128);

ALTER TABLE events
    ADD COLUMN customer TEXT,
    ADD CONSTRAINT events_customer_length CHECK (customer IS NULL OR char_length(customer) BETWEEN 1 AND 128);

CREATE INDEX endpoints_project_customer_idx ON endpoints (project_id, customer);

-- Portal links let one customer manage their own endpoints without an API
-- key. Only the hash of the token is stored.
CREATE TABLE portal_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    customer TEXT NOT NULL CHECK (char_length(customer) BETWEEN 1 AND 128),
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX portal_tokens_project_idx ON portal_tokens (project_id);
CREATE INDEX portal_tokens_expires_idx ON portal_tokens (expires_at);

-- +goose Down
DROP TABLE IF EXISTS portal_tokens;
DROP INDEX IF EXISTS endpoints_project_customer_idx;
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_customer_length, DROP COLUMN IF EXISTS customer;
ALTER TABLE endpoints DROP CONSTRAINT IF EXISTS endpoints_customer_length, DROP COLUMN IF EXISTS customer;
