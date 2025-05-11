#!/system/bin/sh

# 设置文件路径
MODDIR=${0%/*}
LOG_FILE="$MODDIR/XrayCore/Log/service.log"
XRAY_BIN="$MODDIR/XrayCore/xray"
XRAY_CONFIG_PATH="$MODDIR/XrayCore/Config/config.json"
XRAY_LOG_FILE="$MODDIR/XrayCore/Log/xray.log"
DESCRIPTION_FILE="$MODDIR/module.prop"

# 设置文件权限
log "设置文件权限..."
chmod 755 "$XRAY_BIN"
chown root:root "$XRAY_BIN"

log "文件权限设置完成"

# 日志函数
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> "$LOG_FILE"
}

# 清空日志文件
echo "" > "$LOG_FILE"

log "开始启动服务..."

# 检查文件是否存在
if [ ! -f "$XRAY_BIN" ]; then
    log "错误：Xray二进制文件不存在: $XRAY_BIN"
    exit 1
fi

if [ ! -f "$XRAY_CONFIG_PATH" ]; then
    log "错误：配置文件不存在: $XRAY_CONFIG_PATH"
    exit 1
fi

# 启动 Xray
log "正在启动Xray..."
nohup $XRAY_BIN -config $XRAY_CONFIG_PATH > "$XRAY_LOG_FILE" 2>&1 &
XRAY_PID=$!

# 检查Xray是否成功启动
sleep 2
if ps | grep -q "$XRAY_PID"; then
    log "Xray启动成功，PID: $XRAY_PID"
    
    # 修改 description 文件内容
    log "Xray正在运行，修改description文件..."
    sed -i 's/description=.*/description=基于Xray内核的代理工具✅/' "$DESCRIPTION_FILE"
else
    log "错误：Xray启动失败"
    exit 1
fi

# 设置 iptables 规则
log "设置iptables规则..."

log "创建XRAY链"
iptables -w 3 -t nat -N XRAY 2>/dev/null

log "添加端口重定向规则"
iptables -w 3 -t nat -A XRAY -p tcp -j REDIRECT --to-ports 1080

log "添加root进程直连规则"
iptables -w 3 -t nat -I OUTPUT -p tcp -m owner --uid-owner 0 -j RETURN

log "添加全局代理规则"
iptables -w 3 -t nat -A OUTPUT -p tcp -j XRAY

log "服务启动完成"