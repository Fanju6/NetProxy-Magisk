#!/system/bin/sh
#######################################
# 文件: runtime.sh
# 功能: sing-box 运行时配置辅助函数，负责读取模块配置、扫描
#       当前节点目录，并生成出站与 eBPF 入站配置。
# 用法: 由 service.sh 等通过 . "$MODDIR/scripts/core/runtime.sh" 引入。
#       依赖 common.sh (NL/TAB/die)、config.sh (read_conf)、nodes.sh。
#######################################

# 当前出站上下文 (由 initialize_runtime_context 填充)
CUR_OUTBOUND_CONFIG=""   # 当前节点配置文件路径
CUR_OUTBOUND_DIR=""      # 当前节点所在目录
CUR_OUTBOUND_MODE=""     # 出站模式 (rule/global/direct)
CUR_SELECTOR_MODE=""     # 节点选择模式 (urltest/manual)
CUR_CURRENT_TAG=""       # 当前节点的出站标签

# 节点扫描结果 (由 scan_runtime_nodes 填充)
RUNTIME_OUTBOUNDS_FILE=""   # 运行时出站配置输出路径
RUNTIME_NODE_PATHS=""       # 扫描到的节点文件路径列表 (换行分隔)
RUNTIME_NODE_TAGS_JSON=""   # 节点标签的 JSON 数组片段
RUNTIME_NODE_COUNT=0        # 有效节点数
RUNTIME_SKIPPED_COUNT=0     # 跳过 (无 tag) 的文件数
RUNTIME_EBPF_FILE=""        # eBPF 入站配置输出路径

#######################################
# 初始化运行时上下文
# 从模块配置读取当前节点/模式，并校验相关文件与目录。
# 参数: 无
# 全局: 读取 MODULE_CONF/SINGBOX_DIR/CONFDIR/RUNTIME_DIR；填充 CUR_* 与 RUNTIME_OUTBOUNDS_FILE
# 返回: 成功返回 0，校验失败则退出
#######################################
initialize_runtime_context() {
  # 校验必需的配置文件与目录
  require_file "${MODULE_CONF:-}" "模块配置文件不存在: ${MODULE_CONF:-未定义}"
  require_dir "${SINGBOX_DIR:-}" "sing-box 配置目录不存在: ${SINGBOX_DIR:-未定义}"
  require_dir "${CONFDIR:-}" "通用配置目录不存在: ${CONFDIR:-未定义}"
  require_dir "${RUNTIME_DIR:-}" "运行时目录不存在: ${RUNTIME_DIR:-未定义}"

  # 读取当前节点路径与运行模式
  CUR_OUTBOUND_CONFIG="$(read_conf "$MODULE_CONF" "CURRENT_CONFIG" "")"
  CUR_OUTBOUND_MODE="$(read_conf "$MODULE_CONF" "OUTBOUND_MODE" "rule")"
  CUR_SELECTOR_MODE="$(read_conf "$MODULE_CONF" "SELECTOR_MODE" "urltest")"

  # 必须已选择节点且文件存在
  [ -n "$CUR_OUTBOUND_CONFIG" ] || die "CURRENT_CONFIG 未定义，请先选择节点"
  require_file "$CUR_OUTBOUND_CONFIG" "当前节点配置文件不存在: $CUR_OUTBOUND_CONFIG"

  # 解析节点所在目录
  CUR_OUTBOUND_DIR="${CUR_OUTBOUND_CONFIG%/*}"
  [ "$CUR_OUTBOUND_DIR" != "$CUR_OUTBOUND_CONFIG" ] || die "无法解析当前节点目录: $CUR_OUTBOUND_CONFIG"
  require_dir "$CUR_OUTBOUND_DIR" "当前节点目录不存在: $CUR_OUTBOUND_DIR"

  # 读取当前节点标签
  CUR_CURRENT_TAG="$(detect_outbound_tag "$CUR_OUTBOUND_CONFIG" || true)"
  [ -n "$CUR_CURRENT_TAG" ] || die "无法读取当前节点标签: $CUR_OUTBOUND_CONFIG"

  RUNTIME_OUTBOUNDS_FILE="$RUNTIME_DIR/outbounds.json"
  RUNTIME_EBPF_FILE="$RUNTIME_DIR/ebpf.json"
}

#######################################
# 将空格分隔的文本转换为 JSON 字符串数组片段
# 参数:
#   $1  空格分隔列表
# 返回: 标准输出打印逗号分隔的 JSON 字符串
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
# 返回: 标准输出打印合法容量
#######################################
validate_map_capacity() {
  local value="$1"
  local key="$2"

  case "$value" in
    "" | *[!0-9]*) die "$key 必须是 1 到 1048576 之间的整数" ;;
  esac
  [ "$value" -ge 1 ] && [ "$value" -le 1048576 ] \
    || die "$key 必须是 1 到 1048576 之间的整数"
  printf "%s" "$value"
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

  network="$(read_conf "$EBPF_CONF" "EBPF_NETWORK" "")"
  udp_timeout="$(read_conf "$EBPF_CONF" "EBPF_UDP_TIMEOUT" "5m")"
  dns_mode="$(read_conf "$EBPF_CONF" "EBPF_DNS_MODE" "hijack")"
  cgroup_path="$(read_conf "$EBPF_CONF" "EBPF_CGROUP_PATH" "")"
  ipv6="$(read_conf "$EBPF_CONF" "EBPF_IPV6" "1")"
  bypass_rules="$(read_conf "$EBPF_CONF" "EBPF_BYPASS_RULE_SETS" "direct ChinaIP")"

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

  tcp_capacity="$(validate_map_capacity "$(read_conf "$EBPF_CONF" "EBPF_TCP_MAP_CAPACITY" "65536")" "EBPF_TCP_MAP_CAPACITY")" || exit 1
  udp_capacity="$(validate_map_capacity "$(read_conf "$EBPF_CONF" "EBPF_UDP_MAP_CAPACITY" "65536")" "EBPF_UDP_MAP_CAPACITY")" || exit 1
  socket_capacity="$(validate_map_capacity "$(read_conf "$EBPF_CONF" "EBPF_SOCKET_MAP_CAPACITY" "65536")" "EBPF_SOCKET_MAP_CAPACITY")" || exit 1
  shared_capacity="$(validate_map_capacity "$(read_conf "$EBPF_CONF" "EBPF_SHARED_MAP_CAPACITY" "65536")" "EBPF_SHARED_MAP_CAPACITY")" || exit 1

  # 将应用包名转换为 eBPF 可直接使用的 UID 策略
  app_enabled="$(read_conf "$EBPF_CONF" "APP_PROXY_ENABLE" "1")"
  app_mode="$(read_conf "$EBPF_CONF" "APP_PROXY_MODE" "blacklist")"
  if [ "$app_enabled" = "1" ]; then
    case "$app_mode" in
      blacklist)
        package_list="$(read_conf "$EBPF_CONF" "BYPASS_APPS_LIST" "")"
        uids="$(resolve_package_uids "$package_list")"
        exclude_uid_json="$(uid_list_to_json "$uids")"
        [ -z "$package_list" ] || [ -n "$exclude_uid_json" ] \
          || log "WARN" "未能解析应用绕过名单中的任何 UID"
        ;;
      whitelist)
        package_list="$(read_conf "$EBPF_CONF" "PROXY_APPS_LIST" "")"
        uids="$(resolve_package_uids "$package_list")"
        include_uid_json="$(uid_list_to_json "$uids")"
        # 空白名单必须匹配不到任何应用，不能使用空数组回退为代理全部 UID
        if [ -z "$include_uid_json" ]; then
          include_uid_json="4294967295"
          [ -z "$package_list" ] || log "WARN" "未能解析应用代理名单中的任何 UID"
        fi
        ;;
      *) die "未知分应用代理模式: $app_mode" ;;
    esac
  fi

  # shared_network 使用精确接口名，接口可在 sing-box 启动后出现
  shared_enabled="$(read_conf "$EBPF_CONF" "EBPF_SHARED_NETWORK" "0")"
  shared_interfaces="$(read_conf "$EBPF_CONF" "EBPF_SHARED_INTERFACES" "wlan2")"
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

  # network 留空表示同时代理 TCP 与 UDP。当前核心只在字段省略时应用默认值，
  # 若写入空字符串会被配置解码器判定为未知网络类型。
  network_field=""
  if [ -n "$network" ]; then
    network_field="      \"network\": \"$(json_escape "$network")\",
"
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

#######################################
# 清空节点扫描结果缓存
# 参数: 无
# 返回: 无
#######################################
reset_runtime_nodes() {
  RUNTIME_NODE_PATHS=""
  RUNTIME_NODE_TAGS_JSON=""
  RUNTIME_NODE_COUNT=0
  RUNTIME_SKIPPED_COUNT=0
}

#######################################
# 扫描节点目录，收集节点路径与标签
# 参数:
#   $1  节点目录 (默认 CUR_OUTBOUND_DIR)
# 全局: 填充 RUNTIME_NODE_PATHS/RUNTIME_NODE_TAGS_JSON/RUNTIME_NODE_COUNT/RUNTIME_SKIPPED_COUNT
# 返回: 无
#######################################
scan_runtime_nodes() {
  local current_dir="${1:-$CUR_OUTBOUND_DIR}"
  require_dir "$current_dir" "节点目录不存在: $current_dir"
  reset_runtime_nodes

  local parsed_data
  # 用 awk 提取每个文件的首个 tag，输出 "文件名<TAB>标签"
  # (依赖 awk 的 nextfile 扩展)
  parsed_data=$(awk -F'"' '
    # 每个新文件开始时重置标志位
    FNR == 1 { found = 0 }

    # 匹配含有 "tag": "xxx" 的行
    !found && /"tag"[ \t]*:/ {
        tag = $4

        # 输出 文件名[TAB]标签
        printf "%s\t%s\n", FILENAME, tag

        # 标记已找到，跳过该文件后续行
        found = 1
        nextfile
    }
  ' "$current_dir"/*.json 2>/dev/null)

  # 未解析到任何数据则直接返回
  [ -z "$parsed_data" ] && return

  # 逐行读取 awk 结果，累积节点路径与标签
  local file tag
  while IFS="$TAB" read -r file tag; do
    # 标签为空的记录计入跳过
    if [ -z "$tag" ]; then
      RUNTIME_SKIPPED_COUNT=$((RUNTIME_SKIPPED_COUNT + 1))
      continue
    fi

    # 追加节点路径，并将非保留标签拼入 JSON 数组片段
    if [ -n "$RUNTIME_NODE_PATHS" ]; then
      RUNTIME_NODE_PATHS="${RUNTIME_NODE_PATHS}${NL}${file}"

      if ! is_reserved_outbound_tag "$tag"; then
        RUNTIME_NODE_TAGS_JSON="${RUNTIME_NODE_TAGS_JSON}, \"$tag\""
      fi
    else
      RUNTIME_NODE_PATHS="$file"

      if ! is_reserved_outbound_tag "$tag"; then
        RUNTIME_NODE_TAGS_JSON="\"$tag\""
      fi
    fi

    RUNTIME_NODE_COUNT=$((RUNTIME_NODE_COUNT + 1))
  done << EOF
$parsed_data
EOF

  # 统计目录内文件总数，反推被跳过 (无 tag) 的数量
  local total_files=0 _f
  for _f in "$current_dir"/*.json; do
    [ -e "$_f" ] && total_files=$((total_files + 1))
  done
  RUNTIME_SKIPPED_COUNT=$((total_files - RUNTIME_NODE_COUNT))
}

#######################################
# 生成运行时出站配置文件 (outbounds.json)
# 参数:
#   $1  当前节点配置路径 (默认 CUR_OUTBOUND_CONFIG)
#   $2  选择模式 (默认 CUR_SELECTOR_MODE)
# 返回: 标准输出打印输出文件路径；无可用标签或未知模式则退出
#######################################
write_runtime_outbounds() {
  local current_config="${1:-$CUR_OUTBOUND_CONFIG}"
  local selector_mode="${2:-$CUR_SELECTOR_MODE}"
  local tags="$RUNTIME_NODE_TAGS_JSON"

  [ -n "$current_config" ] || die "当前节点配置未初始化"
  [ -n "$selector_mode" ] || selector_mode="urltest"

  # 尚未扫描节点时先执行一次扫描
  if [ "$RUNTIME_NODE_COUNT" -eq 0 ] && [ -z "$RUNTIME_NODE_PATHS" ]; then
    scan_runtime_nodes "$CUR_OUTBOUND_DIR"
    tags="$RUNTIME_NODE_TAGS_JSON"
  fi

  # 扫描无标签时，回退使用当前节点标签 (非保留标签)
  if [ -z "$tags" ] && ! is_reserved_outbound_tag "$CUR_CURRENT_TAG"; then
    tags="\"$CUR_CURRENT_TAG\""
  fi

  [ -n "$tags" ] || die "当前节点目录没有可用的出站标签: $CUR_OUTBOUND_DIR"

  # 按选择模式生成对应结构的出站配置
  case "$selector_mode" in
    urltest | auto | 动态测速)
      # 动态测速模式：额外生成 Auto-Fastest 自动测速组
      cat > "$RUNTIME_OUTBOUNDS_FILE" << EOF
{
  "outbounds": [
    {
      "tag": "direct",
      "type": "direct"
    },
    {
      "tag": "wifi-direct",
      "type": "direct"
    },
    {
      "tag": "block",
      "type": "block"
    },
    {
      "tag": "Proxy",
      "type": "selector",
      "outbounds": [
        "Auto-Fastest",
        "direct",
        $tags
      ],
      "default": "Auto-Fastest",
      "interrupt_exist_connections": true
    },
    {
      "tag": "Auto-Fastest",
      "type": "urltest",
      "outbounds": [
        $tags
      ],
      "url": "https://www.gstatic.com/generate_204",
      "interval": "3m",
      "tolerance": 50
    }
  ]
}
EOF
      ;;
    manual | selector | 手动选择 | 手动)
      # 手动选择模式：仅生成 selector，默认指向当前节点
      cat > "$RUNTIME_OUTBOUNDS_FILE" << EOF
{
  "outbounds": [
    {
      "tag": "direct",
      "type": "direct"
    },
    {
      "tag": "wifi-direct",
      "type": "direct"
    },
    {
      "tag": "block",
      "type": "block"
    },
    {
      "tag": "Proxy",
      "type": "selector",
      "outbounds": [
        "direct",
        $tags
      ],
      "default": "$CUR_CURRENT_TAG",
      "interrupt_exist_connections": true
    }
  ]
}
EOF
      ;;
    *)
      die "未知节点选择模式: $selector_mode"
      ;;
  esac

  # 输出生成的配置文件路径
  printf "%s\n" "$RUNTIME_OUTBOUNDS_FILE"
}
