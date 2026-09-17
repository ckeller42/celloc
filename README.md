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
# on the router — install by name from the GitHub Pages opkg feed
echo 'src/gz celloc https://ckeller42.github.io/celloc/aarch64_cortex-a53' >> /etc/opkg/customfeeds.conf
opkg update; opkg install geolocd   # ';' not '&&': opkg update exits non-zero if ANY feed fails
uci set geolocd.main.google_key='AIza...your_google_geolocation_key'
uci commit geolocd && /etc/init.d/geolocd restart
gpspipe -w <router-ip>:2947     # verify a TPV with lat/lon (wifix + tight eph)
```

`opkg` won't take a release URL directly, and GitHub *Releases* aren't a valid
opkg feed — the Pages feed above is. See [INSTALL.md](docs/INSTALL.md) for the
release-`.ipk` fallback, the HTTPS prerequisites, and auto-restoring geolocd
after a firmware flash.

Then run `geoinflux` on the Pi to push fixes to InfluxDB — see [INSTALL.md](docs/INSTALL.md).

## Google API key (gcloud)

The whole key setup is scripted with the gcloud CLI. The script links billing,
enables the Geolocation API, creates a key restricted to that API, sets a
daily request cap, and can write the key to the router over ssh:

```sh
gcloud auth login
scripts/gcloud-geolocation-key.sh -p <PROJECT> -d          # dry run
scripts/gcloud-geolocation-key.sh -p <PROJECT> -r root@<router>
```

The API gives 10,000 free requests per month. geolocd sends one request per
poll, so `wifi_interval=300` uses about 8,900 in a 31-day month. The default cap
of 320/day keeps usage inside the free tier, even if the daemon keeps
restarting. Claude Code users get the same workflow as the
[`google-geolocation-key`](.claude/skills/google-geolocation-key/SKILL.md) skill.

## License

[MIT](LICENSE).
