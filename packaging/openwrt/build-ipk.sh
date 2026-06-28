#!/bin/sh
# Build a geolocd .ipk from a prebuilt static binary using the canonical
# opkg-build tool (vendored alongside this script). Requires GNU binutils `ar`
# and bash (both present in CI and on a typical Linux dev box; macOS BSD `ar`
# is not supported — build releases in CI).
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
# opkg-build cd's internally, so it needs an absolute destination.
OUTDIR=$(CDPATH= cd -- "$OUTDIR" && pwd)
bash "$here/opkg-build" -o root -g root "$pkg" "$OUTDIR" >&2
echo "$OUTDIR/geolocd_${VERSION}_${ARCH}.ipk"
