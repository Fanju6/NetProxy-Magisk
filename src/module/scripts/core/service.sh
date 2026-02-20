#!/system/bin/sh
# NetProxy 服务管理脚本
# 用法: service.sh {start|stop|restart|status}

set -u

readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly LOG_FILE="$MODDIR/logs/service.log"
readonly XRAY_BIN="$MODDIR/bin/xray"
readonly MODULE_CONF="$MODDIR/config/module.conf"
readonly XRAY_LOG_FILE="$MODDIR/logs/xray.log"
readonly CONFDIR="$MODDIR/config/xray/confdir"
readonly OUTBOUNDS_DIR="$MODDIR/config/xray/outbounds"
readonly TPROXY_CONF="$MODDIR/config/tproxy.conf"

readonly KILL_TIMEOUT=5

# 检测 busybox 路径
detect_busybox() {
  if [ -f "/data/adb/ksu/bin/busybox" ]; then
    echo "/data/adb/ksu/bin/busybox"
  elif [ -f "/data/adb/ap/bin/busybox" ]; then
    echo "/data/adb/ap/bin/busybox"
  elif [ -f "/data/adb/magisk/busybox" ]; then
    echo "/data/adb/magisk/busybox"
  else
    echo "busybox"
  fi
}

readonly BUSYBOX="$(detect_busybox)"

# 导入工具库
. "$MODDIR/scripts/utils/log.sh"

export PATH="$MODDIR/bin:$PATH"

#######################################
# 错误退出
#######################################
die() {
  log "ERROR" "$1"
  exit "${2:-1}"
}

#######################################
# 获取当前配置路径
#######################################
get_config_path() {
  [ -f "$MODULE_CONF" ] || die "模块配置文件不存在: $MODULE_CONF"

  local config_path
  config_path="${MODULE_CONF%/*}"
  config_path=$(grep '^CURRENT_CONFIG=' "$MODULE_CONF" 2> /dev/null)
  config_path="${config_path#*=}"
  config_path="${config_path//\"/}"

  [ -n "$config_path" ] || die "无法解析配置路径"
  echo "$config_path"
}

#######################################
# 检查 Xray 是否运行中
#######################################
is_running() {
  pgrep -f "^$XRAY_BIN" > /dev/null 2>&1
}

#######################################
# 获取 Xray PID
#######################################
get_pid() {
  pidof -s "$XRAY_BIN" 2> /dev/null || true
}

#######################################
# 启动服务
#######################################
do_start() {
  log "INFO" "========== 开始启动 Xray 服务 =========="

  if is_running; then
    log "WARN" "Xray 已在运行中 (PID: $(get_pid))"
    return 0
  fi

  local outbound_config
  outbound_config=$(get_config_path)

  # 读取出站模式
  local outbound_mode
  outbound_mode=$(grep '^OUTBOUND_MODE=' "$MODULE_CONF" 2> /dev/null | cut -d'=' -f2)
  outbound_mode="${outbound_mode:-rule}"
  log "INFO" "当前出站模式: $outbound_mode"

  # 直连模式使用 default.json (freedom 协议)
  if [ "$outbound_mode" = "direct" ]; then
    outbound_config="$OUTBOUNDS_DIR/default.json"
    log "INFO" "直连模式: 使用 default.json"
  fi

  [ -f "$outbound_config" ] || die "出站配置文件不存在: $outbound_config"
  [ -d "$CONFDIR" ] || die "confdir 目录不存在: $CONFDIR"

  log "INFO" "配置目录: $CONFDIR"
  log "INFO" "出站配置: $outbound_config"

  # 启动 Xray (root:net_admin)
  nohup "$BUSYBOX" setuidgid root:net_admin "$XRAY_BIN" run \
    -confdir "$CONFDIR" \
    -config "$outbound_config" \
    > "$XRAY_LOG_FILE" 2>&1 &

  local xray_pid=$!
  log "INFO" "Xray 进程已启动, PID: $xray_pid"

  # 等待进程稳定
  sleep 1

  if ! kill -0 "$xray_pid" 2> /dev/null; then
    die "Xray 进程启动失败，请检查配置"
  fi

  # 启用 TProxy 规则
  "$MODDIR/scripts/network/tproxy.sh" start --verbose -d "$MODDIR/config" >> "$LOG_FILE" 2>&1

  log "INFO" "========== Xray 服务启动完成 =========="
}

#######################################
# 停止服务
#######################################
do_stop() {
  log "INFO" "========== 开始停止 Xray 服务 =========="

  # 先清理 TProxy 规则（避免断网）
  log "INFO" "清理 TProxy 规则..."
  "$MODDIR/scripts/network/tproxy.sh" stop -d "$MODDIR/config" >> "$LOG_FILE" 2>&1

  # 终止 Xray 进程
  local pid
  pid=$(get_pid)

  if [ -z "$pid" ]; then
    log "INFO" "未发现运行中的 Xray 进程"
  else
    log "INFO" "正在终止 Xray 进程 (PID: $pid)..."

    # 优雅终止
    if kill "$pid" 2> /dev/null; then
      local count=0
      while kill -0 "$pid" 2> /dev/null && [ "$count" -lt "$KILL_TIMEOUT" ]; do
        sleep 1
        count=$((count + 1))
      done

      # 强制终止
      if kill -0 "$pid" 2> /dev/null; then
        log "WARN" "进程未响应 SIGTERM，发送 SIGKILL"
        kill -9 "$pid" 2> /dev/null || true
      fi
    fi

    log "INFO" "Xray 进程已终止"
  fi

  log "INFO" "========== Xray 服务停止完成 =========="
}

#######################################
# 重启服务
#######################################
do_restart() {
  log "INFO" "========== 重启 Xray 服务 =========="
  do_stop
  sleep 1
  do_start
}

#######################################
# 查看状态
#######################################
do_status() {
  local pid
  pid=$(get_pid)

  if [ -n "$pid" ]; then
    echo "Xray 运行中 (PID: $pid)"
    # 显示运行时间
    if [ -f "/proc/$pid/stat" ]; then
      local uptime_ticks start_time now_ticks
      start_time=$(awk '{print $22}' "/proc/$pid/stat" 2> /dev/null || echo 0)
      now_ticks=$(awk '{print $1 * 100}' /proc/uptime 2> /dev/null || echo 0)
      if [ "$start_time" -gt 0 ] && [ "$now_ticks" -gt 0 ]; then
        uptime_ticks=$((now_ticks - start_time))
        echo "运行时间: $((uptime_ticks / 100)) 秒"
      fi
    fi
    return 0
  else
    echo "Xray 未运行"
    return 1
  fi
}

#######################################
# 显示帮助
#######################################
show_usage() {
  cat << EOF
用法: $(basename "$0") {start|stop|restart|status}

命令:
  start     启动 Xray 服务
  stop      停止 Xray 服务
  restart   重启 Xray 服务
  status    查看服务状态

示例:
  $(basename "$0") start
  $(basename "$0") restart
EOF
}

#######################################
# 主入口
#######################################
main() {
  case "${1:-}" in
    start)
      do_start
      ;;
    stop)
      do_stop
      ;;
    restart)
      do_restart
      ;;
    status)
      do_status
      ;;
    -h | --help | help)
      show_usage
      ;;
    *)
      show_usage
      exit 1
      ;;
  esac
}

main "$@"
