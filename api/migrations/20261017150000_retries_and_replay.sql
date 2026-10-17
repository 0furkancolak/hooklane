-- +goose Up

-- Retries reschedule a delivery as 'pending'; exhausted or permanent failures
-- become 'dead_letter'. Phase 4 marked every failed attempt as final, so those
-- rows map to dead_letter.
UPDATE deliveries SET status = 'dead_letter' WHERE status = 'failed';

ALTER TABLE deliveries DROP CONSTRAINT deliveries_status_check;
ALTER TABLE deliveries ADD CONSTRAINT deliveries_status_check
    CHECK (status IN ('pending', 'processing', 'succeeded', 'dead_letter'));

-- A replay is a new delivery for the same event and endpoint. Its own attempt
-- history starts empty; the original row and its attempts are never modified.
ALTER TABLE deliveries ADD COLUMN replay_of UUID REFERENCES deliveries(id) ON DELETE CASCADE;

ALTER TABLE deliveries DROP CONSTRAINT deliveries_event_id_endpoint_id_key;

-- Fan-out still creates at most one original delivery per event and endpoint.
CREATE UNIQUE INDEX deliveries_original_idx
    ON deliveries (event_id, endpoint_id)
    WHERE replay_of IS NULL;

-- At most one unfinished replay per delivery, so repeated replay requests
-- cannot queue duplicates.
CREATE UNIQUE INDEX deliveries_active_replay_idx
    ON deliveries (replay_of)
    WHERE replay_of IS NOT NULL AND status IN ('pending', 'processing');

CREATE INDEX deliveries_event_created_idx
    ON deliveries (event_id, created_at);

-- +goose Down

DROP INDEX IF EXISTS deliveries_event_created_idx;
DROP INDEX IF EXISTS deliveries_active_replay_idx;
DROP INDEX IF EXISTS deliveries_original_idx;
-- Replays cannot satisfy the restored unique constraint.
DELETE FROM deliveries WHERE replay_of IS NOT NULL;
ALTER TABLE deliveries DROP COLUMN replay_of;
ALTER TABLE deliveries ADD CONSTRAINT deliveries_event_id_endpoint_id_key UNIQUE (event_id, endpoint_id);

ALTER TABLE deliveries DROP CONSTRAINT deliveries_status_check;
ALTER TABLE deliveries ADD CONSTRAINT deliveries_status_check
    CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'dead_letter'));
