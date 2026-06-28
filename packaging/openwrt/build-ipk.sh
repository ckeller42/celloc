#!/bin/sh
# Build a geolocd .ipk from a prebuilt static binary — no OpenWrt SDK needed.
# An .ipk is an ar archive of: debian-binary, control.tar.gz, data.tar.gz.
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

# --- data: the installed filesystem tree (portable; no GNU `install -D`) ---
mkdir -p "$work/data/usr/bin" "$work/data/etc/init.d" "$work/data/etc/config"
cp "$BIN"                       "$work/data/usr/bin/geolocd";   chmod 0755 "$work/data/usr/bin/geolocd"
cp "$here/files/geolocd.init"   "$work/data/etc/init.d/geolocd"; chmod 0755 "$work/data/etc/init.d/geolocd"
cp "$here/files/geolocd.config" "$work/data/etc/config/geolocd"; chmod 0644 "$work/data/etc/config/geolocd"

# --- control: package metadata + maintainer scripts ---
mkdir -p "$work/control"
cat > "$work/control/control" <<EOF
Package: geolocd
Version: $VERSION
Architecture: $ARCH
Maintainer: Christoph Keller
Section: net
Priority: optional
Description: celloc cell-tower geolocation daemon, served over the gpsd protocol.
EOF
printf '/etc/config/geolocd\n' > "$work/control/conffiles"
cat > "$work/control/postinst" <<'EOF'
#!/bin/sh
[ -n "${IPKG_INSTROOT:-}" ] && exit 0
/etc/init.d/geolocd enable
/etc/init.d/geolocd start
exit 0
EOF
chmod 0755 "$work/control/postinst"

# --- assemble ---
# Classic ustar (no pax/GNU extended headers) — OpenWrt's opkg tar reader is
# picky and rejects pax headers that GNU tar emits with --owner/--group.
TAR_OPTS="--format=ustar --numeric-owner --owner=0 --group=0"
( cd "$work/data"    && tar $TAR_OPTS -czf ../data.tar.gz ./ )
( cd "$work/control" && tar $TAR_OPTS -czf ../control.tar.gz ./ )
printf '2.0\n' > "$work/debian-binary"

mkdir -p "$OUTDIR"
out="$OUTDIR/geolocd_${VERSION}_${ARCH}.ipk"
rm -f "$out"

# Prefer GNU ar — it produces the exact archive layout OpenWrt's opkg expects
# (this is how every .ipk in the OpenWrt feeds is built). Releases are built in
# CI on Linux, so this is the path that matters.
if ar --version 2>/dev/null | grep -qi 'GNU ar'; then
	echo "build-ipk: using GNU ar" >&2
	( cd "$work" && ar rcD "$(basename "$out")" debian-binary control.tar.gz data.tar.gz )
	mv "$work/$(basename "$out")" "$out"
	echo "$out"
	exit 0
fi
echo "build-ipk: using hand-rolled ar fallback" >&2

# Fallback hand-rolled ar (e.g. on macOS, whose BSD `ar` injects a __.SYMDEF
# member that opkg rejects). Use the GNU short-name convention: a trailing '/'
# on the member name, which opkg/libarchive strip. Best-effort for local dev;
# the authoritative artifact comes from CI's GNU ar above.
ar_add() { # <archive> <file> <member-name>
	_sz=$(wc -c < "$2" | tr -d ' ')
	printf '%-16s%-12d%-6d%-6d%-8s%-10d`\n' "$3/" 0 0 0 100644 "$_sz" >> "$1"
	cat "$2" >> "$1"
	[ $((_sz % 2)) -eq 1 ] && printf '\n' >> "$1"
	return 0
}
printf '!<arch>\n' > "$out"
ar_add "$out" "$work/debian-binary"   debian-binary
ar_add "$out" "$work/control.tar.gz"  control.tar.gz
ar_add "$out" "$work/data.tar.gz"     data.tar.gz
echo "$out"
