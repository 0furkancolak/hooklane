-- name: CreateEndpoint :one
INSERT INTO endpoints (project_id, url, secret_encrypted, enabled, customer)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, project_id, url, secret_encrypted, enabled, created_at, updated_at, customer;

-- name: GetEndpoint :one
SELECT e.id, e.project_id, p.organization_id, e.url, e.secret_encrypted, e.enabled,
       e.created_at, e.updated_at, e.customer
FROM endpoints e
JOIN projects p ON p.id = e.project_id
WHERE e.id = $1;

-- name: ListEndpointsByProject :many
SELECT id, project_id, url, secret_encrypted, enabled, created_at, updated_at, customer
FROM endpoints
WHERE project_id = sqlc.arg('project_id')
  AND (sqlc.narg('customer')::text IS NULL OR customer = sqlc.narg('customer')::text)
  AND (sqlc.narg('enabled')::boolean IS NULL OR enabled = sqlc.narg('enabled')::boolean)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateEndpoint :one
UPDATE endpoints
SET url = coalesce(sqlc.narg('url'), url),
    enabled = coalesce(sqlc.narg('enabled'), enabled),
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING id, project_id, url, secret_encrypted, enabled, created_at, updated_at, customer;
