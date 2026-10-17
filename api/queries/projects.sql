-- name: CreateProject :one
INSERT INTO projects (organization_id, name)
VALUES ($1, $2)
RETURNING id, organization_id, name, created_at;

-- name: GetProject :one
SELECT id, organization_id, name, created_at
FROM projects
WHERE id = $1;

-- name: ListProjectsByOrganization :many
SELECT id, organization_id, name, created_at
FROM projects
WHERE organization_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;
