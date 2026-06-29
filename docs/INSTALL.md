# Installing celloc

## Router daemon (`geolocd`) on OpenWrt / GL-iNet

### 1. Install the package

Grab `geolocd_<version>_aarch64_cortex-a53.ipk` from the
[releases](https://github.com/ckeller42/celloc/releases) (or build it — below) and:

```sh
scp -O geolocd_*_aarch64_cortex-a53.ipk root@<router>:/tmp/
ssh root@<router> 'opkg install /tmp/geolocd_*.ipk'
```

The package installs the binary, a procd service (enabled + started), and a default
`/etc/config/geolocd` (preserved across package upgrades as a conffile).

### 2. Set your OpenCelliD key

Get a free key at <https://opencellid.org>, then:

```sh
uci set geolocd.main.key='pk.your_key_here'
uci commit geolocd
/etc/init.d/geolocd restart
```

The key lives only in `/etc/config/geolocd` (and is never passed on the command
line, so it won't show up in `ps`).

### 3. Verify

```sh
gpspipe -w <router-ip>:2947     # expect a TPV with mode:2, lat/lon, eph (~1500m)
# or:
logread -e geolocd
```

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
installed package are **wiped** — but `/etc/config/geolocd` (your key) is preserved
by sysupgrade's default keep-list. To restore:

```sh
opkg install /tmp/geolocd_*.ipk     # config (key) is retained; service re-enables
```

Keep the `.ipk` on the device (e.g. `/root/`) or on the Pi so reinstall is one line.

## Security

- `:2947` is bound on all interfaces but inbound WAN is dropped by the default
  OpenWrt firewall — it is reachable from the LAN (where the Pi lives), not the
  internet. Don't open a WAN port for it. See [SECURITY.md](../SECURITY.md).
- The OpenCelliD key is a secret: keep router config backups out of version control.
