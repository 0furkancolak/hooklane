-- +goose Up

-- ADR 0001: deliveries is the transactional outbox. outbox_events was never
-- read, so dropping it removes no behaviour.
DROP TABLE IF EXISTS outbox_events;

-- +goose Down

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
