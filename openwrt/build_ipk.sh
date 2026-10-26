#!/usr/bin/env bash
# build_ipk.sh — build router-re .ipk for OpenWrt (x86/64) via the SDK.
#
# Prerequisites:
#   - OpenWrt SDK extracted at $SDK_DIR (default /home/jimwong/openwrt-sdk/sdk)
#   - Go 1.24+ on PATH
#   - gawk installed
#
# Usage:
#   ./build_ipk.sh            # build ipk, output path printed at end
#   SDK_DIR=/path/to/sdk ./build_ipk.sh
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
SDK_DIR="${SDK_DIR:-/home/jimwong/openwrt-sdk/sdk}"
PKG_DIR="$SDK_DIR/package/router-re"

if [ ! -d "$SDK_DIR" ]; then
    echo "SDK not found at $SDK_DIR" >&2
    echo "Download from https://downloads.openwrt.org/releases/24.10.8/targets/x86/64/" >&2
    exit 1
fi

# Sync package definition into SDK.
mkdir -p "$PKG_DIR/files"
cp "$ROOT_DIR/openwrt/package/router-re/Makefile" "$PKG_DIR/Makefile"
cp "$ROOT_DIR/openwrt/package/router-re/files/mgmtd.init" "$PKG_DIR/files/mgmtd.init"

# Ensure a sane .config exists.
cd "$SDK_DIR"
if [ ! -f .config ]; then
    printf 'CONFIG_TARGET_x86=y\nCONFIG_TARGET_x86_64=y\nCONFIG_TARGET_x86_64_DEVICE_generic=y\n' > .config
    make defconfig >/dev/null 2>&1
fi

# Clean + build.
make package/router-re/clean >/dev/null 2>&1 || true
make package/router-re/compile V=s

IPK="$(ls -1 "$SDK_DIR"/bin/packages/x86_64/base/router-re_*.ipk 2>/dev/null | tail -1)"
echo "=== IPK built ==="
echo "$IPK"
ls -lh "$IPK"
EOF
chmod +x /home/jimwong/projects/router-re/openwrt/build_ipk.sh
echo written