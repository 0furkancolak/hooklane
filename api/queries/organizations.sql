-- name: CreateOrganization :one
INSERT INTO organizations (name, slug)
VALUES ($1, $2)
RETURNING id, name, slug, created_at;

-- name: GetOrganization :one
SELECT id, name, slug, created_at
FROM organizations
WHERE id = $1;

-- name: CountOrganizations :one
SELECT count(*)::bigint
FROM organizations;
