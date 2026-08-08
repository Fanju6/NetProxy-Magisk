#!/system/bin/sh
#######################################
# 文件: netmon.sh
# 功能: 网络变化监听与「按 WiFi SSID 自动切换出站模式」决策。
#       由 inotifyd 监听 /data/misc/net/rt_tables 的写事件触发(网络切换时
#       Android 会重写该文件)，据当前 SSID + 黑/白名单决定使用基础模式
#       或 Direct 模式，通过 Service API 热切换，不重启 sing-box 核心，
#       也不改动透明代理规则。
# 用法:
#   netmon.sh <events> <dir> [file]   inotifyd 代理(事件触发，含防抖)
#   netmon.sh eval                    立即评估一次(启动时 / 改配置后)
#   netmon.sh startup                 核心启动后初始化，不重复恢复基础模式
#   netmon.sh sync                    按配置启停 inotifyd 守护并评估一次
#   netmon.sh stop                    停止 inotifyd 守护并恢复基础模式
# 依赖: common.sh、config.sh、api.sh、netproxy-native、cmd、dumpsys、inotifyd(busybox)。
#######################################

set -u  # 引用未定义变量报错

# 模块根目录与关键路径
readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly MODULE_CONF="$MODDIR/config/module.conf"
# 运行时临时目录放 tmpfs (/dev)：不磨损 flash、重启自动清空、不污染模块目录
readonly RUN_DIR="/dev/netproxy"
readonly LAST_CHECK_FILE="$RUN_DIR/wifi_last_check"  # 防抖时间戳 (跨 inotifyd 进程)
readonly WIFI_STATE_FILE="$RUN_DIR/wifi_state"       # 当前 WiFi 自动切换决策
readonly RT_TABLES="/data/misc/net/rt_tables"        # inotifyd 监听目标
readonly LOG_FILE="$MODDIR/logs/service.log"
readonly LOG_TAG="netmon"
readonly DEBOUNCE_SEC=2  # 防抖窗口(秒)，抗 WiFi 抖动

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/config.sh"
. "$MODDIR/scripts/utils/api.sh"

export PATH="$MODDIR/bin:$PATH"

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
      if [ -n "$fallback" ]; then
        status="$status$NL$fallback"
      fi
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
      if (length_value >= 2 &&
          substr(value, 1, 1) == "\"" &&
          substr(value, length_value, 1) == "\"") {
        value = substr(value, 2, length_value - 2)
      }

      normalized = tolower(value)
      if (value != "" &&
          normalized != "<unknown ssid>" &&
          normalized != "<none>") {
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
# 判断 SSID 是否在逗号分隔名单内 (归一化全角逗号，trim 两侧空白)
# 参数: $1 当前 SSID  $2 逗号分隔名单
# 返回: 0=命中，非 0=未命中
#######################################
ssid_in_list() {
  local ssid="$1"
  local list="$2"

  awk -v target="$ssid" -v values="$list" '
    BEGIN {
      gsub(/，/, ",", values)
      count = split(values, items, ",")
      for (i = 1; i <= count; i++) {
        s = items[i]
        sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s)
        if (s != "" && s == target) exit 0
      }
      exit 1
    }
  ' < /dev/null
}

#######################################
# 读取 WiFi 自动切换的当前决策
# 返回: 标准输出打印 proxying、bypassed 或 unknown
#######################################
read_wifi_state() {
  local state
  state="$(cat "$WIFI_STATE_FILE" 2> /dev/null || true)"
  case "$state" in
    proxying | bypassed) printf "%s\n" "$state" ;;
    *) printf "unknown\n" ;;
  esac
}

#######################################
# 读取用户配置的基础出站模式
# 返回: 标准输出打印 rule、global、direct 或 AllowAds
#######################################
read_base_mode() {
  local mode
  mode="$($MODDIR/bin/netproxy-native module mode --module-dir "$MODDIR" --format raw 2> /dev/null || printf "%s" "rule")"
  case "$mode" in
    rule | global | direct | AllowAds)
      printf "%s\n" "$mode"
      ;;
    *)
      log "WARN" "基础出站模式无效，已回退为 rule: $mode"
      printf "rule\n"
      ;;
  esac
}

#######################################
# 应用目标态
# 绕过态使用 Direct，代理态恢复 module.conf 中的基础出站模式
# 参数: $1 目标态 (proxying|bypassed)
# 返回: 0=成功，非 0=控制接口不可用或切换失败
#######################################
apply_state() {
  local target="$1"
  local current base_mode desired_mode desired_runtime_mode actual_mode

  current="$(read_wifi_state)"
  base_mode="$(read_base_mode)"
  if [ "$target" = "bypassed" ]; then
    desired_mode="direct"
  else
    desired_mode="$base_mode"
  fi
  desired_runtime_mode="$desired_mode"
  actual_mode="$(service_api_get_mode 2> /dev/null || true)"

  # 决策与实际模式都未变化时无需重复请求或中断连接
  if [ "$target" = "$current" ] && [ "$desired_runtime_mode" = "$actual_mode" ]; then
    return 0
  fi

  if [ "$desired_runtime_mode" != "$actual_mode" ]; then
    if ! "$MODDIR/bin/netproxy-native" module mode --module-dir "$MODDIR" "$desired_mode" > /dev/null 2>&1; then
      log "WARN" "出站模式切换失败，未能应用 WiFi 自动代理状态"
      return 1
    fi

    # 已建立连接不会自动迁移到新模式，仅在运行模式变化后主动关闭
    service_api_close_all_connections > /dev/null 2>&1 || true
  fi

  mkdir -p "$RUN_DIR" 2> /dev/null || true
  printf "%s\n" "$target" > "$WIFI_STATE_FILE"

  if [ "$target" = "bypassed" ]; then
    log "INFO" "已切换为: 绕过代理 (Direct)"
  else
    log "INFO" "已切换为: 走代理 ($desired_runtime_mode)"
  fi
}

#######################################
# 恢复基础出站模式并清理 WiFi 决策状态
# 返回: 0=成功，非 0=控制接口不可用或切换失败
#######################################
restore_base_mode() {
  if apply_state "proxying"; then
    rm -f "$WIFI_STATE_FILE"
    return 0
  fi
  return 1
}

#######################################
# 根据 当前网络 + 模式 + 名单 计算并应用目标态
# 全局: 由 load_wifi_conf 注入的 WIFI_* / PROXY_ON_CELLULAR
# 返回: 无
#######################################
decide_and_apply() {
  local snapshot net_type ssid target

  snapshot="$(get_wifi_snapshot)"
  net_type="${snapshot%%"$TAB"*}"
  if [ "$snapshot" = "$net_type" ]; then
    ssid=""
  else
    ssid="${snapshot#*"$TAB"}"
  fi

  if [ "$net_type" = "wifi" ]; then
    # SSID 暂不可读时不贸然切换，避免误判
    if [ -z "$ssid" ]; then
      log "DEBUG" "WiFi 已连接但 SSID 暂不可读，跳过本次决策"
      return 0
    fi

    if [ "$WIFI_SSID_MODE" = "whitelist" ]; then
      # 白名单：仅名单内 SSID 走代理
      if ssid_in_list "$ssid" "$WIFI_SSID_LIST"; then
        target="proxying"
      else
        target="bypassed"
      fi
    else
      # 黑名单：名单内 SSID 绕过
      if ssid_in_list "$ssid" "$WIFI_SSID_LIST"; then
        target="bypassed"
      else
        target="proxying"
      fi
    fi
    log "DEBUG" "WiFi SSID=[$ssid] 模式=$WIFI_SSID_MODE -> $target"
  else
    # 非 WiFi (移动数据等)：按 PROXY_ON_CELLULAR 决定
    if [ "$PROXY_ON_CELLULAR" = "1" ]; then
      target="proxying"
    else
      target="bypassed"
    fi
    log "DEBUG" "非 WiFi 网络，PROXY_ON_CELLULAR=$PROXY_ON_CELLULAR -> $target"
  fi

  apply_state "$target"
}

#######################################
# 读取 WiFi 自动切换相关配置到全局 (带默认值)
# 全局(写入): WIFI_AUTO_SWITCH WIFI_SSID_MODE WIFI_SSID_LIST PROXY_ON_CELLULAR
# 返回: 无
#######################################
load_wifi_conf() {
  WIFI_AUTO_SWITCH="$(read_module_value "$MODULE_CONF" "WIFI_AUTO_SWITCH" 2> /dev/null || printf "%s" "0")"
  WIFI_SSID_MODE="$(read_module_value "$MODULE_CONF" "WIFI_SSID_MODE" 2> /dev/null || printf "%s" "blacklist")"
  WIFI_SSID_LIST="$(read_module_value "$MODULE_CONF" "WIFI_SSID_LIST" 2> /dev/null || true)"
  PROXY_ON_CELLULAR="$(read_module_value "$MODULE_CONF" "PROXY_ON_CELLULAR" 2> /dev/null || printf "%s" "1")"
}

#######################################
# 停止 netmon 的 inotifyd 守护进程
# 返回: 无
#######################################
stop_watcher() {
  local pid
  for pid in $(pidof inotifyd 2> /dev/null); do
    if [ -f "/proc/$pid/cmdline" ] && grep -q "netmon.sh" "/proc/$pid/cmdline" 2> /dev/null; then
      kill "$pid" 2> /dev/null || true
    fi
  done
}

#######################################
# 启动 inotifyd 守护 (先去重)，监听 rt_tables 写事件
# 返回: 无
#######################################
start_watcher() {
  stop_watcher
  # rt_tables 尚未就绪时后台等待，避免 inotifyd 监听失败
  ( i=0; while [ ! -f "$RT_TABLES" ] && [ "$i" -lt 20 ]; do sleep 3; i=$((i + 1)); done
    [ -f "$RT_TABLES" ] && nohup inotifyd "$0" "$RT_TABLES" > /dev/null 2>&1 & ) &
}

#######################################
# sync：按配置启停守护并立即评估一次
#   开启 -> 起守护 + 立即评估；关闭 -> 停守护 + 恢复基础模式
#######################################
cmd_sync() {
  load_wifi_conf
  if [ "$WIFI_AUTO_SWITCH" = "1" ]; then
    start_watcher
    decide_and_apply
  else
    stop_watcher
    restore_base_mode
  fi
}

#######################################
# startup：核心启动后的轻量初始化
#   开启 -> 起守护 + 立即评估；关闭 -> 仅清理残留守护
#######################################
cmd_startup() {
  load_wifi_conf
  if [ "$WIFI_AUTO_SWITCH" = "1" ]; then
    start_watcher
    decide_and_apply
  else
    stop_watcher
  fi
}

#######################################
# eval：评估一次 (供启动 / CLI 改配置后调用)
#######################################
cmd_eval() {
  load_wifi_conf
  [ "$WIFI_AUTO_SWITCH" = "1" ] || return 0
  decide_and_apply
}

#######################################
# stop：停止守护并恢复基础出站模式
#######################################
cmd_stop() {
  stop_watcher
  restore_base_mode
}

#######################################
# inotifyd 事件入口：防抖后评估
# 参数: $1 事件字符串 (inotifyd 传入)
#######################################
on_inotify_event() {
  local now last diff
  mkdir -p "$RUN_DIR" 2> /dev/null || true

  # 防抖：窗口内的重复事件直接跳过
  now="$(date +%s)"
  last="$(cat "$LAST_CHECK_FILE" 2> /dev/null || echo 0)"
  diff=$((now - last))
  if [ "$diff" -lt "$DEBOUNCE_SEC" ]; then
    return 0
  fi
  printf "%s" "$now" > "$LAST_CHECK_FILE"

  cmd_eval
}

#######################################
# 主入口：区分「命名子命令」与「inotifyd 事件回调」
#######################################
main() {
  mkdir -p "$RUN_DIR" 2> /dev/null || true
  case "${1:-}" in
    startup) cmd_startup ;;
    sync) cmd_sync ;;
    stop) cmd_stop ;;
    eval) cmd_eval ;;
    # inotifyd 以 "<事件字符> <监听目录> [文件名]" 回调，首参为事件字符
    *) on_inotify_event "${1:-}" ;;
  esac
}

main "$@"
