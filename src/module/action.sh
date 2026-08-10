#!/system/bin/sh
#######################################
# 文件: action.sh
# 功能: 模块管理器中的操作按钮入口，根据 sing-box 当前运行状态
#       一键切换：运行中则停止，未运行则启动。
# 用法: 由 Magisk/KernelSU/APatch 管理器点击模块操作按钮时调用。
# 依赖: netproxyctl
#######################################

# 模块根目录与关键路径
readonly MODDIR="${0%/*}"                                  # 模块根目录 (脚本所在目录)
readonly NETPROXY_CTL="$MODDIR/netproxyctl"                # 模块管理入口
readonly LOG_FILE="$MODDIR/logs/service.log"               # 服务日志
readonly SING_BOX_BIN="$MODDIR/bin/sing-box"               # sing-box 二进制
readonly LOG_TAG="action"                                  # 日志组件标签

log() {
  local level="INFO" message="$1"
  if [ "$#" -ge 2 ]; then
    level="$1"
    message="$2"
  fi
  printf '[%s] [%s] [%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$level" "$LOG_TAG" "$message" >> "$LOG_FILE"
}

#######################################
# 检查 sing-box 是否正在运行
# 参数: 无
# 返回: 0=运行中，非 0=未运行
#######################################
is_sing_box_running() {
  pidof -s "$SING_BOX_BIN" > /dev/null 2>&1
}

# 将 stderr 合并到 stdout，使日志在管理器界面中可见
exec 2>&1

echo "==================================="
echo " NetProxy 模块操作         "
echo "==================================="

# 运行中则停止，未运行则启动；按子脚本退出码反馈真实结果
if is_sing_box_running; then
  log "INFO" "检测到 sing-box 正在运行，准备执行停止操作..."
  if "$NETPROXY_CTL" service stop > /dev/null; then
    echo " 操作结果: NetProxy 服务已停止"
    echo "==================================="
  else
    echo " 操作结果: NetProxy 服务停止失败"
    echo "==================================="
  fi
else
  log "INFO" "检测到 sing-box 未运行，准备执行启动操作..."
  # su 包裹：让 sing-box 迁出冻结 cgroup，避免切后台断网
  if su -c "\"$NETPROXY_CTL\" service start" > /dev/null; then
    echo " 操作结果: NetProxy 服务已启动"
    echo "==================================="
  else
    echo " 操作结果: NetProxy 服务启动失败"
    echo "==================================="
  fi
fi

# 短暂停留，确保日志完整显示后再退出
sleep 1
