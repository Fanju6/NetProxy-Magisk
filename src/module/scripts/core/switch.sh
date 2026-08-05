#!/system/bin/sh
#######################################
# 文件: switch.sh
# 功能: 持久化 Catalog 节点选择与路由模式，并通过 Service API 热切换。
# 用法:
#   switch.sh node auto [group-id]
#   switch.sh node <group-id>/<tag>
#   switch.sh mode <rule|global|direct|AllowAds>
# 依赖: common.sh、config.sh、api.sh 与 catalog.sh。
#######################################

set -u

readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly MODULE_CONF="$MODDIR/config/module.conf"
readonly CATALOG_DIR="$MODDIR/config/catalog"
readonly NETPROXY_NATIVE_BIN="$MODDIR/bin/netproxy-native"
readonly SERVICE_SCRIPT="$MODDIR/scripts/core/service.sh"
readonly SING_BOX_BIN="$MODDIR/bin/sing-box"
readonly LOG_FILE="$MODDIR/logs/service.log"
readonly LOG_TAG="switch"
readonly SWITCH_ALLOW_RESTART="${SWITCH_ALLOW_RESTART:-1}"

REQUIRED_PROVIDER_FILE=""
REQUIRED_PROVIDER_SUMMARY=""
REQUIRED_PROVIDER_TAG=""

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/config.sh"
. "$MODDIR/scripts/utils/api.sh"
. "$MODDIR/scripts/utils/catalog.sh"
. "$MODDIR/scripts/utils/metadata.sh"

#######################################
# 判断 sing-box 是否正在运行
# 参数: 无
# 返回: 运行中返回 0，否则返回 1
#######################################
is_service_running() {
  [ -n "$(get_pid "$SING_BOX_BIN")" ]
}

#######################################
# 重新加载运行时结构，失败时按策略回退完整重启
# 参数: 无
# 返回: 应用成功返回 0，否则返回 1
#######################################
reload_service_if_allowed() {
  log "INFO" "运行实例未加载目标结构，尝试原位重新加载配置"
  if su -c "LOG_STDERR=0 LOG_LEVEL=WARN sh \"$SERVICE_SCRIPT\" reload"; then
    return 0
  fi

  if [ "$SWITCH_ALLOW_RESTART" != "1" ]; then
    log "WARN" "原位重新加载失败，当前操作不允许完整重启"
    return 1
  fi
  log "WARN" "原位重新加载失败，回退完整重启 sing-box 服务"
  su -c "LOG_STDERR=0 LOG_LEVEL=WARN sh \"$SERVICE_SCRIPT\" restart" \
    || die "重启 sing-box 服务失败"
}

#######################################
# 校验非空 Catalog 分组
# 参数:
#   $1  分组 ID
# 全局: 设置 REQUIRED_PROVIDER_FILE、REQUIRED_PROVIDER_SUMMARY 与 REQUIRED_PROVIDER_TAG
# 返回: 成功返回 0，否则退出
#######################################
require_catalog_group() {
  local group_id="$1"

  catalog_validate_group_id "$group_id" || die "非法分组 ID: $group_id"
  REQUIRED_PROVIDER_FILE="$(catalog_provider_path "$group_id")" \
    || die "无法解析分组路径: $group_id"
  require_file "$REQUIRED_PROVIDER_FILE" "节点组不存在: $group_id"
  catalog_provider_has_nodes "$REQUIRED_PROVIDER_FILE" || die "节点组为空: $group_id"
  REQUIRED_PROVIDER_SUMMARY="$(catalog_provider_inspect "$REQUIRED_PROVIDER_FILE")" \
    || die "节点组 Provider 配置无效: $group_id"
  REQUIRED_PROVIDER_TAG="$(catalog_runtime_group_tag "$group_id")" \
    || die "无法读取节点组名称: $group_id"
}

#######################################
# 切换到分组自动测速
# 参数:
#   $1  分组 ID
# 返回: 成功返回 0，否则返回 1
#######################################
switch_auto_group() {
  local group_id="$1"

  require_catalog_group "$group_id"
  set_conf "$MODULE_CONF" "ACTIVE_GROUP_ID" "$(quote_conf "$group_id")"
  set_conf "$MODULE_CONF" "SELECTOR_MODE" "urltest"
  set_conf "$MODULE_CONF" "SELECTED_NODE_REF" '""'

  if ! is_service_running; then
    log "INFO" "已选择自动测速分组: $group_id，将在下次启动时生效"
    return 0
  fi

  if service_api_select "Proxy" "Auto/$REQUIRED_PROVIDER_TAG"; then
    log "INFO" "已切换到自动测速分组: $REQUIRED_PROVIDER_TAG"
    return 0
  fi

  reload_service_if_allowed
}

#######################################
# 切换到手动节点
# 参数:
#   $1  节点引用 (<group-id>/<tag>)
# 返回: 成功返回 0，否则返回 1
#######################################
switch_manual_node() {
  local node_ref="$1"
  local group_id tag escaped_tag runtime_node_ref

  group_id="${node_ref%%/*}"
  tag="${node_ref#*/}"
  [ "$group_id" != "$node_ref" ] && [ -n "$tag" ] \
    || die "节点引用格式应为 <group-id>/<tag>"
  case "$node_ref" in
    *\"* | *\\* | *\$* | *\`* | *"$NL"* | *"$CR"*)
      die "节点标签包含无法安全持久化的字符"
      ;;
  esac
  require_catalog_group "$group_id"
  escaped_tag="$(json_escape "$tag")"
  printf "%s" "$REQUIRED_PROVIDER_SUMMARY" | grep -F -q "\"tag\":\"$escaped_tag\"" \
    || die "节点不存在: $node_ref"
  runtime_node_ref="$REQUIRED_PROVIDER_TAG/$tag"

  set_conf "$MODULE_CONF" "ACTIVE_GROUP_ID" "$(quote_conf "$group_id")"
  set_conf "$MODULE_CONF" "SELECTOR_MODE" "manual"
  set_conf "$MODULE_CONF" "SELECTED_NODE_REF" "$(quote_conf "$node_ref")"

  if ! is_service_running; then
    log "INFO" "已选择手动节点: $node_ref，将在下次启动时生效"
    return 0
  fi

  # 先切换分组内部 selector，再让顶层 Proxy 指向该 selector。
  if service_api_select "Select/$REQUIRED_PROVIDER_TAG" "$runtime_node_ref" \
    && service_api_select "Proxy" "Select/$REQUIRED_PROVIDER_TAG"; then
    log "INFO" "已切换到手动节点: $node_ref"
    return 0
  fi

  reload_service_if_allowed
}

#######################################
# 切换节点选择状态
# 参数:
#   $1  auto 或节点引用
#   $2  auto 模式目标分组 (可选)
# 返回: 成功返回 0，否则返回 1
#######################################
switch_node() {
  local target="$1"
  local group_id="${2:-}"

  require_file "$MODULE_CONF" "模块配置文件不存在: $MODULE_CONF"
  if [ "$target" = "auto" ]; then
    [ -n "$group_id" ] || group_id="$(read_conf "$MODULE_CONF" "ACTIVE_GROUP_ID" "default")"
    switch_auto_group "$group_id"
  else
    switch_manual_node "$target"
  fi
}

#######################################
# 切换出站路由模式
# 参数:
#   $1  rule、global、direct 或 AllowAds
# 返回: 成功返回 0，否则返回 1
#######################################
switch_mode() {
  local target_mode="$1"

  case "$target_mode" in
    rule | global | direct | AllowAds) ;;
    *) die "未知出站模式: $target_mode" ;;
  esac
  require_file "$MODULE_CONF" "模块配置文件不存在: $MODULE_CONF"
  set_conf "$MODULE_CONF" "OUTBOUND_MODE" "$target_mode"

  if ! is_service_running; then
    log "INFO" "已保存出站模式: $target_mode，将在下次启动时生效"
    return 0
  fi

  if service_api_set_mode "$target_mode"; then
    log "INFO" "出站模式切换完成: $target_mode"
    return 0
  fi

  log "WARN" "Service API 切换模式失败，尝试重新加载配置"
  reload_service_if_allowed
}

#######################################
# 显示用法
# 参数: 无
# 返回: 无
#######################################
show_usage() {
  cat << EOF
用法:
  $(basename "$0") node auto [group-id]
  $(basename "$0") node <group-id>/<tag>
  $(basename "$0") mode <rule|global|direct|AllowAds>
EOF
}

#######################################
# 主入口
#######################################
main() {
  local command="${1:-}"

  case "$command" in
    node)
      [ -n "${2:-}" ] || { show_usage; exit 1; }
      switch_node "$2" "${3:-}"
      ;;
    mode)
      [ -n "${2:-}" ] || { show_usage; exit 1; }
      switch_mode "$2"
      ;;
    -h | --help | help)
      show_usage
      ;;
    *)
      show_usage
      exit 1
      ;;
  esac
}

main "$@"
