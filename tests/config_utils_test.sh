#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

MODDIR="$ROOT/src/module"
LOG_FILE="$TMP_ROOT/test.log"
LOG_STDERR=0
LOG_TAG="config-test"
CONF="$TMP_ROOT/module.conf"

cat > "$CONF" << 'EOF'
# 测试配置
FIRST=one
SECOND="two"
EOF

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/config.sh"

set_conf_values "$CONF" \
  FIRST '"updated"' \
  SECOND 'three' \
  THIRD '"added"'

[ "$(read_conf "$CONF" FIRST "")" = "updated" ]
[ "$(read_conf "$CONF" SECOND "")" = "three" ]
[ "$(read_conf "$CONF" THIRD "")" = "added" ]
[ "$(grep -c '^FIRST=' "$CONF")" -eq 1 ]
[ "$(grep -c '^SECOND=' "$CONF")" -eq 1 ]
[ "$(grep -c '^THIRD=' "$CONF")" -eq 1 ]
[ ! -e "$CONF.lock" ]

mkdir "$CONF.lock"
printf '999999\n' > "$CONF.lock/pid"
printf '1\n' > "$CONF.lock/start"
set_conf "$CONF" FOUR '4'
[ "$(read_conf "$CONF" FOUR "")" = "4" ]
[ ! -e "$CONF.lock" ]

validate_android_package_list "com.android.chrome org.telegram.messenger" "apps"
! validate_android_package_list "0:com.android.chrome" "apps"
validate_android_user_list "0 10 999" "users"
! validate_android_user_list "0 owner" "users"

printf '%s\n' "config utils test passed"
