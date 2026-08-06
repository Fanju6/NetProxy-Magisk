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

# 锁持有者存活判定：自身 PID + 真实启动时刻视为有效
mkdir "$CONF.lock"
lock_write_owner "$CONF.lock"
lock_owner_alive "$CONF.lock"
# PID 存活但启动时刻不匹配 (PID 被复用) 必须判定为失效，否则残锁永不释放
printf '1\n' > "$CONF.lock/start"
! lock_owner_alive "$CONF.lock"
# 标记缺失同样视为失效
rm -f "$CONF.lock/pid" "$CONF.lock/start"
! lock_owner_alive "$CONF.lock"
set_conf "$CONF" FIVE '5'
[ "$(read_conf "$CONF" FIVE "")" = "5" ]
[ ! -e "$CONF.lock" ]

validate_android_package_list "com.android.chrome org.telegram.messenger" "apps"
! validate_android_package_list "0:com.android.chrome" "apps"
validate_android_user_list "0 10 999" "users"
! validate_android_user_list "0 owner" "users"

printf '%s\n' "config utils test passed"
