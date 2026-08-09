#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NETPROXY_NATIVE_BIN="${1:-$ROOT/src/module/bin/netproxy-native}"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

MODDIR="$ROOT/src/module"
SERVICE_STATE_DIR="$TMP_ROOT/runtime"
SERVICE_STATE_FILE="$SERVICE_STATE_DIR/service.json"
[ -x "$NETPROXY_NATIVE_BIN" ]
export NETPROXY_NATIVE_BIN

write_state() {
  "$NETPROXY_NATIVE_BIN" module state \
    --module-dir "$MODDIR" --state-file "$SERVICE_STATE_FILE" \
    --state "$1" --pid "${2:-0}" --started-at "${3:-0}" \
    --ready-at "${4:-0}" --error "${5:-}" > /dev/null
}

read_string() {
  sed -n 's/.*"'"$1"'":"\([^"]*\)".*/\1/p' "$SERVICE_STATE_FILE"
}

read_number() {
  sed -n 's/.*"'"$1"'":\([0-9][0-9]*\).*/\1/p' "$SERVICE_STATE_FILE"
}

write_state preparing 0 0 0 ""
[ "$(read_string state)" = "preparing" ]
[ "$(read_number pid)" -eq 0 ]

write_state ready 123 1700000000 1700000005 ""
[ "$(read_string state)" = "ready" ]
[ "$(read_number pid)" -eq 123 ]
[ "$(read_number started_at)" -eq 1700000000 ]
[ "$(read_number ready_at)" -eq 1700000005 ]

write_state failed 0 0 0 "核心启动失败"
[ "$(read_string state)" = "failed" ]
[ "$(read_string error)" = "核心启动失败" ]
[ ! -e "$SERVICE_STATE_FILE.tmp.$$" ]

if command -v python3 > /dev/null 2>&1; then
  python3 -m json.tool "$SERVICE_STATE_FILE" > /dev/null
fi

printf '%s\n' "service state test passed"
