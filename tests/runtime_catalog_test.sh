#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NETPROXY_NATIVE_BIN="${1:-$ROOT/src/module/bin/netproxy-native}"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

MODDIR="$ROOT/src/module"
MODULE_CONF="$TMP_ROOT/module.conf"
CATALOG_DIR="$TMP_ROOT/catalog"
SINGBOX_DIR="$MODDIR/config/singbox"
CONFDIR="$SINGBOX_DIR/confdir"
RUNTIME_DIR="$TMP_ROOT/runtime"
LOG_FILE="$TMP_ROOT/test.log"
LOG_STDERR=0
LOG_TAG="runtime-test"
EBPF_CONF="$MODDIR/config/ebpf/ebpf.conf"

mkdir -p "$CATALOG_DIR/default" "$CATALOG_DIR/secondary" "$CATALOG_DIR/staging" "$RUNTIME_DIR"
cp "$MODDIR/config/module.conf" "$MODULE_CONF"
cp "$MODDIR/config/catalog/default/meta.json" "$CATALOG_DIR/default/meta.json"
cp "$MODDIR/config/catalog/default/meta.json" "$CATALOG_DIR/secondary/meta.json"
sed -i 's/"node_count": 0/"node_count": 1/' \
  "$CATALOG_DIR/default/meta.json" "$CATALOG_DIR/secondary/meta.json"
sed -i 's/"id": "default"/"id": "secondary"/; s/"name": "本地配置"/"name": "备用配置"/' \
  "$CATALOG_DIR/secondary/meta.json"

"$NETPROXY_NATIVE_BIN" convert link \
  --input 'socks://example.com:1080#SOCKS' \
  --output "$CATALOG_DIR/default/provider.json" > /dev/null
"$NETPROXY_NATIVE_BIN" convert link \
  --input 'http://example.net:8080#HTTP' \
  --output "$CATALOG_DIR/secondary/provider.json" > /dev/null

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/config.sh"
. "$MODDIR/scripts/utils/catalog.sh"
. "$MODDIR/scripts/utils/apps.sh"
. "$MODDIR/scripts/core/runtime.sh"
. "$MODDIR/scripts/core/ebpf.sh"

initialize_runtime_context
scan_catalog_groups
write_runtime_providers > /dev/null
write_runtime_outbounds > /dev/null
write_runtime_ebpf > /dev/null

[ "$RUNTIME_GROUP_COUNT" -eq 2 ]
[ "$RUNTIME_NODE_COUNT" -eq 2 ]
grep -q '"tag": "本地配置"' "$RUNTIME_PROVIDERS_FILE"
grep -q '"tag": "备用配置"' "$RUNTIME_PROVIDERS_FILE"
grep -q '"default": "Auto/本地配置"' "$RUNTIME_OUTBOUNDS_FILE"
! grep -q '"default": "direct"' "$RUNTIME_OUTBOUNDS_FILE"
grep -q '"external_controller": "0.0.0.0:9999"' "$CONFDIR/02_experimental.json"
grep -q '"listen": "0.0.0.0"' "$CONFDIR/08_services.json"
grep -q '"secret": "singbox"' "$CONFDIR/02_experimental.json"
grep -q '"secret": "singbox"' "$CONFDIR/08_services.json"

sed -i 's/"name": "备用配置"/"name": "本地配置"/' "$CATALOG_DIR/secondary/meta.json"
[ "$(catalog_runtime_group_tag default)" = "本地配置 [default]" ]
[ "$(catalog_runtime_group_tag secondary)" = "本地配置 [secondary]" ]
sed -i 's/"name": "本地配置"/"name": "备用配置"/' "$CATALOG_DIR/secondary/meta.json"

set_conf "$MODULE_CONF" "SELECTOR_MODE" "manual"
set_conf "$MODULE_CONF" "SELECTED_NODE_REF" '"default/SOCKS"'
initialize_runtime_context
scan_catalog_groups
write_runtime_outbounds > /dev/null
! grep -q '"default": "SOCKS"' "$RUNTIME_OUTBOUNDS_FILE"
! grep -q '"default": "default/SOCKS"' "$RUNTIME_OUTBOUNDS_FILE"
grep -q '"default": "Select/本地配置"' "$RUNTIME_OUTBOUNDS_FILE"

if command -v python3 > /dev/null 2>&1; then
  python3 -m json.tool "$RUNTIME_PROVIDERS_FILE" > /dev/null
  python3 -m json.tool "$RUNTIME_OUTBOUNDS_FILE" > /dev/null
  python3 -m json.tool "$RUNTIME_EBPF_FILE" > /dev/null
  python3 -m json.tool "$CONFDIR/02_experimental.json" > /dev/null
  python3 -m json.tool "$CONFDIR/08_services.json" > /dev/null
fi

printf '%s\n' "runtime catalog test passed"
