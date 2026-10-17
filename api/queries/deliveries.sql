-- name: GetDelivery :one
SELECT d.id, d.event_id, d.endpoint_id, p.organization_id, d.status, d.attempt_count,
       d.next_attempt_at, d.last_response_status, d.last_error, d.replay_of,
       d.created_at, d.updated_at
FROM deliveries d
JOIN events e ON e.id = d.event_id
JOIN projects p ON p.id = e.project_id
WHERE d.id = $1;

-- name: ListDeliveriesByEvent :many
SELECT id, event_id, endpoint_id, status, attempt_count,
       next_attempt_at, last_response_status, last_error, replay_of,
       created_at, updated_at
FROM deliveries
WHERE event_id = $1
ORDER BY created_at, id;

-- name: CreateReplayDelivery :one
INSERT INTO deliveries (event_id, endpoint_id, replay_of)
SELECT event_id, endpoint_id, id
FROM deliveries
WHERE deliveries.id = sqlc.arg('original_id')
RETURNING id, event_id, endpoint_id, status, attempt_count,
          next_attempt_at, last_response_status, last_error, replay_of,
          created_at, updated_at;

-- name: CreateDeliveriesForEvent :execrows
INSERT INTO deliveries (event_id, endpoint_id)
SELECT sqlc.arg('event_id'), id
FROM endpoints
WHERE project_id = sqlc.arg('project_id') AND enabled;

-- name: CountDeliveriesByEvent :one
-- Counts the fan-out of an event; replays are not part of it.
SELECT count(*)::bigint
FROM deliveries
WHERE event_id = $1 AND replay_of IS NULL;

-- name: ClaimDeliveries :many
-- Claims due pending deliveries and processing deliveries whose lease expired.
-- Deliveries for disabled endpoints stay pending until the endpoint is
-- re-enabled. SKIP LOCKED lets concurrent workers claim disjoint rows.
WITH claimable AS (
    SELECT d.id
    FROM deliveries d
    JOIN endpoints ep ON ep.id = d.endpoint_id
    WHERE ep.enabled
      AND ((d.status = 'pending' AND d.next_attempt_at <= now())
        OR (d.status = 'processing' AND d.locked_until < now()))
    ORDER BY d.next_attempt_at, d.created_at
    LIMIT sqlc.arg('batch_size')
    FOR UPDATE OF d SKIP LOCKED
), claimed AS (
    UPDATE deliveries d
    SET status = 'processing',
        lease_token = gen_random_uuid(),
        locked_until = now() + make_interval(secs => sqlc.arg('lease_seconds')::double precision),
        attempt_count = d.attempt_count + 1,
        updated_at = now()
    FROM claimable c
    WHERE d.id = c.id
    RETURNING d.id, d.event_id, d.endpoint_id, d.lease_token, d.attempt_count, d.locked_until
)
SELECT claimed.id, claimed.lease_token, claimed.attempt_count, claimed.locked_until,
       ev.id AS event_id, ev.project_id, ev.event_type, ev.payload, ev.created_at AS event_created_at,
       ep.id AS endpoint_id, ep.url, ep.secret_encrypted
FROM claimed
JOIN events ev ON ev.id = claimed.event_id
JOIN endpoints ep ON ep.id = claimed.endpoint_id;

-- name: CompleteDelivery :execrows
UPDATE deliveries
SET status = sqlc.arg('status'),
    lease_token = NULL,
    locked_until = NULL,
    last_response_status = sqlc.narg('last_response_status'),
    last_error = sqlc.narg('last_error'),
    -- Retry delays are applied with the database clock, like leases.
    next_attempt_at = CASE
        WHEN sqlc.narg('retry_delay_seconds')::double precision IS NULL THEN next_attempt_at
        ELSE now() + make_interval(secs => sqlc.narg('retry_delay_seconds')::double precision)
    END,
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND lease_token = sqlc.arg('lease_token')
  AND status = 'processing';

-- name: InsertDeliveryAttempt :exec
INSERT INTO delivery_attempts (delivery_id, attempt_number, status_code, response_body, error, duration_ms)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListDeliveriesByProject :many
-- Scoped through endpoints so deliveries_endpoint_created_idx can serve each
-- endpoint of the project.
SELECT d.id, d.event_id, d.endpoint_id, d.status, d.attempt_count,
       d.next_attempt_at, d.last_response_status, d.last_error, d.replay_of,
       d.created_at, d.updated_at
FROM deliveries d
JOIN endpoints en ON en.id = d.endpoint_id
WHERE en.project_id = sqlc.arg('project_id')
  AND (sqlc.narg('status')::text IS NULL OR d.status = sqlc.narg('status')::text)
  AND (sqlc.narg('endpoint_id')::uuid IS NULL OR d.endpoint_id = sqlc.narg('endpoint_id')::uuid)
ORDER BY d.created_at DESC, d.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListDeliveryAttempts :many
SELECT id, delivery_id, attempt_number, status_code, response_body, error, duration_ms, attempted_at
FROM delivery_attempts
WHERE delivery_id = $1
ORDER BY attempt_number;

-- name: DeliveryStatsByProject :one
-- oldest_due_at ignores disabled endpoints: workers do not claim their
-- deliveries, so they are held rather than late.
SELECT count(*) FILTER (WHERE d.status = 'pending')::bigint AS pending,
       count(*) FILTER (WHERE d.status = 'processing')::bigint AS processing,
       count(*) FILTER (WHERE d.status = 'succeeded')::bigint AS succeeded,
       count(*) FILTER (WHERE d.status = 'dead_letter')::bigint AS dead_letter,
       min(d.next_attempt_at) FILTER (WHERE d.status = 'pending' AND d.next_attempt_at <= now() AND en.enabled)::timestamptz AS oldest_due_at
FROM deliveries d
JOIN endpoints en ON en.id = d.endpoint_id
WHERE en.project_id = $1;

-- name: QueueBacklog :one
-- Instance-wide queue depth for worker metrics. Pending deliveries of
-- disabled endpoints are reported as held, not due, because workers skip them.
SELECT count(*) FILTER (WHERE d.status = 'pending' AND en.enabled AND d.next_attempt_at <= now())::bigint AS due,
       count(*) FILTER (WHERE d.status = 'pending' AND en.enabled AND d.next_attempt_at > now())::bigint AS scheduled,
       count(*) FILTER (WHERE d.status = 'pending' AND NOT en.enabled)::bigint AS held,
       count(*) FILTER (WHERE d.status = 'processing')::bigint AS processing,
       coalesce(extract(epoch FROM now() - min(d.next_attempt_at) FILTER (WHERE d.status = 'pending' AND en.enabled AND d.next_attempt_at <= now())), 0)::float8 AS oldest_due_seconds
FROM deliveries d
JOIN endpoints en ON en.id = d.endpoint_id
WHERE d.status IN ('pending', 'processing');
