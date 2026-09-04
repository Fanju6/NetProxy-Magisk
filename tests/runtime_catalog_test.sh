#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NETPROXYCTL_BIN="${1:-$ROOT/src/module/bin/netproxyctl}"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

MODDIR="$ROOT/src/module"
TEST_MODULE="$TMP_ROOT/module"
MODULE_CONF="$TEST_MODULE/config/module.conf"
CATALOG_DIR="$TEST_MODULE/data/catalog"
SINGBOX_DIR="$MODDIR/config/singbox"
MIXED_INBOUND_FILE="$SINGBOX_DIR/config.json"
SUBSCRIPTION_UPDATE_SOURCE="$ROOT/src/native/netproxy/internal/subscription/update.go"
CATALOG_LIST_OUTPUT="$TMP_ROOT/catalog-list.json"
CATALOG_SHOW_OUTPUT="$TMP_ROOT/catalog-show.json"

mkdir -p "$TEST_MODULE/config" "$CATALOG_DIR/default" "$CATALOG_DIR/secondary" "$CATALOG_DIR/staging"
cp "$MODDIR/config/module.conf" "$MODULE_CONF"
cp "$MODDIR/data/catalog/default/meta.json" "$CATALOG_DIR/default/meta.json"
cp "$MODDIR/data/catalog/default/meta.json" "$CATALOG_DIR/secondary/meta.json"
sed -i 's/"node_count": 0/"node_count": 1/' "$CATALOG_DIR/default/meta.json" "$CATALOG_DIR/secondary/meta.json"
sed -i 's/"id": "default"/"id": "secondary"/; s/"name": "本地配置"/"name": "备用配置"/' "$CATALOG_DIR/secondary/meta.json"

printf '%s\n' '{"outbounds":[{"type":"socks","tag":"SOCKS","server":"example.com","server_port":1080}]}' \
  > "$CATALOG_DIR/default/provider.json"
printf '%s\n' '{"outbounds":[{"type":"http","tag":"HTTP","server":"example.net","server_port":8080}]}' \
  > "$CATALOG_DIR/secondary/provider.json"

NETPROXY_MODULE_DIR="$TEST_MODULE" SUB_RUNTIME_DIR="$TMP_ROOT/subscriptions" \
  "$NETPROXYCTL_BIN" --json catalog list > "$CATALOG_LIST_OUTPUT"
grep -q '"code":"catalog.groups"' "$CATALOG_LIST_OUTPUT"
grep -q '"id":"default"' "$CATALOG_LIST_OUTPUT"
grep -q '"runtime_tag":"本地配置"' "$CATALOG_LIST_OUTPUT"
grep -q '"id":"secondary"' "$CATALOG_LIST_OUTPUT"
grep -q '"runtime_tag":"备用配置"' "$CATALOG_LIST_OUTPUT"

NETPROXY_MODULE_DIR="$TEST_MODULE" SUB_RUNTIME_DIR="$TMP_ROOT/subscriptions" \
  "$NETPROXYCTL_BIN" --json catalog show default > "$CATALOG_SHOW_OUTPUT"
grep -q '"code":"catalog.show"' "$CATALOG_SHOW_OUTPUT"
grep -q '"tag":"SOCKS"' "$CATALOG_SHOW_OUTPUT"

grep -q '"external_controller": "127.0.0.1:9999"' "$SINGBOX_DIR/config.json"
grep -q '"listen": "127.0.0.1"' "$SINGBOX_DIR/config.json"
grep -q '"secret": "singbox"' "$SINGBOX_DIR/config.json"

# mixed 7080 仅供本机订阅下载使用，不得暴露到通配 IPv4/IPv6 地址。
grep -q '"tag": "mixed_in"' "$MIXED_INBOUND_FILE"
grep -q '"listen": "127.0.0.1"' "$MIXED_INBOUND_FILE"
grep -q '"listen_port": 7080' "$MIXED_INBOUND_FILE"
! grep -Eq '"listen"[[:space:]]*:[[:space:]]*"(0\.0\.0\.0|::)"' "$MIXED_INBOUND_FILE"
grep -q 'options.ProxyURL = "http://127.0.0.1:7080"' "$SUBSCRIPTION_UPDATE_SOURCE"
! grep -Eq 'ProxyURL = "http://(0\.0\.0\.0|::):7080"' "$SUBSCRIPTION_UPDATE_SOURCE"

printf '%s\n' "runtime catalog test passed"
