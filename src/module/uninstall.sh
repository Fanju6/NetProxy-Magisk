#!/system/bin/sh
#######################################
# 文件: uninstall.sh
# 功能: 模块卸载清理脚本，由 Magisk/KernelSU/APatch 在卸载模块时执行，
#       优雅停止 sing-box，使 eBPF 程序、Map、TC 挂载与本地路由正常释放。
# 用法: 由管理器在卸载时自动调用，无需手动执行。
#######################################

readonly MODDIR="${0%/*}"

# SIGTERM 关闭核心，由 eBPF 入站生命周期负责清理内核资源。
if [ -x "$MODDIR/netproxyctl" ]; then
  "$MODDIR/netproxyctl" service stop > /dev/null 2>&1 || true
fi

# 订阅 Worker 独立于代理核心运行，卸载时单独停止。
if [ -x "$MODDIR/bin/netproxy-native" ]; then
  "$MODDIR/bin/netproxy-native" subworker stop \
    --module-dir "$MODDIR" > /dev/null 2>&1 || true
fi

rm -rf /dev/netproxy/subscriptions /dev/netproxy/subworker.pid 2> /dev/null || true
