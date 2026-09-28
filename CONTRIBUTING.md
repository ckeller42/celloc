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
pre-commit install   # installs both the pre-commit and pre-push hooks
```

`.pre-commit-config.yaml` pins every lint tool, and CI's `pre-commit` job runs
the same file (`pre-commit run --all-files`), so local and CI versions cannot
drift. On commit it runs whitespace/EOF/YAML checks, gitleaks (staged changes),
markdownlint-cli2 (`.markdownlint-cli2.jsonc`), golangci-lint **v2.1.6**
(`run --fix`, then `fmt`; built from source, so Go must be installed) and
`go build` of both entrypoints; `go test ./...` runs on pre-push. CI also scans
the whole tree with `pre-commit run --hook-stage manual gitleaks-dir --all-files`.

Or run directly:

```sh
make test    # go test ./... -race, writes cover.out (no coverage threshold)
make lint    # golangci-lint run ./... (gofumpt is checked via .golangci.yml formatters)
make ipk     # build the OpenWrt .ipk (needs GNU tar + gzip; CI builds releases)
```

`make lint` only reports: an unformatted file fails the run but is not
rewritten. Fix formatting with `golangci-lint fmt` (or `gofumpt -w .`); the
pre-commit hooks apply both fixes for you.

Requirements: Go 1.23+ (per `go.mod`), `pre-commit`; for `make lint`, `golangci-lint`
v2.1.6 (the version pinned in `.pre-commit-config.yaml`). The coverage gate is
**≥85% over `./internal/...`** and is enforced **in CI only** (the `test` job's
"Coverage gate" step) — `make test` records coverage but does not fail on it.
Check locally with:

```sh
go test ./internal/... -coverprofile=internal.out && go tool cover -func=internal.out | tail -1
```

Don't weaken assertions to hit it.

## Conventions

- `gofumpt`-formatted, `golangci-lint` clean.
- Exported identifiers documented.
- Table-driven tests; cover the edge cases (garbage/partial input, error and
  transient paths, CRLF framing).
- Never fabricate positioning data (see ARCHITECTURE "Honest gpsd semantics").
- Never put secrets (OpenCelliD/InfluxDB tokens) in argv, logs, or commits.
- Commit messages: imperative subject; explain the *why*.
