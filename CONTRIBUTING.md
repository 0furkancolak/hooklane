# Contributing to Hooklane

Thanks for helping. Bug reports, documentation fixes and pull requests are
all welcome.

## Before you start

- For anything larger than a small fix, open an issue first so the approach
  can be agreed before you write code.
- Security problems go through [SECURITY.md](SECURITY.md), not public issues.

## Setting up

Follow [Development](README.md#development) in the README: PostgreSQL in
Docker Compose, then `make migrate-up`, `make test` and
`make test-integration`.

## Making a change

- Go code lives in `api/` (standard `cmd/` + `internal/` layout); the
  dashboard in `web/` (one folder per feature under `src/features`).
- Repository interfaces belong in `internal/application`, next to the code
  that uses them. Keep infrastructure out of the application layer.
- SQL goes in `api/queries/`; run `make sqlc-up` and commit the generated
  code. Schema changes are new Goose migrations named
  `YYYYMMDDHHMMSS_description.sql` (UTC). Never edit a migration that has
  been released.
- Every API route must be described in `api/openapi/openapi.yaml`; a test
  fails otherwise.
- Never log API keys, endpoint secrets or event payloads, and keep every
  query scoped to the caller's organization. New routes belong in the
  cross-tenant test in `internal/transport/http/security_test.go`.
- Add tests with the change. Database behaviour needs an integration test
  (`HOOKLANE_TEST_DATABASE_URL`).
- Update the README, `docs/deployment.md` and `CHANGELOG.md` (under
  `Unreleased`) when behaviour or configuration changes.

## Checks

CI runs the same commands; run them before opening a pull request:

```sh
cd api && gofmt -l . && go vet ./... && go test -race ./...
make test-integration
make check
cd web && npm run build
```

## Pull requests

Keep pull requests focused, explain what changed and why, and say how you
tested it. By contributing you agree that your contribution is licensed under
the [MIT License](LICENSE).
