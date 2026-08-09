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
    "$MODULE_DIR/scripts/network/netmon.sh" \
    "$MODULE_DIR/scripts/network/tproxy.sh"; do
    if [ -e "$script" ]; then
      printf '%s\n' "旧业务脚本仍存在: $script" >&2
      return 1
    fi
  done
}

check_shell_syntax
check_removed_scripts
printf '%s\n' 'module scripts test passed'
