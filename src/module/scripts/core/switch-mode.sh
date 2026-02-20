#!/system/bin/sh
# 切换出站模式 (Xray API 热更新)
# 用法: switch-mode.sh <mode> [routing_json_file]
#   mode: rule | global | direct
#
# Xray API 说明:
# - adrules: 默认**替换**整个路由表
# - rmrules: 按 ruleTag 删除
# - ado/rmo: 添加/删除出站
#
# 逻辑:
# - 直连模式: 替换出站为 freedom + 替换路由
# - 全局模式: 替换路由 (出站不变)
# - 规则模式: 替换路由为 03_routing.json (出站不变)
# - 从直连切换: 先恢复 CURRENT_CONFIG 出站

set -u

readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly XRAY_BIN="$MODDIR/bin/xray"
readonly API_SERVER="127.0.0.1:8080"
readonly MODULE_CONF="$MODDIR/config/module.conf"
readonly DEFAULT_OUTBOUND="$MODDIR/config/xray/outbounds/default.json"
readonly ROUTING_JSON="$MODDIR/config/xray/confdir/03_routing.json"
readonly LOG_FILE="$MODDIR/logs/service.log"

# 导入工具库
. "$MODDIR/scripts/utils/log.sh"

#######################################
# 获取当前节点配置路径
#######################################
get_current_config() {
  grep '^CURRENT_CONFIG=' "$MODULE_CONF" 2> /dev/null | cut -d'=' -f2 | tr -d '"'
}

#######################################
# 获取当前出站模式
#######################################
get_current_mode() {
  local mode
  mode=$(grep '^OUTBOUND_MODE=' "$MODULE_CONF" 2> /dev/null | cut -d'=' -f2)
  echo "${mode:-rule}"
}

#######################################
# 替换路由规则 (adrules 默认是替换模式)
#######################################
replace_routing_rules() {
  local rules_file="$1"
  if [ -f "$rules_file" ]; then
    log "INFO" "替换路由规则: $rules_file"
    if "$XRAY_BIN" api adrules --server="$API_SERVER" "$rules_file" 2> /dev/null; then
      log "INFO" "路由规则替换成功"
      return 0
    else
      log "INFO" "路由规则替换失败"
      return 1
    fi
  else
    log "INFO" "规则文件不存在: $rules_file"
    return 1
  fi
}

#######################################
# 切换出站为 freedom (直连)
#######################################
switch_to_freedom() {
  log "INFO" "切换出站为 freedom..."
  # 删除现有 proxy 出站
  "$XRAY_BIN" api rmo --server="$API_SERVER" "proxy" 2> /dev/null || true
  # 添加 freedom 出站 (default.json)
  if "$XRAY_BIN" api ado --server="$API_SERVER" "$DEFAULT_OUTBOUND" 2> /dev/null; then
    log "INFO" "已切换到 freedom 出站"
    return 0
  else
    log "INFO" "切换 freedom 出站失败"
    return 1
  fi
}

#######################################
# 恢复节点出站配置
#######################################
restore_proxy_outbound() {
  local current_config
  current_config=$(get_current_config)

  if [ -z "$current_config" ] || [ ! -f "$current_config" ]; then
    log "INFO" "无法获取当前节点配置，使用默认配置"
    current_config="$DEFAULT_OUTBOUND"
  fi

  log "INFO" "恢复节点配置: $current_config"
  # 删除 freedom/proxy 出站
  "$XRAY_BIN" api rmo --server="$API_SERVER" "proxy" 2> /dev/null || true
  # 添加节点出站
  if "$XRAY_BIN" api ado --server="$API_SERVER" "$current_config" 2> /dev/null; then
    log "INFO" "节点出站已恢复"
    return 0
  else
    log "INFO" "节点出站恢复失败"
    return 1
  fi
}

#######################################
# 更新 module.conf 中的模式
#######################################
update_mode_config() {
  local mode="$1"
  if grep -q '^OUTBOUND_MODE=' "$MODULE_CONF"; then
    sed -i "s/^OUTBOUND_MODE=.*/OUTBOUND_MODE=$mode/" "$MODULE_CONF"
  else
    echo "OUTBOUND_MODE=$mode" >> "$MODULE_CONF"
  fi
  log "INFO" "已更新 module.conf: OUTBOUND_MODE=$mode"
}

#######################################
# 主函数
#######################################
main() {
  local target_mode="${1:-}"
  local routing_json="${2:-}"

  if [ -z "$target_mode" ]; then
    echo "用法: $0 <mode> [routing_json_file]"
    echo "  mode: rule | global | direct"
    exit 1
  fi

  local current_mode
  current_mode=$(get_current_mode)

  log "INFO" "========== 切换出站模式 =========="
  log "INFO" "当前模式: $current_mode -> 目标模式: $target_mode"

  # ===== 第一步: 处理出站配置 =====

  # 从直连模式切换出去: 需要恢复节点出站
  if [ "$current_mode" = "direct" ] && [ "$target_mode" != "direct" ]; then
    log "INFO" "从直连模式切换，恢复节点出站..."
    restore_proxy_outbound
  fi

  # 切换到直连模式: 需要替换为 freedom 出站
  if [ "$target_mode" = "direct" ] && [ "$current_mode" != "direct" ]; then
    log "INFO" "切换到直连模式，替换为 freedom 出站..."
    switch_to_freedom
  fi

  # ===== 第二步: 替换路由规则 =====
  # adrules 默认是替换模式，会替换整个路由表

  case "$target_mode" in
    rule)
      # 规则模式: 使用 03_routing.json
      log "INFO" "规则模式: 应用 03_routing.json"
      replace_routing_rules "$ROUTING_JSON"
      ;;
    global | direct)
      # 全局/直连模式: 使用前端生成的路由规则
      if [ -n "$routing_json" ] && [ -f "$routing_json" ]; then
        log "INFO" "${target_mode}模式: 应用自定义路由规则"
        replace_routing_rules "$routing_json"
      else
        log "INFO" "错误: 未提供路由规则文件"
        echo "error: no routing rules file"
        exit 1
      fi
      ;;
    *)
      log "INFO" "未知模式: $target_mode"
      echo "error: unknown mode"
      exit 1
      ;;
  esac

  # ===== 第三步: 更新配置 =====
  update_mode_config "$target_mode"

  log "INFO" "========== 模式切换完成 =========="
  echo "success"
}

main "$@"
