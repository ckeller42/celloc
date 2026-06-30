# Architecture

celloc turns a cellular modem's serving-cell identity into a position and serves
it over the gpsd protocol, with an optional uploader to InfluxDB.

```text
                         ROUTER (geolocd)                         PI (geoinflux)
 modem ──AT QENG──▶ atrun ─▶ qeng ─▶ opencellid ─▶ source/cell ─▶ position cache
                                     (HTTP lookup)        │             │
                                                          ▼             │
                                                   gpsd server :2947 ───┼─▶ gpsd client ─▶ influx.Writer ─▶ InfluxDB ─▶ Grafana
                                                   (TPV/SKY JSON)       │
                                          any gpsd client (gpspipe…) ◀──┘
```

## Pure vs I/O split

Mirroring the seed project, parsing/marshaling is pure (no network, filesystem,
env, or wall-clock) so it is exhaustively table-testable; I/O sits behind small
injected interfaces.

| Package | Kind | Responsibility |
|---|---|---|
| `internal/qeng` | pure | parse `AT+QENG="servingcell"` → cells; pick the LTE anchor |
| `internal/opencellid` | pure `ParseResponse` + I/O `Client` (`Doer`) | resolve cell → lat/lon |
| `internal/gpsd` | pure reports + I/O `Server`/`Client` | gpsd TPV/SKY/VERSION/POLL |
| `internal/source` | pure | `Source` interface + `Fix`; priority `Select` |
| `internal/source/cell` | I/O (compose) | runner+qeng+resolver, last-good cache + staleness |
| `internal/geoloc` | pure | neutral `Location{Lat,Lon,Accuracy}` shared by resolvers |
| `internal/wifiscan` | pure parse + I/O scanner | `iw dev <if> scan` → `[]AP` |
| `internal/unwiredlabs` | pure `ParseResponse` + I/O `Client` | LocationAPI `process.php` |
| `internal/google` | pure `ParseResponse` + I/O `Client` | Google `geolocate` |
| `internal/source/wifi` | I/O (compose) | scan + resolve + cache, behind a neutral `Resolver` |
| `internal/atrun` | I/O (`Exec`) | run AT via `gl_modem` / `ubus` |
| `internal/influx` | pure `FixLine` + I/O `Writer` (`Doer`) | line protocol + write |
| `internal/uciconf` | pure parse + I/O load | read `/etc/config/geolocd` via uci |
| `cmd/geolocd` | wiring | uci → source → poll loop → gpsd server |
| `cmd/geoinflux` | wiring | gpsd client → InfluxDB (reconnect, debounce) |

## Pluggable sources (GNSS-ready)

`source.Source` is an interface; `source.Select(ctx, sources...)` returns the
first source with a fix. The WiFi source outranks cell via
`source.Select(wifi, cell)`, and its resolver is provider-pluggable (`google`
default, `unwiredlabs` optional) selected by uci `wifi_provider`. A GNSS source
(reading `AT+QGPS` once an antenna exists) can also be added and listed before
cell or WiFi — no change to the daemon or server.

## Honest gpsd semantics

A cell fix is **not** a GPS fix. It is emitted as TPV `mode=2` with
`eph==epx==epy` set to the OpenCelliD error radius (~hundreds of m to km), and
**no** `alt`/`speed`/`track`. A no-fix or stale position is `mode=0` with no
coordinates. The non-standard `cellfix` object in TPV (mcc/mnc/cid/tac/radio) is
ignored by standard gpsd clients and read by `geoinflux` to preserve the
InfluxDB schema.

## Data flow (one cycle)

1. `geolocd` poll loop calls `source/cell.Fix`: `atrun` runs the QENG AT command,
   `qeng` decodes the LTE anchor, `opencellid.Client` resolves it.
2. The fix is cached (served until `StaleAfter`) and stored atomically.
3. The gpsd `Server` streams `TPVFromFix` to watching clients every `stream`
   interval and answers `?POLL`/`?WATCH`/`?VERSION`.
4. `geoinflux` (Pi) watches the socket, converts each `mode>=2` TPV back to a
   `Fix` (`FixFromTPV`), and writes `influx.FixLine` to the `buspi` bucket.
