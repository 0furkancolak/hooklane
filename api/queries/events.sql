-- name: InsertEvent :one
INSERT INTO events (project_id, event_type, payload, idempotency_key)
VALUES ($1, $2, $3, $4)
ON CONFLICT (project_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
RETURNING id, project_id, event_type, payload, idempotency_key, created_at;

-- name: GetEventByIdempotencyKey :one
SELECT id, project_id, event_type, payload, idempotency_key, created_at,
       (event_type = sqlc.arg('event_type')::text
        AND payload = sqlc.arg('payload')::jsonb)::boolean AS matches
FROM events
WHERE project_id = sqlc.arg('project_id') AND idempotency_key = sqlc.arg('idempotency_key')::text;

-- name: GetEvent :one
SELECT ev.id, ev.project_id, p.organization_id, ev.event_type, ev.payload,
       ev.idempotency_key, ev.created_at
FROM events ev
JOIN projects p ON p.id = ev.project_id
WHERE ev.id = $1;

-- name: ListEventsByProject :many
SELECT id, project_id, event_type, payload, idempotency_key, created_at
FROM events
WHERE project_id = sqlc.arg('project_id')
  AND (sqlc.narg('event_type')::text IS NULL OR event_type = sqlc.narg('event_type')::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
