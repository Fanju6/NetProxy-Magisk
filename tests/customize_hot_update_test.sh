#!/usr/bin/env sh
# 文件: tests/customize_hot_update_test.sh
# 功能: 验证安装选择、当前数据快照、热切换互斥和失败恢复。
# 用法: sh tests/customize_hot_update_test.sh [netproxyctl 测试程序]
# 依赖: POSIX sh、awk、sed、mktemp；Linux 锁测试需要 flock。

set -eu
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
CUSTOMIZE="$ROOT/src/module/customize.sh"
NATIVE_CTL="${1:-}"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT HUP INT TERM
LIVE="$WORKDIR/modules/netproxy"
LIVE_DIR="$LIVE"
STAGE="$WORKDIR/modules_update/netproxy"
FUNCTIONS="$WORKDIR/functions.sh"
CALL_LOG="$WORKDIR/calls"
export CALL_LOG

# 只替换测试状态目录；所有被测函数直接取自生产脚本。
awk '
  /^readonly / { print }
  /^print_title\(\)/ { emit = 1 }
  /^if \[ "\$BACKGROUND" = true \]; then/ { exit }
  emit { print }
' "$CUSTOMIZE" | sed "s@/dev/netproxy@$WORKDIR/state@g" > "$FUNCTIONS"
. "$FUNCTIONS"

ui_print() { printf '%s\n' "$*"; }
chown() { :; }
chcon() { :; }
pidof() { [ "${TEST_RUNNING:-false}" = true ]; }
su() { [ "$1" = -c ]; sh -c "$2"; }

write_module() {
  target="$1" label="$2"
  mkdir -p "$target/bin" "$target/config/ebpf" "$target/config/singbox/rules/local" \
    "$target/config/singbox/rules/remote" "$target/config/singbox/custom-state" \
    "$target/data/catalog/default" "$target/runtime" "$target/logs"
  printf 'id=netproxy\nversion=%s\n' "$label" > "$target/module.prop"
  printf '%s\n' "$label-module" > "$target/config/module.conf"
  printf '%s\n' "$label-ebpf" > "$target/config/ebpf/ebpf.conf"
  printf '%s\n' "$label-config" > "$target/config/singbox/config.json"
  printf '%s\n' "$label-local" > "$target/config/singbox/rules/local/custom.json"
  printf '%s\n' "$label-remote" > "$target/config/singbox/rules/remote/test.srs"
  printf '%s\n' "$label-cache" > "$target/config/singbox/cache.db"
  printf '%s\n' "$label-state" > "$target/config/singbox/custom-state/state.json"
  printf '%s\n' "$label-meta" > "$target/data/catalog/default/meta.json"
  printf '%s\n' "$label-provider" > "$target/data/catalog/default/provider.json"
  printf '%s\n' "$label-history" > "$target/data/catalog/default/history.jsonl"
  printf '%s\n' "$label-log" > "$target/logs/service.log"
  printf '%s\n' "$label-runtime" > "$target/runtime/providers.json"
  printf '%s\n' '#!/bin/sh' \
    'printf "%s\n" "$*" >> "$CALL_LOG"' \
    'case "$1:$2:$3" in' \
    '  __internal:worker:stop) exit "${TEST_WORKER_STOP:-0}" ;;' \
    '  service:stop:) exit "${TEST_SERVICE_STOP:-0}" ;;' \
    '  config:check:) exit "${TEST_CONFIG_CHECK:-0}" ;;' \
    'esac' > "$target/bin/netproxyctl"
  cp "$target/bin/netproxyctl" "$target/netproxyctl"
  printf '%s\n' 'core fixture' > "$target/bin/sing-box"
  for file in service.sh action.sh uninstall.sh; do
    printf '%s\n' '#!/bin/sh' > "$target/$file"
  done
  chmod +x "$target/bin/netproxyctl" "$target/netproxyctl"
}

reset_modules() {
  rm -rf "$WORKDIR/modules" "$WORKDIR/modules_update" "$WORKDIR/state"
  write_module "$STAGE" package
  write_module "$LIVE" current
  mkdir -p "$LIVE/data/catalog/staging" "$LIVE/data/catalog/subscription"
  mkdir -p "$LIVE/config/telemetry"
  printf '%s\n' 'current-telemetry-queue' > "$LIVE/config/telemetry/state.json"
  printf '%s\n' 'temporary' > "$LIVE/data/catalog/staging/download.tmp"
  printf '%s\n' 'subscription-meta' > "$LIVE/data/catalog/subscription/meta.json"
  printf '%s\n' 'subscription-provider' > "$LIVE/data/catalog/subscription/provider.json"
  : > "$LIVE/update"
  : > "$CALL_LOG"
  MODPATH="$STAGE"
  INSTALL_MODE=preserve
  PROXY_WAS_RUNNING=false
  SERVICE_STOPPED=false
  KEY_PID=""
  TEST_RUNNING=false
  unset TEST_WORKER_STOP TEST_SERVICE_STOP TEST_CONFIG_CHECK TEST_PIDOF
}

assert_value() { grep -Fxq "$2" "$1"; }

test_install_choices() (
  reset_modules
  wait_volume_key() {
    printf '%s\n' "$1" >> "$WORKDIR/timeouts"
    VOLUME_KEY="${CHOICES%% *}"
    if [ "$CHOICES" = "$VOLUME_KEY" ]; then CHOICES=timeout; else CHOICES="${CHOICES#* }"; fi
  }
  for pair in 'timeout:preserve' 'down:preserve' 'up down:nodes' \
    'up up down down:fresh' 'up up up down:preserve' 'up up down up down:preserve'; do
    CHOICES="${pair%:*}"
    choose_install_mode > "$WORKDIR/menu"
    [ "$INSTALL_MODE" = "${pair##*:}" ]
  done
  for CHOICES in 'up timeout' 'up up timeout' 'up up down timeout'; do
    if choose_install_mode > "$WORKDIR/menu"; then
      printf '%s\n' '操作后超时不能自动确认安装' >&2
      exit 1
    fi
    assert_value "$LIVE/config/module.conf" current-module
  done
  rm "$LIVE/config/singbox/config.json"
  CHOICES=timeout
  if choose_install_mode > "$WORKDIR/menu"; then exit 1; fi
  CHOICES='up down'
  choose_install_mode > "$WORKDIR/menu"
  [ "$INSTALL_MODE" = nodes ]
  rm -rf "$LIVE"
  CHOICES=timeout
  choose_install_mode > "$WORKDIR/menu"
  [ "$INSTALL_MODE" = fresh ]
  grep -qx 10 "$WORKDIR/timeouts"
  grep -qx 20 "$WORKDIR/timeouts"
)

test_key_events() (
  reset_modules
  INSTALL_TMP="$WORKDIR"
  printf '%s\n' \
    '/dev/input/event0: EV_KEY KEY_VOLUMEUP UP' \
    '/dev/input/event0: EV_KEY KEY_VOLUMEUP REPEAT' \
    '/dev/input/event0: EV_KEY KEY_VOLUMEUP DOWN                ' \
    '/dev/input/event0: EV_KEY KEY_VOLUMEUP UP' \
    '/dev/input/event0: EV_KEY KEY_VOLUMEDOWN 00000001            ' > "$INSTALL_TMP/keys"
  sleep 60 &
  KEY_PID=$!
  trap stop_key_listener EXIT
  KEY_FD=9
  exec 9<"$INSTALL_TMP/keys"
  wait_volume_key 1
  [ "$VOLUME_KEY" = up ]
  wait_volume_key 1
  [ "$VOLUME_KEY" = down ]
  sleep() { :; }
  wait_volume_key 1
  [ "$VOLUME_KEY" = timeout ]
  stop_key_listener
  [ -z "$KEY_PID" ]
)

test_key_output_descriptor() (
  reset_modules
  INSTALL_TMP="$WORKDIR"
  OUTFD=9
  exec 9>"$WORKDIR/installer-output"
  getevent() { :; }
  sleep() { :; }
  wait_volume_key 1
  [ "$KEY_FD" = 8 ]
  stop_key_listener
  stop_key_listener
  printf '%s\n' 'installer-output' >&9
  exec 9>&-
  assert_value "$WORKDIR/installer-output" installer-output
)

test_snapshot_modes() (
  for INSTALL_MODE in preserve nodes fresh; do
    mode="$INSTALL_MODE"
    reset_modules
    INSTALL_MODE="$mode"
    copy_user_data_locked
    if [ "$mode" = fresh ]; then
      assert_value "$STAGE/data/catalog/default/provider.json" package-provider
      assert_value "$STAGE/logs/service.log" package-log
    else
      assert_value "$STAGE/data/catalog/default/provider.json" current-provider
      assert_value "$STAGE/data/catalog/subscription/provider.json" subscription-provider
      assert_value "$STAGE/data/catalog/default/history.jsonl" current-history
      assert_value "$STAGE/logs/service.log" current-log
    fi
    if [ "$mode" = preserve ]; then
      assert_value "$STAGE/config/ebpf/ebpf.conf" current-ebpf
      assert_value "$STAGE/config/module.conf" current-module
      assert_value "$STAGE/config/singbox/config.json" current-config
      assert_value "$STAGE/config/singbox/rules/local/custom.json" current-local
      assert_value "$STAGE/config/singbox/cache.db" current-cache
      assert_value "$STAGE/config/singbox/custom-state/state.json" current-state
      assert_value "$STAGE/config/telemetry/state.json" current-telemetry-queue
    else
      assert_value "$STAGE/config/ebpf/ebpf.conf" package-ebpf
      assert_value "$STAGE/config/module.conf" package-module
      assert_value "$STAGE/config/singbox/rules/local/custom.json" package-local
      assert_value "$STAGE/config/singbox/cache.db" package-cache
      assert_value "$STAGE/config/singbox/custom-state/state.json" package-state
      [ ! -e "$STAGE/config/telemetry/state.json" ]
    fi
    assert_value "$STAGE/config/singbox/rules/remote/test.srs" package-remote
    assert_value "$STAGE/runtime/providers.json" package-runtime
    [ ! -e "$STAGE/data/catalog/staging/download.tmp" ]
    [ -z "$(find "$STAGE" -name '.install-state.*' -print)" ]
  done
)

test_snapshot_failure() (
  reset_modules
  cp() { return 1; }
  if copy_user_data_locked; then exit 1; fi
  assert_value "$STAGE/config/module.conf" package-module
  assert_value "$LIVE/data/catalog/default/provider.json" current-provider
)

test_snapshot_rename_failure() (
  reset_modules
  mv() {
    case "$1" in */current/data/catalog) return 1 ;; esac
    command mv "$@"
  }
  if copy_user_data_locked; then exit 1; fi
  assert_value "$STAGE/config/module.conf" package-module
  assert_value "$STAGE/config/ebpf/ebpf.conf" package-ebpf
  assert_value "$STAGE/config/singbox/cache.db" package-cache
  [ -z "$(find "$STAGE" -name '.install-state.*' -print)" ]
)

test_missing_current_data() (
  reset_modules
  rm "$LIVE/config/singbox/config.json"
  if copy_user_data_locked; then exit 1; fi
  assert_value "$STAGE/config/module.conf" package-module
  INSTALL_MODE=nodes
  copy_user_data_locked
  assert_value "$STAGE/config/singbox/config.json" package-config
  mkdir -p "$LIVE/runtime/.config-apply"
  if copy_user_data_locked; then exit 1; fi
)

test_permissions() (
  reset_modules
  set_permissions
  case "$(uname -s)" in
    MINGW*|MSYS*) printf '%s\n' 'NTFS 权限位不作断言，权限断言需在 Linux 执行' ;;
    *)
      for file in config/module.conf config/ebpf/ebpf.conf data/catalog/default/provider.json logs/service.log; do
        [ "$(stat -c '%a' "$STAGE/$file")" = 600 ]
      done
      [ "$(stat -c '%a' "$STAGE/config")" = 700 ]
      [ "$(stat -c '%a' "$STAGE/service.sh")" = 755 ]
      ;;
  esac
  chcon() { return 1; }
  if set_permissions; then exit 1; fi
)

test_service_failures() (
  reset_modules
  TEST_RUNNING=true
  TEST_WORKER_STOP=1
  export TEST_WORKER_STOP
  if stop_proxy_if_running; then exit 1; fi
  [ "$(wc -l < "$CALL_LOG")" -eq 1 ]
  restore_live_service
  grep -q '^service start$' "$CALL_LOG"
  reset_modules
  TEST_SERVICE_STOP=1
  export TEST_SERVICE_STOP
  if stop_proxy_if_running; then exit 1; fi
  restore_live_service
  grep -q '^__internal worker start' "$CALL_LOG"
  ! grep -q '^service start$' "$CALL_LOG"
)

test_manager_install() (
  reset_modules
  INSTALL_TMP="$WORKDIR/install"
  mkdir -p "$INSTALL_TMP"
  pm() {
    printf 'pm %s\n' "$*" >> "$CALL_LOG"
    [ "$1" = install ] && [ "$2" = -r ] || return 1
    if [ "${MANAGER_INSTALL_EXIT:-0}" != 0 ]; then
      printf 'Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE]' >&2
      return "$MANAGER_INSTALL_EXIT"
    fi
  }
  dumpsys() { printf 'unexpected dumpsys call\n' >> "$CALL_LOG"; return 1; }
  wait_volume_key() { VOLUME_KEY="$CHOICE"; }
  if install_bundled_manager > "$WORKDIR/manager"; then exit 1; fi
  grep -Fq '安装 NetProxy 管理器' "$WORKDIR/manager"
  grep -Fq '缺少' "$WORKDIR/manager"
  [ ! -s "$CALL_LOG" ]
  : > "$STAGE/NetProxy.apk"
  if install_bundled_manager > "$WORKDIR/manager"; then exit 1; fi
  for CHOICE in down timeout up; do
    : > "$CALL_LOG"
    printf 'manager fixture\n' > "$STAGE/NetProxy.apk"
    install_bundled_manager > "$WORKDIR/manager"
    [ ! -e "$STAGE/NetProxy.apk" ]
    if [ "$CHOICE" = down ]; then
      [ ! -s "$CALL_LOG" ]
      grep -Fq '已跳过管理器安装' "$WORKDIR/manager"
    else
      grep -Fxq "pm install -r $STAGE/NetProxy.apk" "$CALL_LOG"
      [ "$(wc -l < "$CALL_LOG")" -eq 1 ]
      grep -Fq '管理器安装成功' "$WORKDIR/manager"
    fi
  done
  # 重复安装仍直接调用覆盖安装，不读取已安装版本或自动跳过。
  printf 'manager fixture\n' > "$STAGE/NetProxy.apk"
  install_bundled_manager > "$WORKDIR/manager"
  [ "$(wc -l < "$CALL_LOG")" -eq 2 ]
  MANAGER_INSTALL_EXIT=1
  CHOICE=up
  printf 'manager fixture\n' > "$STAGE/NetProxy.apk"
  install_bundled_manager > "$WORKDIR/manager"
  grep -Fq '管理器安装失败' "$WORKDIR/manager"
  grep -Fq 'INSTALL_FAILED_UPDATE_INCOMPATIBLE' "$WORKDIR/manager"
  grep -Fq '未卸载或清除现有应用' "$WORKDIR/manager"
  [ ! -e "$STAGE/NetProxy.apk" ]
  printf 'manager fixture\n' > "$STAGE/NetProxy.apk"
  rm() { return 1; }
  if install_bundled_manager > "$WORKDIR/manager"; then exit 1; fi
  [ -f "$STAGE/NetProxy.apk" ]
)

test_hot_update() (
  for mode in preserve nodes fresh; do
    reset_modules
    [ ! -e "$STAGE/NetProxy.apk" ]
    INSTALL_MODE="$mode"
    INSTALLER_PID=99999999
    sleep() { :; }
    with_user_data_locks() {
      CATALOG_LOCK=data/.catalog.netproxy-test.lock
      for entry in config/ebpf/ebpf.conf.lock config/module.conf.lock config/singbox/config.json.lock "$CATALOG_LOCK"; do
        : > "$LIVE/$entry"
      done
      "$@"
    }
    apply_hot_update
    [ ! -e "$STAGE" ] && [ ! -e "$LIVE/update" ]
    assert_value "$LIVE/module.prop" version=package
    grep -Fq '后台热更新已完成' "$LIVE/logs/service.log"
    grep -q '^__internal worker start' "$CALL_LOG"
    [ -z "$(find "$WORKDIR/modules" -name '.netproxy.install-backup.*' -print)" ]
  done
)

test_hot_update_guards() (
  reset_modules
  INSTALLER_PID="$$"
  sleep() { :; }
  if apply_hot_update; then exit 1; fi
  assert_value "$LIVE/module.prop" version=current
  [ ! -s "$CALL_LOG" ]
  INSTALLER_PID=99999999
  rm "$LIVE/update"
  if apply_hot_update; then exit 1; fi
  assert_value "$LIVE/module.prop" version=current
  : > "$LIVE/update"
  rm "$STAGE/bin/sing-box"
  if apply_hot_update; then exit 1; fi
  [ -d "$STAGE" ] && [ -f "$LIVE/update" ]
)

test_hot_update_latest_data() (
  reset_modules
  copy_user_data_locked
  assert_value "$STAGE/config/module.conf" current-module
  INSTALLER_PID=99999999
  sleep() {
    printf '%s\n' 'latest-module' > "$LIVE/config/module.conf"
    printf '%s\n' 'latest-provider' > "$LIVE/data/catalog/default/provider.json"
  }
  with_user_data_locks() {
    CATALOG_LOCK=data/.catalog.netproxy-test.lock
    for entry in config/ebpf/ebpf.conf.lock config/module.conf.lock config/singbox/config.json.lock "$CATALOG_LOCK"; do
      : > "$LIVE/$entry"
    done
    "$@"
  }
  apply_hot_update
  assert_value "$LIVE/config/module.conf" latest-module
  assert_value "$LIVE/data/catalog/default/provider.json" latest-provider

  reset_modules
  rm "$LIVE/config/singbox/config.json"
  if apply_hot_update; then exit 1; fi
  restore_live_service
  [ -d "$STAGE" ] && [ -f "$LIVE/update" ]
  assert_value "$LIVE/data/catalog/default/provider.json" latest-provider
  grep -q '^__internal worker start' "$CALL_LOG"
)

test_hot_rename_failure() (
  reset_modules
  CATALOG_LOCK=data/.catalog.netproxy-test.lock
  for entry in config/ebpf/ebpf.conf.lock config/module.conf.lock config/singbox/config.json.lock "$CATALOG_LOCK"; do
    : > "$LIVE/$entry"
  done
  mv() {
    [ "$1" != "$STAGE" ] || return 1
    command mv "$@"
  }
  if commit_hot_update; then exit 1; fi
  assert_value "$LIVE/module.prop" version=current
  assert_value "$LIVE/config/ebpf/ebpf.conf" current-ebpf
  [ -f "$LIVE/update" ] && [ -d "$STAGE" ]
)

test_linux_locks() (
  command -v flock >/dev/null 2>&1 || {
    printf '%s\n' '当前 Shell 无 flock，互斥测试需在 Linux 执行'
    exit 0
  }
  reset_modules
  with_user_data_locks copy_user_data_locked
  catalog_lock="$(find "$LIVE/data" -name '.catalog.netproxy-*.lock')"
  inode="$(stat -c '%i' "$catalog_lock")"
  config_inode="$(stat -c '%i' "$LIVE/config/module.conf.lock")"
  for busy in "$catalog_lock" "$LIVE/config/module.conf.lock" "$WORKDIR/state/service.lock.flock"; do
    (
      exec 3>"$busy"
      flock -n 3 3>&3
      if with_user_data_locks commit_hot_update; then exit 1; fi
      assert_value "$LIVE/module.prop" version=current
    )
  done
  group_lock="$LIVE/data/.catalog.netproxy-group.lock"
  (
    exec 3>"$group_lock"
    flock -n 3 3>&3
    if with_user_data_locks commit_hot_update; then exit 1; fi
  )
  with_user_data_locks commit_hot_update
  [ "$(stat -c '%i' "$LIVE/data/${catalog_lock##*/}")" = "$inode" ]
  [ "$(stat -c '%i' "$LIVE/config/module.conf.lock")" = "$config_inode" ]
  [ -f "$LIVE/data/${group_lock##*/}" ]
)

test_native_catalog_lock() (
  [ -n "$NATIVE_CTL" ] && command -v flock >/dev/null 2>&1 || exit 0
  reset_modules
  cp "$ROOT/src/module/config/module.conf" "$LIVE/config/module.conf"
  cp "$ROOT/src/module/data/catalog/default/meta.json" "$LIVE/data/catalog/default/meta.json"
  cp "$ROOT/src/module/data/catalog/default/provider.json" "$LIVE/data/catalog/default/provider.json"
  rm -rf "$LIVE/data/catalog/subscription"
  native_must_wait() {
    if NETPROXY_MODULE_DIR="$LIVE" "$NATIVE_CTL" --timeout 100ms catalog list > "$WORKDIR/lock-result" 2>&1; then
      printf '%s\n' 'Shell 快照锁未阻止 Go Catalog 读取' >&2
      return 1
    fi
    grep -Fq '"code":"command.timeout"' "$WORKDIR/lock-result" || { cat "$WORKDIR/lock-result" >&2; return 1; }
  }
  with_user_data_locks native_must_wait
  NETPROXY_MODULE_DIR="$LIVE" "$NATIVE_CTL" catalog list > "$WORKDIR/lock-result"
  grep -Fq '"ok":true' "$WORKDIR/lock-result"
)

test_installer_entrypoints() (
  ASH_STANDALONE=0
  export ASH_STANDALONE
  command -v flock >/dev/null 2>&1 || exit 0
  mocks="$WORKDIR/mocks"
  mkdir -p "$mocks"
  for name in chown chcon getevent sleep; do
    printf '#!/bin/sh\nexit 0\n' > "$mocks/$name"
  done
  printf '#!/bin/sh\nexit "${TEST_PIDOF:-1}"\n' > "$mocks/pidof"
  printf '%s\n' '#!/bin/sh' '[ -p /proc/self/fd/1 ] || exit 2' \
    '[ "$#" -eq 3 ] && [ "$1" = install ] && [ "$2" = -r ] && [ -s "$3" ]' > "$mocks/pm"
  printf '%s\n' '#!/bin/sh' '[ "$1" = -c ] || exit 1' \
    'case "$2" in */system/bin/sh*) exit 0 ;; esac' 'exec sh -c "$2"' > "$mocks/su"
  chmod +x "$mocks"/*
  PATH="$mocks:$PATH"
  export PATH
  script="$WORKDIR/customize.sh"
  sed "s@^LIVE_DIR=/data/adb/modules/netproxy@LIVE_DIR=$LIVE@;s@/dev/netproxy@$WORKDIR/state@g" "$CUSTOMIZE" > "$script"
  SCRIPT="$script"
  export SCRIPT

  run_foreground() {
    TMPDIR="$WORKDIR" MODPATH="$STAGE" ZIPFILE=fixture \
      sh -c '
        ui_print() { printf "%s\n" "$*"; }
        grep_prop() { sed -n "s/^$1=//p" "$2"; }
        unzip() {
          if [ "$3" = module.prop ]; then
            cp "$FIXTURE/module.prop" "$5/"
          else
            cp -a "$FIXTURE/." "$MODPATH/"
          fi
        }
        . "$SCRIPT"
      ' > "$WORKDIR/install-output" 2>&1
  }

  reset_modules
  FIXTURE="$WORKDIR/package"
  export FIXTURE
  cp -a "$STAGE" "$FIXTURE"
  printf 'manager fixture\n' > "$FIXTURE/NetProxy.apk"
  rm -rf "$LIVE" "$STAGE"
  mkdir -p "$STAGE"
  BOOTMODE=false
  export BOOTMODE
  run_foreground
  [ ! -e "$LIVE" ]
  [ ! -e "$STAGE/NetProxy.apk" ]
  grep -Fq '管理器安装成功' "$WORKDIR/install-output"
  assert_value "$STAGE/config/ebpf/ebpf.conf" package-ebpf
  [ ! -s "$CALL_LOG" ]

  original_stage="$STAGE"
  STAGE="$LIVE"
  mkdir -p "$LIVE"
  run_foreground
  assert_value "$LIVE/config/ebpf/ebpf.conf" package-ebpf
  [ ! -s "$CALL_LOG" ]
  if run_foreground; then exit 1; fi
  assert_value "$LIVE/config/ebpf/ebpf.conf" package-ebpf
  STAGE="$original_stage"

  reset_modules
  run_foreground
  assert_value "$STAGE/config/ebpf/ebpf.conf" current-ebpf
  assert_value "$LIVE/module.prop" version=current

  reset_modules
  BOOTMODE=true TEST_CONFIG_CHECK=1 TEST_PIDOF=0
  export TEST_CONFIG_CHECK TEST_PIDOF
  if run_foreground; then exit 1; fi
  assert_value "$LIVE/module.prop" version=current
  [ -d "$STAGE" ]
  grep -q '^config check$' "$CALL_LOG"
  ! grep -q '^service stop$' "$CALL_LOG"
  ! grep -q '^__internal worker stop' "$CALL_LOG"

  reset_modules
  rm "$STAGE/bin/sing-box"
  if sh "$script" --apply-update 99999999 "$STAGE" "$LIVE" preserve; then exit 1; fi
  assert_value "$LIVE/module.prop" version=current
  [ -f "$LIVE/update" ]
  [ ! -s "$CALL_LOG" ]

  reset_modules
  # 安装器可以清理 customize.sh，后台只依赖已经打开的标准输入。
  sh -s -- --apply-update 99999999 "$STAGE" "$LIVE" preserve < "$script"
  [ ! -e "$STAGE" ] && [ ! -e "$LIVE/update" ]
  assert_value "$LIVE/config/ebpf/ebpf.conf" current-ebpf
  grep -q '^__internal worker start' "$CALL_LOG"
  ! grep -q '^service start$' "$CALL_LOG"
)

test_install_choices
test_key_events
test_key_output_descriptor
test_snapshot_modes
test_snapshot_failure
test_snapshot_rename_failure
test_missing_current_data
test_permissions
test_service_failures
test_manager_install
test_hot_update
test_hot_update_guards
test_hot_update_latest_data
test_hot_rename_failure
test_linux_locks
test_native_catalog_lock
test_installer_entrypoints
printf '%s\n' 'customize hot update test passed'
