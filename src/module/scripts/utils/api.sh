#!/system/bin/sh
#######################################
# 文件: api.sh
# 功能: 统一调用 reF1nd Service API。HTTP 与 protobuf 解析全部交由
#       netproxy-native，Shell 层不直接操作控制接口。
# 用法: 由其他脚本通过 . "$MODDIR/scripts/utils/api.sh" 引入。
# 依赖: common.sh；允许调用方覆盖 API 地址、密钥和原生组件路径。
#######################################

SERVICE_API="${SERVICE_API:-127.0.0.1:9090}"
SERVICE_SECRET="${SERVICE_SECRET:-singbox}"

#######################################
# 调用 reF1nd Service API
# 参数:
#   $@  service 子命令及参数
# 返回: 成功打印统一 JSON，失败返回非 0
#######################################
service_api_call() {
  local action="$1"
  shift

  "${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}" service "$action" \
    --address "$SERVICE_API" \
    --secret "$SERVICE_SECRET" \
    "$@"
}

#######################################
# 判断 Service API 与核心实例是否完整就绪
# 参数: 无
# 返回: 0=就绪，非 0=未就绪
#######################################
service_api_is_ready() {
  service_api_call ready --timeout 2s > /dev/null 2>&1
}

#######################################
# 读取 Service API 的核心启动时间
# 参数: 无
# 返回: 标准输出打印 Unix 毫秒时间戳
#######################################
service_api_started_at() {
  local result

  result="$(service_api_call started-at --timeout 2s 2> /dev/null)" || return 1
  printf '%s' "$result" | sed -n 's/.*"unix_milli"[^0-9]*\([0-9][0-9]*\).*/\1/p'
}

#######################################
# 将 Unix 毫秒时间戳转换为秒
# 参数:
#   $1  Unix 毫秒时间戳
# 返回: 标准输出打印 Unix 秒时间戳
#######################################
unix_millis_to_seconds() {
  local value="${1:-}"

  case "$value" in "" | *[!0-9]*) return 1 ;; esac
  [ "${#value}" -gt 3 ] || { printf '0\n'; return 0; }
  printf '%s\n' "${value%???}"
}

#######################################
# 通过 Service API 切换节点组中的出站
# 参数:
#   $1  运行时节点组标签
#   $2  出站标签
# 返回: 请求成功返回 0，否则返回非 0
#######################################
service_api_select() {
  service_api_call select --group "$1" --outbound "$2" --timeout 5s > /dev/null 2>&1
}

#######################################
# 通过 Service API 设置出站模式
# 参数:
#   $1  module.conf 模式名
# 返回: 成功返回 0，否则返回非 0
#######################################
service_api_set_mode() {
  "${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}" control set-mode \
    --mode "$1" \
    --address "$SERVICE_API" \
    --secret "$SERVICE_SECRET" \
    --timeout 5s \
    --format json > /dev/null 2>&1
}

#######################################
# 读取 Service API 当前出站模式
# 参数: 无
# 返回: 标准输出打印模式名称
#######################################
service_api_get_mode() {
  local result

  result="$("${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}" control runtime-mode \
    --address "$SERVICE_API" \
    --secret "$SERVICE_SECRET" \
    --timeout 5s \
    --format text 2> /dev/null)" || return 1
  printf '%s\n' "$result"
}

#######################################
# 通过 Service API 关闭全部连接
# 参数:
#   无
# 返回: 请求成功返回 0，否则返回非 0
#######################################
service_api_close_all_connections() {
  "${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}" control close-all \
    --address "$SERVICE_API" \
    --secret "$SERVICE_SECRET" \
    --timeout 3s \
    --format json > /dev/null 2>&1
}
