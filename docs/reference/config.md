# Configuration

## `geolocd`: uci options

`geolocd` reads `uci -N show geolocd` at start (section `geolocd.main`, file
`/etc/config/geolocd`). Source: `internal/uciconf`. Unknown options are ignored. An unparsable or
non-positive number, or an empty string, leaves the default in place. If uci itself fails, `geolocd`
logs it and continues with the defaults.

| Option | Default | Description |
| --- | --- | --- |
| `wifi_enable` | `1` | Enable the WiFi source (`1`/`true`, or `0`/`false`). With `0` there is no source and `geolocd` exits |
| `wifi_provider` | `google` | Provider: `google` or `unwiredlabs`. Any other value makes `geolocd` exit |
| `google_key` | _(none)_ | Google Geolocation API key. Required for `google`; `geolocd` exits without it |
| `key` | _(none)_ | Unwired Labs (OpenCelliD) token. Required for `unwiredlabs` |
| `ula_endpoint` | `eu1` | Unwired Labs region, used as `https://<endpoint>.unwiredlabs.com` |
| `wifi_iface` | `wlan0` | Space-separated WiFi interfaces to scan. Results are merged by BSSID |
| `wifi_interval` | `300` | Seconds between scan and resolve cycles |
| `wifi_min_aps` | `2` | Minimum APs before they are sent to the provider. Below it the cell alone resolves |
| `listen` | `:2947` | gpsd socket bind address |
| `runner` | `glmodem` | AT runner: `glmodem` or `ubus`. Any other name uses `glmodem` |
| `bus` | `cpu` | Modem bus. `gl_modem -B <bus>`, or the ubus object `modem.<BUS>.AT` |

The installed default file is `packaging/openwrt/files/geolocd.config`. Secrets (`google_key`,
`key`) are never logged and never passed on the command line.

Derived values: the cached fix is served for `2 × wifi_interval`, at least 2 minutes
(`StaleAfter`). Each poll is limited to 60 s and each AT or `iw` subprocess to 20 s.

## `geolocd`: flags

| Flag | Default | Description |
| --- | --- | --- |
| `-stream` | `1s` | How often the gpsd server streams a TPV to watching clients |

The procd service starts `geolocd` without flags, so the default applies. The
stream cadence is independent of `wifi_interval`: clients receive the cached
fix every `-stream`, while the provider is queried once per `wifi_interval`.

## `geoinflux`: flags and environment

Every setting except the token can also be given as a flag; a flag overrides its
environment variable, which overrides the built-in default. Source: `cmd/geoinflux`. The shipped
example is `pi/geoinflux.env.example`, loaded by `pi/geoinflux.service`.

| Flag | Env var | Default | Description |
| --- | --- | --- | --- |
| `-gpsd` | `GPSD_ADDR` | `192.168.8.1:2947` | Router gpsd address |
| `-influx-url` | `INFLUX_URL` | `http://localhost:8086` | InfluxDB base URL |
| `-org` | `INFLUX_ORG` | `home` | InfluxDB org |
| `-bucket` | `INFLUX_BUCKET` | `buspi` | InfluxDB bucket |
| `-min-interval` | `UPLOAD_MIN_INTERVAL` | `30s` | Minimum time between `geo` writes (and between `geo_status` heartbeats) |
| — | `INFLUXDB_TOKEN` | _(required)_ | InfluxDB write token; env only, `geoinflux` exits if unset |

`UPLOAD_MIN_INTERVAL` takes a Go duration (`30s`, `2m`); an unparsable value
silently falls back to the default. The `192.168.8.1` default is the GL-iNet factory router address.
