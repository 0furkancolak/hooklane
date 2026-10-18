-- name: CreateAPIKey :one
INSERT INTO api_keys (project_id, name, key_prefix, key_hash, scopes)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, project_id, name, key_prefix, key_hash, last_used_at, revoked_at, created_at, scopes;

-- name: GetAPIKey :one
SELECT k.id, k.project_id, p.organization_id, k.name, k.key_prefix, k.key_hash, k.scopes,
       k.last_used_at, k.revoked_at, k.created_at
FROM api_keys k
JOIN projects p ON p.id = k.project_id
WHERE k.id = $1;

-- name: GetAPIKeyByHash :one
SELECT k.id, k.project_id, p.organization_id, k.name, k.key_prefix, k.key_hash, k.scopes,
       k.last_used_at, k.revoked_at, k.created_at
FROM api_keys k
JOIN projects p ON p.id = k.project_id
WHERE k.key_hash = $1;

-- name: ListAPIKeysByProject :many
SELECT k.id, k.project_id, p.organization_id, k.name, k.key_prefix, k.key_hash, k.scopes,
       k.last_used_at, k.revoked_at, k.created_at
FROM api_keys k
JOIN projects p ON p.id = k.project_id
WHERE k.project_id = $1
ORDER BY k.created_at DESC;

-- name: RevokeAPIKey :one
UPDATE api_keys
SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL
RETURNING id, project_id, name, key_prefix, key_hash, last_used_at, revoked_at, created_at, scopes;

-- name: TouchAPIKeyLastUsed :exec
UPDATE api_keys
SET last_used_at = now()
WHERE id = $1;
