# Design: WiFi-AP geolocation source for celloc

> **Amendment (2026-06-30):** Implemented with a provider-pluggable resolver — Google
> Geolocation API is the default provider (free under ~10k req/mo, WiFi included),
> Unwired Labs is optional via uci `wifi_provider`. Decision 2 (combined wifi+cell
> request) was realized as daemon-level fallback (wifi-over-cell provider) instead.

**Status:** approved (2026-06-29) · **Target:** new `geolocd` positioning source + `geoinflux`/schema support

## Problem

celloc v1 resolves position from the modem's **single serving cell** via OpenCelliD
`/cell/get`. On the buspi (Quectel RG650V in a GL-E5800 / Mudi 7) that yields a
coarse fix — OpenCelliD returns the tower's crowd-sourced centroid plus an error
radius (`range`), typically **~1.5 km** (`eph` is reported honestly as that range).
The user is near many towers and wants a materially better fix.

## Investigation (why the obvious alternatives are out)

Verified live on the router before designing:

- **Multi-tower trilateration — impossible on this hardware.** A UE decodes a
  cell's global identity (ECGI, in SIB1) only for cells it camps on. Neighbours are
  identified by **PCI** only:
  - `AT+QENG="neighbourcell"` → `EARFCN, PCI, RSRQ, RSRP, RSSI` (no LAC/CID).
  - `AT+QCAINFO` (carrier-aggregation serving carriers) → PCI only, and same site.
  - `AT+QENG=?` → only `("servingcell","neighbourcell")`; no neighbor-CGI read.
  PCI is not globally unique, so neighbours cannot be looked up in a cell DB. We have
  exactly **one** resolvable global cell ID (the LTE serving cell).
- **GNSS — structurally unavailable.** The RG650V has a multi-constellation GNSS
  receiver (`AT+QGPS`), but enabling it shows **0 satellites / no fix**: the
  GL-E5800 has only 2× TS-9 *cellular* ports and 8 internal antennas (6 cellular,
  2 WiFi) — **no GPS antenna port**. No antenna can be attached.
- **Modem-impact (for the cell path) is negligible** — AT runs on a dedicated SMD
  control channel (`/dev/smd9`), independent of the rmnet/MBIM data path; 10× rapid
  `QENG` reads caused 0% packet loss.

**Conclusion:** the only path to better accuracy on this hardware is **WiFi-AP
geolocation**.

### WiFi-scan impact check (measured)

Both router radios are AP-only (`wlan0` 2.4 GHz, `wlan1` 5 GHz); WAN is cellular, so
a scan never touches the uplink. Forced fresh active scans while pinging a wired-grade
Linux client (the Pi, on 5 GHz):

| scan | client packet loss | latency avg→max | APs found |
|---|---|---|---|
| baseline (no scan) | 10% (jittery link) | 6 ms / 8 ms | — |
| `wlan0` (2.4 GHz, other radio) | **0%** | 11 ms / 43 ms | 59 |
| `wlan1` (5 GHz, client's radio) | **0%** | 19 ms / 48 ms | 56 |

No disconnects, no packet loss — only a brief tens-of-ms latency bump during the
~1–5 s scan. 56–98 APs visible (dense urban) → WiFi geolocation should give
**tens of metres**.

## Goals / non-goals

**Goals:** add a WiFi-AP positioning source that outranks cell; preserve honest gpsd
semantics; keep the existing Influx schema working; TDD with the pure/IO split;
configurable and privacy-respecting.

**Non-goals:** GNSS support (no antenna); multi-tower cell trilateration (impossible);
client-side scanning on the Pi (the radios live on the router).

## Architecture

A new `source.Source` on the router (`geolocd`), ranked **above** cell.
`source.Select(ctx, wifiSource, cellSource)` already provides priority fallback:
WiFi wins when it has a fix; cell covers when WiFi can't (too few APs, API failure).

Three new packages mirror the existing cell stack's pure/IO split:

| Package | Kind | Responsibility |
|---|---|---|
| `internal/wifiscan` | pure parse + IO scanner | parse `iw dev <if> scan` → `[]AP{BSSID, SignalDBm, SSID}`; IO runs the scan behind an injected `Exec` (as in `atrun`) |
| `internal/unwiredlabs` | pure `ParseResponse` + request builder + IO `Client` (injected `Doer`) | `process.php` JSON → `Location{Lat,Lon,Accuracy}` + `Status{OK,Auth,RateLimited,Server,NotFound}` |
| `internal/source/wifi` | IO compose | scanner + resolver + last-good cache/staleness + classified, throttled logging (reuses the cell source's pattern) |

### Data flow (one WiFi cycle)

```text
iw scan ─▶ wifiscan ─▶ [APs] ─▶ unwiredlabs process.php ─▶ Location ─▶ Fix(source=wifi) ─▶ cache ─▶ gpsd :2947 / influx
                        (+ serving cell included as in-request fallback)
```

## Decisions

1. **Resolver = Unwired Labs LocationAPI** (`process.php`), not OpenCelliD
   `/cell/get`. The same OpenCelliD `pk` token authenticates it. Region endpoint is
   configurable; **default `eu1`** (`https://eu1.unwiredlabs.com/v2/process.php`).
2. **Single request carries WiFi APs *and* the serving cell**
   (`{token, wifi:[{bssid,signal}], cells:[{lac,cid,mcc,mnc,signal}], address:0}`).
   WiFi drives accuracy; the cell is a built-in fallback within the same call.
3. **Scan interface: `wlan0` (2.4 GHz) by default** (most APs; impact-tested at 0%
   loss). `wifi_iface` accepts a space-separated list to scan multiple radios
   (e.g. `wlan0 wlan1`); results are merged before the query.
4. **Cadence: separate `wifi_interval`, default 300 s** (parked van; minimizes scan
   blips). The cell poll stays at 60 s.
5. **Privacy:** honor the `_nomap` SSID opt-out (skip APs whose SSID ends `_nomap`);
   skip hidden/empty SSIDs for the opt-out check; require **≥ `wifi_min_aps` (default 2)**
   APs before querying.
6. **Coexist, not replace** — the cell source remains as fallback; WiFi is additive.

## Fix / gpsd / Influx semantics

- A WiFi fix is emitted as TPV **`mode 2`**, `epx=epy=eph=` the LocationAPI
  `accuracy`, with **no** `alt`/`speed`/`track` (same honesty rule as cell).
  `Fix.Source = "wifi"`.
- `Fix` gains an optional `APCount int` (APs used) for telemetry; the cell-only
  fields (`Radio/MCC/MNC/CID/TAC`) stay zero for a WiFi fix.
- **gpsd TPV** carries the source type so `geoinflux` writes the correct line. The
  existing `cellfix` object is emitted only for cell fixes; a WiFi fix carries a
  minimal `wifix` object `{ap_count}` (ignored by standard gpsd clients).
- **Influx:** `FixLine` branches on `Source`:
  - cell → unchanged, **byte-identical** to today.
  - wifi → `geo,source=wifi lat=,lon=,range_m=Ni,ap_count=Ni`.
  Existing `lat`/`lon`/`range_m` Grafana panels keep working for both.

## Config (uci `geolocd.main`)

New options (token reuses the existing `key`):

| Option | Default | Meaning |
|---|---|---|
| `wifi_enable` | `1` | enable the WiFi source |
| `wifi_iface` | `wlan0` | scan interface(s), space-separated for multiple radios |
| `wifi_interval` | `300` | WiFi scan/resolve cadence (seconds) |
| `wifi_min_aps` | `2` | minimum APs before querying LocationAPI |
| `ula_endpoint` | `eu1` | LocationAPI region subdomain |

## Security / privacy

- BSSIDs of nearby networks are sent to Unwired Labs (LocationAPI). Documented in
  `SECURITY.md`; `:2947` stays LAN-only.
- `_nomap` opt-out honored.
- Token lives in uci (`0600`), read in-process — never in argv. The on-device call to
  `unwiredlabs.com` is the daemon's own egress (no agent/credential-domain concern).

## TDD matrix (tests first)

- **wifiscan:** multi-BSS parse; BSSID/signal/SSID extraction; `_nomap` + hidden
  filter; malformed/CRLF/empty input; signal sign handling.
- **unwiredlabs:** `ParseResponse` ok / `{status:"error"}` / 429 / 401 / 5xx /
  bad-json → correct `Status`; request-body golden (token, wifi list, cell fallback,
  `address:0`); region BaseURL override.
- **source/wifi:** scan→resolve→Fix happy path; `< wifi_min_aps` → ErrNoFix;
  resolver auth/rate-limited → classified + throttled log; cache served then stale;
  scanner error → no panic; **selection: wifi outranks cell**.
- **influx:** wifi `FixLine` golden; cell line unchanged (regression).
- **gpsd:** wifi TPV (source carried, no alt/speed, `wifix` present);
  `geoinflux` maps a wifi TPV back to the wifi Influx line.
- Coverage gate ≥ 85% over `./internal/...` (unchanged).

## Milestones

1. **M1 — pure core + tests.** `wifiscan` parse, `unwiredlabs` parse/request,
   `influx` wifi line, `Fix.APCount` + selection wiring. ~100% on pure code.
2. **M2 — IO + wiring.** `wifiscan` scanner (`iw` exec), `unwiredlabs` client,
   `source/wifi`; `cmd/geolocd` builds the WiFi source and selects wifi-over-cell;
   on-router verify (`gpspipe -w` shows a tighter `eph`).
3. **M3 — uploader/docs/release.** `geoinflux` wifi line, uci defaults + INSTALL/
   SECURITY updates (BSSID-privacy note), cut a release.

## Verification

- `go test ./... -race`; coverage gate ≥ 85%.
- On router: `gpspipe -w <router>:2947` shows `TPV mode=2` with `eph` ≪ 1548 when
  WiFi resolves; falls back to the cell fix (eph ~1548) when `< wifi_min_aps`.
- Grafana geomap shows the tighter WiFi points; `source=wifi` points carry `ap_count`.
