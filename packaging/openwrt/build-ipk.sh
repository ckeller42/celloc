#!/bin/sh
# Build a geolocd .ipk from a prebuilt static binary.
#
# An OpenWrt .ipk is a *gzip-compressed tar* of three members —
# ./debian-binary, ./control.tar.gz, ./data.tar.gz — NOT a Debian `ar` archive.
# (opkg on the router rejects the ar form with "Malformed package file".)
# Requires GNU tar + gzip (present in CI and on a typical Linux dev box; macOS
# BSD tar is not supported — build releases in CI).
#
# Usage: build-ipk.sh <geolocd-binary> <version> [arch] [outdir]
#   arch defaults to aarch64_cortex-a53 (GL-E5800); outdir defaults to ./dist
set -eu

BIN=${1:?usage: build-ipk.sh <binary> <version> [arch] [outdir]}
VERSION=${2:?missing version}
ARCH=${3:-aarch64_cortex-a53}
OUTDIR=${4:-dist}

# opkg splits Version into upstream version and revision at the LAST '-', so
# "1.0.0-rc1" sorts ABOVE "1.0.0", and `git describe --dirty` output
# ("v1.0.0-3-gabc123-dirty") compares nonsensically. Refuse such versions.
case $VERSION in
*-*)
	echo "build-ipk.sh: version '$VERSION' contains '-'. opkg reads the part after the last '-' as the package revision, which breaks upgrade ordering. Use a plain version such as '1.0.0'." >&2
	exit 1
	;;
esac

# Every mode below is set on purpose; don't inherit a restrictive builder umask.
umask 022

here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
pkg="$work/pkg"

# --- data tree ---
mkdir -p "$pkg/usr/bin" "$pkg/etc/init.d" "$pkg/etc/config" "$pkg/CONTROL"
cp "$BIN"                       "$pkg/usr/bin/geolocd";    chmod 0755 "$pkg/usr/bin/geolocd"
cp "$here/files/geolocd.init"   "$pkg/etc/init.d/geolocd"; chmod 0755 "$pkg/etc/init.d/geolocd"
cp "$here/files/geolocd.config" "$pkg/etc/config/geolocd"; chmod 0644 "$pkg/etc/config/geolocd"

# opkg applies the archived mode to directories that already exist on the
# router (/etc, /usr/bin, ...), so a 0700 dir here would lock them down there.
# Force 0755 on every dir (data tree and CONTROL), whatever the umask was.
find "$pkg" -type d -exec chmod 0755 {} +

# --- CONTROL ---
# Depends: the daemon execs `iw` for Wi-Fi scans (iw-full also Provides: iw).
# gl_modem is deliberately NOT listed: it is a GL-iNet firmware binary, not an
# opkg package, so a Depends on it could never be satisfied.
cat > "$pkg/CONTROL/control" <<EOF
Package: geolocd
Version: $VERSION
Depends: iw
Architecture: $ARCH
Maintainer: Christoph Keller
Section: net
Priority: optional
Description: celloc cell-tower geolocation daemon, served over the gpsd protocol.
EOF
printf '/etc/config/geolocd\n' > "$pkg/CONTROL/conffiles"

# postinst uses `restart`, not `start`: on `opkg upgrade` procd would see an
# identical instance (same command line) and keep the replaced binary running.
cat > "$pkg/CONTROL/postinst" <<'EOF'
#!/bin/sh
[ -n "${IPKG_INSTROOT:-}" ] && exit 0
/etc/init.d/geolocd enable
/etc/init.d/geolocd restart
exit 0
EOF

# prerm stops the daemon and removes the /etc/rc.d/{S95,K10}geolocd links,
# which would otherwise dangle after `opkg remove`. On upgrade, the new
# postinst enables and restarts it again.
cat > "$pkg/CONTROL/prerm" <<'EOF'
#!/bin/sh
[ -n "${IPKG_INSTROOT:-}" ] && exit 0
/etc/init.d/geolocd stop
/etc/init.d/geolocd disable
exit 0
EOF
chmod 0644 "$pkg/CONTROL/control" "$pkg/CONTROL/conffiles"
chmod 0755 "$pkg/CONTROL/postinst" "$pkg/CONTROL/prerm"

mkdir -p "$OUTDIR"
OUTDIR=$(CDPATH='' cd -- "$OUTDIR" && pwd)
out="$OUTDIR/geolocd_${VERSION}_${ARCH}.ipk"

# --- assemble the OpenWrt ipk (gzip-tar of the three members) ---
TAR="tar --numeric-owner --owner=0 --group=0"
ipk="$work/ipk"; mkdir -p "$ipk"
printf '2.0\n' > "$ipk/debian-binary"
( cd "$pkg/CONTROL" && $TAR -czf "$ipk/control.tar.gz" . )
( cd "$pkg"         && $TAR -czf "$ipk/data.tar.gz" ./usr ./etc )
( cd "$ipk"         && $TAR -czf "$out" ./debian-binary ./control.tar.gz ./data.tar.gz )
echo "$out"
