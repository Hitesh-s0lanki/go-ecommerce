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

# Pinned so local runs match CI.
GOLANGCI_LINT_VERSION := v2.12.2
MIGRATE_VERSION       := v4.19.1

.DEFAULT_GOAL := help

## help: Show this help.
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed -e 's/## //' | awk -F': ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## tools: Install pinned dev tooling (golangci-lint, migrate).
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

## tidy: Sync go.mod and go.sum.
tidy:
	go mod tidy

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

## fmt: Apply the configured formatters in place.
fmt:
	golangci-lint fmt

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
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

## migrate-up: Apply all pending migrations.
migrate-up:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

## migrate-down: Roll back the last migration.
migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1

## migrate-status: Show the current migration version.
migrate-status:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" version

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

.PHONY: help tools tidy build run dev test cover lint lint-fix fmt fmt-check vet ci \
	migrate-create migrate-up migrate-down migrate-status \
	docker-up docker-down docker-logs docker-reset clean
