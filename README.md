# go-ecommerce

An e-commerce backend written in Go.

> **Status:** early scaffold. Tooling and CI are set up; `cmd/api` is a placeholder
> entrypoint that only logs a startup line.

## Requirements

- Go 1.26 or newer
- [golangci-lint](https://golangci-lint.run/) v2.12.2 (`brew install golangci-lint`)

## Getting started

```bash
git clone https://github.com/Hitesh-s0lanki/go-ecommerce.git
cd go-ecommerce
go mod download
```

## Development

```bash
go run ./cmd/api      # run the API server
go build ./...        # build
go test ./...         # run tests
golangci-lint run     # lint (also reports formatting issues)
golangci-lint fmt     # apply formatters in place
```

## Linting

Linters are configured in [`.golangci.yml`](.golangci.yml) and run in CI on every push
to `main` and every pull request via [`.github/workflows/lint.yml`](.github/workflows/lint.yml).

Enabled on top of golangci-lint's standard set (`errcheck`, `govet`, `ineffassign`,
`staticcheck`, `unused`): `bodyclose`, `errorlint`, `gosec`, `misspell`, `revive`,
and `unconvert`. Formatting is handled by `gofmt` and `goimports`, with local imports
grouped separately.

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
