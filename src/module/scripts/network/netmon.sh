#!/system/bin/sh
#######################################
# 文件: netmon.sh
# 功能: 采集 Android 网络事件、读取当前 WiFi SSID 并调用 Go 网络策略评估。
# 用法: netmon.sh {startup|sync|stop|eval}；或由 inotifyd 传入事件参数。
# 依赖: netproxyctl、cmd、dumpsys、awk、inotifyd
#######################################

set -u

readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly NETPROXYCTL="$MODDIR/netproxyctl"
readonly RUN_DIR="/dev/netproxy"
readonly LAST_CHECK_FILE="$RUN_DIR/wifi_last_check"
readonly WIFI_STATE_FILE="$RUN_DIR/wifi_state"
readonly RT_TABLES="/data/misc/net/rt_tables"
readonly LOG_FILE="$MODDIR/logs/service.log"
readonly LOG_TAG="netmon"
readonly DEBOUNCE_SEC=2

NL='
'
TAB="$(printf '\t')"

log() {
  local level="INFO" message="$1"
  if [ "$#" -ge 2 ]; then
    level="$1"
    message="$2"
  fi
  printf '[%s] [%s] [%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$level" "$LOG_TAG" "$message" >> "$LOG_FILE"
}

#######################################
# 获取 WiFi 连接类型与当前 SSID
# 参数: 无
# 返回: 标准输出打印 <wifi|not_wifi><TAB><SSID>
#######################################
get_wifi_snapshot() {
  local status fallback

  status="$(cmd wifi status 2> /dev/null || true)"
  case "$status" in
    *"Wifi is disabled"* | *"Wifi is connected to"*) ;;
    *)
      fallback="$(dumpsys wifi 2> /dev/null || true)"
      [ -z "$fallback" ] || status="$status$NL$fallback"
      ;;
  esac
  printf '%s\n' "$status" | awk '
    function trim(value) {
      sub(/^[ \t]+/, "", value)
      sub(/[ \t]+$/, "", value)
      return value
    }

    function parse_ssid(value, length_value, normalized) {
      value = trim(value)
      sub(/,[ \t]+BSSID:.*/, "", value)
      value = trim(value)
      length_value = length(value)
      if (length_value >= 2 && substr(value, 1, 1) == "\"" &&
          substr(value, length_value, 1) == "\"") {
        value = substr(value, 2, length_value - 2)
      }
      normalized = tolower(value)
      if (value != "" && normalized != "<unknown ssid>" && normalized != "<none>") {
        ssid = value
        connected = 1
      }
    }

    /Wifi is connected to[ \t]/ {
      connected = 1
      line = $0
      sub(/^.*Wifi is connected to[ \t]+/, "", line)
      parse_ssid(line)
    }

    /mWifiInfo|WifiInfo:/ {
      line = $0
      if (match(line, /(^|[ \t,=:])SSID:[ \t]*/)) {
        line = substr(line, RSTART + RLENGTH)
        parse_ssid(line)
      }
    }

    /state:[ \t]*CONNECTED|detailed state:[ \t]*CONNECTED/ { connected = 1 }
    /Wifi is disabled/ { disabled = 1 }

    END {
      if (connected && !disabled) printf "wifi\t%s\n", ssid
      else print "not_wifi\t"
    }
  '
}

#######################################
# 将当前网络快照交给 Go 进行策略判定和模式切换
# 参数: 无
# 返回: Go 评估成功返回 0，否则返回 1
#######################################
evaluate_network() {
  local snapshot network_type ssid

  snapshot="$(get_wifi_snapshot)"
  network_type="${snapshot%%"$TAB"*}"
  if [ "$snapshot" = "$network_type" ]; then
    ssid=""
  else
    ssid="${snapshot#*"$TAB"}"
  fi

  if [ "$network_type" = "wifi" ] && [ -n "$ssid" ]; then
    "$NETPROXYCTL" --json network evaluate --type "$network_type" --ssid "$ssid" \
      > /dev/null 2>&1
  else
    "$NETPROXYCTL" --json network evaluate --type "$network_type" \
      > /dev/null 2>&1
  fi
}

#######################################
# 停止由本模块启动的 inotifyd 监听进程
# 参数: 无
# 返回: 始终返回 0
#######################################
stop_watcher() {
  local pid

  for pid in $(pidof inotifyd 2> /dev/null); do
    if [ -r "/proc/$pid/cmdline" ] && grep -q "netmon.sh" "/proc/$pid/cmdline" 2> /dev/null; then
      kill "$pid" 2> /dev/null || true
    fi
  done
}

#######################################
# 启动 inotifyd 监听网络路由表变化
# 参数: 无
# 返回: 始终返回 0
#######################################
start_watcher() {
  stop_watcher
  (
    local attempts=0
    while [ ! -f "$RT_TABLES" ] && [ "$attempts" -lt 20 ]; do
      sleep 3
      attempts=$((attempts + 1))
    done
    [ -f "$RT_TABLES" ] && nohup inotifyd "$0" "$RT_TABLES" > /dev/null 2>&1 &
  ) &
}

#######################################
# 启动监听并立即评估一次当前网络
# 参数: 无
# 返回: 始终返回 0
#######################################
cmd_startup() {
  start_watcher
  evaluate_network || log "WARN" "网络策略初始化失败"
}

#######################################
# 重启监听并立即评估一次当前网络
# 参数: 无
# 返回: 始终返回 0
#######################################
cmd_sync() {
  start_watcher
  evaluate_network || log "WARN" "网络策略同步失败"
}

#######################################
# 只评估一次当前网络
# 参数: 无
# 返回: 始终返回 0
#######################################
cmd_eval() {
  evaluate_network || log "WARN" "网络策略评估失败"
}

#######################################
# 停止监听并清理本次运行的临时决策状态
# 参数: 无
# 返回: 始终返回 0
#######################################
cmd_stop() {
  stop_watcher
  rm -f "$LAST_CHECK_FILE" "$WIFI_STATE_FILE" 2> /dev/null || true
}

#######################################
# 处理 inotifyd 事件并做时间防抖
# 参数: $1 inotifyd 事件字符串
# 返回: 始终返回 0
#######################################
on_inotify_event() {
  local now last diff

  mkdir -p "$RUN_DIR" 2> /dev/null || true
  now="$(date +%s)"
  last="$(cat "$LAST_CHECK_FILE" 2> /dev/null || printf '0')"
  diff=$((now - last))
  [ "$diff" -ge "$DEBOUNCE_SEC" ] || return 0
  printf '%s\n' "$now" > "$LAST_CHECK_FILE"
  cmd_eval
}

#######################################
# 主入口
# 参数: startup、sync、stop、eval 或 inotifyd 事件
# 返回: 始终返回 0
#######################################
main() {
  mkdir -p "$RUN_DIR" 2> /dev/null || true
  case "${1:-}" in
    startup) cmd_startup ;;
    sync) cmd_sync ;;
    stop) cmd_stop ;;
    eval) cmd_eval ;;
    *) on_inotify_event "${1:-}" ;;
  esac
}

main "$@"
