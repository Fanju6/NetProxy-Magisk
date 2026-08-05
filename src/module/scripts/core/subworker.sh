#!/system/bin/sh
#######################################
# 文件: subworker.sh
# 功能: 独立订阅调度 worker，按各订阅 next_update_epoch 顺序更新到期任务。
#       worker 与 sing-box 生命周期解耦，不依赖 crond。
# 用法: subworker.sh {start|stop|restart|wake|run|once|status}
# 依赖: common.sh、metadata.sh 与 subscription.sh。
#######################################

MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
CATALOG_DIR="$MODDIR/config/catalog"
CATALOG_STAGING_DIR="$CATALOG_DIR/staging"
NETPROXY_NATIVE_BIN="$MODDIR/bin/netproxy-native"
MODULE_CONF="$MODDIR/config/module.conf"
SERVICE_SCRIPT="$MODDIR/scripts/core/service.sh"
SWITCH_SCRIPT="$MODDIR/scripts/core/switch.sh"
SING_BOX_BIN="$MODDIR/bin/sing-box"
SUB_RUNTIME_DIR="/dev/netproxy/subscriptions"
WORKER_PID_FILE="/dev/netproxy/subworker.pid"
LOG_FILE="$MODDIR/logs/subscription.log"
LOG_TAG="subworker"
SUBSCRIPTION_LIBRARY_ONLY=1

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/metadata.sh"
. "$MODDIR/scripts/core/subscription.sh"

BUSYBOX="$(detect_busybox)"
WORKER_STOP=0
WORKER_SLEEP_PID=""

#######################################
# 读取有效 worker PID
# 参数: 无
# 返回: 标准输出打印 PID；未运行返回 1
#######################################
worker_pid() {
  local pid

  pid="$(sed -n '1p' "$WORKER_PID_FILE" 2> /dev/null || true)"
  [ -n "$pid" ] && kill -0 "$pid" 2> /dev/null || return 1
  printf "%s\n" "$pid"
}

#######################################
# 收到唤醒信号时中断当前 sleep
# 参数: 无
# 返回: 无
#######################################
wake_worker_loop() {
  [ -z "$WORKER_SLEEP_PID" ] || kill "$WORKER_SLEEP_PID" 2> /dev/null || true
}

#######################################
# 收到停止信号时结束循环
# 参数: 无
# 返回: 无
#######################################
stop_worker_loop() {
  WORKER_STOP=1
  wake_worker_loop
}

#######################################
# 扫描最近一次自动更新时间
# 参数:
#   $1  当前 epoch 秒
# 返回: 标准输出打印最近 epoch；没有自动订阅时返回 0
#######################################
find_nearest_update_epoch() {
  local now="$1"
  local group_dir meta_file type enabled epoch nearest=0

  for group_dir in "$CATALOG_DIR"/*; do
    [ -d "$group_dir" ] || continue
    [ "${group_dir##*/}" != "staging" ] || continue
    meta_file="$group_dir/meta.json"
    type="$(meta_get_string "$meta_file" "type" "")"
    enabled="$(meta_get_raw "$meta_file" "auto_update" "false")"
    [ "$type" = "subscription" ] && [ "$enabled" = "true" ] || continue
    epoch="$(meta_get_raw "$meta_file" "next_update_epoch" "0")"
    case "$epoch" in "" | *[!0-9]*) epoch=0 ;; esac
    [ "$epoch" -gt 0 ] || epoch="$now"
    if [ "$nearest" -eq 0 ] || [ "$epoch" -lt "$nearest" ]; then
      nearest="$epoch"
    fi
  done
  printf "%s\n" "$nearest"
}

#######################################
# 顺序更新当前全部到期订阅
# 参数:
#   $1  当前 epoch 秒
# 返回: 始终返回 0，单项失败不阻断后续任务
#######################################
update_due_subscriptions() {
  local now="$1"
  local group_dir group_id meta_file type enabled epoch

  for group_dir in "$CATALOG_DIR"/*; do
    [ "$WORKER_STOP" = "0" ] || return 0
    [ -d "$group_dir" ] || continue
    group_id="${group_dir##*/}"
    [ "$group_id" != "staging" ] || continue
    meta_file="$group_dir/meta.json"
    type="$(meta_get_string "$meta_file" "type" "")"
    enabled="$(meta_get_raw "$meta_file" "auto_update" "false")"
    [ "$type" = "subscription" ] && [ "$enabled" = "true" ] || continue
    epoch="$(meta_get_raw "$meta_file" "next_update_epoch" "0")"
    case "$epoch" in "" | *[!0-9]*) epoch=0 ;; esac
    [ "$epoch" -le "$now" ] || continue
    log "INFO" "自动更新到期订阅: $group_id"
    update_subscription "$group_id" || true
  done
}

#######################################
# worker 主循环
# 参数: 无
# 返回: 收到停止信号后返回 0
#######################################
run_worker() {
  local now nearest delay

  initialize_catalog_storage || return 1
  mkdir -p "${WORKER_PID_FILE%/*}" || return 1
  printf '%s\n' "$$" > "$WORKER_PID_FILE"
  chmod 0600 "$WORKER_PID_FILE" 2> /dev/null || true
  trap stop_worker_loop TERM INT
  trap wake_worker_loop HUP
  trap 'rm -f "$WORKER_PID_FILE"' EXIT
  log "INFO" "订阅自动更新 worker 已启动"

  while [ "$WORKER_STOP" = "0" ]; do
    now="$(date +%s)"
    update_due_subscriptions "$now"
    [ "$WORKER_STOP" = "0" ] || break

    now="$(date +%s)"
    nearest="$(find_nearest_update_epoch "$now")"
    if [ "$nearest" -eq 0 ]; then
      delay=3600
    elif [ "$nearest" -le "$now" ]; then
      delay=1
    else
      delay=$((nearest - now))
    fi

    sleep "$delay" &
    WORKER_SLEEP_PID=$!
    wait "$WORKER_SLEEP_PID" 2> /dev/null || true
    WORKER_SLEEP_PID=""
  done

  log "INFO" "订阅自动更新 worker 已停止"
}

#######################################
# 后台启动 worker
# 参数: 无
# 返回: 启动成功返回 0
#######################################
start_worker() {
  local pid

  if pid="$(worker_pid)"; then
    log "DEBUG" "订阅 worker 已在运行 (PID: $pid)"
    return 0
  fi
  mkdir -p "${WORKER_PID_FILE%/*}" "$MODDIR/logs" || return 1
  "$BUSYBOX" nohup sh "$0" run > /dev/null 2>&1 < /dev/null &
  pid=$!
  sleep 1
  if kill -0 "$pid" 2> /dev/null || worker_pid > /dev/null 2>&1; then
    return 0
  fi
  log "ERROR" "订阅自动更新 worker 启动失败"
  return 1
}

#######################################
# 停止 worker
# 参数: 无
# 返回: 无
#######################################
stop_worker() {
  local pid count=0

  pid="$(worker_pid 2> /dev/null || true)"
  if [ -z "$pid" ]; then
    rm -f "$WORKER_PID_FILE"
    return 0
  fi
  kill "$pid" 2> /dev/null || true
  while kill -0 "$pid" 2> /dev/null && [ "$count" -lt 5 ]; do
    sleep 1
    count=$((count + 1))
  done
  kill -9 "$pid" 2> /dev/null || true
  rm -f "$WORKER_PID_FILE"
}

#######################################
# 唤醒 worker 重新计算最近任务
# 参数: 无
# 返回: worker 未运行时自动启动
#######################################
wake_worker() {
  local pid

  pid="$(worker_pid 2> /dev/null || true)"
  if [ -n "$pid" ]; then
    kill -HUP "$pid" 2> /dev/null || return 1
  else
    start_worker
  fi
}

#######################################
# 显示 worker 状态
# 参数: 无
# 返回: 运行中返回 0，否则返回 1
#######################################
show_worker_status() {
  local pid nearest now

  pid="$(worker_pid 2> /dev/null || true)"
  [ -n "$pid" ] || { printf "stopped\n"; return 1; }
  now="$(date +%s)"
  nearest="$(find_nearest_update_epoch "$now")"
  printf "running pid=%s next=%s\n" "$pid" "$nearest"
}

#######################################
# 主入口
#######################################
case "${1:-}" in
  start) start_worker ;;
  stop) stop_worker ;;
  restart) stop_worker; start_worker ;;
  wake) wake_worker ;;
  run) run_worker ;;
  once) update_due_subscriptions "$(date +%s)" ;;
  status) show_worker_status ;;
  *)
    printf "用法: %s {start|stop|restart|wake|run|once|status}\n" "$(basename "$0")" >&2
    exit 1
    ;;
esac
