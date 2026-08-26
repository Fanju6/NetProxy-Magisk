#!/system/bin/sh
#######################################
# 文件: service.sh
# 功能: Magisk/KernelSU/APatch service 阶段启动桥接。
# 用法: 由模块框架无参数调用。
# 依赖: su、netproxyctl
#######################################

readonly MODDIR="$(cd "$(dirname "$0")" && pwd)"
readonly NETPROXY_BIN="$MODDIR/bin/netproxyctl"

[ "$#" -eq 0 ] || {
  printf '%s\n' 'service.sh 仅供模块 service 阶段调用；请使用 netproxyctl service 管理服务。' >&2
  exit 2
}

[ -x "$NETPROXY_BIN" ] || exit 1

# 通过 su 启动原生进程；sing-box 的最终 cgroup 归属由 Go 在启动时显式收敛。
if command -v su > /dev/null 2>&1; then
  exec su -c "\"$NETPROXY_BIN\" __internal module boot --module-dir \"$MODDIR\""
fi
exec "$NETPROXY_BIN" __internal module boot --module-dir "$MODDIR"
