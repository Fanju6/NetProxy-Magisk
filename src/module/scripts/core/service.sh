#!/system/bin/sh
#######################################
# 文件: service.sh
# 功能: 管理 sing-box 生命周期、生成运行时配置、配置检查与就绪状态。
# 用法: service.sh {start|stop|restart|reload|status|check}
# 依赖: netproxy-native、Android 基础命令
#######################################

set -u

readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly LOG_FILE="$MODDIR/logs/service.log"
readonly LOG_TAG="service"
readonly SING_BOX_BIN="$MODDIR/bin/sing-box"
readonly NETPROXY_NATIVE_BIN="$MODDIR/bin/netproxy-native"
readonly MODULE_CONF="${NETPROXY_MODULE_CONF:-$MODDIR/config/module.conf}"
readonly EBPF_CONF="${NETPROXY_EBPF_CONF:-$MODDIR/config/ebpf/ebpf.conf}"
readonly CATALOG_DIR="${NETPROXY_CATALOG_DIR:-$MODDIR/data/catalog}"
readonly SERVICE_STATE_DIR="$MODDIR/runtime"
readonly SINGBOX_LOG_FILE="$MODDIR/logs/sing-box.log"
readonly SINGBOX_DIR="${NETPROXY_SINGBOX_DIR:-$MODDIR/config/singbox}"
readonly CONFDIR="$SINGBOX_DIR/confdir"
readonly RUNTIME_DIR="${NETPROXY_RUNTIME_DIR:-$MODDIR/runtime}"
readonly KILL_TIMEOUT=10
readonly SERVICE_LOCK_DIR="/dev/netproxy/service.lock"

export PATH="$MODDIR/bin:$PATH"
NL='
'
TAB="$(printf '\t')"
SERVICE_STATE_FILE="$SERVICE_STATE_DIR/service.json"

log_level_value() {
  case "$1" in
    DEBUG) printf '10' ;;
    INFO) printf '20' ;;
    WARN) printf '30' ;;
    ERROR) printf '40' ;;
    *) printf '20' ;;
  esac
}

log() {
  local level="INFO" message timestamp
  if [ "$#" -ge 2 ]; then
    level="$1"
    message="$2"
  else
    message="$1"
  fi
  [ "$(log_level_value "$level")" -ge "$(log_level_value "${LOG_LEVEL:-INFO}")" ] || return 0
  timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
  [ -n "${LOG_FILE:-}" ] && printf '[%s] [%s] [%s] %s\n' "$timestamp" "$level" "$LOG_TAG" "$message" >> "$LOG_FILE"
  [ "${LOG_STDERR:-1}" = "0" ] || printf '[%s] [%s] [%s] %s\n' "$timestamp" "$level" "$LOG_TAG" "$message" >&2
}

die() {
  log "ERROR" "$1"
  exit "${2:-1}"
}

command_exists() {
  command -v "$1" > /dev/null 2>&1
}

detect_busybox() {
  local path
  for path in /data/adb/ksu/bin/busybox /data/adb/ap/bin/busybox /data/adb/magisk/busybox; do
    if [ -x "$path" ]; then
      printf '%s\n' "$path"
      return 0
    fi
  done
  printf '%s\n' busybox
}

require_cmds() {
  local cmd missing=""
  for cmd in "$@"; do
    command_exists "$cmd" || missing="$missing $cmd"
  done
  [ -z "$missing" ] || die "缺少必要的命令:$missing"
}

require_file() {
  [ -f "$1" ] || die "${2:-文件不存在: $1}"
}

require_dir() {
  [ -d "$1" ] || die "${2:-目录不存在: $1}"
}

ensure_dir() {
  [ -d "$1" ] || mkdir -p "$1" || die "${2:-无法创建目录: $1}"
}

get_pid() {
  local bin="$1"
  [ -n "$bin" ] || return 1
  pidof -s "$bin" 2> /dev/null || pgrep -f "^$bin" 2> /dev/null | head -1 || true
}

lock_write_owner() {
  printf '%s\n' "$$" > "$1/pid"
  awk '{print $22}' "/proc/$$/stat" > "$1/start" 2> /dev/null || true
}

lock_owner_alive() {
  local lock_dir="$1" pid owner_start current_start
  pid="$(sed -n '1p' "$lock_dir/pid" 2> /dev/null || true)"
  owner_start="$(sed -n '1p' "$lock_dir/start" 2> /dev/null || true)"
  current_start="$(awk '{print $22}' "/proc/$pid/stat" 2> /dev/null || true)"
  [ -n "$pid" ] && [ -n "$owner_start" ] && [ "$owner_start" = "$current_start" ] && kill -0 "$pid" 2> /dev/null
}

SERVICE_STATE_VALUE="stopped"
SERVICE_STATE_PID_VALUE=0
SERVICE_STATE_STARTED_AT_VALUE=0
SERVICE_STATE_READY_AT_VALUE=0
SERVICE_STATE_ERROR_VALUE=""

write_service_state() {
  local status="$1" pid="${2:-0}" started_at="${3:-0}" ready_at="${4:-0}" error_message="${5:-}"
  [ -x "${NETPROXY_NATIVE_BIN:-}" ] || return 1
  "$NETPROXY_NATIVE_BIN" module state \
    --module-dir "$MODDIR" --state-file "$SERVICE_STATE_FILE" \
    --state "$status" --pid "$pid" --started-at "$started_at" \
    --ready-at "$ready_at" --error "$error_message" > /dev/null 2>&1
}

service_state_get_string() {
  local key="$1" fallback="${2:-}" value=""
  if [ -f "$SERVICE_STATE_FILE" ]; then
    value="$(sed -n 's/.*"'"$key"'":"\([^"]*\)".*/\1/p' "$SERVICE_STATE_FILE")"
  fi
  [ -n "$value" ] && printf '%s' "$value" || printf '%s' "$fallback"
}

service_state_get_number() {
  local key="$1" fallback="${2:-0}" value=""
  if [ -f "$SERVICE_STATE_FILE" ]; then
    value="$(sed -n 's/.*"'"$key"'":\([0-9][0-9]*\).*/\1/p' "$SERVICE_STATE_FILE")"
  fi
  case "$value" in
    "" | *[!0-9]*) printf '%s' "$fallback" ;;
    *) printf '%s' "$value" ;;
  esac
}

readonly BUSYBOX="$(detect_busybox)"

SERVICE_ACTION=""
SERVICE_STATE_PID=0
SERVICE_STATE_STARTED_AT=0
SERVICE_STATE_ERROR="服务操作失败"
SERVICE_STATE_FINALIZED=0
SERVICE_STATE_TRACK_FAILURE=0
VALIDATION_RUNTIME_DIR=""
SERVICE_LOCK_HELD=0
RUNTIME_PROVIDERS_FILE="$RUNTIME_DIR/providers.json"
RUNTIME_OUTBOUNDS_FILE="$RUNTIME_DIR/outbounds.json"
RUNTIME_EBPF_FILE="$RUNTIME_DIR/ebpf.json"
RUNTIME_CATALOG_STATE_FILE="$RUNTIME_DIR/catalog.state"

#######################################
# 调用 Service API 检查核心是否就绪
# 参数: 无
# 返回: 就绪返回 0，否则返回 1
#######################################
service_api_is_ready() {
  "$NETPROXY_NATIVE_BIN" service ready \
    --address "127.0.0.1:9090" --secret "singbox" --timeout 2s > /dev/null 2>&1
}

#######################################
# 读取 Service API 返回的核心启动时间
# 参数: 无
# 返回: 输出 Unix 毫秒时间戳
#######################################
service_api_started_at() {
  "$NETPROXY_NATIVE_BIN" service started-at \
    --address "127.0.0.1:9090" --secret "singbox" \
    --timeout 2s --format raw 2> /dev/null
}

#######################################
# 将 Unix 毫秒时间戳转换为秒
# 参数: $1 毫秒时间戳
# 返回: 输出 Unix 秒时间戳
#######################################
unix_millis_to_seconds() {
  local value="${1:-}"

  case "$value" in
    "" | *[!0-9]*) return 1 ;;
  esac
  [ "${#value}" -gt 3 ] || { printf '0\n'; return 0; }
  printf '%s\n' "${value%???}"
}

#######################################
# 调用 Go 生成 Catalog、Provider、选择器和 eBPF 入站
# 参数: $1 可选 allow-empty
# 返回: 生成成功返回 0，否则返回 1
#######################################
build_runtime_catalog() {
  local mode="${1:-strict}"
  if [ "$mode" = "allow-empty" ]; then
    "$NETPROXY_NATIVE_BIN" module prepare \
      --module-dir "$MODDIR" \
      --catalog-root "$CATALOG_DIR" \
      --module-config "$MODULE_CONF" \
      --ebpf-config "$EBPF_CONF" \
      --singbox-dir "$SINGBOX_DIR" \
      --runtime-dir "${RUNTIME_PROVIDERS_FILE%/*}" \
      --allow-empty > /dev/null || {
        SERVICE_STATE_ERROR="运行时配置生成失败"
        return 1
      }
  elif ! "$NETPROXY_NATIVE_BIN" module prepare \
      --module-dir "$MODDIR" \
      --catalog-root "$CATALOG_DIR" \
      --module-config "$MODULE_CONF" \
      --ebpf-config "$EBPF_CONF" \
      --singbox-dir "$SINGBOX_DIR" \
      --runtime-dir "${RUNTIME_PROVIDERS_FILE%/*}" \
      > /dev/null; then
    SERVICE_STATE_ERROR="运行时配置生成失败"
    return 1
  fi
  if [ ! -s "$RUNTIME_PROVIDERS_FILE" ] \
    || [ ! -s "$RUNTIME_OUTBOUNDS_FILE" ] \
    || [ ! -s "$RUNTIME_EBPF_FILE" ]; then
    SERVICE_STATE_ERROR="运行时配置文件不完整"
    return 1
  fi
}

#######################################
# 获取服务生命周期操作锁
# 参数:
#   $1  当前操作名称
# 返回: 成功返回 0，已有操作执行时返回 1
#######################################
acquire_service_lock() {
  local action="$1"

  mkdir -p "${SERVICE_LOCK_DIR%/*}" || return 1
  if mkdir "$SERVICE_LOCK_DIR" 2> /dev/null; then
    lock_write_owner "$SERVICE_LOCK_DIR"
    printf '%s\n' "$action" > "$SERVICE_LOCK_DIR/action"
    SERVICE_LOCK_HELD=1
    return 0
  fi

  # 持有者已消失则接管残锁
  if ! lock_owner_alive "$SERVICE_LOCK_DIR"; then
    rm -rf "$SERVICE_LOCK_DIR" 2> /dev/null || return 1
    mkdir "$SERVICE_LOCK_DIR" 2> /dev/null || return 1
    lock_write_owner "$SERVICE_LOCK_DIR"
    printf '%s\n' "$action" > "$SERVICE_LOCK_DIR/action"
    SERVICE_LOCK_HELD=1
    return 0
  fi

  log "WARN" "已有服务操作正在执行: $(sed -n '1p' "$SERVICE_LOCK_DIR/action" 2> /dev/null || printf 'unknown')"
  return 1
}

#######################################
# 释放服务生命周期操作锁
# 参数: 无
# 返回: 无
#######################################
release_service_lock() {
  [ "$SERVICE_LOCK_HELD" = "1" ] || return 0
  if [ "$(sed -n '1p' "$SERVICE_LOCK_DIR/pid" 2> /dev/null || true)" = "$$" ]; then
    rm -rf "$SERVICE_LOCK_DIR" 2> /dev/null || true
  fi
  SERVICE_LOCK_HELD=0
}

#######################################
# 异常退出时收敛服务状态
# 参数:
#   $1  退出码
# 返回: 无
#######################################
service_exit_handler() {
  local status="$1"

  if [ "$status" -ne 0 ] && [ "$SERVICE_STATE_FINALIZED" != "1" ] \
    && [ "$SERVICE_STATE_TRACK_FAILURE" = "1" ]; then
    case "$SERVICE_ACTION" in
      start | reload | restart)
        write_service_state failed "$SERVICE_STATE_PID" "$SERVICE_STATE_STARTED_AT" 0 "$SERVICE_STATE_ERROR" 2> /dev/null || true
        ;;
    esac
  fi
  release_service_lock
}

trap 'service_exit_handler $?' 0

#######################################
# 校验运行环境并准备受保护目录
# 参数: 无
# 返回: 全部就绪返回 0，否则退出
#######################################
verify_environment() {
  require_cmds awk sed nohup
  require_file "$SING_BOX_BIN" "sing-box 二进制不存在: $SING_BOX_BIN"
  require_file "$NETPROXY_NATIVE_BIN" "NetProxy 原生组件不存在: $NETPROXY_NATIVE_BIN"
  require_file "$MODULE_CONF" "模块配置文件不存在: $MODULE_CONF"
  require_file "$EBPF_CONF" "eBPF 配置文件不存在: $EBPF_CONF"
  require_dir "$SINGBOX_DIR" "sing-box 配置目录不存在: $SINGBOX_DIR"
  require_dir "$CONFDIR" "通用配置目录不存在: $CONFDIR"
  require_dir "$CATALOG_DIR" "Catalog 目录不存在: $CATALOG_DIR"
  ensure_dir "$MODDIR/logs" "无法创建日志目录: $MODDIR/logs"
  ensure_dir "$RUNTIME_DIR" "无法创建运行时目录: $RUNTIME_DIR"
  ensure_dir "$SERVICE_STATE_DIR" "无法创建状态目录: $SERVICE_STATE_DIR"
  chmod 0700 "$SERVICE_STATE_DIR" 2> /dev/null || true
}

#######################################
# 清理运行时生成文件
# 参数: 无
# 返回: 无
#######################################
cleanup_runtime_files() {
  rm -f \
    "$RUNTIME_DIR/providers.json" \
    "$RUNTIME_DIR/outbounds.json" \
    "$RUNTIME_DIR/ebpf.json" \
    "$RUNTIME_DIR/catalog.state" \
    2> /dev/null || true
}

#######################################
# 生成完整运行时配置
# 参数: 无
# 返回: 成功返回 0，失败则退出
#######################################
build_runtime_configuration() {
  build_runtime_catalog
}

#######################################
# 生成仅用于配置检查的运行时配置
# 参数: 无
# 返回: 成功返回 0，失败返回 1
# 说明: 空 Catalog 使用临时直连占位出站，只参与 sing-box check，绝不用于启动。
#######################################
build_validation_configuration() {
  VALIDATION_RUNTIME_DIR="/dev/netproxy/config-check.$$"
  rm -rf "$VALIDATION_RUNTIME_DIR" 2> /dev/null || true
  ensure_dir "$VALIDATION_RUNTIME_DIR" "无法创建配置检查目录"
  RUNTIME_PROVIDERS_FILE="$VALIDATION_RUNTIME_DIR/providers.json"
  RUNTIME_OUTBOUNDS_FILE="$VALIDATION_RUNTIME_DIR/outbounds.json"
  RUNTIME_EBPF_FILE="$VALIDATION_RUNTIME_DIR/ebpf.json"
  build_runtime_catalog allow-empty
}

#######################################
# 清理配置检查临时目录
# 参数: 无
# 返回: 无
#######################################
cleanup_validation_configuration() {
  [ -z "$VALIDATION_RUNTIME_DIR" ] || rm -rf "$VALIDATION_RUNTIME_DIR" 2> /dev/null || true
  VALIDATION_RUNTIME_DIR=""
}

#######################################
# 调用 sing-box 检查当前生成配置
# 参数: 无
# 返回: 配置有效返回 0，否则返回非 0
#######################################
check_runtime_configuration() {
  cd "$SINGBOX_DIR" || return 1
  "$SING_BOX_BIN" check -C "$CONFDIR" \
    -c "$RUNTIME_PROVIDERS_FILE" \
    -c "$RUNTIME_OUTBOUNDS_FILE" \
    -c "$RUNTIME_EBPF_FILE"
}

#######################################
# 等待 Service API 与核心实例就绪
# 参数:
#   $1  最大等待秒数
#   $2  sing-box 进程 PID
# 返回: 全部就绪返回 0，超时返回 1，进程提前退出返回 2
#######################################
wait_control_plane_ready() {
  local timeout="${1:-30}"
  local pid="${2:-}"
  local max_checks=$((timeout * 5))
  local count=0

  while [ "$count" -lt "$max_checks" ]; do
    if [ -n "$pid" ] && ! kill -0 "$pid" 2> /dev/null; then
      return 2
    fi
    if service_api_is_ready; then
      return 0
    fi
    sleep 0.2
    count=$((count + 1))
  done
  return 1
}

#######################################
# 让 Go 读取持久状态并同步运行时模式和节点选择
# 参数: 无
# 返回: 同步成功返回 0，否则返回 1
#######################################
sync_runtime_selection() {
  "$NETPROXY_NATIVE_BIN" module sync --module-dir "$MODDIR" \
    --skip-service-adapter > /dev/null
}

#######################################
# 优雅停止指定 sing-box 进程
# 参数:
#   $1  PID
# 返回: 已退出返回 0，否则返回 1
#######################################
terminate_sing_box() {
  local pid="$1"
  local count=0

  [ -n "$pid" ] || return 0
  kill "$pid" 2> /dev/null || true
  while kill -0 "$pid" 2> /dev/null && [ "$count" -lt "$KILL_TIMEOUT" ]; do
    sleep 1
    count=$((count + 1))
  done
  if kill -0 "$pid" 2> /dev/null; then
    log "WARN" "sing-box 未在限定时间内退出，改用 SIGKILL"
    kill -9 "$pid" 2> /dev/null || true
    sleep 1
  fi
  ! kill -0 "$pid" 2> /dev/null
}

#######################################
# 启动 sing-box 服务
# 参数: 无
# 返回: 成功返回 0，否则返回 1
#######################################
do_start() {
  local pid new_pid started_millis started_seconds ready_at wait_status=0

  log "INFO" "启动 sing-box 服务"
  SERVICE_STATE_TRACK_FAILURE=1
  SERVICE_STATE_ERROR="启动前环境校验失败"
  verify_environment

  pid="$(get_pid "$SING_BOX_BIN")"
  if [ -n "$pid" ]; then
    if service_api_is_ready; then
      started_millis="$(service_api_started_at 2> /dev/null || true)"
      started_seconds="$(unix_millis_to_seconds "$started_millis" 2> /dev/null || true)"
      case "$started_seconds" in
        "" | 0 | *[!0-9]*) SERVICE_STATE_STARTED_AT="$(service_state_get_number started_at 0)" ;;
        *) SERVICE_STATE_STARTED_AT="$started_seconds" ;;
      esac
      [ "$SERVICE_STATE_STARTED_AT" -gt 0 ] || SERVICE_STATE_STARTED_AT="$(date +%s)"
      ready_at="$(service_state_get_number ready_at 0)"
      [ "$ready_at" -gt 0 ] || ready_at="$(date +%s)"
      write_service_state ready "$pid" "$SERVICE_STATE_STARTED_AT" "$ready_at" ""
      SERVICE_STATE_FINALIZED=1
      log "WARN" "sing-box 已在运行中 (PID: $pid)"
      return 0
    fi
    SERVICE_STATE_PID="$pid"
    SERVICE_STATE_ERROR="检测到无响应的 sing-box 进程"
    log "ERROR" "$SERVICE_STATE_ERROR (PID: $pid)"
    return 1
  fi

  write_service_state preparing 0 0 0 ""
  SERVICE_STATE_ERROR="运行时配置生成失败"
  build_runtime_configuration || return 1
  log "INFO" "运行时配置准备完成"

  SERVICE_STATE_ERROR="sing-box 进程启动失败"
  cd "$SINGBOX_DIR" || die "无法进入配置目录: $SINGBOX_DIR"
  nohup "$BUSYBOX" setuidgid root:net_admin "$SING_BOX_BIN" run -C "$CONFDIR" \
    -c "$RUNTIME_PROVIDERS_FILE" \
    -c "$RUNTIME_OUTBOUNDS_FILE" \
    -c "$RUNTIME_EBPF_FILE" \
    > "$SINGBOX_LOG_FILE" 2>&1 &
  new_pid=$!
  SERVICE_STATE_PID="$new_pid"
  SERVICE_STATE_STARTED_AT="$(date +%s)"
  write_service_state starting "$new_pid" "$SERVICE_STATE_STARTED_AT" 0 ""

  SERVICE_STATE_ERROR="核心或控制接口未在限定时间内就绪"
  wait_control_plane_ready 30 "$new_pid" || wait_status=$?
  if [ "$wait_status" -ne 0 ]; then
    [ "$wait_status" -ne 2 ] || SERVICE_STATE_ERROR="sing-box 进程启动失败"
    log "ERROR" "$SERVICE_STATE_ERROR"
    terminate_sing_box "$new_pid" || true
    SERVICE_STATE_PID=0
    cleanup_runtime_files
    return 1
  fi

  started_millis="$(service_api_started_at 2> /dev/null || true)"
  started_seconds="$(unix_millis_to_seconds "$started_millis" 2> /dev/null || true)"
  case "$started_seconds" in "" | 0 | *[!0-9]*) ;; *) SERVICE_STATE_STARTED_AT="$started_seconds" ;; esac
  [ "$SERVICE_STATE_STARTED_AT" -gt 0 ] || SERVICE_STATE_STARTED_AT="$(date +%s)"
  SERVICE_STATE_ERROR="运行时节点选择同步失败"
  if ! sync_runtime_selection; then
    log "ERROR" "$SERVICE_STATE_ERROR"
    terminate_sing_box "$new_pid" || true
    SERVICE_STATE_PID=0
    cleanup_runtime_files
    return 1
  fi
  ready_at="$(date +%s)"
  write_service_state ready "$new_pid" "$SERVICE_STATE_STARTED_AT" "$ready_at" ""
  SERVICE_STATE_FINALIZED=1
  log "INFO" "sing-box 服务启动完成"
}

#######################################
# 停止 sing-box 服务
# 参数: 无
# 返回: 成功返回 0，否则返回 1
#######################################
do_stop() {
  local pid started_at

  log "INFO" "停止 sing-box 服务"
  ensure_dir "$SERVICE_STATE_DIR" "无法创建状态目录: $SERVICE_STATE_DIR"
  pid="$(get_pid "$SING_BOX_BIN")"
  started_at="$(service_state_get_number started_at 0)"
  write_service_state stopping "${pid:-0}" "$started_at" 0 ""

  if [ -n "$pid" ] && ! terminate_sing_box "$pid"; then
    write_service_state failed "$pid" "$started_at" 0 "sing-box 进程停止失败"
    SERVICE_STATE_FINALIZED=1
    return 1
  fi
  cleanup_runtime_files
  write_service_state stopped 0 0 0 ""
  SERVICE_STATE_FINALIZED=1
  log "INFO" "sing-box 服务已停止"
}

#######################################
# 原位重新加载 sing-box 配置
# 参数: 无
# 返回: 成功返回 0，否则返回 1
#######################################
do_reload() {
  local pid old_started new_started started_seconds count=0 ready_at previous_ready_at

  log "INFO" "重新加载 sing-box 配置"
  SERVICE_STATE_ERROR="重新加载前环境校验失败"
  verify_environment
  pid="$(get_pid "$SING_BOX_BIN")"
  [ -n "$pid" ] || { SERVICE_STATE_ERROR="sing-box 未运行，无法重新加载"; log "ERROR" "$SERVICE_STATE_ERROR"; return 1; }
  SERVICE_STATE_PID="$pid"
  SERVICE_STATE_STARTED_AT="$(service_state_get_number started_at 0)"
  previous_ready_at="$(service_state_get_number ready_at 0)"

  old_started="$(service_api_started_at 2> /dev/null || true)"
  case "$old_started" in
    "" | *[!0-9]*)
      SERVICE_STATE_ERROR="Service API 未就绪，无法确认原位重新加载"
      log "ERROR" "$SERVICE_STATE_ERROR"
      return 1
      ;;
  esac
  started_seconds="$(unix_millis_to_seconds "$old_started" 2> /dev/null || true)"
  case "$started_seconds" in
    "" | 0 | *[!0-9]*)
      SERVICE_STATE_ERROR="Service API 返回的启动时间无效"
      log "ERROR" "$SERVICE_STATE_ERROR"
      return 1
      ;;
  esac
  SERVICE_STATE_STARTED_AT="$started_seconds"
  SERVICE_STATE_ERROR="重新加载配置生成或校验失败"
  build_runtime_configuration || return 1
  check_runtime_configuration >> "$SINGBOX_LOG_FILE" 2>&1 \
    || { log "ERROR" "$SERVICE_STATE_ERROR"; return 1; }

  SERVICE_STATE_ERROR="sing-box 原位重新加载失败"
  SERVICE_STATE_TRACK_FAILURE=1
  write_service_state starting "$pid" "$SERVICE_STATE_STARTED_AT" 0 ""
  kill -HUP "$pid" 2> /dev/null || return 1
  while [ "$count" -lt 30 ]; do
    kill -0 "$pid" 2> /dev/null || break
    new_started="$(service_api_started_at 2> /dev/null || true)"
    case "$new_started" in "" | *[!0-9]*) new_started="" ;; esac
    if [ -n "$new_started" ] && [ "$new_started" != "$old_started" ] \
      && service_api_is_ready; then
      started_seconds="$(unix_millis_to_seconds "$new_started" 2> /dev/null || true)"
      case "$started_seconds" in "" | 0 | *[!0-9]*) sleep 1; count=$((count + 1)); continue ;; esac
      SERVICE_STATE_STARTED_AT="$started_seconds"
      if ! sync_runtime_selection; then
        SERVICE_STATE_ERROR="重新加载后节点选择同步失败"
        log "ERROR" "$SERVICE_STATE_ERROR"
        return 1
      fi
      ready_at="$(date +%s)"
      write_service_state ready "$pid" "$SERVICE_STATE_STARTED_AT" "$ready_at" ""
      SERVICE_STATE_FINALIZED=1
      log "INFO" "sing-box 配置重新加载完成"
      return 0
    fi
    sleep 1
    count=$((count + 1))
  done

  if kill -0 "$pid" 2> /dev/null && service_api_is_ready; then
    new_started="$(service_api_started_at 2> /dev/null || true)"
    case "$new_started" in
      "" | *[!0-9]*) ;;
      *)
        started_seconds="$(unix_millis_to_seconds "$new_started" 2> /dev/null || true)"
        case "$started_seconds" in "" | 0 | *[!0-9]*) ;; *) SERVICE_STATE_STARTED_AT="$started_seconds" ;; esac
        ;;
    esac
    write_service_state ready "$pid" "$SERVICE_STATE_STARTED_AT" "$previous_ready_at" "$SERVICE_STATE_ERROR"
    SERVICE_STATE_FINALIZED=1
  fi
  log "ERROR" "$SERVICE_STATE_ERROR"
  return 1
}

#######################################
# 重启 sing-box 服务
# 参数: 无
# 返回: 成功返回 0，否则返回 1
#######################################
do_restart() {
  log "INFO" "重启 sing-box 服务"
  do_stop || return 1
  SERVICE_STATE_FINALIZED=0
  SERVICE_STATE_TRACK_FAILURE=0
  do_start
}

#######################################
# 检查完整 sing-box 配置
# 参数: 无
# 返回: 配置有效返回 0，否则返回非 0
#######################################
do_check() {
  local status=0

  verify_environment
  build_validation_configuration || { cleanup_validation_configuration; return 1; }
  check_runtime_configuration || status=$?
  cleanup_validation_configuration
  return "$status"
}

#######################################
# 查看服务状态
# 参数: 无
# 返回: ready 返回 0，其他状态返回 1
#######################################
do_status() {
  local pid state ready_at uptime=0 now

  pid="$(get_pid "$SING_BOX_BIN")"
  state="$(service_state_get_string state stopped)"
  ready_at="$(service_state_get_number ready_at 0)"
  if [ -z "$pid" ] && [ "$state" != "stopped" ] && [ "$state" != "failed" ]; then
    state="failed"
    write_service_state failed 0 0 0 "sing-box 进程已意外退出" 2> /dev/null || true
  fi
  if [ "$state" = "ready" ] && [ "$ready_at" -gt 0 ]; then
    now="$(date +%s)"
    [ "$now" -ge "$ready_at" ] && uptime=$((now - ready_at))
  fi
  printf "服务状态: %s\n" "$state"
  [ -z "$pid" ] || printf "sing-box PID: %s\n" "$pid"
  printf "完整运行时间: %s 秒\n" "$uptime"
  [ "$state" = "ready" ]
}

#######################################
# 显示帮助
#######################################
show_usage() {
  cat << EOF
用法: $(basename "$0") {start|stop|restart|reload|status|check}

命令:
  start     启动服务
  stop      停止服务
  restart   完整重启服务
  reload    原位重新加载配置
  status    查看持久服务状态
  check     检查完整配置
EOF
}

#######################################
# 主入口
#######################################
main() {
  SERVICE_ACTION="${1:-}"
  case "$SERVICE_ACTION" in
    start | stop | restart | reload)
      acquire_service_lock "$SERVICE_ACTION" || return 1
      ;;
  esac
  case "$SERVICE_ACTION" in
    start) do_start ;;
    stop) do_stop ;;
    restart) do_restart ;;
    reload) do_reload ;;
    status) SERVICE_STATE_FINALIZED=1; do_status ;;
    check) SERVICE_STATE_FINALIZED=1; do_check ;;
    help | -h | --help) SERVICE_STATE_FINALIZED=1; show_usage ;;
    *) SERVICE_STATE_FINALIZED=1; show_usage; return 2 ;;
  esac
}

main "$@"
