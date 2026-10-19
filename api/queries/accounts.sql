-- name: CreateUser :one
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
RETURNING id, email, password_hash, created_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, created_at
FROM users
WHERE email = $1;

-- name: GetUser :one
SELECT id, email, password_hash, created_at
FROM users
WHERE id = $1;

-- name: CountUsers :one
SELECT count(*)::bigint FROM users;

-- name: CreateMembership :exec
INSERT INTO memberships (user_id, organization_id, role)
VALUES ($1, $2, $3);

-- name: ListMemberships :many
SELECT o.id, o.name, o.slug, o.created_at
FROM memberships m
JOIN organizations o ON o.id = m.organization_id
WHERE m.user_id = $1
ORDER BY m.created_at, o.id;

-- name: IsMember :one
SELECT EXISTS (
    SELECT 1 FROM memberships WHERE user_id = $1 AND organization_id = $2
);

-- name: CreateSession :one
INSERT INTO sessions (user_id, organization_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, organization_id, token_hash, expires_at, created_at;

-- name: GetSessionByHash :one
SELECT id, user_id, organization_id, token_hash, expires_at, created_at
FROM sessions
WHERE token_hash = $1;

-- name: UpdateSessionOrganization :exec
UPDATE sessions SET organization_id = $2 WHERE id = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at < now();

-- name: HitRateLimit :one
WITH w AS (
    SELECT to_timestamp(floor(extract(epoch FROM now()) / sqlc.arg(window_seconds)::int) * sqlc.arg(window_seconds)::int) AS start
), hit AS (
    INSERT INTO rate_limit_windows (key, window_start, hits)
    SELECT sqlc.arg(key)::text, w.start, 1 FROM w
    ON CONFLICT (key, window_start) DO UPDATE SET hits = rate_limit_windows.hits + 1
    RETURNING hits, window_start
)
SELECT hit.hits::int AS hits,
       GREATEST(0, extract(epoch FROM (hit.window_start + make_interval(secs => sqlc.arg(window_seconds)::int) - now())))::float8 AS seconds_left
FROM hit;

-- name: DeleteOldRateLimitWindows :execrows
DELETE FROM rate_limit_windows WHERE window_start < now() - interval '10 minutes';
