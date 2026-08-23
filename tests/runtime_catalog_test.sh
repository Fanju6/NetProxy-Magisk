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
MIXED_INBOUND_FILE="$SINGBOX_DIR/confdir/04_inbounds.json"
SUBSCRIPTION_UPDATE_SOURCE="$ROOT/src/native/netproxy/internal/subscription/update.go"
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

printf '%s\n' '{"outbounds":[{"type":"socks","tag":"SOCKS","server":"example.com","server_port":1080}]}' \
  > "$CATALOG_DIR/default/provider.json"
printf '%s\n' '{"outbounds":[{"type":"http","tag":"HTTP","server":"example.net","server_port":8080}]}' \
  > "$CATALOG_DIR/secondary/provider.json"

set_conf() {
  local file="$1" key="$2" value="$3"
  local candidate="$file.candidate"
  awk -v target="$key" -v replacement="$value" '
    BEGIN { found = 0 }
    index($0, target "=") == 1 { print target "=" replacement; found = 1; next }
    { print }
    END { if (!found) print target "=" replacement }
  ' "$file" > "$candidate"
  mv "$candidate" "$file"
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

# mixed 7080 仅供本机订阅下载使用，不得暴露到通配 IPv4/IPv6 地址。
grep -q '"tag": "mixed_in"' "$MIXED_INBOUND_FILE"
grep -q '"listen": "127.0.0.1"' "$MIXED_INBOUND_FILE"
grep -q '"listen_port": 7080' "$MIXED_INBOUND_FILE"
! grep -Eq '"listen"[[:space:]]*:[[:space:]]*"(0\.0\.0\.0|::)"' "$MIXED_INBOUND_FILE"
grep -q 'options.ProxyURL = "http://127.0.0.1:7080"' "$SUBSCRIPTION_UPDATE_SOURCE"
! grep -Eq 'ProxyURL = "http://(0\.0\.0\.0|::):7080"' "$SUBSCRIPTION_UPDATE_SOURCE"

json_contains '"mode":"local"'
json_contains '"local":{"dns_mode":"hijack","ipv6_mode":"auto","bypass_private_address":true'
! json_contains '"shared"'

set_conf "$EBPF_CONF" "EBPF_BYPASS_RULE_SET" '""'
prepare_runtime
json_contains '"bypass_rule_set":\[\]'

set_conf_values "$EBPF_CONF"   "EBPF_MODE" '"hybrid"'   "EBPF_TCP_SPLICE" "1"   "APP_PROXY_ENABLE" "0"   "APP_PROXY_MODE" '"blacklist"'   "EBPF_SHARED_INCLUDE_SOURCE_CIDR" '"192.168.43.0/24,fd00::/64"'   "EBPF_SHARED_EXCLUDE_SOURCE_CIDR" '"192.168.43.10/32"'   "EBPF_SHARED_INCLUDE_MAC_ADDRESS" '"02:11:22:33:44:55,AA:BB:CC:DD:EE:FF"'   "EBPF_SHARED_EXCLUDE_MAC_ADDRESS" '"12:34:56:78:9A:BC"'
prepare_runtime
json_contains '"tcp_splice":true'
json_contains '"include_source_cidr":\["192.168.43.0/24","fd00::/64"\]'
json_contains '"exclude_source_cidr":\["192.168.43.10/32"\]'
json_contains '"include_mac_address":\["02:11:22:33:44:55","AA:BB:CC:DD:EE:FF"\]'
json_contains '"exclude_mac_address":\["12:34:56:78:9A:BC"\]'
json_contains '"advanced":{"tc_priority":1,"data_plane":"auto"}'

INVALID_EBPF_CONF="$TMP_ROOT/invalid-ebpf.conf"
cp "$EBPF_CONF" "$INVALID_EBPF_CONF"
sed -i 's/02:11:22:33:44:55/02:11:22:33:44:5G/' "$INVALID_EBPF_CONF"
INVALID_EBPF_OUTPUT="$TMP_ROOT/invalid-ebpf.json"
INVALID_EBPF_ERROR="$TMP_ROOT/invalid-ebpf.error"
if "$NETPROXY_NATIVE_BIN" ebpf runtime \
  --config "$INVALID_EBPF_CONF" \
  --output "$INVALID_EBPF_OUTPUT" \
  --format json > /dev/null 2> "$INVALID_EBPF_ERROR"; then
  printf '%s\n' 'invalid eBPF config should fail' >&2
  exit 1
fi
grep -q '"code":"ebpf.config_invalid"' "$INVALID_EBPF_ERROR"
[ ! -e "$INVALID_EBPF_OUTPUT" ]

set_conf_values "$EBPF_CONF" "APP_PROXY_ENABLE" "1" "APP_PROXY_MODE" '"whitelist"' "PROXY_APPS_LIST" '""' "BYPASS_APPS_LIST" '""'
prepare_runtime
json_contains '"include_uid":\[0\]'
! json_contains '"include_package"'

set_conf_values "$EBPF_CONF" "APP_PROXY_ENABLE" "0" "EBPF_LOCAL_IPV6_MODE" '"off"'
prepare_runtime
json_contains '"mode":"hybrid"'
json_contains '"local":{"dns_mode":"hijack","ipv6_mode":"off"'

set_conf "$EBPF_CONF" "EBPF_LOCAL_IPV6_MODE" '"off"'
prepare_runtime
json_contains '"local":{"dns_mode":"hijack","ipv6_mode":"off"'

set_conf "$EBPF_CONF" "EBPF_LOCAL_IPV6_MODE" '"always"'
prepare_runtime
json_contains '"local":{"dns_mode":"hijack","ipv6_mode":"always"'

set_conf_values "$EBPF_CONF" "EBPF_MODE" '"shared"' "EBPF_SHARED_INTERFACES" '"wlan2"' "APP_PROXY_ENABLE" "0"
prepare_runtime
json_contains '"mode":"shared"'
! json_contains '"cgroup_path"'
! json_contains '"local":'
json_contains '"shared".*"dns_mode":"hijack","interface":\["wlan2"\],"ipv6_mode":"always","bypass_private_address":true'
json_contains '"advanced":{"tc_priority":1,"data_plane":"auto"}'

sed -i 's/"name": "备用配置"/"name": "本地配置"/' "$CATALOG_DIR/secondary/meta.json"
prepare_runtime
grep -q '"tag": "本地配置 \[default\]"' "$RUNTIME_PROVIDERS_FILE"
grep -q '"tag": "本地配置 \[secondary\]"' "$RUNTIME_PROVIDERS_FILE"
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
