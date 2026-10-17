-- name: CreateEndpoint :one
INSERT INTO endpoints (project_id, url, secret_encrypted, enabled)
VALUES ($1, $2, $3, $4)
RETURNING id, project_id, url, secret_encrypted, enabled, created_at, updated_at;

-- name: GetEndpoint :one
SELECT e.id, e.project_id, p.organization_id, e.url, e.secret_encrypted, e.enabled,
       e.created_at, e.updated_at
FROM endpoints e
JOIN projects p ON p.id = e.project_id
WHERE e.id = $1;

-- name: ListEndpointsByProject :many
SELECT id, project_id, url, secret_encrypted, enabled, created_at, updated_at
FROM endpoints
WHERE project_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateEndpoint :one
UPDATE endpoints
SET url = coalesce(sqlc.narg('url'), url),
    enabled = coalesce(sqlc.narg('enabled'), enabled),
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING id, project_id, url, secret_encrypted, enabled, created_at, updated_at;
