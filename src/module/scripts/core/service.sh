#!/system/bin/sh
#######################################
# 文件: service.sh
# 功能: NetProxy sing-box 服务管理脚本，负责构建运行时配置、
#       启动/停止核心，并随核心管理 eBPF 入站的生命周期。
# 用法: service.sh {start|stop|restart|status}
# 依赖: common.sh、config.sh、api.sh、nodes.sh、apps.sh、runtime.sh。
#######################################

set -u  # 引用未定义变量报错

# 模块根目录与关键路径
readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly LOG_FILE="$MODDIR/logs/service.log"          # 服务日志
readonly SING_BOX_BIN="$MODDIR/bin/sing-box"          # sing-box 二进制
readonly MODULE_CONF="$MODDIR/config/module.conf"     # 模块配置
readonly EBPF_CONF="$MODDIR/config/ebpf/ebpf.conf"    # eBPF 配置
readonly SINGBOX_LOG_FILE="$MODDIR/logs/sing-box.log" # sing-box 运行日志
readonly SINGBOX_DIR="$MODDIR/config/singbox"         # sing-box 配置根目录
readonly CONFDIR="$SINGBOX_DIR/confdir"               # 通用配置目录
readonly RUNTIME_DIR="$SINGBOX_DIR/runtime"           # 运行时生成目录
readonly SWITCH_SCRIPT="$MODDIR/scripts/core/switch.sh"     # 模式/节点切换脚本
readonly NETMON_SCRIPT="$MODDIR/scripts/network/netmon.sh"  # WiFi 自动切换监听脚本
readonly SUBSCHED_SCRIPT="$MODDIR/scripts/core/subsched.sh" # 订阅定时更新调度脚本
readonly KILL_TIMEOUT=10                              # 等待 sing-box 优雅退出的秒数上限
readonly LOG_TAG="service"                            # 日志组件标签

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/config.sh"
. "$MODDIR/scripts/utils/api.sh"
. "$MODDIR/scripts/utils/nodes.sh"
. "$MODDIR/scripts/utils/apps.sh"
. "$MODDIR/scripts/core/runtime.sh"

# 将模块 bin 目录加入 PATH，便于调用自带二进制
export PATH="$MODDIR/bin:$PATH"

readonly BUSYBOX="$(detect_busybox)"  # busybox 路径 (用于 setuidgid)

#######################################
# 校验服务运行所需的命令、文件与目录
# 参数: 无
# 返回: 全部就绪返回 0，否则退出
#######################################
verify_environment() {
  # 关键外部命令检查
  require_cmds awk sed nohup

  # 关键文件与目录检查
  require_file "$SING_BOX_BIN" "sing-box 二进制不存在: $SING_BOX_BIN"
  require_file "$MODULE_CONF" "模块配置文件不存在: $MODULE_CONF"
  require_file "$EBPF_CONF" "eBPF 配置文件不存在: $EBPF_CONF"
  require_dir "$SINGBOX_DIR" "sing-box 配置目录不存在: $SINGBOX_DIR"
  require_dir "$CONFDIR" "通用配置目录不存在: $CONFDIR"

  # 确保日志与运行时目录存在
  ensure_dir "$MODDIR/logs" "无法创建日志目录: $MODDIR/logs"
  ensure_dir "$RUNTIME_DIR" "无法创建运行时目录: $RUNTIME_DIR"
}

#######################################
# 清理运行时生成的临时文件
# 参数: 无
# 返回: 无
#######################################
cleanup_runtime_files() {
  rm -f \
    "$RUNTIME_DIR/outbounds.json" \
    "$RUNTIME_DIR/ebpf.json" \
    2> /dev/null || true
}

#######################################
# 打印服务动作横幅
# 参数:
#   $1  动作动词 (启动/停止/重启)
# 返回: 无
#######################################
log_service_action() {
  log "INFO" "$1 sing-box 服务"
}

#######################################
# 判断节点选择模式是否为手动选择
# 参数:
#   $1  选择模式 (CUR_SELECTOR_MODE)
# 返回: 0=手动选择，非 0=其他
#######################################
is_manual_selector() {
  case "$1" in
    manual | selector | 手动选择 | 手动) return 0 ;;
    *) return 1 ;;
  esac
}

#######################################
# 启动 sing-box 服务
# 参数: 无
# 返回: 成功返回 0，启动失败则退出
#######################################
do_start() {
  local pid runtime_outbounds runtime_ebpf new_pid count
  local node_path

  log_service_action "启动"
  verify_environment

  # 已在运行则直接返回，保证幂等
  pid="$(get_pid "$SING_BOX_BIN")"
  if [ -n "$pid" ]; then
    log "WARN" "sing-box 已在运行中 (PID: $pid)"
    return 0
  fi

  # 初始化上下文并扫描节点、生成运行时出站配置
  initialize_runtime_context
  scan_runtime_nodes "$CUR_OUTBOUND_DIR"
  write_runtime_outbounds > /dev/null
  write_runtime_ebpf > /dev/null
  runtime_outbounds="$RUNTIME_OUTBOUNDS_FILE"
  runtime_ebpf="$RUNTIME_EBPF_FILE"

  [ "$RUNTIME_NODE_COUNT" -gt 0 ] || die "当前节点目录没有可加载的节点配置: $CUR_OUTBOUND_DIR"

  # 节点与模式概要 (单行)
  log "INFO" "节点目录=$CUR_OUTBOUND_DIR 模式=$CUR_OUTBOUND_MODE 选择=$CUR_SELECTOR_MODE 已加载=$RUNTIME_NODE_COUNT 跳过=$RUNTIME_SKIPPED_COUNT"

  # 构造启动参数：基础配置、节点配置、运行时出站与 eBPF 入站
  set -- run -C "$CONFDIR"
  while IFS= read -r node_path; do
    [ -n "$node_path" ] || continue
    set -- "$@" -c "$node_path"
  done << EOF
$RUNTIME_NODE_PATHS
EOF
  set -- "$@" -c "$runtime_outbounds" -c "$runtime_ebpf"

  # 以 root:net_admin 身份后台启动进程
  log "DEBUG" "正在启动 sing-box 进程..."
  cd "$SINGBOX_DIR" || die "无法进入配置目录: $SINGBOX_DIR"
  nohup "$BUSYBOX" setuidgid root:net_admin "$SING_BOX_BIN" "$@" > "$SINGBOX_LOG_FILE" 2>&1 &

  # 短暂等待后确认进程存活
  new_pid=$!
  sleep 1

  if kill -0 "$new_pid" 2> /dev/null; then
    log "INFO" "sing-box 启动成功 (PID: $new_pid)"
  else
    die "sing-box 启动失败，请检查日志: $SINGBOX_LOG_FILE"
  fi

  # 控制接口就绪代表 sing-box 与 eBPF 入站均已完成启动
  if ! api_wait_available 30 1; then
    log "ERROR" "控制接口未就绪，sing-box 服务启动失败"
    kill "$new_pid" 2> /dev/null || true
    count=0
    while kill -0 "$new_pid" 2> /dev/null && [ "$count" -lt "$KILL_TIMEOUT" ]; do
      sleep 1
      count=$((count + 1))
    done
    kill -9 "$new_pid" 2> /dev/null || true
    cleanup_runtime_files
    die "请检查 sing-box 启动日志: $SINGBOX_LOG_FILE"
  fi

  LOG_STDERR=0 LOG_LEVEL=WARN SWITCH_ALLOW_RESTART=0 sh "$SWITCH_SCRIPT" mode "$CUR_OUTBOUND_MODE" \
    || log "WARN" "运行模式同步失败，将沿用配置默认模式"
  # 手动选择模式下额外同步当前节点
  if is_manual_selector "$CUR_SELECTOR_MODE"; then
    LOG_STDERR=0 LOG_LEVEL=WARN SWITCH_ALLOW_RESTART=0 sh "$SWITCH_SCRIPT" config "$CUR_OUTBOUND_CONFIG" \
      || log "WARN" "节点配置同步失败，将沿用配置默认节点"
  fi

  # WiFi 自动切换与订阅调度不属于透明数据面，按各自配置独立启动
  sh "$NETMON_SCRIPT" sync > /dev/null 2>&1 || log "WARN" "WiFi 自动切换初始化失败"
  sh "$SUBSCHED_SCRIPT" sync > /dev/null 2>&1 || log "WARN" "订阅定时更新初始化失败"

  log "INFO" "sing-box 服务启动完成"
}

#######################################
# 停止 sing-box 服务
# 参数: 无
# 返回: 无
#######################################
do_stop() {
  local pid count

  log_service_action "停止"
  verify_environment

  # 先停止外围守护，避免停机期间触发模式或订阅操作
  sh "$NETMON_SCRIPT" stop > /dev/null 2>&1 || true
  sh "$SUBSCHED_SCRIPT" stop > /dev/null 2>&1 || true

  # 进程不存在则清理运行时文件后返回，保证幂等
  pid="$(get_pid "$SING_BOX_BIN")"
  if [ -z "$pid" ]; then
    log "DEBUG" "未发现运行中的 sing-box 进程"
    cleanup_runtime_files
    return 0
  fi

  log "DEBUG" "正在停止 sing-box 进程 (PID: $pid)..."

  # 先发送 SIGTERM，超时未退出再强制 SIGKILL
  if kill "$pid" 2> /dev/null; then
    count=0
    while kill -0 "$pid" 2> /dev/null && [ "$count" -lt "$KILL_TIMEOUT" ]; do
      sleep 1
      count=$((count + 1))
    done

    if kill -0 "$pid" 2> /dev/null; then
      log "WARN" "进程未完成 eBPF 清理，改用 SIGKILL；下次启动将接管残留挂载"
      kill -9 "$pid" 2> /dev/null || true
      # 给 SIGKILL 留出回收时间
      sleep 1
    fi
  fi

  # 最终确认进程是否已退出，未退出则视为停止失败
  if kill -0 "$pid" 2> /dev/null; then
    log "ERROR" "sing-box 进程仍在运行 (PID: $pid)，停止失败"
    return 1
  fi

  cleanup_runtime_files
  log "INFO" "sing-box 服务已停止"
}

#######################################
# 重启 sing-box 服务
# 参数: 无
# 返回: 无
#######################################
do_restart() {
  log_service_action "重启"

  do_stop
  do_start
}

#######################################
# 检查完整 sing-box 配置
# 参数: 无
# 返回: 配置有效返回 0，检查失败返回非 0
#######################################
do_check() {
  local runtime_outbounds runtime_ebpf node_path

  verify_environment
  initialize_runtime_context
  scan_runtime_nodes "$CUR_OUTBOUND_DIR"
  write_runtime_outbounds > /dev/null
  write_runtime_ebpf > /dev/null
  runtime_outbounds="$RUNTIME_OUTBOUNDS_FILE"
  runtime_ebpf="$RUNTIME_EBPF_FILE"

  [ "$RUNTIME_NODE_COUNT" -gt 0 ] || die "当前节点目录没有可加载的节点配置: $CUR_OUTBOUND_DIR"

  set -- check -C "$CONFDIR"
  while IFS= read -r node_path; do
    [ -n "$node_path" ] || continue
    set -- "$@" -c "$node_path"
  done << EOF
$RUNTIME_NODE_PATHS
EOF
  set -- "$@" -c "$runtime_outbounds" -c "$runtime_ebpf"

  cd "$SINGBOX_DIR" || die "无法进入配置目录: $SINGBOX_DIR"
  "$SING_BOX_BIN" "$@"
}

#######################################
# 查看服务运行状态
# 参数: 无
# 返回: 运行中返回 0，未运行返回 1
#######################################
do_status() {
  local pid uptime

  pid="$(get_pid "$SING_BOX_BIN")"
  if [ -n "$pid" ]; then
    printf "sing-box 运行中 (PID: %s)\n" "$pid"
    uptime="$(get_process_uptime "$pid")"
    if [ "$uptime" -gt 0 ]; then
      printf "运行时间: %s 秒\n" "$uptime"
    fi
    return 0
  fi

  printf "sing-box 未运行\n"
  return 1
}

#######################################
# 显示帮助信息
# 参数: 无
# 返回: 无
#######################################
show_usage() {
  cat << EOF
用法: $(basename "$0") {start|stop|restart|status|check}

命令:
  start     启动 sing-box 服务
  stop      停止 sing-box 服务
  restart   重启 sing-box 服务
  status    查看服务状态
  check     检查完整配置
EOF
}

#######################################
# 主入口：解析命令并分发
# 参数:
#   $1  命令 (start/stop/restart/status/check)
# 返回: 依命令而定
#######################################
main() {
  local cmd="${1:-}"

  case "$cmd" in
    start) do_start ;;
    stop) do_stop ;;
    restart) do_restart ;;
    status) do_status ;;
    check) do_check ;;
    -h | --help | help) show_usage ;;
    *)
      show_usage
      exit 1
      ;;
  esac
}

main "$@"
