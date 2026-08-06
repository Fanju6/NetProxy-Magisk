#!/system/bin/sh
#######################################
# 文件: ebpf.sh
# 功能: 读取 ebpf.conf 并生成 sing-box eBPF 入站运行时配置。
# 用法: 由 service.sh 通过 . "$MODDIR/scripts/core/ebpf.sh" 引入。
# 依赖: common.sh 与 config.sh。
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
# 将非负整数列表转换为 JSON 数组片段
# 参数:
#   $1  空格分隔的非负整数
#   $2  配置键名
# 返回: 标准输出打印 JSON 数字片段
#######################################
integer_list_to_json() {
  local values="${1:-}"
  local key="$2"
  local value output=""

  validate_android_user_list "$values" "$key" || die "$key 只能包含非负整数"
  for value in $values; do
    if [ -n "$output" ]; then
      output="$output, $value"
    else
      output="$value"
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
  local network network_field udp_timeout dns_mode cgroup_enabled cgroup_json_enabled
  local cgroup_path ipv6_mode ipv6_enabled cgroup_ipv6_mode cgroup_fields=""
  local bypass_rules bypass_json app_enabled app_mode app_users app_users_json=""
  local package_list include_package_json="" exclude_package_json=""
  local include_uid_json="" exclude_uid_json=""
  local shared_enabled shared_interfaces shared_interfaces_json shared_json_enabled
  local shared_include_cidrs shared_exclude_cidrs shared_include_json shared_exclude_json
  local shared_tc_priority tcp_capacity udp_capacity socket_capacity shared_capacity redirect_json

  require_file "${EBPF_CONF:-}" "eBPF 配置文件不存在: ${EBPF_CONF:-未定义}"

  # ebpf.conf 本身采用 Shell 赋值格式；生成入站时一次加载全部设置。
  EBPF_NETWORK=""
  EBPF_UDP_TIMEOUT="5m"
  EBPF_DNS_MODE="hijack"
  EBPF_CGROUP_ENABLED=1
  EBPF_CGROUP_PATH=""
  EBPF_IPV6_MODE="auto"
  EBPF_BYPASS_RULE_SETS="direct ChinaIP"
  APP_PROXY_ENABLE=1
  APP_PROXY_MODE="blacklist"
  APP_ANDROID_USERS=""
  PROXY_APPS_LIST=""
  BYPASS_APPS_LIST=""
  EBPF_SHARED_NETWORK=0
  EBPF_SHARED_INTERFACES="wlan2"
  EBPF_SHARED_INCLUDE_SOURCE_CIDRS=""
  EBPF_SHARED_EXCLUDE_SOURCE_CIDRS=""
  EBPF_SHARED_TC_PRIORITY=1
  EBPF_TCP_MAP_CAPACITY=65536
  EBPF_UDP_MAP_CAPACITY=65536
  EBPF_SOCKET_MAP_CAPACITY=65536
  EBPF_SHARED_MAP_CAPACITY=65536
  . "$EBPF_CONF"

  network="${EBPF_NETWORK:-}"
  udp_timeout="${EBPF_UDP_TIMEOUT:-5m}"
  dns_mode="${EBPF_DNS_MODE:-hijack}"
  cgroup_enabled="${EBPF_CGROUP_ENABLED:-1}"
  cgroup_path="${EBPF_CGROUP_PATH:-}"
  ipv6_mode="${EBPF_IPV6_MODE:-auto}"
  # 配置允许显式留空；空值表示不在 eBPF 层提前绕过任何规则集。
  bypass_rules="$EBPF_BYPASS_RULE_SETS"

  case "$network" in
    "" | tcp | udp) ;;
    *) die "未知 eBPF 网络类型: $network" ;;
  esac
  case "$dns_mode" in
    hijack | off) ;;
    *) die "未知 eBPF DNS 模式: $dns_mode" ;;
  esac
  case "$cgroup_enabled" in
    0) cgroup_json_enabled=false ;;
    1) cgroup_json_enabled=true ;;
    *) die "EBPF_CGROUP_ENABLED 只能为 0 或 1" ;;
  esac
  case "$ipv6_mode" in
    disabled)
      ipv6_enabled=0
      cgroup_ipv6_mode="off"
      ;;
    auto | always)
      ipv6_enabled=1
      cgroup_ipv6_mode="$ipv6_mode"
      ;;
    shared)
      ipv6_enabled=1
      cgroup_ipv6_mode="off"
      ;;
    *) die "EBPF_IPV6_MODE 只能为 disabled、auto、always 或 shared" ;;
  esac

  tcp_capacity="${EBPF_TCP_MAP_CAPACITY:-65536}"
  udp_capacity="${EBPF_UDP_MAP_CAPACITY:-65536}"
  socket_capacity="${EBPF_SOCKET_MAP_CAPACITY:-65536}"
  shared_capacity="${EBPF_SHARED_MAP_CAPACITY:-65536}"
  if [ "$cgroup_enabled" = "1" ]; then
    validate_map_capacity "$tcp_capacity" "EBPF_TCP_MAP_CAPACITY"
    validate_map_capacity "$udp_capacity" "EBPF_UDP_MAP_CAPACITY"
    validate_map_capacity "$socket_capacity" "EBPF_SOCKET_MAP_CAPACITY"
  fi
  validate_map_capacity "$shared_capacity" "EBPF_SHARED_MAP_CAPACITY"

  # 包名由 sing-box 通过 Android PackageManager 解析，无需模块转换 UID。
  app_enabled="${APP_PROXY_ENABLE:-1}"
  app_mode="${APP_PROXY_MODE:-blacklist}"
  app_users="${APP_ANDROID_USERS:-}"
  case "$app_enabled" in 0 | 1) ;; *) die "APP_PROXY_ENABLE 只能为 0 或 1" ;; esac
  if [ "$cgroup_enabled" = "1" ] && [ "$app_enabled" = "1" ]; then
    app_users_json="$(integer_list_to_json "$app_users" "APP_ANDROID_USERS")"
    case "$app_mode" in
      blacklist)
        package_list="${BYPASS_APPS_LIST:-}"
        validate_android_package_list "$package_list" "BYPASS_APPS_LIST" \
          || die "BYPASS_APPS_LIST 包含无效包名"
        exclude_package_json="$(word_list_to_json "$package_list")"
        ;;
      whitelist)
        package_list="${PROXY_APPS_LIST:-}"
        validate_android_package_list "$package_list" "PROXY_APPS_LIST" \
          || die "PROXY_APPS_LIST 包含无效包名"
        include_package_json="$(word_list_to_json "$package_list")"
        # 空白名单必须匹配不到任何应用，不能使用空数组回退为代理全部 UID。
        if [ -z "$include_package_json" ]; then
          include_uid_json="4294967295"
        fi
        ;;
      *) die "未知分应用代理模式: $app_mode" ;;
    esac
  fi

  # shared_network 使用精确接口名，接口可在 sing-box 启动后出现。
  shared_enabled="${EBPF_SHARED_NETWORK:-0}"
  shared_interfaces="${EBPF_SHARED_INTERFACES:-wlan2}"
  shared_include_cidrs="${EBPF_SHARED_INCLUDE_SOURCE_CIDRS:-}"
  shared_exclude_cidrs="${EBPF_SHARED_EXCLUDE_SOURCE_CIDRS:-}"
  shared_tc_priority="${EBPF_SHARED_TC_PRIORITY:-1}"
  shared_interfaces_json="$(word_list_to_json "$shared_interfaces")"
  shared_include_json="$(word_list_to_json "$shared_include_cidrs")"
  shared_exclude_json="$(word_list_to_json "$shared_exclude_cidrs")"
  case "$shared_tc_priority" in
    "" | *[!0-9]*) die "EBPF_SHARED_TC_PRIORITY 必须是 1 到 65535 之间的整数" ;;
  esac
  [ "$shared_tc_priority" -ge 1 ] && [ "$shared_tc_priority" -le 65535 ] \
    || die "EBPF_SHARED_TC_PRIORITY 必须是 1 到 65535 之间的整数"
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
  [ "$cgroup_enabled" = "1" ] || [ "$shared_enabled" = "1" ] \
    || die "本机 cgroup 与共享网络不能同时禁用"

  redirect_json='"127.128.0.0/9"'
  [ "$ipv6_enabled" = "1" ] && redirect_json="$redirect_json, \"fd53:696e:672d:626f::/64\""
  bypass_json="$(word_list_to_json "$bypass_rules")"

  # 留空表示同时代理 TCP 与 UDP；空字符串不能直接写入 network 字段。
  network_field=""
  if [ -n "$network" ]; then
    network_field="      \"network\": \"$(json_escape "$network")\",$NL"
  fi

  if [ "$cgroup_enabled" = "1" ]; then
    cgroup_fields="      \"cgroup_path\": \"$(json_escape "$cgroup_path")\",$NL"
    cgroup_fields="$cgroup_fields      \"cgroup_ipv6_mode\": \"$cgroup_ipv6_mode\",$NL"
    cgroup_fields="$cgroup_fields      \"include_uid\": [$include_uid_json],$NL"
    cgroup_fields="$cgroup_fields      \"include_uid_range\": [],$NL"
    cgroup_fields="$cgroup_fields      \"exclude_uid\": [$exclude_uid_json],$NL"
    cgroup_fields="$cgroup_fields      \"exclude_uid_range\": [],$NL"
    cgroup_fields="$cgroup_fields      \"include_android_user\": [$app_users_json],$NL"
    cgroup_fields="$cgroup_fields      \"include_package\": [$include_package_json],$NL"
    cgroup_fields="$cgroup_fields      \"exclude_package\": [$exclude_package_json],$NL"
    cgroup_fields="$cgroup_fields      \"map_capacity\": {$NL"
    cgroup_fields="$cgroup_fields        \"tcp_redirect\": $tcp_capacity,$NL"
    cgroup_fields="$cgroup_fields        \"udp_redirect\": $udp_capacity,$NL"
    cgroup_fields="$cgroup_fields        \"socket_bypass\": $socket_capacity$NL"
    cgroup_fields="$cgroup_fields      },$NL"
  fi

  cat > "$RUNTIME_EBPF_FILE" << EOF
{
  "inbounds": [
    {
      "type": "ebpf",
      "tag": "ebpf-in",
      "cgroup_enabled": $cgroup_json_enabled,
${network_field}      "udp_timeout": "$(json_escape "$udp_timeout")",
      "dns_mode": "$dns_mode",
${cgroup_fields}      "redirect_address": [$redirect_json],
      "bypass_rule_set": [$bypass_json],
      "shared_network": {
        "enabled": $shared_json_enabled,
        "include_interface": [$shared_interfaces_json],
        "include_source_cidr": [$shared_include_json],
        "exclude_source_cidr": [$shared_exclude_json],
        "tc_priority": $shared_tc_priority,
        "map_capacity": $shared_capacity
      }
    }
  ]
}
EOF

  printf "%s\n" "$RUNTIME_EBPF_FILE"
}
