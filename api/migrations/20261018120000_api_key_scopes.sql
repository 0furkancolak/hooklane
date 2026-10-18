-- +goose Up
-- Keys carry scopes: admin (everything), ingest (send events only) and
-- read (GET only). Existing keys keep full access.
ALTER TABLE api_keys
    ADD COLUMN scopes TEXT[] NOT NULL DEFAULT ARRAY['admin'],
    ADD CONSTRAINT api_keys_scopes_valid CHECK (
        cardinality(scopes) > 0 AND scopes <@ ARRAY['admin', 'ingest', 'read']::text[]
    );

-- +goose Down
ALTER TABLE api_keys
    DROP CONSTRAINT IF EXISTS api_keys_scopes_valid,
    DROP COLUMN IF EXISTS scopes;
