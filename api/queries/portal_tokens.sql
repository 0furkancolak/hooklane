-- name: CreatePortalToken :one
INSERT INTO portal_tokens (project_id, customer, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, project_id, customer, token_hash, expires_at, created_at;

-- name: GetPortalTokenByHash :one
SELECT t.id, t.project_id, p.organization_id, t.customer, t.token_hash, t.expires_at, t.created_at
FROM portal_tokens t
JOIN projects p ON p.id = t.project_id
WHERE t.token_hash = $1;
