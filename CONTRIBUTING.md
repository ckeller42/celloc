# Contributing

Thanks for helping! celloc is small, test-first Go.

## Workflow

1. Branch off `main`.
2. **Write the failing test first**, then the code (TDD). Keep the pure-vs-I/O
   split (see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)): parsing/marshaling
   packages stay free of network/filesystem/env/clock — inject those.
3. Open a PR. CI must pass and CodeRabbit's review threads must be resolved
   before merge (branch protection enforces conversation resolution).

## Local checks

Install the hooks so you don't bounce off CI:

```sh
pipx install pre-commit
pre-commit install
pre-commit install --hook-type pre-push
```

Or run directly:

```sh
make test    # go test ./... -race + coverage
make lint    # golangci-lint v2 (run + fmt)
make ipk     # build the OpenWrt .ipk (needs GNU ar; CI builds releases)
```

Requirements: Go 1.23+, `golangci-lint` v2, `gofumpt`. The coverage gate is
**≥85% over `./internal/...`**; don't weaken assertions to hit it.

## Conventions

- `gofumpt`-formatted, `golangci-lint` clean.
- Exported identifiers documented.
- Table-driven tests; cover the edge cases (garbage/partial input, error and
  transient paths, CRLF framing).
- Never fabricate positioning data (see ARCHITECTURE "Honest gpsd semantics").
- Never put secrets (OpenCelliD/InfluxDB tokens) in argv, logs, or commits.
- Commit messages: imperative subject; explain the *why*.
