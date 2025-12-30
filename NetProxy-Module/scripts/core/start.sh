#!/system/bin/sh
set -e
set -u

readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly LOG_FILE="$MODDIR/logs/service.log"
readonly XRAY_BIN="$MODDIR/bin/xray"
readonly STATUS_FILE="$MODDIR/config/status.conf"
readonly XRAY_LOG_FILE="$MODDIR/logs/xray.log"
readonly CONFDIR="$MODDIR/config/xray/confdir"
readonly OUTBOUNDS_DIR="$MODDIR/config/xray/outbounds"

# 根据运行环境设置 busybox 路径
# KSU 变量在 KernelSU 环境下为 true
if [ "${KSU:-false}" = "true" ]; then
    BUSYBOX="/data/adb/ksu/bin/busybox"
else
    BUSYBOX="/data/adb/magisk/busybox"
fi

#######################################
# 记录日志
# Arguments:
#   $1 - 日志级别
#   $2 - 日志消息
#######################################
log() {
    local level="${1:-INFO}"
    local message="$2"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [$level] $message" >> "$LOG_FILE"
}

#######################################
# 错误退出
# Arguments:
#   $1 - 错误消息
#   $2 - 退出码（可选，默认1）
#######################################
die() {
    log "ERROR" "$1"
    exit "${2:-1}"
}

#######################################
# 从状态文件获取配置路径
# Returns:
#   配置文件路径
#######################################
get_config_path() {
    if [ ! -f "$STATUS_FILE" ]; then
        die "状态文件不存在: $STATUS_FILE" 1
    fi
    
    local config_path
    config_path=$(grep '^config=' "$STATUS_FILE" | cut -d'"' -f2)
    
    if [ -z "$config_path" ]; then
        die "无法从状态文件解析配置路径" 1
    fi
    
    echo "$config_path"
}




#######################################
# 更新状态文件
# Arguments:
#   $1 - 配置文件路径
#######################################
update_status() {
    local config_path="$1"
    
    {
        echo "status=\"running\""
        echo "config=\"$config_path\""
    } > "$STATUS_FILE"
    
    log "INFO" "状态已更新: running, config: $config_path"
}

#######################################
# 检查 Xray 是否已运行
# Returns:
#   0 运行中, 1 未运行
#######################################
is_xray_running() {
    pgrep -f "^$XRAY_BIN" >/dev/null 2>&1
}

#######################################
# 启动 Xray 服务
#######################################
start_xray() {
    local outbound_config
    
    log "INFO" "========== 开始启动 Xray 服务 =========="
    
    # 获取出站配置文件路径
    outbound_config=$(get_config_path)
    
    if [ ! -f "$outbound_config" ]; then
        die "出站配置文件不存在: $outbound_config" 1
    fi
    
    # 检查 confdir 目录
    if [ ! -d "$CONFDIR" ]; then
        die "confdir 目录不存在: $CONFDIR" 1
    fi
    
    log "INFO" "使用模块化配置: confdir=$CONFDIR"
    log "INFO" "使用出站配置: $outbound_config"
    
    # 启动 Xray 进程（使用 root:net_admin 运行）
    nohup "$BUSYBOX" setuidgid root:net_admin "$XRAY_BIN" run -confdir "$CONFDIR" -config "$outbound_config" > "$XRAY_LOG_FILE" 2>&1 &
    local xray_pid=$!
    
    log "INFO" "Xray 进程已启动, PID: $xray_pid"
    
    # 等待进程稳定
    sleep 1
    
    # 验证进程是否仍在运行
    if ! kill -0 "$xray_pid" 2>/dev/null; then
        die "Xray 进程启动后立即退出，请检查配置" 1
    fi
    
    # 启用 TProxy 规则
    "$MODDIR/scripts/network/tproxy.sh" start
    
    # 更新状态
    update_status "$outbound_config"
    
    log "INFO" "========== Xray 服务启动完成 =========="
}

# 主流程
if is_xray_running; then
    log "WARN" "Xray 已在运行，跳过启动"
    exit 0
fi

start_xray
