# Architecture

celloc turns a cellular modem's serving-cell identity into a position and serves
it over the gpsd protocol, with an optional uploader to InfluxDB.

```text
                         ROUTER (geolocd)                              PI (geoinflux)
 iw scan ──▶ wifiscan ─┐
 modem ──AT QENG──▶ qeng ─▶ source/wifi ─▶ provider ─▶ position cache
                       (google/unwiredlabs, HTTP)          │             │
                                                           ▼             │
                                                   gpsd server :2947 ────┼─▶ gpsd client ─▶ influx.Writer ─▶ InfluxDB ─▶ Grafana
                                                   (TPV/SKY JSON)        │
                                          any gpsd client (gpspipe…) ◀───┘
```

## Pure vs I/O split

Mirroring the seed project, parsing/marshaling is pure (no network, filesystem,
env, or wall-clock) so it is exhaustively table-testable; I/O sits behind small
injected interfaces.

| Package | Kind | Responsibility |
|---|---|---|
| `internal/qeng` | pure | parse `AT+QENG="servingcell"` → cells (skipping a leading `"servingcell",<state>` prefix); pick the geolocatable cell — LTE (also the NSA anchor), else NR5G-SA |
| `internal/gpsd` | pure reports + I/O `Server`/`Client` | gpsd TPV/SKY/VERSION/POLL |
| `internal/source` | pure | `Source` interface + `Fix`; priority `Select` |
| `internal/source/cell` | I/O | `ServingCellReader` (AT+qeng → serving cell for blending) |
| `internal/geoloc` | pure | neutral `Location{Lat,Lon,Accuracy}` shared by resolvers |
| `internal/wifiscan` | pure parse + I/O scanner | `iw dev <if> scan` → `[]AP` |
| `internal/unwiredlabs` | pure `ParseResponse` + I/O `Client` | LocationAPI `process.php` |
| `internal/google` | pure `ParseResponse` + I/O `Client` | Google `geolocate` |
| `internal/source/wifi` | I/O (compose) | scan + resolve + cache, behind a neutral `Resolver` |
| `internal/atrun` | I/O (`Exec`) | run AT via `gl_modem` / `ubus` |
| `internal/influx` | pure `FixLine`/`StatusLine` + I/O `Writer` (`Doer`) | `geo` + `geo_status` line protocol + write (`precision=ns`) |
| `internal/uciconf` | pure parse + I/O load | read `/etc/config/geolocd` via uci |
| `internal/ratelog` | helper (injectable clock) | throttle repeated log lines (per key, once a minute by default) so a persistent failure doesn't flood the router's log buffer |
| `cmd/geolocd` | wiring | uci → source → poll loop → gpsd server |
| `cmd/geoinflux` | wiring | gpsd client → InfluxDB (reconnect, debounce) |

## Pluggable sources (GNSS-ready)

`source.Source` is a small interface (`Name`, `Fix`). `geolocd` runs a **single
WiFi source** that blends the serving cell into the provider request (via
`cell.ServingCellReader`); the resolver is provider-pluggable (`google` default,
`unwiredlabs` optional) selected by uci `wifi_provider`. A future GNSS source
(`AT+QGPS`, once an antenna exists) just implements `source.Source` — no change
to the poll loop or server.

## Honest gpsd semantics

Neither a WiFi nor a cell fix is a GPS fix. It is emitted as TPV `mode=2` with
`eph==epx==epy` set to the provider's accuracy/error radius, and **no**
`alt`/`speed`/`track`. A no-fix or stale position is `mode=0` with no coordinates.
A WiFi-dominant fix carries a non-standard `wifix` object (`ap_count`); a
cell-only fix carries `cellfix` (mcc/mnc/cid/tac/radio). Both are ignored by
standard gpsd clients and read by `geoinflux` to tag the InfluxDB point.

## Data flow (one cycle)

1. `geolocd` poll loop (every `wifi_interval`) calls the WiFi `source.Fix`:
   `wifiscan` runs `iw scan`, `cell.ServingCellReader` reads the serving cell
   (`atrun`+`qeng`), and the provider (`google`/`unwiredlabs`) resolves the WiFi
   APs + cell together. Each call is bounded by `pollTimeout` (60 s), and each
   AT subprocess (`gl_modem`/`ubus`) by `atrun.ExecTimeout` (20 s), so a hung
   modem helper or provider can't stall the loop. A failed or timed-out poll
   falls back to the source's cached fix while it is younger than `StaleAfter`,
   and otherwise serves no-fix (`mode=0`).
2. The fix is cached (served until `StaleAfter`) and stored atomically.
3. The gpsd `Server` streams `TPVFromFix` to watching clients every `-stream`
   interval (default 1 s) and answers `?POLL`/`?WATCH`/`?VERSION`.
4. `geoinflux` (Pi) watches the socket, converts each TPV back to a `Fix`
   (`FixFromTPV`), and for `mode>=2` writes `influx.FixLine` (measurement
   `geo`, stamped with the fix's own time) to the configured bucket (default
   `buspi`), at most once per `-min-interval`.
5. Alongside, `geoinflux` writes an `influx.StatusLine` heartbeat (measurement
   `geo_status`: `mode`, `fix_age_s`, `connected`) at most once per
   `-min-interval`, and immediately when the gpsd connection drops or comes
   back — so a dead uploader, an unreachable router, no fix and a stale fix
   are distinguishable in InfluxDB. Schemas: [INSTALL](INSTALL.md#what-gets-written).
