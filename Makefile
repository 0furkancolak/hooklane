.PHONY: test test-integration test-db build sqlc-up db-up db-down migrate-up migrate-status dev-api dev-worker dev-web bootstrap

DATABASE_URL ?= postgres://hooklane:local_dev_only_change_me@localhost:5433/hooklane?sslmode=disable
TEST_DATABASE_URL ?= postgres://hooklane:local_dev_only_change_me@localhost:5433/hooklane_test?sslmode=disable
GOOSE = cd api && go run github.com/pressly/goose/v3/cmd/goose@v3.26.0 -dir migrations postgres

test:
	cd api && go vet ./... && go test -race ./...

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
