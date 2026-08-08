#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NATIVE="${1:-$ROOT/src/module/bin/netproxy-native}"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

MODULE_CONF="$TMP_ROOT/module.conf"
EBPF_CONF="$TMP_ROOT/ebpf.conf"
cp "$ROOT/src/module/config/module.conf" "$MODULE_CONF"
cp "$ROOT/src/module/config/ebpf/ebpf.conf" "$EBPF_CONF"

"$NATIVE" config module-set --path "$MODULE_CONF" --set 'OUTBOUND_MODE=global' > /dev/null
[ "$("$NATIVE" config module-get --path "$MODULE_CONF" --key OUTBOUND_MODE --format text)" = "global" ]

if "$NATIVE" config module-set --path "$MODULE_CONF" --set 'OUTBOUND_MODE=invalid' > /dev/null 2>&1; then
  printf '%s\n' '非法模块配置应该被拒绝' >&2
  exit 1
fi
[ "$("$NATIVE" config module-get --path "$MODULE_CONF" --key OUTBOUND_MODE --format text)" = "global" ]

"$NATIVE" config ebpf-set --path "$EBPF_CONF" --set 'APP_PROXY_MODE="whitelist"' > /dev/null
[ "$("$NATIVE" config ebpf-get --path "$EBPF_CONF" --key APP_PROXY_MODE --format text)" = "whitelist" ]

printf '%s\n' "config native test passed"
