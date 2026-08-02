#!/system/bin/sh
#######################################
# 文件: uninstall.sh
# 功能: 模块卸载清理脚本，由 Magisk/KernelSU/APatch 在卸载模块时执行，
#       优雅停止 sing-box，使 eBPF 程序、Map、TC 挂载与本地路由正常释放。
# 用法: 由管理器在卸载时自动调用，无需手动执行。
#######################################

readonly MODDIR="${0%/*}"

# SIGTERM 关闭核心，由 eBPF 入站生命周期负责清理内核资源
if [ -f "$MODDIR/scripts/core/service.sh" ]; then
  sh "$MODDIR/scripts/core/service.sh" stop > /dev/null 2>&1 || true
fi
