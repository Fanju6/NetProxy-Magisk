#!/usr/bin/env sh
# 文件: tests/module_scripts_test.sh
# 功能: 检查模块 Shell 的 POSIX 语法和运行时桥接边界
# 用法: sh tests/module_scripts_test.sh
# 依赖: POSIX sh、find、sort

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
MODULE_DIR="$ROOT/src/module"

#######################################
# 检查所有保留 Shell 的语法
#######################################
check_shell_syntax() {
  find "$MODULE_DIR" -type f -name '*.sh' -print | sort | while IFS= read -r script; do
    sh -n "$script"
  done
}

#######################################
# 确认仅保留根目录开机桥接脚本
#######################################
check_service_bridge() {
  [ -f "$MODULE_DIR/service.sh" ] || {
    printf '%s\n' "缺少模块开机桥接脚本: $MODULE_DIR/service.sh" >&2
    return 1
  }
  grep -q '__internal boot' "$MODULE_DIR/service.sh"
  ! grep -q 'module boot' "$MODULE_DIR/service.sh"
  ! grep -q 'setuidgid\|nohup\|service_main' "$MODULE_DIR/service.sh"
}

#######################################
# 确认管理器操作按钮不在 Shell 中推断服务状态
#######################################
check_action_bridge() {
  grep -q 'service toggle' "$MODULE_DIR/action.sh"
  ! grep -q 'pidof\|is_sing_box_running' "$MODULE_DIR/action.sh"
  ! grep -q 'grep.*schema\|grep.*json' "$MODULE_DIR/action.sh"
}

#######################################
# 确认运行时 Shell 保持在根目录桥接层
#######################################
check_runtime_scripts() {
  [ -f "$MODULE_DIR/service.sh" ] || return 1
  if [ -d "$MODULE_DIR/scripts" ] \
    && find "$MODULE_DIR/scripts" -type f -name '*.sh' -print \
      | grep -q .; then
    printf '%s\n' '运行时业务 Shell 应通过 Go 组件提供' >&2
    return 1
  fi
}

#######################################
# 确认模块脚本使用 Android mksh 可执行的 POSIX 读取语法
#######################################
check_mksh_compatible_helpers() {
  ! grep -q -- '-print0\|read -r -d' "$MODULE_DIR/customize.sh"
}

#######################################
# 确认默认透明代理配置不再携带 FakeIP 或内部 bpftool
# 参数: 无
# 返回: 0=通过，1=仍有旧生成物或默认字段。
#######################################
check_removed_legacy_ebpf_assets() {
  [ ! -e "$MODULE_DIR/bin/bpftool" ]
  ! grep -Rqi 'fakeip\|fake-ip\|198\.18\.0\.0/15\|fc00::/18\|store_fakeip' \
    "$MODULE_DIR/config/singbox/config.json"
}

#######################################
# 确认升级/卸载只通过 PID 感知的 Worker 入口操作
#######################################
check_worker_lifecycle() {
  ! grep -q 'pkill -f' "$MODULE_DIR/customize.sh"
  grep -q 'cleanup_worker_state' "$MODULE_DIR/customize.sh"
  grep -q 'worker stop' "$MODULE_DIR/customize.sh"
  grep -q -- '--module-dir' "$MODULE_DIR/customize.sh"
  grep -q 'worker stop' "$MODULE_DIR/uninstall.sh"
  grep -q 'stop_worker_processes' "$MODULE_DIR/uninstall.sh"
  grep -q -- '--module-dir' "$MODULE_DIR/uninstall.sh"
  ! grep -q 'sync_to_live\|restart_proxy_if_needed' "$MODULE_DIR/customize.sh"
  grep -q 'schedule_hot_update' "$MODULE_DIR/customize.sh"
  grep -q 'NETPROXY_HOT_UPDATE_WORKER_BEGIN' "$MODULE_DIR/customize.sh"
}

#######################################
# 确认安装方式与随附管理器由安装脚本自身决定
#######################################
check_install_choices() {
  grep -q 'choose_install_mode' "$MODULE_DIR/customize.sh"
  grep -q '保留现有数据' "$MODULE_DIR/customize.sh"
  grep -q '全新安装' "$MODULE_DIR/customize.sh"
  grep -q '\[ "$(wait_volume_key 10)" = "down" \]' "$MODULE_DIR/customize.sh"
  grep -q 'install_bundled_manager' "$MODULE_DIR/customize.sh"
  grep -q 'print_title "安装 NetProxy 管理器"' "$MODULE_DIR/customize.sh"
  grep -q '\[ ! -f "\$MODPATH/NetProxy.apk" \]' "$MODULE_DIR/customize.sh"
  grep -q 'MANAGER_PACKAGE="com.fanjv.netproxy"' "$MODULE_DIR/customize.sh"
  grep -q 'get_installed_manager_version' "$MODULE_DIR/customize.sh"
  grep -q 'dumpsys package' "$MODULE_DIR/customize.sh"
  ! grep -q 'am start -a android.intent.action.VIEW' "$MODULE_DIR/customize.sh"
  grep -q 'getevent -lqc 1 > "\$event_file"' "$MODULE_DIR/customize.sh"
}

#######################################
# 确认安装前先停止旧服务，再进入管理器安装区块
#######################################
check_install_order() {
  main_flow="$(sed -n '/unzip -o "\$ZIPFILE" "module.prop"/,/else$/p' "$MODULE_DIR/customize.sh")"
  choice_line="$(printf '%s\n' "$main_flow" | grep -n '^choose_install_mode || exit 1$' | head -n 1 | cut -d: -f1)"
  stop_line="$(printf '%s\n' "$main_flow" | grep -n 'stop_proxy_if_running' | head -n 1 | cut -d: -f1)"
  manager_line="$(printf '%s\n' "$main_flow" | grep -n 'install_bundled_manager' | head -n 1 | cut -d: -f1)"
  [ -n "$choice_line" ] && [ -n "$stop_line" ] && [ -n "$manager_line" ] \
    && [ "$choice_line" -lt "$stop_line" ] && [ "$stop_line" -lt "$manager_line" ]
}

check_shell_syntax
check_service_bridge
check_action_bridge
check_runtime_scripts
check_mksh_compatible_helpers
check_removed_legacy_ebpf_assets
check_worker_lifecycle
check_install_choices
check_install_order
printf '%s\n' 'module scripts test passed'
