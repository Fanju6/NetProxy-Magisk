#!/usr/bin/env sh
# 文件: tests/customize_hot_update_test.sh
# 功能: 验证 customize.sh 的后台热更新仅在安装器结束后原子切换模块并保留最新用户状态
# 用法: sh tests/customize_hot_update_test.sh
# 依赖: POSIX sh、awk、grep、mktemp、/proc

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
CUSTOMIZE="$ROOT/src/module/customize.sh"
WORKDIR="$(mktemp -d)"
WORKER="$WORKDIR/hot-update-worker.sh"
MODULE_ROOT="$WORKDIR/data/adb"
STAGE="$MODULE_ROOT/modules_update/netproxy"
LIVE="$MODULE_ROOT/modules/netproxy"

cleanup() {
  rm -rf "$WORKDIR"
}
trap cleanup EXIT INT TERM

extract_worker() {
  awk '
    /^# NETPROXY_HOT_UPDATE_WORKER_BEGIN$/ { emit = 1; next }
    /^# NETPROXY_HOT_UPDATE_WORKER_END$/ { emit = 0 }
    emit { print }
  ' "$CUSTOMIZE" > "$WORKER"
  [ -s "$WORKER" ]
}

write_stage_module() {
  mkdir -p "$STAGE/bin" "$STAGE/config/ebpf" "$STAGE/config/singbox/confdir" "$STAGE/data/catalog/default" "$STAGE/data/catalog/staging"
  printf '%s\n' 'id=netproxy' 'version=test-new' > "$STAGE/module.prop"
  : > "$STAGE/netproxyctl"
  : > "$STAGE/bin/netproxy-native"
  : > "$STAGE/bin/sing-box"
  printf '%s\n' 'OUTBOUND_MODE=rule' > "$STAGE/config/module.conf"
  printf '%s\n' 'EBPF_DNS_MODE=off' > "$STAGE/config/ebpf/ebpf.conf"
  printf '%s\n' 'stage-confdir' > "$STAGE/config/singbox/confdir/03_dns.json"
  printf '%s\n' 'stage-provider' > "$STAGE/data/catalog/default/provider.json"
  printf '%s\n' 'stage-temporary' > "$STAGE/data/catalog/staging/download.tmp"
}

write_live_module() {
  mkdir -p "$LIVE/config/ebpf" "$LIVE/config/singbox/confdir" "$LIVE/data/catalog/default" "$LIVE/data/catalog/staging"
  printf '%s\n' 'id=netproxy' 'version=test-old' > "$LIVE/module.prop"
  printf '%s\n' 'OUTBOUND_MODE=global' > "$LIVE/config/module.conf"
  printf '%s\n' 'EBPF_DNS_MODE=hijack' > "$LIVE/config/ebpf/ebpf.conf"
  printf '%s\n' 'live-confdir' > "$LIVE/config/singbox/confdir/03_dns.json"
  printf '%s\n' 'live-provider' > "$LIVE/data/catalog/default/provider.json"
  printf '%s\n' 'live-temporary' > "$LIVE/data/catalog/staging/download.tmp"
  : > "$LIVE/update"
}

assert_file_contains() {
  grep -qx "$2" "$1" || {
    printf '断言失败: %s 未包含 %s\n' "$1" "$2" >&2
    return 1
  }
}

test_hot_commit_preserves_latest_state() {
  extract_worker
  write_stage_module
  write_live_module

  sh -c 'sleep 1' &
  installer_pid=$!
  sh "$WORKER" "$installer_pid" "$STAGE" "$LIVE" false preserve netproxy
  wait "$installer_pid"

  [ -d "$LIVE" ]
  [ ! -e "$STAGE" ]
  [ ! -e "$LIVE/update" ]
  [ -z "$(find "$MODULE_ROOT/modules" -maxdepth 1 -type d -name '.netproxy.hot-update.*' -print -quit)" ]
  assert_file_contains "$LIVE/module.prop" 'version=test-new'
  assert_file_contains "$LIVE/config/module.conf" 'OUTBOUND_MODE=global'
  assert_file_contains "$LIVE/config/ebpf/ebpf.conf" 'EBPF_DNS_MODE=hijack'
  assert_file_contains "$LIVE/config/singbox/confdir/03_dns.json" 'live-confdir'
  assert_file_contains "$LIVE/data/catalog/default/provider.json" 'live-provider'
  [ ! -e "$LIVE/data/catalog/staging/download.tmp" ]
  grep -Eq '^\[[^]]+\] \[INFO\] \[module\] \[module\.update\] \[success\] \[-\] 后台热更新已完成' "$LIVE/logs/service.log"
}

test_invalid_stage_keeps_kernel_su_fallback() {
  rm -rf "$MODULE_ROOT"
  write_stage_module
  write_live_module
  rm -f "$STAGE/bin/sing-box"

  sh -c 'sleep 1' &
  installer_pid=$!
  sh "$WORKER" "$installer_pid" "$STAGE" "$LIVE" false preserve netproxy
  wait "$installer_pid"

  [ -d "$STAGE" ]
  [ -f "$LIVE/update" ]
  assert_file_contains "$LIVE/module.prop" 'version=test-old'
}

test_hot_commit_through_su() {
  [ -x /system/bin/sh ] && [ -d /data/adb ] && command -v su > /dev/null 2>&1 || return 0

  rm -rf "$MODULE_ROOT"
  write_stage_module
  write_live_module

  sh -c 'sleep 1' &
  installer_pid=$!
  su -c "/system/bin/sh -s -- '$installer_pid' '$STAGE' '$LIVE' false preserve netproxy" < "$WORKER"
  wait "$installer_pid"

  [ -d "$LIVE" ]
  [ ! -e "$STAGE" ]
  [ ! -e "$LIVE/update" ]
  assert_file_contains "$LIVE/module.prop" 'version=test-new'
  assert_file_contains "$LIVE/data/catalog/default/provider.json" 'live-provider'
}

test_fresh_install_discards_existing_state() {
  rm -rf "$MODULE_ROOT"
  write_stage_module
  write_live_module

  sh -c 'sleep 1' &
  installer_pid=$!
  sh "$WORKER" "$installer_pid" "$STAGE" "$LIVE" false fresh netproxy
  wait "$installer_pid"

  [ -d "$LIVE" ]
  [ ! -e "$STAGE" ]
  [ ! -e "$LIVE/update" ]
  assert_file_contains "$LIVE/module.prop" 'version=test-new'
  assert_file_contains "$LIVE/config/module.conf" 'OUTBOUND_MODE=rule'
  assert_file_contains "$LIVE/config/ebpf/ebpf.conf" 'EBPF_DNS_MODE=off'
  assert_file_contains "$LIVE/config/singbox/confdir/03_dns.json" 'stage-confdir'
  assert_file_contains "$LIVE/data/catalog/default/provider.json" 'stage-provider'
}

test_hot_commit_preserves_latest_state
test_invalid_stage_keeps_kernel_su_fallback
test_hot_commit_through_su
test_fresh_install_discards_existing_state
printf '%s\n' 'customize hot update test passed'
