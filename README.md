# celloc

[![CI](https://github.com/ckeller42/celloc/actions/workflows/ci.yml/badge.svg)](https://github.com/ckeller42/celloc/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ckeller42/celloc)](https://github.com/ckeller42/celloc/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/ckeller42/celloc)](https://goreportcard.com/report/github.com/ckeller42/celloc)
[![License: MIT](https://img.shields.io/github/license/ckeller42/celloc)](LICENSE)

**WiFi + cell-tower geolocation for OpenWrt / GL-iNet routers, exposed over the gpsd protocol.**

No GPS antenna? `celloc` reads your modem's serving cell (`AT+QENG`) **and** nearby WiFi
access points, resolves them to coordinates, and serves the position on a real **gpsd** socket
(TCP `2947`) so any gpsd client can consume it. A companion uploader pushes fixes to InfluxDB.

With a **Google Geolocation API key** (or Unwired Labs paid plan), `geolocd` sends the WiFi
APs **and** the serving cell in one request; the provider fuses them to **tens of metres where
APs are well-mapped** — far better than the single-cell ~1.5 km estimate. When WiFi is too
sparse, the serving cell still anchors the fix on its own.

> ⚠️ **Accuracy:** a cell-tower fix is a coarse estimate — typically **hundreds of metres to
> a few km** (the error radius is reported honestly as the gpsd `eph`). It is *not* a GPS fix.
> WiFi accuracy improves this significantly but still needs a provider key and nearby mapped
> APs. `celloc` flags every fix as 2D (`mode=2`) with no altitude/speed so clients never
> mistake it for GNSS.

## Components

| Binary | Runs on | Role |
|---|---|---|
| `geolocd` | the router | AT + WiFi → position cache → gpsd server (`:2947`) |
| `geoinflux` | the Pi / a host | gpsd client → InfluxDB uploader |

```text
WiFi scan + modem (AT+QENG) ─▶ geolocd ─▶ provider (Google) ─▶ position ─▶ gpsd :2947
                                                                              │
                                         gpsd clients ◀─────────────────────┤
                                         geoinflux ◀────────────────────────┴─▶ InfluxDB ─▶ Grafana
```

## Status

Working end to end: `geolocd` (router daemon + gpsd server) and `geoinflux` (Pi uploader) are
implemented and tested, and the OpenWrt `.ipk` builds in CI. Docs:
[ARCHITECTURE](docs/ARCHITECTURE.md) · [INSTALL](docs/INSTALL.md) ·
[CONTRIBUTING](CONTRIBUTING.md) · [SECURITY](SECURITY.md).

## Quick start

```sh
# on the router — install from a release (or build the ipk yourself, see INSTALL.md)
opkg install https://github.com/ckeller42/celloc/releases/latest/download/geolocd_aarch64_cortex-a53.ipk
uci set geolocd.main.google_key='AIza...your_google_geolocation_key'
uci commit geolocd && /etc/init.d/geolocd restart
gpspipe -w <router-ip>:2947     # verify a TPV with lat/lon (wifix + tight eph)
```

Then run `geoinflux` on the Pi to push fixes to InfluxDB — see [INSTALL.md](docs/INSTALL.md).

## License

[MIT](LICENSE).
