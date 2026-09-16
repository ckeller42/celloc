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
opkg update && opkg install geolocd
```

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
`opkg update && opkg upgrade geolocd`.

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
2. Enable billing (the free tier covers ~10,000 requests/month; the default
   5-minute poll interval uses roughly 8,600 requests/month).
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

### 3. Install and start the service

```sh
sudo cp pi/geoinflux.service /etc/systemd/system/geoinflux.service
sudo systemctl daemon-reload
sudo systemctl enable --now geoinflux
journalctl -u geoinflux -f      # watch it connect and write points
```

Fixes land in the configured bucket as the `geo` measurement (`source=cell`),
identical to the legacy schema, so existing Grafana panels keep working.

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

### Auto-restore from the feed (recommended)

Because the opkg feed makes `geolocd` installable by name, you can have the router
**reinstall it automatically on the first boot after a flash**. Scripts in
`/etc/uci-defaults/` run once at first boot (then delete themselves on success),
and files listed in `/etc/sysupgrade.conf` are carried across a keep-settings
flash. Combine the two — do this once on the running router:

```sh
cat > /etc/uci-defaults/99-geolocd-autoinstall <<'EOF'
#!/bin/sh
# Auto-reinstall geolocd from the celloc opkg feed after a sysupgrade.
# uci-defaults runs this once at first boot; returning 0 removes it.
opkg update && opkg install geolocd
exit 0
EOF
chmod +x /etc/uci-defaults/99-geolocd-autoinstall

# Preserve the script across the flash (and re-arm it for the next one).
grep -qxF '/etc/uci-defaults/99-geolocd-autoinstall' /etc/sysupgrade.conf \
  || echo '/etc/uci-defaults/99-geolocd-autoinstall' >> /etc/sysupgrade.conf
```

After the next _keep-settings_ sysupgrade the script is restored, runs on first
boot, reinstalls `geolocd` from the feed, and then removes itself. Because it
self-removes on success, re-run the block above (or just re-add the script) to
re-arm it for a subsequent flash. This needs the feed reachable at boot and the
HTTPS bits present (see [Method A](#method-a--opkg-feed-recommended-install-by-name)).

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

Without the auto-install script, restore in one line — from the feed:

```sh
opkg update && opkg install geolocd    # config (key) is retained; service re-enables
```

or from a `.ipk` you kept on the device (e.g. `/root/`) or on the Pi:

```sh
opkg install /root/geolocd_*.ipk
```

## Security

- `:2947` is bound on all interfaces but inbound WAN is dropped by the default
  OpenWrt firewall — it is reachable from the LAN (where the Pi lives), not the
  internet. Don't open a WAN port for it. See [SECURITY.md](../SECURITY.md).
- The provider keys (`google_key` / `key`) are secrets: keep router config backups out of version control.
