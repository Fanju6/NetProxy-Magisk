#!/usr/bin/env sh
# 文件: tests/module_scripts_test.sh
# 功能: 检查模块保留脚本的 POSIX 语法和已删除业务脚本的文件边界
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
  grep -q 'module boot' "$MODULE_DIR/service.sh"
  ! grep -q 'setuidgid\|nohup\|service_main' "$MODULE_DIR/service.sh"
}

#######################################
# 确认已删除的旧业务脚本没有重新进入模块
#######################################
check_removed_scripts() {
  for script in \
    "$MODULE_DIR/scripts/utils/common.sh" \
    "$MODULE_DIR/scripts/utils/state.sh" \
    "$MODULE_DIR/scripts/core/subscription.sh" \
    "$MODULE_DIR/scripts/core/subworker.sh" \
    "$MODULE_DIR/scripts/core/ebpf.sh" \
    "$MODULE_DIR/scripts/core/switch.sh" \
    "$MODULE_DIR/scripts/core/runtime.sh" \
    "$MODULE_DIR/scripts/utils/api.sh" \
    "$MODULE_DIR/scripts/utils/catalog.sh" \
    "$MODULE_DIR/scripts/utils/metadata.sh" \
    "$MODULE_DIR/scripts/core/service.sh" \
    "$MODULE_DIR/scripts/network/netmon.sh" \
    "$MODULE_DIR/scripts/network/tproxy.sh"; do
    if [ -e "$script" ]; then
      printf '%s\n' "旧业务脚本仍存在: $script" >&2
      return 1
    fi
  done
}

#######################################
# 确认升级/卸载只通过 PID 感知的 Worker 入口操作
#######################################
check_worker_lifecycle() {
  ! grep -q 'pkill -f.*subworker' "$MODULE_DIR/customize.sh"
  grep -q 'subworker stop' "$MODULE_DIR/customize.sh"
  grep -q -- '--module-dir' "$MODULE_DIR/customize.sh"
  grep -q 'subworker stop' "$MODULE_DIR/uninstall.sh"
  grep -q -- '--module-dir' "$MODULE_DIR/uninstall.sh"
  grep -q 'webroot' "$MODULE_DIR/customize.sh"
}

check_shell_syntax
check_service_bridge
check_removed_scripts
check_worker_lifecycle
printf '%s\n' 'module scripts test passed'
