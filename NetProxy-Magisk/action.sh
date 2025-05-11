#!/system/bin/sh

# 设置日志文件
MODDIR=${0%/*}
LOG_FILE="$MODDIR/XrayCore/Log/service.log"
XRAY_BIN="$MODDIR/XrayCore/xray"
XRAY_CONFIG_PATH="$MODDIR/XrayCore/Config/config.json"
XRAY_LOG_FILE="$MODDIR/XrayCore/Log/xray.log"
DESCRIPTION_FILE="$MODDIR/module.prop"

# 日志函数
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> "$LOG_FILE"
}

# 修改 description 文件
update_description() {
    sed -i "s/description=.*/description=$1/" "$DESCRIPTION_FILE"
    log "修改 description 文件：$1"
}

# 启动Xray
start_xray() {
    log "开始启动Xray服务..."
    # 检查Xray二进制文件
    if [ ! -f "$XRAY_BIN" ]; then
        log "错误：Xray二进制文件不存在: $XRAY_BIN"
        exit 1
    fi

    # 检查配置文件
    if [ ! -f "$XRAY_CONFIG_PATH" ]; then
        log "错误：配置文件不存在: $XRAY_CONFIG_PATH"
        exit 1
    fi

    # 启动 Xray
    nohup $XRAY_BIN -config $XRAY_CONFIG_PATH > "$XRAY_LOG_FILE" 2>&1 &
    XRAY_PID=$!
    log "Xray启动成功，PID: $XRAY_PID"

    # 更新 description 文件
    update_description "基于Xray内核的代理工具✅"

    # 设置 iptables 规则
    log "设置iptables规则..."
    iptables -w 3 -t nat -N XRAY 2>/dev/null
   
    iptables -w 3 -t nat -A XRAY -p tcp -j REDIRECT --to-ports 1080
    log "添加端口重定向规则"

    iptables -w 3 -t nat -I OUTPUT -p tcp -m owner --uid-owner 0 -j RETURN
    log "添加root进程直连规则"

    iptables -w 3 -t nat -A OUTPUT -p tcp -j XRAY
    log "添加全局代理规则"
}

# 停止Xray
stop_xray() {
    log "开始停止Xray服务..."
    if [ -n "$PID" ]; then
        kill -9 $PID
        log "Xray进程已终止，PID: $PID"
    else
        log "错误：未找到Xray进程PID"
    fi

    # 更新 description 文件
    update_description "基于Xray内核的代理工具⏸"

    # 恢复 iptables 规则
    log "恢复 iptables 规则..."

    # 删除 XRAY 链
    iptables -w 3 -t nat -F XRAY
    iptables -w 3 -t nat -X XRAY
    log "删除 XRAY 链"

    # 恢复 OUTPUT 规则
    iptables -w 3 -t nat -D OUTPUT -p tcp -j XRAY
    log "删除全局代理规则"

    log "iptables规则恢复完成"
}

# 检查Xray是否正在运行
check_xray_running() {
    # 查找 Xray 进程
    PID=$(pgrep xray)

    if [ -n "$PID" ]; then
        log "Xray正在运行，PID: $PID"
        return 0  # 进程正在运行
    else
        log "Xray未运行"
        return 1  # 进程未运行
    fi
}

# 检查Xray是否运行，并决定启动或停止
check_xray_running
if [ $? -eq 0 ]; then
    stop_xray  # 如果Xray在运行，停止它
else
    start_xray  # 如果Xray没有运行，启动它
fi