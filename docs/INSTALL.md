# Installing celloc

## Router daemon (`geolocd`) on OpenWrt / GL-iNet

### 1. Install the package

#### Method A — opkg feed (recommended): install by name

celloc publishes a GitHub Pages **opkg feed**, so the router can install (and
later upgrade) `geolocd` by name — no manual `.ipk` copying. Add the feed once,
then install:

```sh
echo 'src/gz celloc https://ckeller42.github.io/celloc/aarch64_cortex-a53' \
  >> /etc/opkg/customfeeds.conf
opkg update; opkg install geolocd
```

Use `;`, not `&&`: `opkg update` exits non-zero if **any** configured feed fails
to download (a flaky vendor feed is enough), and `&&` would then skip the install
even though the celloc feed was fetched fine.

**Signature checking:** the celloc feed is unsigned. GL-iNet 23.05 firmware has
no `check_signature` in `/etc/opkg.conf`, so it installs as-is; stock OpenWrt may
enable it — check with `grep check_signature /etc/opkg.conf`.

> **HTTPS support required.** opkg needs TLS to fetch from `https://`. GL-iNet
> firmware ships it, but if `opkg update` fails on the feed URL, install the TLS
> bits from the stock (HTTP) OpenWrt feed first:
>
> ```sh
> opkg update
> opkg install libustream-mbedtls ca-bundle   # or: ca-certificates
> ```
>
> Check first with
> `opkg list-installed | grep -E 'libustream|ca-bundle|ca-certificates'`.

`opkg install geolocd` resolves the newest version in the feed; the feed keeps
older versions too, so a specific pin stays installable. Upgrading later is just
`opkg update; opkg upgrade geolocd`.

#### Method B — download a release `.ipk` (fallback / offline)

Grab `geolocd_<version>_aarch64_cortex-a53.ipk` from the
[releases](https://github.com/ckeller42/celloc/releases) (or build it — below) and:

```sh
scp -O geolocd_*_aarch64_cortex-a53.ipk root@<router>:/tmp/
ssh root@<router> 'opkg install /tmp/geolocd_*.ipk'
```

Either way the package installs the binary, a procd service (enabled + started),
and a default `/etc/config/geolocd` (preserved across package upgrades as a
conffile).

### 2. Set your provider key

`geolocd` resolves position through a geolocation **provider** — **Google by
default** — sending the WiFi scan and the modem's serving cell together. Create a
**Google Geolocation API key** (full steps in [WiFi geolocation](#wifi-geolocation)
below), then:

```sh
uci set geolocd.main.google_key='AIza...'
uci commit geolocd
/etc/init.d/geolocd restart
```

The key lives only in `/etc/config/geolocd` (and is never passed on the command
line, so it won't show up in `ps`). OpenCelliD is no longer used by default; it is
only relevant for the optional Unwired Labs provider below.

### 3. Verify

```sh
gpspipe -w <router-ip>:2947     # expect a TPV with mode:2, lat/lon, a wifix object,
                                # and a tight eph (tens of m where APs are mapped)
# or:
logread -e geolocd
```

## WiFi geolocation

WiFi geolocation is **on by default** (`wifi_enable '1'`). Each cycle `geolocd`
scans nearby APs and reads the serving cell, then sends **both** to the provider in
one request — WiFi drives the fine fix and the cell anchors it when APs are sparse.

### Default provider: Google

`geolocd` uses the
[Google Geolocation API](https://developers.google.com/maps/documentation/geolocation/overview)
by default. It needs a **Google Geolocation API key**:

1. Create a Google Cloud project and enable the **Geolocation API**.
2. Enable billing (the free tier covers 10,000 requests/month; the default
   5-minute poll interval uses 288 requests/day, i.e. 8,928 in a 31-day month).
3. Create an API key (restrict it to the Geolocation API).
4. Set it on the router:

```sh
uci set geolocd.main.google_key='AIza...'
uci commit geolocd && /etc/init.d/geolocd restart
```

### Alternative provider: Unwired Labs

Switch with:

```sh
uci set geolocd.main.wifi_provider='unwiredlabs'
uci commit geolocd && /etc/init.d/geolocd restart
```

This reuses the OpenCelliD `key` and `ula_endpoint` (e.g. `eu1`).

> **Note:** Unwired Labs WiFi geolocation requires a **paid LocationAPI plan**.
> The free OpenCelliD tier returns "WiFi access not enabled". Cell still works
> on the free tier regardless.

### WiFi options

| Option | Default | Description |
|---|---|---|
| `wifi_enable` | `1` | Enable WiFi geolocation (`0` to disable) |
| `wifi_provider` | `google` | Provider: `google` or `unwiredlabs` |
| `google_key` | _(none)_ | Google Geolocation API key (required for Google) |
| `wifi_iface` | `wlan0` | Space-separated list of WiFi interfaces to scan |
| `wifi_interval` | `300` | Seconds between WiFi scans |
| `wifi_min_aps` | `2` | Minimum visible APs required before querying provider |
| `ula_endpoint` | `eu1` | Unwired Labs region (only used with `unwiredlabs`) |

### Disable WiFi geolocation

```sh
uci set geolocd.main.wifi_enable='0'
uci commit geolocd && /etc/init.d/geolocd restart
```

### Verify

```sh
gpspipe -w <router-ip>:2947
```

When WiFi resolves, expect a `TPV mode=2` with a `wifix` object and an `eph`
far below the ~1.5 km cell radius (tens of metres where APs are well-mapped).
If WiFi is not resolving, `logread -e geolocd` will show the reason.

### Command-line flags

All configuration comes from uci; `geolocd` has a single flag:

| Flag | Default | Description |
|---|---|---|
| `-stream` | `1s` | How often the gpsd server streams a TPV to watching clients |

The procd service starts `geolocd` without flags, so the default applies. The
stream cadence is independent of `wifi_interval`: clients receive the cached
fix every `-stream`, while the provider is queried once per `wifi_interval`.

## Pi uploader (`geoinflux`)

`geoinflux` is a gpsd client that reads fixes from `geolocd` on the router and
writes them to InfluxDB. Run it on the Pi (or any host that can reach both the
router's `:2947` and InfluxDB).

### 1. Install the binary

Download the build for your arch from the
[releases](https://github.com/ckeller42/celloc/releases) and install it at the
path the service expects (`/usr/local/bin/geoinflux`):

```sh
# pick the asset matching your arch: linux_arm64 (64-bit Pi), linux_armv7
# (32-bit Pi), or linux_amd64 (x86 host)
sudo install -m 0755 geoinflux_*_linux_arm64 /usr/local/bin/geoinflux
```

### 2. Configure

Create the env file (`0600`, it holds the InfluxDB token) from the example:

```sh
sudo install -d /etc/buspi
sudo cp geoinflux.env.example /etc/buspi/geo.env
sudo chmod 600 /etc/buspi/geo.env
sudo "${EDITOR:-vi}" /etc/buspi/geo.env   # set GPSD_ADDR, INFLUX_URL, token, org, bucket
```

`GPSD_ADDR` is the router's gpsd socket, e.g. `192.168.8.1:2947` (or its
Tailscale IP). The token is read from the environment only — never passed on the
command line. See [SECURITY.md](../SECURITY.md).

Every setting except the token can also be given as a flag; a flag overrides its
environment variable, which overrides the built-in default:

| Flag | Env var | Default | Description |
|---|---|---|---|
| `-gpsd` | `GPSD_ADDR` | `192.168.8.1:2947` | Router gpsd address |
| `-influx-url` | `INFLUX_URL` | `http://localhost:8086` | InfluxDB base URL |
| `-org` | `INFLUX_ORG` | `home` | InfluxDB org |
| `-bucket` | `INFLUX_BUCKET` | `buspi` | InfluxDB bucket |
| `-min-interval` | `UPLOAD_MIN_INTERVAL` | `30s` | Minimum time between `geo` writes (and between `geo_status` heartbeats) |
| — | `INFLUXDB_TOKEN` | _(required)_ | InfluxDB write token; env only, `geoinflux` exits if unset |

`UPLOAD_MIN_INTERVAL` takes a Go duration (`30s`, `2m`); an unparsable value
silently falls back to the default.

### 3. Install and start the service

```sh
sudo cp pi/geoinflux.service /etc/systemd/system/geoinflux.service
sudo systemctl daemon-reload
sudo systemctl enable --now geoinflux
journalctl -u geoinflux -f      # watch it connect and write points
```

### What gets written

`geoinflux` writes two measurements to the configured bucket, posting with
`precision=ns`.

**`geo`** — one point per fix (TPV `mode>=2`), at most once per `-min-interval`.
The point is stamped with the fix's own time from the TPV, so a fix uploaded
late is stored at the time it was taken (a TPV without a time gets InfluxDB's
server time). The tags and fields depend on what resolved the fix:

```text
geo,source=wifi lat=<f>,lon=<f>,range_m=<n>i,ap_count=<n>i <ns>
geo,source=cell,radio=<LTE|NR5G-SA> lat=<f>,lon=<f>,range_m=<n>i,mcc=<n>i,mnc=<n>i,cid=<n>i,tac=<n>i <ns>
```

- `source=wifi` — the provider resolved the WiFi scan (plus the serving cell).
  Carries `ap_count` (APs sent) and has **no** `radio` tag or cell fields.
- `source=cell` — only the serving cell anchored the fix. This line keeps the
  legacy `geo` schema (tags, fields and field order), so existing cell-based
  Grafana panels keep working; panels that filter on `source="cell"` will not
  show WiFi fixes.

`range_m` is the reported error radius (gpsd `eph`) in metres, rounded down.

**`geo_status`** — an uploader heartbeat, written whether or not there is a fix:

```text
geo_status mode=<n>i,fix_age_s=<f>,connected=<bool> <now-ns>
```

| Field | Meaning |
|---|---|
| `mode` | gpsd TPV mode from the router (`2` = fix, below `2` = no fix); `0` while disconnected |
| `fix_age_s` | Seconds since the fix's own time (ms resolution), `-1` when the TPV has no time |
| `connected` | `false` while the router's gpsd socket is unreachable |

It is written at most once per `-min-interval` while TPVs arrive, and
immediately whenever connectivity changes (connect or disconnect). This tells a
dead uploader (no points at all) apart from an unreachable router
(`connected=false`), no fix (`mode<2`) and a stale fix (growing `fix_age_s`).

## Build from source

```sh
make ipk            # builds dist/geolocd (arm64, static) and dist/geolocd_*.ipk
make test lint      # go test -race + golangci-lint
```

`geolocd` is a static `CGO_ENABLED=0` binary, so it has no libc/runtime dependency
on the router.

## ⚠️ After a firmware upgrade

A GL/OpenWrt **firmware flash replaces the rootfs**, so `/usr/bin/geolocd` and the
installed package are **wiped**. A _keep-settings_ sysupgrade preserves everything
under `/etc/config`, so `/etc/config/geolocd` — including a `google_key` you set
there — survives; only the package (binary + service) needs reinstalling.

### Auto-restore after a flash (recommended)

Keep a copy of the package on the router and have `/etc/rc.local` reinstall it
on boot if the binary is missing. Do this once on the running router (copy the
`.ipk` over first, e.g. `scp geolocd_<version>_aarch64_cortex-a53.ipk root@<router>:/tmp/`):

```sh
cp /tmp/geolocd_*_aarch64_cortex-a53.ipk /etc/geolocd.ipk

# Carry the ipk across a keep-settings sysupgrade.
grep -qxF '/etc/geolocd.ipk' /etc/sysupgrade.conf \
  || echo '/etc/geolocd.ipk' >> /etc/sysupgrade.conf

# Idempotent guard in rc.local, inserted before its final `exit 0`.
grep -qF '/etc/geolocd.ipk' /etc/rc.local \
  || sed -i '/^exit 0/i [ -x /usr/bin/geolocd ] || opkg install /etc/geolocd.ipk' /etc/rc.local

sysupgrade -l | grep -E 'geolocd|rc.local'   # verify both are kept
```

Why this and not a feed install from `/etc/uci-defaults/`: `/etc/config` and
`/etc/rc.local` are kept by a keep-settings sysupgrade by default, `/root` is not,
and files listed in `/etc/sysupgrade.conf` are added (check with `sysupgrade -l`).
uci-defaults scripts run inside `/etc/init.d/boot` (START=10), before the network
is up (netifd, START=20), so an `opkg update` there fails — and a script that
exits 0 deletes itself anyway. `rc.local` runs late, needs no network (the ipk is
local), stays armed for every future flash, and is a no-op while geolocd is
installed. When you upgrade geolocd, refresh `/etc/geolocd.ipk` too.

Reinstalling over a kept `/etc/config/geolocd` prints an
"Existing conffile … is different from the conffile in the new package" message
and leaves the package default as `/etc/config/geolocd-opkg`; the kept config
(including the key) wins.

> **The `google_key` is not restored by this.** It is a secret and is never
> shipped in the package. If your `/etc/config/geolocd` survived the flash
> (keep-settings sysupgrade), the key is still there and nothing else is needed.
> If you did a **clean flash** that wiped `/etc`, re-add it after the package is
> back:
>
> ```sh
> uci set geolocd.main.google_key='AIza...'
> uci commit geolocd && /etc/init.d/geolocd restart
> ```

### Manual restore

Without the rc.local guard, restore in one line — from the feed:

```sh
opkg update; opkg install geolocd    # config (key) is retained; service re-enables
```

or from the `.ipk` kept on the device (`/root/` does not survive a flash —
use `/etc/geolocd.ipk` as above) or copied over from the Pi:

```sh
opkg install /etc/geolocd.ipk
```

## Security

- `:2947` is bound on all interfaces but inbound WAN is dropped by the default
  OpenWrt firewall — it is reachable from the LAN (where the Pi lives), not the
  internet. Don't open a WAN port for it. See [SECURITY.md](../SECURITY.md).
- The provider keys (`google_key` / `key`) are secrets: keep router config backups out of version control.
