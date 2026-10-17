-- name: GetDelivery :one
SELECT id, event_id, endpoint_id, status, attempt_count,
       next_attempt_at, locked_until, last_response_status, last_error,
       created_at, updated_at
FROM deliveries
WHERE id = $1;

-- name: ListDeliveriesByEndpoint :many
SELECT id, event_id, endpoint_id, status, attempt_count,
       next_attempt_at, locked_until, last_response_status, last_error,
       created_at, updated_at
FROM deliveries
WHERE endpoint_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CreateDeliveriesForEvent :execrows
INSERT INTO deliveries (event_id, endpoint_id)
SELECT sqlc.arg('event_id'), id
FROM endpoints
WHERE project_id = sqlc.arg('project_id') AND enabled;

-- name: CountDeliveriesByEvent :one
SELECT count(*)::bigint
FROM deliveries
WHERE event_id = $1;

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
    next_attempt_at = coalesce(sqlc.narg('next_attempt_at'), next_attempt_at),
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND lease_token = sqlc.arg('lease_token')
  AND status = 'processing';

-- name: InsertDeliveryAttempt :exec
INSERT INTO delivery_attempts (delivery_id, attempt_number, status_code, response_body, error, duration_ms)
VALUES ($1, $2, $3, $4, $5, $6);
