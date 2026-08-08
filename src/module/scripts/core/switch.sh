#!/system/bin/sh
#######################################
# 文件: switch.sh
# 功能: 将节点和出站模式切换转交给 Go 业务层。
# 用法: switch.sh node auto [分组]、switch.sh node <分组>/<标签>、switch.sh mode <模式>
# 依赖: netproxy-native。
#######################################

set -u

readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly NETPROXY_NATIVE_BIN="$MODDIR/bin/netproxy-native"

#######################################
# 显示用法
# 参数: 无
# 返回: 始终返回 0
#######################################
show_usage() {
  cat << EOF
用法:
  $(basename "$0") node auto [分组]
  $(basename "$0") node <分组>/<标签>
  $(basename "$0") mode <rule|global|direct|AllowAds>
EOF
}

#######################################
# 执行 Go 节点或模式切换
# 参数: 业务类型及其参数
# 返回: 沿用 Go 命令退出码
#######################################
run_native() {
  [ -x "$NETPROXY_NATIVE_BIN" ] || {
    printf '%s\n' "netproxy-native 不存在或不可执行: $NETPROXY_NATIVE_BIN" >&2
    return 1
  }
  action="$1"
  shift
  case "$action" in
    select)
      exec "$NETPROXY_NATIVE_BIN" module select --module-dir "$MODDIR" "$@"
      ;;
    mode)
      exec "$NETPROXY_NATIVE_BIN" module mode --module-dir "$MODDIR" "$@"
      ;;
    *)
      printf '%s\n' "不支持的切换操作: $action" >&2
      return 1
      ;;
  esac
}

#######################################
# 主入口
# 参数: node 或 mode 及其参数
# 返回: 成功返回 0，参数错误返回 1
#######################################
main() {
  case "${1:-}" in
    node)
      [ -n "${2:-}" ] || { show_usage >&2; return 1; }
      if [ -n "${3:-}" ]; then
        run_native select "$2" "$3"
      else
        run_native select "$2"
      fi
      ;;
    mode)
      [ -n "${2:-}" ] || { show_usage >&2; return 1; }
      run_native mode "$2"
      ;;
    -h|--help|help)
      show_usage
      ;;
    *)
      show_usage >&2
      return 1
      ;;
  esac
}

main "$@"
