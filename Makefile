.PHONY: dev check lint vuln test test-integration test-db build sqlc-up db-up db-down migrate-up migrate-status dev-api dev-worker dev-web bootstrap

DATABASE_URL ?= postgres://hooklane:local_dev_only_change_me@localhost:5433/hooklane?sslmode=disable
TEST_DATABASE_URL ?= postgres://hooklane:local_dev_only_change_me@localhost:5433/hooklane_test?sslmode=disable
GOOSE = cd api && go run github.com/pressly/goose/v3/cmd/goose@v3.26.0 -dir migrations postgres

# Everything CI runs, in one command. Needs golangci-lint and govulncheck:
#   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
#   go install golang.org/x/vuln/cmd/govulncheck@latest
check: lint test vuln

lint:
	cd api && test -z "$$(gofmt -l .)" && golangci-lint run ./...
	cd web && npm run typecheck && npm run lint

vuln:
	cd api && govulncheck ./...

test:
	cd api && go vet ./... && go test -race ./...
	cd web && npm test

# Creates the hooklane_test database if missing and migrates it. The
# development database is never touched.
test-db:
	@docker compose exec -T postgres psql -U hooklane -d hooklane -tAc \
		"SELECT 1 FROM pg_database WHERE datname = 'hooklane_test'" | grep -q 1 || \
		docker compose exec -T postgres createdb -U hooklane hooklane_test
	$(GOOSE) "$(TEST_DATABASE_URL)" up

test-integration: test-db
	cd api && HOOKLANE_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -count=1 -race ./internal/infrastructure/database/...

build:
	cd api && go build ./cmd/...
	cd web && npm run build

sqlc-up:
	cd api && sqlc generate

db-up:
	docker compose up -d postgres

db-down:
	docker compose stop postgres

migrate-up:
	$(GOOSE) "$(DATABASE_URL)" up

migrate-status:
	$(GOOSE) "$(DATABASE_URL)" status

# Starts PostgreSQL, migrations, the API, the worker and the dashboard.
dev:
	./scripts/dev.sh

dev-api:
	cd api && go run ./cmd/api

dev-worker:
	cd api && go run ./cmd/worker

dev-web:
	cd web && npm run dev

bootstrap:
	cd api && go run ./cmd/admin bootstrap \
		-org-name "$(ORG_NAME)" \
		-org-slug "$(ORG_SLUG)" \
		-project-name "$(PROJECT_NAME)" \
		-key-name "$(KEY_NAME)"
