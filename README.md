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
internal/auth/      password hashing and JWT issuing/verification
internal/config/    environment loading and validation
internal/database/  Postgres connection, pooling, health check
internal/logger/    zerolog setup (console in dev, JSON in release)
internal/dto/       request/response shapes and model mappers
internal/models/    GORM models: users, products, carts, orders
internal/server/    gin engine, middleware, health endpoints
internal/utils/     HTTP response envelope
```

## Auth

`internal/auth` holds the security primitives, kept apart from `internal/utils`
(the HTTP envelope) so the code deciding who a caller is stays easy to find and
review.

**Access and refresh tokens are not interchangeable.** Both are signed with the
same secret, so the signature alone proves only that a token is ours — not what
it is for. Every token carries a `use` claim, and `ParseAccessToken` rejects a
refresh token. Without this, a 72-hour refresh token authenticates anywhere a
24-hour access token does.

Other properties worth knowing:

- The HS256 algorithm is **pinned** at parse time. A parser that trusts the
  token's own header is the root of the classic JWT confusion attacks
- `exp` is required — a token without one would be valid forever
- Every token carries a `jti`, so one session can be revoked without dropping
  all of a user's sessions
- `JWT_SECRET` must be at least 32 bytes (HS256's key size), checked at
  startup rather than at first use. Generate one with `openssl rand -hex 32`
- Passwords longer than 72 bytes are **rejected, not truncated**: bcrypt only
  ever reads the first 72, so a longer password would silently authenticate on
  a prefix. bcrypt's cost is 12 rather than the library default of 10

Protect a route with `srv.Authenticate()`, and gate admin-only routes with
`srv.RequireAdmin()` after it. Handlers read the caller via `CurrentUserID`,
`CurrentUserEmail`, and `CurrentUserRole` rather than raw context keys.

### Refresh tokens

Only a **SHA-256 hash** of each refresh token is stored, never the token — a
database leak must not hand out working sessions. Plain SHA-256 rather than
bcrypt is correct here: the token is already high-entropy, so there is nothing
to brute force.

Refresh **rotates**: the presented token is revoked as the new pair is issued,
so any refresh token works exactly once. Replaying a rotated token is treated as
theft — a legitimate client never does it — and **every session for that user is
revoked**, forcing a fresh login.

Login is deliberately uniform: a wrong password, an unknown email, and a
deactivated account all return the same error, and an unknown email still pays
for a bcrypt comparison so it cannot be identified with a stopwatch.

### Profiles

`GET /users/profile` and `/auth/me` are the same request under two names, sharing
one implementation. Both re-check the account: an access token stays valid for
its full lifetime, so a deactivated user is reported as not found rather than
served from the token's claims.

`PUT /users/profile` writes only `first_name`, `last_name`, and `phone`. Role,
email, and status are not editable here — and the update names its columns rather
than saving a loaded row, which would revert anything committed in between.

## DTOs

`internal/dto` holds what crosses the wire, deliberately separate from the
models. Binding a request straight onto a model invites mass assignment — a
caller setting `Role: "admin"` on registration — and serialising models back
leaks columns as they are added. `UserResponse` has no password field by
construction rather than by remembering a `json:"-"` tag.

Requests are validated with gin's `binding` tags. Prices cross the wire as
`price_cents`, matching the models; accepting decimals would reintroduce the
float rounding the schema avoids. `NewXResponse` mappers convert models to
responses, omitting relations that were not preloaded.

## API

Interactive docs at **http://localhost:8080/swagger/index.html** while the server
is running. They are disabled in release mode: a full description of every
endpoint and payload is reconnaissance in production.

The spec in [`docs/`](docs) is generated from annotations above the handlers.
After changing a handler, run `make docs` and commit the result; `make docs-check`
fails if it is stale.

| Endpoint | Purpose |
| --- | --- |
| `GET /health` | Liveness. Never touches the database, so a database blip cannot restart-loop the process |
| `GET /health/ready` | Readiness. Pings the database; 503 when it is unreachable |
| `POST /api/v1/auth/register` | Create a customer account, returns a token pair |
| `POST /api/v1/auth/login` | Exchange credentials for a token pair |
| `POST /api/v1/auth/refresh` | Rotate a refresh token |
| `POST /api/v1/auth/logout` | Revoke a refresh token |
| `GET /api/v1/auth/me` | The authenticated user (requires an access token) |
| `GET /api/v1/users/profile` | The authenticated user — identical to `/auth/me` |
| `PUT /api/v1/users/profile` | Replace your name and phone |

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
make hooks       # enable the git hooks
make docker-up   # start Postgres and LocalStack
make migrate-up  # create the schema
make run
```

### Git hooks

`make hooks` points `core.hooksPath` at [`.githooks/`](.githooks). They are plain
shell — nothing to install beyond the tools the Makefile already uses.

| Hook | Runs | Why there |
| --- | --- | --- |
| `pre-commit` | format check + lint | Fast, and only when Go files are staged |
| `pre-push` | `go vet` + `go test -race` | Too slow for every commit; still blocks broken code from the remote |

Bypass a single run with `git commit --no-verify` or `git push --no-verify`.
Disable entirely with `make unhooks`.

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
