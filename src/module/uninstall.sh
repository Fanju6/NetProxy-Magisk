#!/system/bin/sh
#######################################
# 文件: uninstall.sh
# 功能: 模块卸载清理脚本，由 Magisk/KernelSU/APatch 在卸载模块时执行，
#       优雅停止 sing-box，使 eBPF 程序、Map、TC 挂载与本地路由正常释放。
# 用法: 由管理器在卸载时自动调用，无需手动执行。
#######################################

readonly MODDIR="${0%/*}"

#######################################
# 清理卸载前残留的后台 Worker 进程和状态。
# 参数: 无
# 返回: 始终返回 0。
#######################################
stop_worker_processes() {
  [ -d /dev/netproxy ] || return 0
  for pid_file in /dev/netproxy/*worker.pid; do
    [ -f "$pid_file" ] || continue
    pid="$(cat "$pid_file" 2> /dev/null || true)"
    case "$pid" in
      ''|*[!0-9]*) pid='' ;;
    esac
    if [ -n "$pid" ] && [ -r "/proc/$pid/cmdline" ] \
      && grep -q "$MODDIR/bin/netproxyctl" "/proc/$pid/cmdline" 2> /dev/null; then
      kill -TERM "$pid" 2> /dev/null || true
    fi
    rm -f "$pid_file" "$pid_file.lock" 2> /dev/null || true
  done
  rm -rf /dev/netproxy/*worker.pid.lock 2> /dev/null || true
}

# SIGTERM 关闭核心，由 eBPF 入站生命周期负责清理内核资源。
if [ -x "$MODDIR/netproxyctl" ]; then
  "$MODDIR/netproxyctl" service stop > /dev/null 2>&1 || true
fi

# 后台 Worker 独立于代理核心运行，卸载时单独停止。
if [ -x "$MODDIR/bin/netproxyctl" ]; then
  "$MODDIR/bin/netproxyctl" __internal worker stop \
    --module-dir "$MODDIR" > /dev/null 2>&1 || true
fi
stop_worker_processes

rm -rf /dev/netproxy/subscriptions 2> /dev/null || true
