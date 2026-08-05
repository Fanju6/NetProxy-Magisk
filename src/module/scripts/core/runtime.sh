#!/system/bin/sh
#######################################
# 文件: runtime.sh
# 功能: 调用 NetProxy 原生组件，将 Catalog 一次性投影为 sing-box Provider
#       与分组选择器运行时配置，并规范化持久选择状态。
# 用法: 由 service.sh 通过 . "$MODDIR/scripts/core/runtime.sh" 引入。
# 依赖: common.sh、config.sh 与 netproxy-native。
#######################################

# 当前运行上下文
CUR_ACTIVE_GROUP_ID=""
CUR_OUTBOUND_MODE=""
CUR_SELECTOR_MODE=""
CUR_SELECTED_NODE_REF=""
CUR_ACTIVE_GROUP_TAG=""

# Catalog 投影结果
RUNTIME_GROUP_COUNT=0
RUNTIME_NODE_COUNT=0
RUNTIME_BUILD_ERROR=""

# 运行时配置输出路径
RUNTIME_PROVIDERS_FILE=""
RUNTIME_OUTBOUNDS_FILE=""
RUNTIME_EBPF_FILE=""
RUNTIME_CATALOG_STATE_FILE=""

#######################################
# 初始化运行时上下文
# 参数: 无
# 全局: 读取 module.conf 并填充 CUR_* 与运行时配置路径
# 返回: 成功返回 0，配置非法则退出
#######################################
initialize_runtime_context() {
  require_file "${MODULE_CONF:-}" "模块配置文件不存在: ${MODULE_CONF:-未定义}"
  require_file "${NETPROXY_NATIVE_BIN:-}" "NetProxy 原生组件不存在: ${NETPROXY_NATIVE_BIN:-未定义}"
  require_dir "${SINGBOX_DIR:-}" "sing-box 配置目录不存在: ${SINGBOX_DIR:-未定义}"
  require_dir "${CONFDIR:-}" "通用配置目录不存在: ${CONFDIR:-未定义}"
  require_dir "${RUNTIME_DIR:-}" "运行时目录不存在: ${RUNTIME_DIR:-未定义}"
  require_dir "${CATALOG_DIR:-}" "Catalog 目录不存在: ${CATALOG_DIR:-未定义}"

  ACTIVE_GROUP_ID="default"
  OUTBOUND_MODE="rule"
  SELECTOR_MODE="urltest"
  SELECTED_NODE_REF=""
  . "$MODULE_CONF"
  CUR_ACTIVE_GROUP_ID="${ACTIVE_GROUP_ID:-default}"
  CUR_OUTBOUND_MODE="${OUTBOUND_MODE:-rule}"
  CUR_SELECTOR_MODE="${SELECTOR_MODE:-urltest}"
  CUR_SELECTED_NODE_REF="${SELECTED_NODE_REF:-}"

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
  RUNTIME_CATALOG_STATE_FILE="$RUNTIME_DIR/catalog.state"
}

#######################################
# 读取原生组件生成的运行时状态
# 参数: 无
# 全局: 更新 CUR_*、RUNTIME_GROUP_COUNT 与 RUNTIME_NODE_COUNT
# 返回: 成功返回 0，状态非法返回 1
#######################################
load_runtime_catalog_state() {
  local key value

  [ -f "$RUNTIME_CATALOG_STATE_FILE" ] || return 1
  while IFS="$TAB" read -r key value; do
    case "$key" in
      active_group_id) CUR_ACTIVE_GROUP_ID="$value" ;;
      active_group_tag) CUR_ACTIVE_GROUP_TAG="$value" ;;
      selector_mode) CUR_SELECTOR_MODE="$value" ;;
      selected_node_ref) CUR_SELECTED_NODE_REF="$value" ;;
      group_count) RUNTIME_GROUP_COUNT="$value" ;;
      node_count) RUNTIME_NODE_COUNT="$value" ;;
    esac
  done < "$RUNTIME_CATALOG_STATE_FILE"

  case "$RUNTIME_GROUP_COUNT:$RUNTIME_NODE_COUNT" in
    *[!0-9:]* | :* | *:) return 1 ;;
  esac
  return 0
}

#######################################
# 一次性生成 Catalog 运行时投影
# 参数:
#   $1  strict 或 allow-empty
# 全局: 写入 providers.json、outbounds.json 与运行时状态
# 返回: 成功返回 0，失败返回 1
#######################################
build_runtime_catalog() {
  local mode="${1:-strict}"
  local original_group="$CUR_ACTIVE_GROUP_ID"
  local original_selector="$CUR_SELECTOR_MODE"
  local original_selected="$CUR_SELECTED_NODE_REF"
  local output=""

  rm -f "$RUNTIME_CATALOG_STATE_FILE" 2> /dev/null || true
  if [ "$mode" = "allow-empty" ]; then
    output="$("$NETPROXY_NATIVE_BIN" catalog runtime \
      --root "$CATALOG_DIR" \
      --providers-output "$RUNTIME_PROVIDERS_FILE" \
      --outbounds-output "$RUNTIME_OUTBOUNDS_FILE" \
      --state-output "$RUNTIME_CATALOG_STATE_FILE" \
      --active "$CUR_ACTIVE_GROUP_ID" \
      --selector "$CUR_SELECTOR_MODE" \
      --selected "$CUR_SELECTED_NODE_REF" \
      --allow-empty 2>&1)" || {
        RUNTIME_BUILD_ERROR="$(printf '%s' "$output" | sed -n 's/.*"message":"\([^"]*\)".*/\1/p')"
        [ -n "$RUNTIME_BUILD_ERROR" ] || RUNTIME_BUILD_ERROR="Catalog 运行时配置生成失败"
        return 1
      }
  else
    output="$("$NETPROXY_NATIVE_BIN" catalog runtime \
      --root "$CATALOG_DIR" \
      --providers-output "$RUNTIME_PROVIDERS_FILE" \
      --outbounds-output "$RUNTIME_OUTBOUNDS_FILE" \
      --state-output "$RUNTIME_CATALOG_STATE_FILE" \
      --active "$CUR_ACTIVE_GROUP_ID" \
      --selector "$CUR_SELECTOR_MODE" \
      --selected "$CUR_SELECTED_NODE_REF" 2>&1)" || {
        RUNTIME_BUILD_ERROR="$(printf '%s' "$output" | sed -n 's/.*"message":"\([^"]*\)".*/\1/p')"
        [ -n "$RUNTIME_BUILD_ERROR" ] || RUNTIME_BUILD_ERROR="Catalog 运行时配置生成失败"
        log "ERROR" "$RUNTIME_BUILD_ERROR"
        return 1
      }
  fi

  load_runtime_catalog_state || {
    RUNTIME_BUILD_ERROR="Catalog 运行时状态无效"
    return 1
  }
  RUNTIME_BUILD_ERROR=""

  # 配置检查不应修改用户选择；正式启动才持久化原生组件的规范化结果。
  if [ "$mode" != "allow-empty" ] \
    && { [ "$CUR_ACTIVE_GROUP_ID" != "$original_group" ] \
      || [ "$CUR_SELECTOR_MODE" != "$original_selector" ] \
      || [ "$CUR_SELECTED_NODE_REF" != "$original_selected" ]; }; then
    set_conf_values "$MODULE_CONF" \
      "ACTIVE_GROUP_ID" "$(quote_conf "$CUR_ACTIVE_GROUP_ID")" \
      "SELECTOR_MODE" "$CUR_SELECTOR_MODE" \
      "SELECTED_NODE_REF" "$(quote_conf "$CUR_SELECTED_NODE_REF")"
  fi
}

#######################################
# 扫描并投影 Catalog
# 参数:
#   $1  strict 或 allow-empty
# 返回: 成功返回 0，失败返回 1
#######################################
scan_catalog_groups() {
  build_runtime_catalog "${1:-strict}"
}

#######################################
# 返回已生成的 Local Provider 配置
# 参数: 无
# 返回: 文件存在返回 0，否则返回 1
#######################################
write_runtime_providers() {
  [ -f "$RUNTIME_PROVIDERS_FILE" ] || build_runtime_catalog || return 1
  printf '%s\n' "$RUNTIME_PROVIDERS_FILE"
}

#######################################
# 返回已生成的分组选择器配置
# 参数: 无
# 返回: 文件存在返回 0，否则返回 1
#######################################
write_runtime_outbounds() {
  [ -f "$RUNTIME_OUTBOUNDS_FILE" ] || build_runtime_catalog || return 1
  printf '%s\n' "$RUNTIME_OUTBOUNDS_FILE"
}
