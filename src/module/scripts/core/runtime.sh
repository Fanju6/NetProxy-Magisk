#!/system/bin/sh
#######################################
# 文件: runtime.sh
# 功能: 根据 Catalog 生成 sing-box Local Provider 与分组选择器配置。
# 用法: 由 service.sh 通过 . "$MODDIR/scripts/core/runtime.sh" 引入。
# 依赖: common.sh、config.sh、catalog.sh 与 metadata.sh。
#######################################

. "$MODDIR/scripts/utils/metadata.sh"

# 当前运行上下文
CUR_ACTIVE_GROUP_ID=""    # 当前活动分组 ID
CUR_OUTBOUND_MODE=""      # 出站模式 (rule/global/direct)
CUR_SELECTOR_MODE=""      # 节点选择模式 (urltest/manual)
CUR_SELECTED_NODE_REF=""  # 手动节点引用 (<group-id>/<tag>)
CUR_ACTIVE_GROUP_TAG=""   # 当前活动分组的运行时显示标签

# Catalog 投影结果
RUNTIME_GROUP_IDS=""        # 非空分组 ID (换行分隔)
RUNTIME_GROUP_RECORDS=""    # 分组 ID 与运行时标签 (制表符分隔)
RUNTIME_GROUP_COUNT=0       # 非空分组数
RUNTIME_NODE_COUNT=0        # 全部分组节点总数
RUNTIME_PROVIDER_ENTRIES="" # Provider JSON 对象片段
RUNTIME_BUILD_ERROR=""      # 最近一次运行时投影失败原因

# 运行时配置输出路径
RUNTIME_PROVIDERS_FILE=""
RUNTIME_OUTBOUNDS_FILE=""
RUNTIME_EBPF_FILE=""

#######################################
# 初始化运行时上下文
# 参数: 无
# 全局: 读取模块配置并填充 CUR_* 与运行时配置路径
# 返回: 成功返回 0，配置非法则退出
#######################################
initialize_runtime_context() {
  require_file "${MODULE_CONF:-}" "模块配置文件不存在: ${MODULE_CONF:-未定义}"
  require_dir "${SINGBOX_DIR:-}" "sing-box 配置目录不存在: ${SINGBOX_DIR:-未定义}"
  require_dir "${CONFDIR:-}" "通用配置目录不存在: ${CONFDIR:-未定义}"
  require_dir "${RUNTIME_DIR:-}" "运行时目录不存在: ${RUNTIME_DIR:-未定义}"
  require_dir "${CATALOG_DIR:-}" "Catalog 目录不存在: ${CATALOG_DIR:-未定义}"

  # module.conf 本身采用 Shell 赋值格式；运行时一次加载可避免逐键创建子进程。
  ACTIVE_GROUP_ID="default"
  OUTBOUND_MODE="rule"
  SELECTOR_MODE="urltest"
  SELECTED_NODE_REF=""
  . "$MODULE_CONF"
  CUR_ACTIVE_GROUP_ID="${ACTIVE_GROUP_ID:-default}"
  CUR_OUTBOUND_MODE="${OUTBOUND_MODE:-rule}"
  CUR_SELECTOR_MODE="${SELECTOR_MODE:-urltest}"
  CUR_SELECTED_NODE_REF="${SELECTED_NODE_REF:-}"

  catalog_validate_group_id "$CUR_ACTIVE_GROUP_ID" \
    || die "ACTIVE_GROUP_ID 非法: $CUR_ACTIVE_GROUP_ID"
  case "$CUR_OUTBOUND_MODE" in
    rule | global | direct | AllowAds) ;;
    *) die "未知出站模式: $CUR_OUTBOUND_MODE" ;;
  esac
  case "$CUR_SELECTOR_MODE" in
    urltest | auto | manual | selector) ;;
    *) die "未知节点选择模式: $CUR_SELECTOR_MODE" ;;
  esac

  RUNTIME_PROVIDERS_FILE="$RUNTIME_DIR/providers.json"
  RUNTIME_OUTBOUNDS_FILE="$RUNTIME_DIR/outbounds.json"
  RUNTIME_EBPF_FILE="$RUNTIME_DIR/ebpf.json"
}

#######################################
# 清空 Catalog 投影结果
# 参数: 无
# 返回: 无
#######################################
reset_catalog_projection() {
  RUNTIME_GROUP_IDS=""
  RUNTIME_GROUP_RECORDS=""
  RUNTIME_GROUP_COUNT=0
  RUNTIME_NODE_COUNT=0
  RUNTIME_PROVIDER_ENTRIES=""
}

#######################################
# 将分组加入运行时 Provider 投影
# 参数:
#   $1  分组 ID
#   $2  运行时标签
#   $3  provider.json 路径
#   $4  节点数
# 返回: 无
#######################################
append_runtime_provider() {
  local group_id="$1"
  local runtime_tag="$2"
  local provider_file="$3"
  local node_count="$4"
  local entry escaped_runtime_tag

  escaped_runtime_tag="$(json_escape "$runtime_tag")"

  entry="    {
      \"type\": \"local\",
      \"tag\": \"$escaped_runtime_tag\",
      \"path\": \"$(json_escape "$provider_file")\",
      \"health_check\": {
        \"enabled\": true,
        \"url\": \"https://www.gstatic.com/generate_204\",
        \"interval\": \"10m\",
        \"timeout\": \"5s\"
      }
    }"

  if [ -n "$RUNTIME_PROVIDER_ENTRIES" ]; then
    RUNTIME_PROVIDER_ENTRIES="$RUNTIME_PROVIDER_ENTRIES,$NL$entry"
    RUNTIME_GROUP_IDS="$RUNTIME_GROUP_IDS$NL$group_id"
    RUNTIME_GROUP_RECORDS="$RUNTIME_GROUP_RECORDS$NL$group_id$TAB$runtime_tag"
  else
    RUNTIME_PROVIDER_ENTRIES="$entry"
    RUNTIME_GROUP_IDS="$group_id"
    RUNTIME_GROUP_RECORDS="$group_id$TAB$runtime_tag"
  fi
  RUNTIME_GROUP_COUNT=$((RUNTIME_GROUP_COUNT + 1))
  RUNTIME_NODE_COUNT=$((RUNTIME_NODE_COUNT + node_count))
}

#######################################
# 规范化活动分组与节点选择状态
# 参数: 无
# 全局: 读取 RUNTIME_GROUP_IDS，并按需更新 module.conf
# 返回: 无
#######################################
normalize_runtime_selection() {
  local fallback_group selected_group selected_tag provider_file

  # 活动分组已被删除或为空时，切换到第一个非空分组。
  if ! catalog_group_list_contains "$RUNTIME_GROUP_IDS" "$CUR_ACTIVE_GROUP_ID"; then
    fallback_group="$(printf "%s\n" "$RUNTIME_GROUP_IDS" | sed -n '1p')"
    [ -n "$fallback_group" ] || die "Catalog 中没有可用节点组"
    log "WARN" "活动分组不可用，切换到: $fallback_group"
    CUR_ACTIVE_GROUP_ID="$fallback_group"
    set_conf "$MODULE_CONF" "ACTIVE_GROUP_ID" "$(quote_conf "$CUR_ACTIVE_GROUP_ID")"
  fi

  case "$CUR_SELECTOR_MODE" in
    urltest | auto)
      CUR_SELECTOR_MODE="urltest"
      if [ -n "$CUR_SELECTED_NODE_REF" ]; then
        CUR_SELECTED_NODE_REF=""
        set_conf "$MODULE_CONF" "SELECTED_NODE_REF" '""'
      fi
      set_conf "$MODULE_CONF" "SELECTOR_MODE" "urltest"
      ;;
    manual | selector)
      CUR_SELECTOR_MODE="manual"
      selected_group="${CUR_SELECTED_NODE_REF%%/*}"
      selected_tag="${CUR_SELECTED_NODE_REF#*/}"
      provider_file="$(catalog_provider_path "$CUR_ACTIVE_GROUP_ID")" || provider_file=""

      if [ -z "$CUR_SELECTED_NODE_REF" ] \
        || [ "$selected_group" = "$CUR_SELECTED_NODE_REF" ] \
        || [ "$selected_group" != "$CUR_ACTIVE_GROUP_ID" ] \
        || [ -z "$selected_tag" ] \
        || ! catalog_provider_contains_tag "$provider_file" "$selected_tag"; then
        log "WARN" "手动节点不可用，回退到活动分组自动测速"
        CUR_SELECTOR_MODE="urltest"
        CUR_SELECTED_NODE_REF=""
        set_conf "$MODULE_CONF" "SELECTOR_MODE" "urltest"
        set_conf "$MODULE_CONF" "SELECTED_NODE_REF" '""'
      else
        set_conf "$MODULE_CONF" "SELECTOR_MODE" "manual"
      fi
      ;;
  esac

  CUR_ACTIVE_GROUP_TAG="$(catalog_runtime_group_tag "$CUR_ACTIVE_GROUP_ID")" \
    || die "无法读取活动分组名称: $CUR_ACTIVE_GROUP_ID"
}

#######################################
# 扫描 Catalog 中全部非空节点组
# 参数: 无
# 全局: 填充 RUNTIME_GROUP_*、RUNTIME_NODE_COUNT 与 Provider 片段
# 返回: 无可用节点或 Provider 非法时退出
#######################################
scan_catalog_groups() {
  local mode="${1:-strict}"
  local group_dir group_id provider_file meta_file node_count runtime_tag

  reset_catalog_projection
  for group_dir in "$CATALOG_DIR"/*; do
    [ -d "$group_dir" ] || continue
    group_id="${group_dir##*/}"
    [ "$group_id" != "staging" ] || continue
    if ! catalog_validate_group_id "$group_id"; then
      log "WARN" "跳过非法 Catalog 分组目录: $group_id"
      continue
    fi

    provider_file="$group_dir/provider.json"
    meta_file="$group_dir/meta.json"
    catalog_provider_has_nodes "$provider_file" || continue
    require_file "$meta_file" "Catalog 分组缺少 meta.json: $group_id"
    # 节点数由 Catalog 事务与 Provider 同步提交；启动只读取所需字段。
    node_count="$(meta_get_raw "$meta_file" "node_count" "0")" \
      || die "Catalog 分组元数据无效: $group_id"
    case "$node_count" in
      "" | *[!0-9]*) die "Catalog Provider 节点数非法: $provider_file" ;;
      0) continue ;;
    esac
    runtime_tag="$(catalog_runtime_group_tag "$group_id")" \
      || die "无法读取 Catalog 分组名称: $group_id"
    append_runtime_provider "$group_id" "$runtime_tag" "$provider_file" "$node_count"
  done

  if [ "$RUNTIME_GROUP_COUNT" -le 0 ]; then
    RUNTIME_BUILD_ERROR="Catalog 中没有可用节点，请先导入单节点、文件或订阅"
    [ "$mode" = "allow-empty" ] && return 2
    log "ERROR" "$RUNTIME_BUILD_ERROR"
    return 1
  fi
  RUNTIME_BUILD_ERROR=""
  normalize_runtime_selection
}

#######################################
# 生成 Local Provider 运行时配置
# 参数: 无
# 返回: 标准输出打印生成文件路径
#######################################
write_runtime_providers() {
  [ -n "$RUNTIME_PROVIDER_ENTRIES" ] || scan_catalog_groups || return 1

  cat > "$RUNTIME_PROVIDERS_FILE" << EOF
{
  "providers": [
$RUNTIME_PROVIDER_ENTRIES
  ]
}
EOF

  printf "%s\n" "$RUNTIME_PROVIDERS_FILE"
}

#######################################
# 生成分组选择器运行时配置
# 参数: 无
# 返回: 标准输出打印生成文件路径
#######################################
write_runtime_outbounds() {
  local group_id runtime_tag escaped_runtime_tag group_entries="" proxy_options=""
  local entry top_default

  [ "$RUNTIME_GROUP_COUNT" -gt 0 ] || scan_catalog_groups || return 1

  while IFS="$TAB" read -r group_id runtime_tag; do
    [ -n "$group_id" ] || continue
    escaped_runtime_tag="$(json_escape "$runtime_tag")"
    entry="    {
      \"type\": \"urltest\",
      \"tag\": \"Auto/$escaped_runtime_tag\",
      \"providers\": [\"$escaped_runtime_tag\"],
      \"url\": \"https://www.gstatic.com/generate_204\",
      \"interval\": \"3m\",
      \"tolerance\": 50,
      \"interrupt_exist_connections\": true
    },
    {
      \"type\": \"selector\",
      \"tag\": \"Select/$escaped_runtime_tag\",
      \"providers\": [\"$escaped_runtime_tag\"],
      \"interrupt_exist_connections\": true
    }"

    if [ -n "$group_entries" ]; then
      group_entries="$group_entries,$NL$entry"
      proxy_options="$proxy_options, \"Auto/$escaped_runtime_tag\", \"Select/$escaped_runtime_tag\""
    else
      group_entries="$entry"
      proxy_options="\"Auto/$escaped_runtime_tag\", \"Select/$escaped_runtime_tag\""
    fi
  done << EOF
$RUNTIME_GROUP_RECORDS
EOF

  if [ "$CUR_SELECTOR_MODE" = "manual" ]; then
    top_default="Select/$(json_escape "$CUR_ACTIVE_GROUP_TAG")"
  else
    top_default="Auto/$(json_escape "$CUR_ACTIVE_GROUP_TAG")"
  fi

  cat > "$RUNTIME_OUTBOUNDS_FILE" << EOF
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
$group_entries,
    {
      "type": "selector",
      "tag": "Proxy",
      "outbounds": [$proxy_options],
      "default": "$top_default",
      "interrupt_exist_connections": true
    }
  ]
}
EOF

  printf "%s\n" "$RUNTIME_OUTBOUNDS_FILE"
}
