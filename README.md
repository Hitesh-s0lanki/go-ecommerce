# go-ecommerce

An e-commerce backend written in Go.

> **Status:** early scaffold. Config, database, logging and the HTTP server are
> wired up, with health endpoints only — no business routes yet.

## Stack

| Concern | Choice |
| --- | --- |
| HTTP | [gin](https://github.com/gin-gonic/gin) v1.12.0 |
| ORM | [GORM](https://gorm.io/) v1.31.2 + Postgres driver v1.6.0 |
| Logging | [zerolog](https://github.com/rs/zerolog) v1.35.1 |
| Config | [godotenv](https://github.com/joho/godotenv) v1.5.1 + environment |

## Layout

```
cmd/api/            entrypoint: config, database, HTTP server, graceful shutdown
internal/config/    environment loading and validation
internal/database/  Postgres connection, pooling, health check
internal/logger/    zerolog setup (console in dev, JSON in release)
internal/models/    GORM models: users, products, carts, orders
internal/server/    gin engine, middleware, health endpoints
internal/utils/     HTTP response envelope
```

## API

| Endpoint | Purpose |
| --- | --- |
| `GET /health` | Liveness. Never touches the database, so a database blip cannot restart-loop the process |
| `GET /health/ready` | Readiness. Pings the database; 503 when it is unreachable |

Every response uses one envelope:

```json
{ "success": true, "message": "ok", "data": { "status": "ok" } }
```

Error detail is only returned for 4xx (what the caller did wrong). A 5xx returns
a generic message and logs the detail, since internal errors can carry schema
details, file paths, or driver internals.

Every request carries an `X-Request-ID`, reusing an inbound one if present so
correlation ids survive across services, and it appears on the request's log line.

CORS comes from `ALLOWED_ORIGINS` (comma-separated). `*` allows any origin and
never sends credentials; explicit origins are echoed back with `Vary: Origin` and
do allow credentials.

## Models

Nine models: `User`, `RefreshToken`, `Category`, `Product`, `ProductImage`,
`Cart`, `CartItem`, `Order`, `OrderItem`. `models.All()` lists them in dependency
order, and is the single source of truth for migrations and tests.

Two conventions worth knowing:

**Money is stored as integer cents** (`PriceCents`, `TotalAmountCents`,
`UnitPriceCents`), never as a float — binary floating point cannot represent
decimal cents exactly, so summing line items drifts. `1999` means $19.99.

**Unique indexes are partial** (`WHERE deleted_at IS NULL`). All models soft
delete, and a plain unique index would let a deleted row reserve its email or SKU
forever.

`OrderItem` snapshots the price at purchase; `CartItem` deliberately does not, so
carts always price from the current product.

## Migrations

Versioned SQL under [`db/migrations/`](db/migrations), applied with
[golang-migrate](https://github.com/golang-migrate/migrate). The models never
migrate themselves — `AutoMigrate` is used only to build throwaway schemas in
tests, so the SQL files are the single source of truth for the real database.

```bash
make docker-up                       # Postgres must be running
make migrate-up                      # apply
make migrate-status                  # current version
make migrate-down                    # roll back one
make migrate-create name=add_widgets # scaffold a new pair
```

The SQL is kept equivalent to the models: applying the migrations and running
`AutoMigrate` produce a byte-identical schema. If you change a model, change the
migrations too — nothing enforces this automatically.

## Requirements

- Go 1.26 or newer
- Docker (for Postgres and LocalStack)
- [golangci-lint](https://golangci-lint.run/) v2.12.2 and
  [golang-migrate](https://github.com/golang-migrate/migrate) v4.19.1 — `make tools`
  installs both at the pinned versions

## Getting started

```bash
git clone https://github.com/Hitesh-s0lanki/go-ecommerce.git
cd go-ecommerce
cp .env.example .env
make tools       # install pinned dev tooling
make docker-up   # start Postgres and LocalStack
make run
```

## Development

`make help` lists every target. The common ones:

| Target | Does |
| --- | --- |
| `make run` / `make dev` | Run the API server |
| `make build` | Compile to `bin/api` |
| `make test` | Run tests with the race detector |
| `make test-integration` | Also run tests needing Postgres (`make docker-up` first) |
| `make lint` / `make lint-fix` | Lint, optionally applying autofixes |
| `make fmt` / `make fmt-check` | Apply formatters, or just show the diff |
| `make ci` | Run tidy, vet, lint and test together |
| `make docker-up` / `make docker-down` | Start/stop containers |
| `make docker-reset` | Destroy containers **and volumes**, then restart |
| `make migrate-up` / `make migrate-down` | Apply / roll back one migration |
| `make migrate-create name=add_users` | Scaffold a new migration |

## Services

`docker/docker-compose.yml` runs:

- **Postgres 18.4** on `localhost:5432`
- **LocalStack** (S3, SQS) on `localhost:4566`

Configuration comes from `.env` (copy from `.env.example`). The Makefile reads the
same file, so `make migrate-*` targets the database you configured there. Real
environment variables take precedence over `.env`.

`GIN_MODE` drives more than routing: `release` switches logs to JSON, quiets GORM
statement logging, and makes a default `JWT_SECRET` a startup error rather than a
warning. Config is validated at startup, so a malformed duration or size fails
immediately instead of silently becoming zero.

> **Note on the Postgres volume:** Postgres 18 stores data in
> `/var/lib/postgresql/18/docker` and expects the volume at `/var/lib/postgresql`.
> The pre-18 path (`/var/lib/postgresql/data`) is not interchangeable — an 18
> container pointed at it refuses to start.

## Linting

Linters are configured in [`.golangci.yml`](.golangci.yml) and run in CI on every push
to `main` and every pull request via [`.github/workflows/lint.yml`](.github/workflows/lint.yml).

Enabled on top of golangci-lint's standard set (`errcheck`, `govet`, `ineffassign`,
`staticcheck`, `unused`): `bodyclose`, `errorlint`, `gocritic`, `gosec`, `misspell`,
`revive`, and `unconvert`. Formatting is handled by `gofmt` and `goimports`, with
local imports grouped separately.

Useful commands:

```bash
golangci-lint run --fix    # apply autofixes where linters support them
golangci-lint fmt --diff   # show formatting changes without writing
golangci-lint config verify # validate .golangci.yml against the schema
golangci-lint linters      # show which linters are active
```

## Module

```
github.com/Hitesh-s0lanki/go-ecommerce
```

## License

Not yet specified.
