#!/system/bin/sh
#######################################
# 文件: service.sh
# 功能: 构建 sing-box 运行时配置，管理核心生命周期、配置检查与就绪状态。
# 用法: service.sh {start|stop|restart|reload|status|check}
# 依赖: common.sh、config.sh、api.sh、state.sh、catalog.sh、runtime.sh、ebpf.sh。
#######################################

set -u

readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly LOG_FILE="$MODDIR/logs/service.log"
readonly LOG_TAG="service"
readonly SING_BOX_BIN="$MODDIR/bin/sing-box"
readonly NETPROXY_NATIVE_BIN="$MODDIR/bin/netproxy-native"
readonly MODULE_CONF="$MODDIR/config/module.conf"
readonly EBPF_CONF="$MODDIR/config/ebpf/ebpf.conf"
readonly CATALOG_DIR="$MODDIR/config/catalog"
readonly SERVICE_STATE_DIR="$MODDIR/config/runtime"
readonly SINGBOX_LOG_FILE="$MODDIR/logs/sing-box.log"
readonly SINGBOX_DIR="$MODDIR/config/singbox"
readonly CONFDIR="$SINGBOX_DIR/confdir"
readonly RUNTIME_DIR="$SINGBOX_DIR/runtime"
readonly SWITCH_SCRIPT="$MODDIR/scripts/core/switch.sh"
readonly NETMON_SCRIPT="$MODDIR/scripts/network/netmon.sh"
readonly KILL_TIMEOUT=10

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/config.sh"
. "$MODDIR/scripts/utils/api.sh"
. "$MODDIR/scripts/utils/state.sh"
. "$MODDIR/scripts/utils/catalog.sh"
. "$MODDIR/scripts/utils/apps.sh"
. "$MODDIR/scripts/core/runtime.sh"
. "$MODDIR/scripts/core/ebpf.sh"

export PATH="$MODDIR/bin:$PATH"
readonly BUSYBOX="$(detect_busybox)"

SERVICE_ACTION=""
SERVICE_STATE_PID=0
SERVICE_STATE_STARTED_AT=0
SERVICE_STATE_ERROR="服务操作失败"
SERVICE_STATE_FINALIZED=0
SERVICE_STATE_TRACK_FAILURE=0
VALIDATION_RUNTIME_DIR=""

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
    2> /dev/null || true
}

#######################################
# 生成完整运行时配置
# 参数: 无
# 返回: 成功返回 0，失败则退出
#######################################
build_runtime_configuration() {
  initialize_runtime_context
  if ! scan_catalog_groups; then
    [ -z "$RUNTIME_BUILD_ERROR" ] || SERVICE_STATE_ERROR="$RUNTIME_BUILD_ERROR"
    return 1
  fi
  write_runtime_providers > /dev/null
  write_runtime_outbounds > /dev/null
  write_runtime_ebpf > /dev/null
}

#######################################
# 生成仅用于配置检查的运行时配置
# 参数: 无
# 返回: 成功返回 0，失败返回 1
# 说明: 空 Catalog 使用临时直连占位出站，只参与 sing-box check，绝不用于启动。
#######################################
build_validation_configuration() {
  local scan_status=0

  initialize_runtime_context
  VALIDATION_RUNTIME_DIR="/dev/netproxy/config-check.$$"
  rm -rf "$VALIDATION_RUNTIME_DIR" 2> /dev/null || true
  ensure_dir "$VALIDATION_RUNTIME_DIR" "无法创建配置检查目录"
  RUNTIME_PROVIDERS_FILE="$VALIDATION_RUNTIME_DIR/providers.json"
  RUNTIME_OUTBOUNDS_FILE="$VALIDATION_RUNTIME_DIR/outbounds.json"
  RUNTIME_EBPF_FILE="$VALIDATION_RUNTIME_DIR/ebpf.json"

  scan_catalog_groups allow-empty || scan_status=$?
  case "$scan_status" in
    0)
      write_runtime_providers > /dev/null || return 1
      write_runtime_outbounds > /dev/null || return 1
      ;;
    2)
      cat > "$RUNTIME_PROVIDERS_FILE" << 'EOF'
{
  "providers": []
}
EOF
      cat > "$RUNTIME_OUTBOUNDS_FILE" << 'EOF'
{
  "outbounds": [
    {
      "type": "direct",
      "tag": "direct"
    },
    {
      "type": "block",
      "tag": "block"
    },
    {
      "type": "direct",
      "tag": "Proxy"
    }
  ]
}
EOF
      ;;
    *) return 1 ;;
  esac

  write_runtime_ebpf > /dev/null || return 1
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
# 等待两套控制接口与核心实例就绪
# 参数:
#   $1  最大等待秒数
#   $2  sing-box 进程 PID
# 返回: 全部就绪返回 0，超时返回 1，进程提前退出返回 2
#######################################
wait_control_planes_ready() {
  local timeout="${1:-30}"
  local pid="${2:-}"
  local max_checks=$((timeout * 5))
  local count=0

  while [ "$count" -lt "$max_checks" ]; do
    if [ -n "$pid" ] && ! kill -0 "$pid" 2> /dev/null; then
      return 2
    fi
    if service_api_is_ready && api_is_available; then
      return 0
    fi
    sleep 0.2
    count=$((count + 1))
  done
  return 1
}

#######################################
# 同步 Catalog 节点选择到运行实例
# 参数: 无
# 返回: 全部选择生效返回 0，否则返回 1
# 说明: sing-box 缓存可能恢复上次选择，因此每次启动或重新加载后都必须
#       用 module.conf 中的活动分组与选择模式覆盖缓存状态。
#######################################
sync_runtime_selection() {
  local selected_tag runtime_node_ref

  if [ "$CUR_SELECTOR_MODE" = "manual" ]; then
    selected_tag="${CUR_SELECTED_NODE_REF#*/}"
    runtime_node_ref="$CUR_ACTIVE_GROUP_TAG/$selected_tag"
    service_api_select "Select/$CUR_ACTIVE_GROUP_TAG" "$runtime_node_ref" \
      && service_api_select "Proxy" "Select/$CUR_ACTIVE_GROUP_TAG"
  else
    service_api_select "Proxy" "Auto/$CUR_ACTIVE_GROUP_TAG"
  fi
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
    if service_api_is_ready && api_is_available; then
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
  log "INFO" "活动分组=$CUR_ACTIVE_GROUP_ID 分组=$RUNTIME_GROUP_COUNT 节点=$RUNTIME_NODE_COUNT 模式=$CUR_OUTBOUND_MODE 选择=$CUR_SELECTOR_MODE"

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
  wait_control_planes_ready 30 "$new_pid" || wait_status=$?
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
  service_api_set_mode "$CUR_OUTBOUND_MODE" \
    || log "WARN" "运行模式同步失败，将沿用配置默认模式"
  SERVICE_STATE_ERROR="运行时节点选择同步失败"
  if ! sync_runtime_selection; then
    log "ERROR" "$SERVICE_STATE_ERROR"
    terminate_sing_box "$new_pid" || true
    SERVICE_STATE_PID=0
    cleanup_runtime_files
    return 1
  fi
  if [ "${WIFI_AUTO_SWITCH:-0}" = "1" ]; then
    sh "$NETMON_SCRIPT" startup > /dev/null 2>&1 || log "WARN" "WiFi 自动切换初始化失败"
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
  sh "$NETMON_SCRIPT" stop > /dev/null 2>&1 || true
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
      && service_api_is_ready && api_is_available; then
      started_seconds="$(unix_millis_to_seconds "$new_started" 2> /dev/null || true)"
      case "$started_seconds" in "" | 0 | *[!0-9]*) sleep 1; count=$((count + 1)); continue ;; esac
      SERVICE_STATE_STARTED_AT="$started_seconds"
      service_api_set_mode "$CUR_OUTBOUND_MODE" \
        || log "WARN" "重新加载后运行模式同步失败"
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

  if kill -0 "$pid" 2> /dev/null && service_api_is_ready && api_is_available; then
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
