# Values in .env win over the defaults below; both can be overridden on the
# command line, e.g. `make migrate-up DB_PORT=5433`.
-include .env

APP_NAME       := api
BIN_DIR        := bin
CMD_DIR        := ./cmd/api
MIGRATIONS_DIR := db/migrations
COMPOSE        := docker compose -f docker/docker-compose.yml

DB_HOST     ?= localhost
DB_PORT     ?= 5432
DB_USER     ?= postgres
DB_PASSWORD ?= password
DB_NAME     ?= ecommerce_shop
DB_SSLMODE  ?= disable
DATABASE_URL ?= postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)

# `make tools` installs into GOPATH/bin, which is usually not on PATH — so
# `make docs` would fail with "command not found" right after installing swag.
# Exporting PATH does not help: make execs simple recipe lines itself, using
# its own PATH rather than the recipe's. Resolve the binaries explicitly,
# preferring one already on PATH (a brew install, say) and falling back to
# whatever `make tools` produced. Override like `make docs SWAG=/path/to/swag`.
GOPATH_BIN := $(shell go env GOPATH)/bin
SWAG       ?= $(shell command -v swag 2>/dev/null || echo $(GOPATH_BIN)/swag)
MIGRATE    ?= $(shell command -v migrate 2>/dev/null || echo $(GOPATH_BIN)/migrate)

# Pinned so local runs match CI.
GOLANGCI_LINT_VERSION := v2.12.2
MIGRATE_VERSION       := v4.19.1
SWAG_VERSION          := v1.16.6

.DEFAULT_GOAL := help

## help: Show this help.
# -h suppresses filenames: MAKEFILE_LIST also holds .env once it exists, and
# grep prefixes every line when given more than one file.
help:
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed -e 's/## //' | awk -F': ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## hooks: Enable the repo's git hooks (format+lint on commit, test on push).
hooks:
	git config core.hooksPath .githooks
	@echo "git hooks enabled. Bypass a single run with --no-verify."

## unhooks: Disable the repo's git hooks.
unhooks:
	git config --unset core.hooksPath
	@echo "git hooks disabled."

## tools: Install pinned dev tooling (golangci-lint, migrate).
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)
	go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION)

## tidy: Sync go.mod and go.sum.
tidy:
	go mod tidy

## docs: Regenerate the Swagger spec from the handler annotations.
docs:
	$(SWAG) init -g cmd/api/main.go -o docs --parseDependency --parseInternal

## docs-check: Fail if docs/ is stale relative to the annotations.
docs-check: docs
	@git diff --exit-code --stat docs/ \
		|| { echo "docs/ is out of date. Run 'make docs' and commit the result." >&2; exit 1; }

## build: Compile the binary into bin/.
build:
	go build -o $(BIN_DIR)/$(APP_NAME) $(CMD_DIR)

## run: Run the application.
run:
	go run $(CMD_DIR)

## dev: Alias for run.
dev: run

## test: Run tests with the race detector.
test:
	go test -race ./...

## test-integration: Run tests against the database from .env (needs make docker-up).
test-integration:
	TEST_DATABASE_DSN="host=$(DB_HOST) user=$(DB_USER) password=$(DB_PASSWORD) dbname=$(DB_NAME) port=$(DB_PORT) sslmode=$(DB_SSLMODE) TimeZone=UTC" \
		go test -race -count=1 ./...

## cover: Run tests and open an HTML coverage report.
cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

## lint: Run golangci-lint (also reports formatting issues).
lint:
	golangci-lint run

## lint-fix: Run golangci-lint and apply autofixes.
lint-fix:
	golangci-lint run --fix

## fmt: Apply the configured formatters in place (gofmt + goimports).
fmt:
	golangci-lint fmt

## format: Alias for fmt.
format: fmt

## fmt-check: Fail if any file needs formatting.
fmt-check:
	golangci-lint fmt --diff

## vet: Run go vet.
vet:
	go vet ./...

## ci: Everything CI runs, locally.
ci: tidy vet lint test

## migrate-create: Create a migration, e.g. make migrate-create name=create_users.
migrate-create:
	@test -n "$(name)" || { echo "usage: make migrate-create name=<migration_name>"; exit 1; }
	@mkdir -p $(MIGRATIONS_DIR)
	$(MIGRATE) create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

## migrate-up: Apply all pending migrations.
migrate-up:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

## migrate-down: Roll back the last migration.
migrate-down:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1

## migrate-status: Show the current migration version.
migrate-status:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" version

## docker-up: Start Postgres and LocalStack in the background.
docker-up:
	$(COMPOSE) up -d

## docker-down: Stop the containers.
docker-down:
	$(COMPOSE) down

## docker-logs: Follow container logs.
docker-logs:
	$(COMPOSE) logs -f

## docker-reset: Destroy the containers and their volumes, then restart.
docker-reset:
	$(COMPOSE) down -v
	$(COMPOSE) up -d

## clean: Remove build and coverage artifacts.
clean:
	rm -rf $(BIN_DIR) coverage.out

.PHONY: help hooks unhooks tools tidy docs docs-check build run dev test test-integration cover \
	lint lint-fix fmt format fmt-check vet ci \
	migrate-create migrate-up migrate-down migrate-status \
	docker-up docker-down docker-logs docker-reset clean
