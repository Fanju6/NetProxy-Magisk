#!/system/bin/sh
#######################################
# 文件: netmon.sh
# 功能: 网络变化监听与「按 WiFi SSID 自动开/关代理」决策。
#       由 inotifyd 监听 /data/misc/net/rt_tables 的写事件触发(网络切换时
#       Android 会重写该文件)，据当前 SSID + 黑/白名单决定走代理还是绕过，
#       据当前 SSID + 黑/白名单决定走代理还是绕过，自行增删 iptables 短路规则
#       (NETMON_BYPASS 链) 实现热切换，不重启 sing-box 核心，也不改动 tproxy.sh。
# 用法:
#   netmon.sh <events> <dir> [file]   inotifyd 代理(事件触发，含防抖)
#   netmon.sh eval [--force]          立即评估一次(启动时 / 改配置后)
#   netmon.sh sync                    按配置启停 inotifyd 守护并评估一次
#   netmon.sh stop                    停止 inotifyd 守护并恢复代理
# 依赖: common.sh、config.sh、tproxy.sh、dumpsys、ip、inotifyd(busybox)。
#######################################

set -u  # 引用未定义变量报错

# 模块根目录与关键路径
readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly TPROXY_DIR="$MODDIR/config/tproxy"
readonly TPROXY_CONF="$TPROXY_DIR/tproxy.conf"
# 运行时临时目录放 tmpfs (/dev)：不磨损 flash、重启自动清空、不污染模块目录
readonly RUN_DIR="/dev/netproxy"
readonly LAST_CHECK_FILE="$RUN_DIR/wifi_last_check"  # 防抖时间戳 (跨 inotifyd 进程)
readonly RT_TABLES="/data/misc/net/rt_tables"        # inotifyd 监听目标
readonly LOG_FILE="$MODDIR/logs/service.log"
readonly LOG_TAG="netmon"
readonly DEBOUNCE_SEC=2  # 防抖窗口(秒)，抗 WiFi 抖动

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/config.sh"

export PATH="$MODDIR/bin:$PATH"

# PLACEHOLDER_NETMON

#######################################
# 获取当前连接的 WiFi SSID
# 返回: 标准输出打印 SSID；无法确定时打印空
#######################################
get_current_ssid() {
  dumpsys wifi 2> /dev/null | awk '
    /mWifiInfo[[:space:]]+SSID:/ {
      s = $0

      # 精确匹配 mWifiInfo SSID，避免误匹配 BSSID
      sub(/^.*mWifiInfo[[:space:]]+SSID:[[:space:]]*/, "", s)

      # 兼容带引号和不带引号的 SSID
      if (substr(s, 1, 1) == "\"") {
        s = substr(s, 2)
        sub(/".*$/, "", s)
      } else {
        sub(/,.*/, "", s)
      }

      gsub(/^[ \t]+|[ \t]+$/, "", s)

      if (s != "" && s != "<unknown ssid>") {
        print s
        exit
      }
    }
  '
}

#######################################
# 判断当前是否为 WiFi 连接 (WiFi 已启用且 wlan0 有 IPv4)
# 全局: WIFI_INTERFACE (网卡名)
# 返回: 标准输出 "wifi" 或 "not_wifi"
#######################################
get_net_type() {
  local enabled ip4
  enabled="$(dumpsys wifi 2> /dev/null | awk '/Wi-Fi is enabled/ {print 1; exit}')"
  ip4="$(ip -4 addr show "$WIFI_INTERFACE" 2> /dev/null | awk '/inet / {sub(/\/.*/, "", $2); print $2; exit}')"
  if [ -n "$enabled" ] && [ -n "$ip4" ]; then
    printf "wifi"
  else
    printf "not_wifi"
  fi
}

#######################################
# 判断 SSID 是否在逗号分隔名单内 (归一化全角逗号，trim 两侧空白)
# 参数: $1 当前 SSID  $2 逗号分隔名单
# 返回: 0=命中，非 0=未命中
#######################################
ssid_in_list() {
  local ssid="$1"
  local list
  list="$(printf "%s" "$2" | sed 's/，/,/g')"
  printf "%s" "$list" | awk -v target="$ssid" -F',' '
    BEGIN { rc = 1 }
    {
      for (i = 1; i <= NF; i++) {
        s = $i
        sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s)
        if (s != "" && s == target) { rc = 0; exit }
      }
    }
    END { exit rc }
  '
}

#######################################
# 在 iptables/ip6tables 上执行变更命令 (失败吞掉)
# 参数: $@ 完整命令
#######################################
ipt_run() {
  "$@" 2> /dev/null || true
}

#######################################
# WiFi 热切换原语：在代理入口链首插/删 NETMON_BYPASS(ACCEPT)子链，
# 使流量短路绕过代理而无需重启 sing-box 核心。自含、不依赖 tproxy.sh。
# 参数: $1  enable=绕过代理  disable=恢复代理
# 全局: PROXY_IPV6 (是否处理 ip6tables)
# 返回: 无
# 说明: 仅对实际存在的链操作 (tproxy=mangle / redirect=nat，含 redirect2 的
#       NAT_DNS_HIJACK)，IPv4 + (启用时)IPv6 各一遍；幂等。
#######################################
apply_bypass() {
  local action="$1"
  local cmd suffix table chain

  for cmd in iptables ip6tables; do
    if [ "$cmd" = "ip6tables" ]; then
      [ "${PROXY_IPV6:-0}" = "1" ] || continue
      suffix="6"
    else
      suffix=""
    fi

    for table in mangle nat; do
      for chain in "PROXY_PREROUTING$suffix" "PROXY_OUTPUT$suffix" "NAT_DNS_HIJACK$suffix"; do
        # 仅处理该表中确实存在的链
        $cmd -t "$table" -L "$chain" -n > /dev/null 2>&1 || continue

        if [ "$action" = "enable" ]; then
          $cmd -t "$table" -L NETMON_BYPASS -n > /dev/null 2>&1 || ipt_run "$cmd" -t "$table" -N NETMON_BYPASS
          $cmd -t "$table" -C NETMON_BYPASS -j ACCEPT > /dev/null 2>&1 || ipt_run "$cmd" -t "$table" -A NETMON_BYPASS -j ACCEPT
          $cmd -t "$table" -C "$chain" -j NETMON_BYPASS > /dev/null 2>&1 || ipt_run "$cmd" -t "$table" -I "$chain" 1 -j NETMON_BYPASS
        else
          # 删除该链上所有指向 NETMON_BYPASS 的跳转 (失败即停，防死循环)
          while $cmd -t "$table" -C "$chain" -j NETMON_BYPASS > /dev/null 2>&1; do
            $cmd -t "$table" -D "$chain" -j NETMON_BYPASS 2> /dev/null || break
          done
        fi
      done

      # disable：清空并删除已无引用的 NETMON_BYPASS 链
      if [ "$action" = "disable" ]; then
        if $cmd -t "$table" -L NETMON_BYPASS -n > /dev/null 2>&1; then
          ipt_run "$cmd" -t "$table" -F NETMON_BYPASS
          ipt_run "$cmd" -t "$table" -X NETMON_BYPASS
        fi
      fi
    done
  done
}

#######################################
# 查询当前是否处于「绕过」态 (以 iptables 实际规则为真相源，免状态文件)
# 探测主代理链(mangle 优先 tproxy，nat 兜底 redirect)上是否存在 NETMON_BYPASS 跳转
# 返回: 0=绕过中(bypassed)，非 0=走代理(proxying)
#######################################
is_bypassed() {
  iptables -t mangle -C PROXY_PREROUTING -j NETMON_BYPASS > /dev/null 2>&1 && return 0
  iptables -t nat -C PROXY_PREROUTING -j NETMON_BYPASS > /dev/null 2>&1 && return 0
  return 1
}

#######################################
# 应用目标态 (与 iptables 实际态不同或强制时才切换，避免重复操作/日志)
# 参数: $1 目标态 (proxying|bypassed)  $2 是否强制 (1=强制)
# 返回: 无
#######################################
apply_state() {
  local target="$1"
  local force="${2:-0}"
  local current="proxying"

  is_bypassed && current="bypassed"

  if [ "$target" = "$current" ] && [ "$force" != "1" ]; then
    return 0
  fi

  if [ "$target" = "bypassed" ]; then
    apply_bypass enable
    log "INFO" "已切换为: 绕过代理 (bypassed)"
  else
    apply_bypass disable
    log "INFO" "已切换为: 走代理 (proxying)"
  fi
}

#######################################
# 根据 当前网络 + 模式 + 名单 计算并应用目标态
# 参数: $1 是否强制应用 (1=强制)
# 全局: 由 load_wifi_conf 注入的 WIFI_* / PROXY_ON_CELLULAR
# 返回: 无
#######################################
decide_and_apply() {
  local force="${1:-0}"
  local net_type ssid target

  net_type="$(get_net_type)"

  if [ "$net_type" = "wifi" ]; then
    ssid="$(get_current_ssid)"
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

  apply_state "$target" "$force"
}

#######################################
# 读取 WiFi 自动切换相关配置到全局 (带默认值)
# 全局(写入): WIFI_AUTO_SWITCH WIFI_SSID_MODE WIFI_SSID_LIST
#             PROXY_ON_CELLULAR WIFI_INTERFACE PROXY_IPV6
# 返回: 无
#######################################
load_wifi_conf() {
  WIFI_AUTO_SWITCH="$(read_conf "$TPROXY_CONF" "WIFI_AUTO_SWITCH" "0")"
  WIFI_SSID_MODE="$(read_conf "$TPROXY_CONF" "WIFI_SSID_MODE" "blacklist")"
  WIFI_SSID_LIST="$(read_conf "$TPROXY_CONF" "WIFI_SSID_LIST" "")"
  PROXY_ON_CELLULAR="$(read_conf "$TPROXY_CONF" "PROXY_ON_CELLULAR" "1")"
  WIFI_INTERFACE="$(read_conf "$TPROXY_CONF" "WIFI_INTERFACE" "wlan0")"
  PROXY_IPV6="$(read_conf "$TPROXY_CONF" "PROXY_IPV6" "0")"
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
#   开启 -> 起守护 + 强制评估；关闭 -> 停守护 + 恢复代理
#######################################
cmd_sync() {
  load_wifi_conf
  if [ "$WIFI_AUTO_SWITCH" = "1" ]; then
    start_watcher
    decide_and_apply 1
  else
    stop_watcher
    # 关闭功能时确保恢复为正常代理 (移除 NETMON_BYPASS)
    apply_bypass disable
  fi
}

#######################################
# eval：评估一次 (供启动 / CLI 改配置后调用)
# 参数: $1 可为 --force
#######################################
cmd_eval() {
  load_wifi_conf
  [ "$WIFI_AUTO_SWITCH" = "1" ] || return 0
  if [ "${1:-}" = "--force" ]; then
    decide_and_apply 1
  else
    decide_and_apply 0
  fi
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
    sync) cmd_sync ;;
    stop) stop_watcher ;;
    eval) shift 2> /dev/null || true; cmd_eval "$@" ;;
    # inotifyd 以 "<事件字符> <监听目录> [文件名]" 回调，首参为事件字符
    *) on_inotify_event "${1:-}" ;;
  esac
}

main "$@"



