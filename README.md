# go-ecommerce

An e-commerce backend written in Go.

> **Status:** early scaffold. Config, database and logging are wired up; the HTTP
> server itself is not implemented yet — `cmd/api` connects to Postgres, logs, and
> waits for a shutdown signal.

## Stack

| Concern | Choice |
| --- | --- |
| HTTP | [gin](https://github.com/gin-gonic/gin) v1.12.0 |
| ORM | [GORM](https://gorm.io/) v1.31.2 + Postgres driver v1.6.0 |
| Logging | [zerolog](https://github.com/rs/zerolog) v1.35.1 |
| Config | [godotenv](https://github.com/joho/godotenv) v1.5.1 + environment |

## Layout

```
cmd/api/            entrypoint: loads config, connects, waits for signals
internal/config/    environment loading and validation
internal/database/  Postgres connection, pooling, health check
internal/logger/    zerolog setup (console in dev, JSON in release)
```

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
