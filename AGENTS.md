# AGENTS.md

Rules for agents and contributors working on celloc. `CLAUDE.md` only imports
this file.

## What this is

WiFi + cell-tower geolocation for OpenWrt / GL.iNet routers, served over the
gpsd protocol. Two Go binaries, standard library only (no third-party modules):

| Binary | Runs on | Role |
| --- | --- | --- |
| `geolocd` | the GL.iNet router | modem `AT+QENG` + WiFi scan → provider (Google / Unwired Labs) → gpsd server on `:2947` |
| `geoinflux` | the Pi / a host | gpsd client → InfluxDB uploader |

## Layout

| Path | What |
| --- | --- |
| `cmd/geolocd`, `cmd/geoinflux` | the two entrypoints |
| `internal/` | packages: `qeng`, `wifiscan`, `atrun`, `source` (+ `cell`, `wifi`), `google`, `unwiredlabs`, `gpsd`, `influx`, `uciconf`, `ratelog`, `geoloc` |
| `packaging/openwrt/` | `build-ipk.sh` + procd init script and uci config for the `.ipk` |
| `pi/` | `geoinflux.service` + env example |
| `scripts/gcloud-geolocation-key.sh` | Google Geolocation API key setup |
| `docs/` | the Sphinx site: `index.rst`, `getting-started.md`, `INSTALL.md` (how-to), `reference/`, `ARCHITECTURE.md`, `conf.py`, `requirements.txt`; `docs/superpowers/` = design spec + plan, local-only (gitignored) |

## Build, test, lint

```sh
pipx install pre-commit && pre-commit install   # installs pre-commit AND pre-push hooks
make test    # go test ./... -race with coverage profile
make lint    # golangci-lint run ./...
make ipk     # static arm64 geolocd packaged as an OpenWrt .ipk (GNU tar + gzip)
pre-commit run --all-files                      # what CI's pre-commit job runs
```

- `.pre-commit-config.yaml` is the single source of truth for tool versions:
  golangci-lint **v2.1.6** (built from the pinned tag, same in CI), gitleaks,
  markdownlint-cli2 (config: `.markdownlint-cli2.jsonc`), pre-commit-hooks.
  A standalone `golangci-lint` used by `make lint` should be the same version.
- `go test ./...` runs on **pre-push**; `go build` of both entrypoints on every commit.
- Coverage gate: **≥85% over `./internal/...`**, enforced in CI only (see
  CONTRIBUTING for the local command). Never weaken assertions to reach it.
- Go version comes from `go.mod` (CI uses `go-version-file: go.mod`).

## Rules

- **Pure vs I/O split.** Parsing/marshaling packages stay free of network,
  filesystem, env and wall-clock access; inject those (docs/ARCHITECTURE.md).
- **Honest gpsd semantics — never fabricate positioning data.** A WiFi or cell
  fix is TPV `mode=2` with `eph==epx==epy` from the provider's accuracy and no
  `alt`/`speed`/`track`; no fix or a stale fix is `mode=0` with no coordinates.
  `geoinflux` writes no point for a TPV that is not a celloc fix.
- **Secrets** (Google / Unwired Labs keys, InfluxDB token) never go in argv,
  logs or commits. Use placeholders (`<GOOGLE_KEY>`, `AIza...`).
- **No deployment specifics.** No real keys, IMEIs/ICCIDs, MACs, SSIDs,
  coordinates, hostnames or IPs. Allowed: the GL.iNet default `192.168.8.1` /
  `192.168.8.0/24` and RFC 5737 / RFC 2606 documentation placeholders
  (`.coderabbit.yaml` has the full list).
- Table-driven tests covering garbage/partial input, error and transient
  paths, CRLF framing. `gofumpt`-formatted, exported identifiers documented.

## Workflow

Branch off `main`, test first, open a PR. CI must be green and every
CodeRabbit (and Qodo, `.pr_agent.toml`) review thread resolved before merge.

For the Google Geolocation API key (create, restrict, cap, rotate, write to the
router) use the `google-geolocation-key` skill in `.claude/skills/`.

## Docs contract

- **Four groups, one home per fact.** `docs/index.rst` has the toctrees Getting started, How-to
  guides, Reference, Explanation. A page belongs to exactly one; other pages link to it, never copy it.
- **One architecture page.** `docs/ARCHITECTURE.md` is structured by the 12 arc42 sections and drawn
  with C4-styled Mermaid (flowcharts and sequence diagrams; classDef colours person `#08427b`,
  system `#1168bd`, container `#438dd5`, external `#999999`). Mermaid lint: no `;`, no bare `&`, `<`
  or `>` in free text, no `<-->` (use `---`), no `:` in a loop or opt label. Diagram text must match
  the code (`cmd/`, `internal/`).
- **The build is gated.** `sphinx-build -b html -W docs docs/_build/html` (deps pinned in
  `docs/requirements.txt`, Sphinx + Furo + myst-parser + sphinxcontrib-mermaid) runs as the `docs`
  CI job. It is not a required check; never add it to the required list.
- **Examples are synthetic.** Docs follow the no-deployment-specifics rule: placeholders and made-up
  coordinates only.
- **Publishing.** GitHub Pages is the opkg feed's host, and a second Pages deploy would replace its
  artifact, so `pages-feed.yml` is the only Pages deploy: it builds the site into `public/docs/` next
  to the feed (`public/<arch>/`, path unchanged) and runs after each release. Do not change repo
  settings from a PR.
- The cross-repo concept is `DOCUMENTATION.md` in `ckeller42/buspi-config`.
