#!/system/bin/sh
#######################################
# 文件: state.sh
# 功能: 原子记录与读取 NetProxy 服务生命周期状态。
# 用法: 由 service.sh 与 netproxyctl 引入。
# 依赖: common.sh 提供 json_escape。
#######################################

[ -n "${SERVICE_STATE_DIR:-}" ] || SERVICE_STATE_DIR="${MODDIR:-}/config/runtime"
[ -n "${SERVICE_STATE_FILE:-}" ] || SERVICE_STATE_FILE="$SERVICE_STATE_DIR/service.json"

SERVICE_STATE_VALUE="stopped"
SERVICE_STATE_PID_VALUE=0
SERVICE_STATE_STARTED_AT_VALUE=0
SERVICE_STATE_READY_AT_VALUE=0
SERVICE_STATE_ERROR_VALUE=""

#######################################
# 一次性读取服务状态
# 参数: 无
# 全局: 写入 SERVICE_STATE_*_VALUE
# 返回: 状态文件有效返回 0，否则返回 1 并保留默认值
#######################################
load_service_state() {
  local line value

  SERVICE_STATE_VALUE="stopped"
  SERVICE_STATE_PID_VALUE=0
  SERVICE_STATE_STARTED_AT_VALUE=0
  SERVICE_STATE_READY_AT_VALUE=0
  SERVICE_STATE_ERROR_VALUE=""
  [ -f "$SERVICE_STATE_FILE" ] || return 1
  IFS= read -r line < "$SERVICE_STATE_FILE" || return 1
  case "$line" in *'"state":"'*'","pid":'*'"started_at":'*'"ready_at":'*'"error":"'*) ;; *) return 1 ;; esac

  value="${line#*\"state\":\"}"
  SERVICE_STATE_VALUE="${value%%\",\"pid\":*}"
  value="${line#*\"pid\":}"
  SERVICE_STATE_PID_VALUE="${value%%,*}"
  value="${line#*\"started_at\":}"
  SERVICE_STATE_STARTED_AT_VALUE="${value%%,*}"
  value="${line#*\"ready_at\":}"
  SERVICE_STATE_READY_AT_VALUE="${value%%,*}"
  value="${line#*\"error\":\"}"
  SERVICE_STATE_ERROR_VALUE="${value%%\",\"updated_at\":*}"

  case "$SERVICE_STATE_PID_VALUE:$SERVICE_STATE_STARTED_AT_VALUE:$SERVICE_STATE_READY_AT_VALUE" in
    *[!0-9:]*) return 1 ;;
  esac
  return 0
}

#######################################
# 写入服务状态
# 参数:
#   $1 状态  $2 PID  $3 started_at  $4 ready_at  $5 错误信息
# 返回: 成功返回 0，否则返回 1
#######################################
write_service_state() {
  local status="$1"
  local pid="${2:-0}"
  local started_at="${3:-0}"
  local ready_at="${4:-0}"
  local error_message="${5:-}"
  local temporary="$SERVICE_STATE_FILE.tmp.$$"

  case "$pid" in "" | *[!0-9]*) pid=0 ;; esac
  case "$started_at" in "" | *[!0-9]*) started_at=0 ;; esac
  case "$ready_at" in "" | *[!0-9]*) ready_at=0 ;; esac
  mkdir -p "$SERVICE_STATE_DIR" || return 1
  cat > "$temporary" << EOF
{"schema":1,"state":"$(json_escape "$status")","pid":$pid,"started_at":$started_at,"ready_at":$ready_at,"error":"$(json_escape "$error_message")","updated_at":$(date +%s)}
EOF
  chmod 0600 "$temporary" 2> /dev/null || true
  mv -f "$temporary" "$SERVICE_STATE_FILE"
}

#######################################
# 从状态文件读取字符串字段
# 参数:
#   $1  字段名
#   $2  默认值
# 返回: 标准输出打印字段值
#######################################
service_state_get_string() {
  local key="$1"
  local fallback="${2:-}"
  local value=""

  if [ -f "$SERVICE_STATE_FILE" ]; then
    value="$(sed -n 's/.*"'"$key"'":"\([^"]*\)".*/\1/p' "$SERVICE_STATE_FILE")"
  fi
  [ -n "$value" ] && printf "%s" "$value" || printf "%s" "$fallback"
}

#######################################
# 从状态文件读取非负整数字段
# 参数:
#   $1  字段名
#   $2  默认值
# 返回: 标准输出打印字段值
#######################################
service_state_get_number() {
  local key="$1"
  local fallback="${2:-0}"
  local value=""

  if [ -f "$SERVICE_STATE_FILE" ]; then
    value="$(sed -n 's/.*"'"$key"'":\([0-9][0-9]*\).*/\1/p' "$SERVICE_STATE_FILE")"
  fi
  case "$value" in
    "" | *[!0-9]*) printf "%s" "$fallback" ;;
    *) printf "%s" "$value" ;;
  esac
}
