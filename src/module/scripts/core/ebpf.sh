#!/system/bin/sh
#######################################
# 文件: ebpf.sh
# 功能: 读取 ebpf.conf 并生成 sing-box eBPF 入站运行时配置。
# 用法: 由 service.sh 通过 . "$MODDIR/scripts/core/ebpf.sh" 引入。
# 依赖: common.sh、config.sh 与 apps.sh。
#######################################

#######################################
# 将空格分隔文本转换为 JSON 字符串数组片段
# 参数:
#   $1  空格分隔列表
# 返回: 标准输出打印 JSON 字符串片段
#######################################
word_list_to_json() {
  local values="${1:-}"
  local value escaped output=""

  for value in $values; do
    escaped="$(json_escape "$value")"
    if [ -n "$output" ]; then
      output="$output, \"$escaped\""
    else
      output="\"$escaped\""
    fi
  done

  printf "%s" "$output"
}

#######################################
# 校验并返回 eBPF Map 容量
# 参数:
#   $1  配置值
#   $2  配置键名
# 返回: 合法返回 0，否则退出
#######################################
validate_map_capacity() {
  local value="$1"
  local key="$2"

  case "$value" in
    "" | *[!0-9]*) die "$key 必须是 1 到 1048576 之间的整数" ;;
  esac
  [ "$value" -ge 1 ] && [ "$value" -le 1048576 ] \
    || die "$key 必须是 1 到 1048576 之间的整数"
}

#######################################
# 生成 eBPF 入站运行时配置
# 参数: 无
# 全局: 读取 EBPF_CONF，写入 RUNTIME_EBPF_FILE
# 返回: 标准输出打印输出文件路径
#######################################
write_runtime_ebpf() {
  local network network_field udp_timeout dns_mode cgroup_path ipv6
  local bypass_rules bypass_json app_enabled app_mode package_list uids
  local include_uid_json="" exclude_uid_json=""
  local shared_enabled shared_interfaces shared_interfaces_json shared_json_enabled
  local tcp_capacity udp_capacity socket_capacity shared_capacity redirect_json

  require_file "${EBPF_CONF:-}" "eBPF 配置文件不存在: ${EBPF_CONF:-未定义}"

  # ebpf.conf 本身采用 Shell 赋值格式；生成入站时一次加载全部设置。
  EBPF_NETWORK=""
  EBPF_UDP_TIMEOUT="5m"
  EBPF_DNS_MODE="hijack"
  EBPF_CGROUP_PATH=""
  EBPF_IPV6=1
  EBPF_BYPASS_RULE_SETS="direct ChinaIP"
  APP_PROXY_ENABLE=1
  APP_PROXY_MODE="blacklist"
  PROXY_APPS_LIST=""
  BYPASS_APPS_LIST=""
  EBPF_SHARED_NETWORK=0
  EBPF_SHARED_INTERFACES="wlan2"
  EBPF_TCP_MAP_CAPACITY=65536
  EBPF_UDP_MAP_CAPACITY=65536
  EBPF_SOCKET_MAP_CAPACITY=65536
  EBPF_SHARED_MAP_CAPACITY=65536
  . "$EBPF_CONF"

  network="${EBPF_NETWORK:-}"
  udp_timeout="${EBPF_UDP_TIMEOUT:-5m}"
  dns_mode="${EBPF_DNS_MODE:-hijack}"
  cgroup_path="${EBPF_CGROUP_PATH:-}"
  ipv6="${EBPF_IPV6:-1}"
  bypass_rules="${EBPF_BYPASS_RULE_SETS:-direct ChinaIP}"

  case "$network" in
    "" | tcp | udp) ;;
    *) die "未知 eBPF 网络类型: $network" ;;
  esac
  case "$dns_mode" in
    hijack | off) ;;
    *) die "未知 eBPF DNS 模式: $dns_mode" ;;
  esac
  case "$ipv6" in
    0 | 1) ;;
    *) die "EBPF_IPV6 只能为 0 或 1" ;;
  esac

  tcp_capacity="${EBPF_TCP_MAP_CAPACITY:-65536}"
  udp_capacity="${EBPF_UDP_MAP_CAPACITY:-65536}"
  socket_capacity="${EBPF_SOCKET_MAP_CAPACITY:-65536}"
  shared_capacity="${EBPF_SHARED_MAP_CAPACITY:-65536}"
  validate_map_capacity "$tcp_capacity" "EBPF_TCP_MAP_CAPACITY"
  validate_map_capacity "$udp_capacity" "EBPF_UDP_MAP_CAPACITY"
  validate_map_capacity "$socket_capacity" "EBPF_SOCKET_MAP_CAPACITY"
  validate_map_capacity "$shared_capacity" "EBPF_SHARED_MAP_CAPACITY"

  # 将应用包名转换为 eBPF 可直接使用的 UID 策略。
  app_enabled="${APP_PROXY_ENABLE:-1}"
  app_mode="${APP_PROXY_MODE:-blacklist}"
  if [ "$app_enabled" = "1" ]; then
    case "$app_mode" in
      blacklist)
        package_list="${BYPASS_APPS_LIST:-}"
        uids="$(resolve_package_uids "$package_list")"
        exclude_uid_json="$(uid_list_to_json "$uids")"
        [ -z "$package_list" ] || [ -n "$exclude_uid_json" ] \
          || log "WARN" "未能解析应用绕过名单中的任何 UID"
        ;;
      whitelist)
        package_list="${PROXY_APPS_LIST:-}"
        uids="$(resolve_package_uids "$package_list")"
        include_uid_json="$(uid_list_to_json "$uids")"
        # 空白名单必须匹配不到任何应用，不能使用空数组回退为代理全部 UID。
        if [ -z "$include_uid_json" ]; then
          include_uid_json="4294967295"
          [ -z "$package_list" ] || log "WARN" "未能解析应用代理名单中的任何 UID"
        fi
        ;;
      *) die "未知分应用代理模式: $app_mode" ;;
    esac
  fi

  # shared_network 使用精确接口名，接口可在 sing-box 启动后出现。
  shared_enabled="${EBPF_SHARED_NETWORK:-0}"
  shared_interfaces="${EBPF_SHARED_INTERFACES:-wlan2}"
  shared_interfaces_json="$(word_list_to_json "$shared_interfaces")"
  case "$shared_enabled" in
    0) shared_json_enabled=false ;;
    1)
      [ -n "$shared_interfaces_json" ] || die "启用共享网络时必须配置 EBPF_SHARED_INTERFACES"
      [ "$dns_mode" != "hijack" ] || [ "$network" != "tcp" ] \
        || die "共享网络启用 DNS 劫持时必须代理 UDP"
      shared_json_enabled=true
      ;;
    *) die "EBPF_SHARED_NETWORK 只能为 0 或 1" ;;
  esac

  redirect_json='"127.128.0.0/9"'
  [ "$ipv6" = "1" ] && redirect_json="$redirect_json, \"fd53:696e:672d:626f::/64\""
  bypass_json="$(word_list_to_json "$bypass_rules")"

  # 留空表示同时代理 TCP 与 UDP；空字符串不能直接写入 network 字段。
  network_field=""
  if [ -n "$network" ]; then
    network_field="      \"network\": \"$(json_escape "$network")\",$NL"
  fi

  cat > "$RUNTIME_EBPF_FILE" << EOF
{
  "inbounds": [
    {
      "type": "ebpf",
      "tag": "ebpf-in",
      "cgroup_enabled": true,
${network_field}      "udp_timeout": "$(json_escape "$udp_timeout")",
      "dns_mode": "$dns_mode",
      "cgroup_path": "$(json_escape "$cgroup_path")",
      "redirect_address": [$redirect_json],
      "bypass_rule_set": [$bypass_json],
      "include_uid": [$include_uid_json],
      "include_uid_range": [],
      "exclude_uid": [$exclude_uid_json],
      "exclude_uid_range": [],
      "map_capacity": {
        "tcp_redirect": $tcp_capacity,
        "udp_redirect": $udp_capacity,
        "socket_bypass": $socket_capacity
      },
      "shared_network": {
        "enabled": $shared_json_enabled,
        "include_interface": [$shared_interfaces_json],
        "map_capacity": $shared_capacity
      }
    }
  ]
}
EOF

  printf "%s\n" "$RUNTIME_EBPF_FILE"
}
