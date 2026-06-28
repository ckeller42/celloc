# celloc

[![CI](https://github.com/ckeller42/celloc/actions/workflows/ci.yml/badge.svg)](https://github.com/ckeller42/celloc/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/ckeller42/celloc/badges/coverage.json)](https://github.com/ckeller42/celloc/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ckeller42/celloc)](https://github.com/ckeller42/celloc/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/ckeller42/celloc)](https://goreportcard.com/report/github.com/ckeller42/celloc)
[![License: MIT](https://img.shields.io/github/license/ckeller42/celloc)](LICENSE)

**Cell-tower geolocation for OpenWrt / GL-iNet routers, exposed over the gpsd protocol.**

No GPS antenna? `celloc` reads your modem's serving cell (`AT+QENG`), resolves it to a
coordinate via [OpenCelliD](https://opencellid.org), and serves the position on a real
**gpsd** socket (TCP `2947`) so any gpsd client can consume it. A companion uploader pushes
fixes to InfluxDB.

> ⚠️ **Accuracy:** a cell-tower fix is a coarse estimate — typically **hundreds of metres to
> a few km** (the error radius is reported honestly as the gpsd `eph`). It is *not* a GPS fix.
> `celloc` flags every fix as 2D (`mode=2`) with no altitude/speed so clients never mistake it
> for GNSS.

## Components

| Binary | Runs on | Role |
|---|---|---|
| `geolocd` | the router | AT → OpenCelliD → position cache → gpsd server (`:2947`) |
| `geoinflux` | the Pi / a host | gpsd client → InfluxDB uploader |

```text
modem (AT+QENG) ─▶ geolocd ─▶ OpenCelliD ─▶ position ─▶ gpsd :2947
                                                            │
                                       gpsd clients ◀───────┤
                                       geoinflux ◀──────────┴─▶ InfluxDB ─▶ Grafana
```

## Status

Early development. Pure core (AT/QENG parsing, OpenCelliD resolution, gpsd reports, InfluxDB
line protocol) is implemented and tested; the daemon, packaging, and uploader land next. See
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Quick start (planned)

```sh
# on the router
opkg install geolocd_*_aarch64_cortex-a53.ipk
uci set geolocd.main.key='pk.your_opencellid_key'
uci commit geolocd && /etc/init.d/geolocd restart
gpspipe -w <router-ip>:2947     # verify a TPV with lat/lon
```

## License

[MIT](LICENSE).
