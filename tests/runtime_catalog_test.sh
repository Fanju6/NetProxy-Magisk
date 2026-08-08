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
EBPF_CONF="$TMP_ROOT/ebpf.conf"

mkdir -p "$CATALOG_DIR/default" "$CATALOG_DIR/secondary" "$CATALOG_DIR/staging" "$RUNTIME_DIR"
cp "$MODDIR/config/module.conf" "$MODULE_CONF"
cp "$MODDIR/config/ebpf/ebpf.conf" "$EBPF_CONF"
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
. "$MODDIR/scripts/core/runtime.sh"

write_runtime_ebpf() {
  "$NETPROXY_NATIVE_BIN" ebpf runtime \
    --config "$EBPF_CONF" \
    --output "$RUNTIME_EBPF_FILE" \
    --format json > /dev/null
}

validate_ebpf_config() {
  "$NETPROXY_NATIVE_BIN" ebpf validate \
    --config "$EBPF_CONF" \
    --format json > /dev/null 2>&1
}

json_contains() {
  pattern="$1"
  tr -d ' \t\r\n' < "$RUNTIME_EBPF_FILE" | grep -q "$pattern"
}

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
json_contains '"cgroup_enabled":true'
json_contains '"cgroup_ipv6_mode":"auto"'
json_contains '"include_package":\[\]'
json_contains '"exclude_package":\[\]'
json_contains '"include_android_user":\[\]'
json_contains '"tc_priority":1'

set_conf "$EBPF_CONF" "EBPF_BYPASS_RULE_SETS" '""'
write_runtime_ebpf > /dev/null
json_contains '"bypass_rule_set":\[\]'

set_conf_values "$EBPF_CONF" \
  "APP_PROXY_MODE" '"blacklist"' \
  "APP_ANDROID_USERS" '"0 999"' \
  "BYPASS_APPS_LIST" '"com.android.chrome org.telegram.messenger"' \
  "EBPF_SHARED_INCLUDE_SOURCE_CIDRS" '"192.168.43.0/24 fd00::/64"' \
  "EBPF_SHARED_EXCLUDE_SOURCE_CIDRS" '"192.168.43.10/32"' \
  "EBPF_SHARED_INCLUDE_MAC_ADDRESSES" '"02:11:22:33:44:55 AA:BB:CC:DD:EE:FF"' \
  "EBPF_SHARED_EXCLUDE_MAC_ADDRESSES" '"12:34:56:78:9A:BC"'
write_runtime_ebpf > /dev/null
json_contains '"include_android_user":\[0,999\]'
json_contains '"exclude_package":\["com.android.chrome","org.telegram.messenger"\]'
json_contains '"include_source_cidr":\["192.168.43.0/24","fd00::/64"\]'
json_contains '"exclude_source_cidr":\["192.168.43.10/32"\]'
json_contains '"include_mac_address":\["02:11:22:33:44:55","AA:BB:CC:DD:EE:FF"\]'
json_contains '"exclude_mac_address":\["12:34:56:78:9A:BC"\]'
json_contains '"tc_priority":1'

set_conf "$EBPF_CONF" "EBPF_SHARED_INCLUDE_MAC_ADDRESSES" '"02:11:22:33:44:5G"'
! validate_ebpf_config
set_conf "$EBPF_CONF" "EBPF_SHARED_INCLUDE_MAC_ADDRESSES" '"02:11:22:33:44:55 AA:BB:CC:DD:EE:FF"'

set_conf_values "$EBPF_CONF" \
  "APP_PROXY_MODE" '"whitelist"' \
  "PROXY_APPS_LIST" '""' \
  "BYPASS_APPS_LIST" '""'
write_runtime_ebpf > /dev/null
json_contains '"include_uid":\[4294967295\]'
json_contains '"include_package":\[\]'

set_conf_values "$EBPF_CONF" \
  "PROXY_APPS_LIST" '"com.google.android.youtube"' \
  "EBPF_IPV6_MODE" '"disabled"'
write_runtime_ebpf > /dev/null
json_contains '"include_uid":\[\]'
json_contains '"include_package":\["com.google.android.youtube"\]'
json_contains '"cgroup_ipv6_mode":"off"'
! json_contains 'fd53:696e:672d:626f::/64'

set_conf "$EBPF_CONF" "EBPF_IPV6_MODE" '"shared"'
write_runtime_ebpf > /dev/null
json_contains '"cgroup_ipv6_mode":"off"'
json_contains 'fd53:696e:672d:626f::/64'

set_conf "$EBPF_CONF" "EBPF_IPV6_MODE" '"always"'
write_runtime_ebpf > /dev/null
json_contains '"cgroup_ipv6_mode":"always"'
json_contains 'fd53:696e:672d:626f::/64'

set_conf_values "$EBPF_CONF" \
  "EBPF_CGROUP_ENABLED" "0" \
  "EBPF_SHARED_NETWORK" "1" \
  "EBPF_SHARED_INTERFACES" '"wlan2"'
write_runtime_ebpf > /dev/null
json_contains '"cgroup_enabled":false'
! json_contains '"cgroup_path"'
! json_contains '"include_package"'
json_contains '"map_capacity":65536'

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
