#!/system/bin/sh
#######################################
# 文件: emulated-soft-reboot.sh
# 功能: KernelSU 软重启前停止 Worker 与 sing-box，释放 eBPF 挂载。
# 用法: 由 KernelSU emulated-soft-reboot 阶段同步调用。
# 依赖: bin/netproxyctl
#######################################

readonly MODDIR="$(cd "$(dirname "$0")" && pwd)"
readonly NETPROXY_BIN="$MODDIR/bin/netproxyctl"

[ -x "$NETPROXY_BIN" ] || {
  printf '%s\n' 'KernelSU 软重启前无法停止 NetProxy：netproxyctl 不可执行。' >&2
  exit 1
}

status=0

NETPROXY_MODULE_DIR="$MODDIR" "$NETPROXY_BIN" __internal worker stop \
  --module-dir "$MODDIR" > /dev/null || {
  printf '%s\n' 'KernelSU 软重启前停止 NetProxy Worker 失败。' >&2
  status=1
}

NETPROXY_MODULE_DIR="$MODDIR" "$NETPROXY_BIN" service stop > /dev/null || {
  printf '%s\n' 'KernelSU 软重启前停止 sing-box 失败。' >&2
  status=1
}

exit "$status"
