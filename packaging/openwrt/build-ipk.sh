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

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
pkg="$work/pkg"

# --- data tree ---
mkdir -p "$pkg/usr/bin" "$pkg/etc/init.d" "$pkg/etc/config" "$pkg/CONTROL"
cp "$BIN"                       "$pkg/usr/bin/geolocd";    chmod 0755 "$pkg/usr/bin/geolocd"
cp "$here/files/geolocd.init"   "$pkg/etc/init.d/geolocd"; chmod 0755 "$pkg/etc/init.d/geolocd"
cp "$here/files/geolocd.config" "$pkg/etc/config/geolocd"; chmod 0644 "$pkg/etc/config/geolocd"

# --- CONTROL ---
cat > "$pkg/CONTROL/control" <<EOF
Package: geolocd
Version: $VERSION
Architecture: $ARCH
Maintainer: Christoph Keller
Section: net
Priority: optional
Description: celloc cell-tower geolocation daemon, served over the gpsd protocol.
EOF
printf '/etc/config/geolocd\n' > "$pkg/CONTROL/conffiles"
cat > "$pkg/CONTROL/postinst" <<'EOF'
#!/bin/sh
[ -n "${IPKG_INSTROOT:-}" ] && exit 0
/etc/init.d/geolocd enable
/etc/init.d/geolocd start
exit 0
EOF
chmod 0755 "$pkg/CONTROL/postinst"

mkdir -p "$OUTDIR"
OUTDIR=$(CDPATH= cd -- "$OUTDIR" && pwd)
out="$OUTDIR/geolocd_${VERSION}_${ARCH}.ipk"

# --- assemble the OpenWrt ipk (gzip-tar of the three members) ---
TAR="tar --numeric-owner --owner=0 --group=0"
ipk="$work/ipk"; mkdir -p "$ipk"
printf '2.0\n' > "$ipk/debian-binary"
( cd "$pkg/CONTROL" && $TAR -czf "$ipk/control.tar.gz" . )
( cd "$pkg"         && $TAR -czf "$ipk/data.tar.gz" ./usr ./etc )
( cd "$ipk"         && $TAR -czf "$out" ./debian-binary ./control.tar.gz ./data.tar.gz )
echo "$out"
