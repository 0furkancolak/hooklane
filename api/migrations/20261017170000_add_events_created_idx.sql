-- +goose Up
-- Retention deletes events by age across all projects.
CREATE INDEX events_created_idx ON events (created_at);

-- +goose Down
DROP INDEX IF EXISTS events_created_idx;
