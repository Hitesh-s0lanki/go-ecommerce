# go-ecommerce

An e-commerce backend written in Go.

> **Status:** early scaffold. Tooling and CI are set up; `cmd/api` is a placeholder
> entrypoint that only logs a startup line.

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
same file, so `make migrate-*` targets the database you configured there.

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
