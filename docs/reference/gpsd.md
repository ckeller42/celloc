# gpsd output

`geolocd` implements the JSON subset of the gpsd protocol (version 3.14) that common clients
(`gpspipe -w`, `cgps`, `gps.py`) need. Source: `internal/gpsd`. Every message is one JSON object
terminated by CRLF. The coordinates below are made-up example values.

## Connection and commands

On connect the server sends a `VERSION` report at once. It then accepts commands, one per line
(a trailing `;` is tolerated). Unknown commands are ignored.

| Command | Reply |
| --- | --- |
| `?WATCH={"enable":true,"json":true}` | `DEVICES`, then `WATCH`, then one `TPV` and `SKY` immediately. The server then streams `TPV` and `SKY` every `-stream` interval (default 1 s). `{"enable":false}` stops the stream |
| `?POLL` | One `POLL` report with the latest `TPV` and `SKY` |
| `?VERSION` | `VERSION` |
| `?DEVICES` | `DEVICES` |

```json
{"class":"VERSION","release":"celloc-dev","rev":"","proto_major":3,"proto_minor":14}
{"class":"DEVICES","devices":[{"class":"DEVICE","path":"cell0","driver":"celloc","activated":"2025-01-01T12:00:00.000Z"}]}
{"class":"WATCH","enable":true,"json":true,"nmea":false}
```

`release` is `celloc-` plus the build version. The single synthetic device is `cell0`, driver
`celloc`.

## TPV

A time-position-velocity report. Optional fields are omitted when unknown, never zero-filled.

| Field | Present when | Meaning |
| --- | --- | --- |
| `class` | always | `"TPV"` |
| `device` | always | `"cell0"` |
| `mode` | always | `2` = 2D fix, `0` = no fix. Never `3` |
| `time` | fix | UTC, ISO 8601 with milliseconds (`2025-01-01T12:00:00.000Z`) |
| `lat`, `lon` | fix | WGS84 degrees |
| `eph`, `epx`, `epy` | fix | The provider's accuracy radius in metres. All three are equal |
| `wifix` | WiFi fix | Non-standard. `{"ap_count": n}`, the number of APs sent to the provider |
| `cellfix` | cell fix | Non-standard. `radio`, `mcc`, `mnc`, `cid`, `tac`, `range` (`eph` truncated to an integer) |

There is never `alt`, `speed` or `track`: they are unknown, and are not faked. A fix carries
`wifix` when its source is `wifi`, and `cellfix` when its source is `cell` (or cell identifiers are
present). Standard clients ignore both objects; `geoinflux` uses them to tag InfluxDB points.

```json
{"class":"TPV","device":"cell0","mode":2,"time":"2025-01-01T12:00:00.000Z","lat":12.3456,"lon":65.4321,"epx":35,"epy":35,"eph":35,"wifix":{"ap_count":7}}
{"class":"TPV","device":"cell0","mode":2,"time":"2025-01-01T12:00:00.000Z","lat":12.3456,"lon":65.4321,"epx":1500,"epy":1500,"eph":1500,"cellfix":{"radio":"LTE","mcc":1,"mnc":1,"cid":12345,"tac":100,"range":1500}}
{"class":"TPV","device":"cell0","mode":0}
```

The last line is the no-fix report: there is no position, never `0,0`. It is served when no fix has
been resolved yet, or when the last fix is older than `StaleAfter` (twice `wifi_interval`, at least
2 minutes).

## SKY

celloc has no satellites, so `SKY` is always empty and carries no DOP values.

```json
{"class":"SKY","device":"cell0","satellites":[]}
```

## POLL

```json
{"class":"POLL","time":"2025-01-01T12:00:01.000Z","active":1,"tpv":[{"class":"TPV","device":"cell0","mode":0}],"sky":[{"class":"SKY","device":"cell0","satellites":[]}]}
```

## Client behaviour in celloc

The bundled client (`gpsd.Client`, used by `geoinflux`) sends `?WATCH={"enable":true,"json":true};`
and reads only `TPV` reports. `FixFromTPV` turns a TPV back into a fix. A TPV claiming `mode>=2`
without both `lat` and `lon` is treated as no fix.

See also: [Architecture](../ARCHITECTURE.md#8-crosscutting-concepts) for why the semantics are
strict, and [InfluxDB schema](influxdb.md) for how a TPV is stored.
