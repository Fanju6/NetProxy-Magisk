#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NETPROXY_NATIVE_BIN="${1:-$ROOT/src/module/bin/netproxy-native}"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

MODDIR="$ROOT/src/module"
SERVICE_STATE_DIR="$TMP_ROOT/runtime"
SERVICE_STATE_FILE="$SERVICE_STATE_DIR/service.json"
LOG_FILE="$TMP_ROOT/test.log"
LOG_STDERR=0
LOG_TAG="state-test"

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/state.sh"

[ -x "$NETPROXY_NATIVE_BIN" ]
export NETPROXY_NATIVE_BIN

write_service_state preparing 0 0 0 ""
[ "$(service_state_get_string state stopped)" = "preparing" ]
[ "$(service_state_get_number pid 99)" -eq 0 ]

write_service_state ready 123 1700000000 1700000005 ""
[ "$(service_state_get_string state stopped)" = "ready" ]
[ "$(service_state_get_number pid 0)" -eq 123 ]
[ "$(service_state_get_number started_at 0)" -eq 1700000000 ]
[ "$(service_state_get_number ready_at 0)" -eq 1700000005 ]
load_service_state
[ "$SERVICE_STATE_VALUE" = "ready" ]
[ "$SERVICE_STATE_PID_VALUE" -eq 123 ]
[ "$SERVICE_STATE_STARTED_AT_VALUE" -eq 1700000000 ]
[ "$SERVICE_STATE_READY_AT_VALUE" -eq 1700000005 ]

write_service_state failed 0 0 0 "核心启动失败"
[ "$(service_state_get_string state stopped)" = "failed" ]
[ "$(service_state_get_string error '')" = "核心启动失败" ]
[ ! -e "$SERVICE_STATE_FILE.tmp.$$" ]

if command -v python3 > /dev/null 2>&1; then
  python3 -m json.tool "$SERVICE_STATE_FILE" > /dev/null
fi

printf '%s\n' "service state test passed"
