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
RUNTIME_DIR="$TMP_ROOT/runtime"
EBPF_CONF="$TMP_ROOT/ebpf.conf"
RUNTIME_PROVIDERS_FILE="$RUNTIME_DIR/providers.json"
RUNTIME_OUTBOUNDS_FILE="$RUNTIME_DIR/outbounds.json"
RUNTIME_EBPF_FILE="$RUNTIME_DIR/ebpf.json"

mkdir -p "$CATALOG_DIR/default" "$CATALOG_DIR/secondary" "$CATALOG_DIR/staging" "$RUNTIME_DIR"
cp "$MODDIR/config/module.conf" "$MODULE_CONF"
cp "$MODDIR/config/ebpf/ebpf.conf" "$EBPF_CONF"
cp "$MODDIR/data/catalog/default/meta.json" "$CATALOG_DIR/default/meta.json"
cp "$MODDIR/data/catalog/default/meta.json" "$CATALOG_DIR/secondary/meta.json"
sed -i 's/"node_count": 0/"node_count": 1/' "$CATALOG_DIR/default/meta.json" "$CATALOG_DIR/secondary/meta.json"
sed -i 's/"id": "default"/"id": "secondary"/; s/"name": "本地配置"/"name": "备用配置"/' "$CATALOG_DIR/secondary/meta.json"

"$NETPROXY_NATIVE_BIN" convert link --input 'socks://example.com:1080#SOCKS' --output "$CATALOG_DIR/default/provider.json" > /dev/null
"$NETPROXY_NATIVE_BIN" convert link --input 'http://example.net:8080#HTTP' --output "$CATALOG_DIR/secondary/provider.json" > /dev/null

set_conf() {
  local file="$1" key="$2" value="$3"
  case "$file" in
    "$MODULE_CONF") "$NETPROXY_NATIVE_BIN" config module-set --path "$file" --set "$key=$value" > /dev/null 2>&1 ;;
    "$EBPF_CONF") "$NETPROXY_NATIVE_BIN" config ebpf-set --path "$file" --set "$key=$value" > /dev/null 2>&1 ;;
    *) return 1 ;;
  esac
}

set_conf_values() {
  local file="$1" key value
  shift
  while [ "$#" -gt 0 ]; do
    key="$1"
    value="$2"
    shift 2
    set_conf "$file" "$key" "$value"
  done
}

prepare_runtime() {
  "$NETPROXY_NATIVE_BIN" module prepare     --module-dir "$MODDIR" --catalog-root "$CATALOG_DIR"     --module-config "$MODULE_CONF" --ebpf-config "$EBPF_CONF"     --singbox-dir "$SINGBOX_DIR" --runtime-dir "$RUNTIME_DIR"     --state-file "$TMP_ROOT/dev/netproxy/service.json" > /dev/null
}

json_contains() {
  tr -d ' \t\r\n' < "$RUNTIME_EBPF_FILE" | grep -q "$1"
}

prepare_runtime
grep -q '"tag": "本地配置"' "$RUNTIME_PROVIDERS_FILE"
grep -q '"tag": "备用配置"' "$RUNTIME_PROVIDERS_FILE"
grep -q '"default": "Auto/本地配置"' "$RUNTIME_OUTBOUNDS_FILE"
! grep -q '"default": "direct"' "$RUNTIME_OUTBOUNDS_FILE"
grep -q '"external_controller": "127.0.0.1:9999"' "$SINGBOX_DIR/confdir/02_experimental.json"
grep -q '"listen": "127.0.0.1"' "$SINGBOX_DIR/confdir/08_services.json"
grep -q '"secret": "singbox"' "$SINGBOX_DIR/confdir/02_experimental.json"
grep -q '"secret": "singbox"' "$SINGBOX_DIR/confdir/08_services.json"
json_contains '"cgroup_enabled":true'
json_contains '"cgroup_ipv6_mode":"always"'
json_contains '"bypass_private_address":true'
json_contains '"include_package":\[\]'
json_contains '"exclude_package":\[\]'
json_contains '"include_android_user":\[\]'
json_contains '"tc_priority":1'

set_conf "$EBPF_CONF" "EBPF_BYPASS_RULE_SETS" '""'
prepare_runtime
json_contains '"bypass_rule_set":\[\]'

set_conf_values "$EBPF_CONF"   "APP_PROXY_MODE" '"blacklist"'   "APP_ANDROID_USERS" '"0,999"'   "BYPASS_APPS_LIST" '"com.android.chrome,org.telegram.messenger"'   "EBPF_SHARED_INCLUDE_SOURCE_CIDRS" '"192.168.43.0/24,fd00::/64"'   "EBPF_SHARED_EXCLUDE_SOURCE_CIDRS" '"192.168.43.10/32"'   "EBPF_SHARED_INCLUDE_MAC_ADDRESSES" '"02:11:22:33:44:55,AA:BB:CC:DD:EE:FF"'   "EBPF_SHARED_EXCLUDE_MAC_ADDRESSES" '"12:34:56:78:9A:BC"'
prepare_runtime
json_contains '"include_android_user":\[0,999\]'
json_contains '"exclude_package":\["com.android.chrome","org.telegram.messenger"\]'
json_contains '"include_source_cidr":\["192.168.43.0/24","fd00::/64"\]'
json_contains '"exclude_source_cidr":\["192.168.43.10/32"\]'
json_contains '"include_mac_address":\["02:11:22:33:44:55","AA:BB:CC:DD:EE:FF"\]'
json_contains '"exclude_mac_address":\["12:34:56:78:9A:BC"\]'
json_contains '"tc_priority":1'

INVALID_EBPF_CONF="$TMP_ROOT/invalid-ebpf.conf"
cp "$EBPF_CONF" "$INVALID_EBPF_CONF"
sed -i 's/02:11:22:33:44:55/02:11:22:33:44:5G/' "$INVALID_EBPF_CONF"
! "$NETPROXY_NATIVE_BIN" ebpf validate --config "$INVALID_EBPF_CONF" --format json > /dev/null 2>&1

set_conf_values "$EBPF_CONF" "APP_PROXY_MODE" '"whitelist"' "PROXY_APPS_LIST" '""' "BYPASS_APPS_LIST" '""'
prepare_runtime
json_contains '"include_uid":\[4294967295\]'
json_contains '"include_package":\[\]'

set_conf_values "$EBPF_CONF" "PROXY_APPS_LIST" '"com.google.android.youtube"' "EBPF_CGROUP_IPV6_MODE" '"off"'
prepare_runtime
json_contains '"include_uid":\[\]'
json_contains '"include_package":\["com.google.android.youtube"\]'
json_contains '"cgroup_ipv6_mode":"off"'
! json_contains 'fd53:696e:672d:626f::/64'

set_conf "$EBPF_CONF" "EBPF_CGROUP_IPV6_MODE" '"off"'
prepare_runtime
json_contains '"cgroup_ipv6_mode":"off"'
! json_contains 'fd53:696e:672d:626f::/64'

set_conf "$EBPF_CONF" "EBPF_CGROUP_IPV6_MODE" '"always"'
prepare_runtime
json_contains '"cgroup_ipv6_mode":"always"'
json_contains 'fd53:696e:672d:626f::/64'

set_conf_values "$EBPF_CONF" "EBPF_SHARED_NETWORK" "1" "EBPF_SHARED_INTERFACES" '"wlan2"' "EBPF_CGROUP_ENABLED" "0"
prepare_runtime
json_contains '"cgroup_enabled":false'
! json_contains '"cgroup_path"'
! json_contains '"include_package"'
json_contains '"map_capacity":{"proxy":65536,"bypass":65536,"fragment":65536}'
json_contains 'fd53:696e:672d:626f::/64'

sed -i 's/"name": "备用配置"/"name": "本地配置"/' "$CATALOG_DIR/secondary/meta.json"
[ "$("$NETPROXY_NATIVE_BIN" catalog tag --root "$CATALOG_DIR" --group default --format raw)" = "本地配置 [default]" ]
[ "$("$NETPROXY_NATIVE_BIN" catalog tag --root "$CATALOG_DIR" --group secondary --format raw)" = "本地配置 [secondary]" ]
sed -i 's/"name": "本地配置"/"name": "备用配置"/' "$CATALOG_DIR/secondary/meta.json"

set_conf "$MODULE_CONF" "SELECTOR_MODE" "manual"
set_conf "$MODULE_CONF" "SELECTED_NODE_REF" '"default/SOCKS"'
prepare_runtime
! grep -q '"default": "SOCKS"' "$RUNTIME_OUTBOUNDS_FILE"
! grep -q '"default": "default/SOCKS"' "$RUNTIME_OUTBOUNDS_FILE"
grep -q '"default": "Select/本地配置"' "$RUNTIME_OUTBOUNDS_FILE"

if command -v python3 > /dev/null 2>&1; then
  python3 -m json.tool "$RUNTIME_PROVIDERS_FILE" > /dev/null
  python3 -m json.tool "$RUNTIME_OUTBOUNDS_FILE" > /dev/null
  python3 -m json.tool "$RUNTIME_EBPF_FILE" > /dev/null
fi

printf '%s\n' "runtime catalog test passed"
