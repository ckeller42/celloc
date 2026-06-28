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

See the project README (lands in milestone M4).

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
