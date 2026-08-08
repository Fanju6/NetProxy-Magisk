#!/system/bin/sh
#######################################
# 文件: subscription.sh
# 功能: Catalog 节点编排与订阅入口，负责本地节点变更、运行时 reload、
#       Go 订阅事务调用与取消控制。
# 用法: 由 netproxyctl 与 subworker.sh 引入；也可执行 update/update-all/cancel。
# 依赖: common.sh、config.sh、catalog.sh 与 netproxy-native。
#######################################

if [ -z "${MODDIR:-}" ]; then
  MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
fi
[ -n "${MODULE_CONF:-}" ] || MODULE_CONF="$MODDIR/config/module.conf"
[ -n "${CATALOG_DIR:-}" ] || CATALOG_DIR="$MODDIR/config/catalog"
[ -n "${CATALOG_STAGING_DIR:-}" ] || CATALOG_STAGING_DIR="$CATALOG_DIR/staging"
[ -n "${NETPROXY_NATIVE_BIN:-}" ] || NETPROXY_NATIVE_BIN="$MODDIR/bin/netproxy-native"
[ -n "${SERVICE_SCRIPT:-}" ] || SERVICE_SCRIPT="$MODDIR/scripts/core/service.sh"
[ -n "${SWITCH_SCRIPT:-}" ] || SWITCH_SCRIPT="$MODDIR/scripts/core/switch.sh"
[ -n "${SING_BOX_BIN:-}" ] || SING_BOX_BIN="$MODDIR/bin/sing-box"
[ -n "${SUB_RUNTIME_DIR:-}" ] || SUB_RUNTIME_DIR="/dev/netproxy/subscriptions"
[ -n "${LOG_FILE:-}" ] || LOG_FILE="$MODDIR/logs/subscription.log"
[ -n "${LOG_TAG:-}" ] || LOG_TAG="subscription"

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/config.sh"
. "$MODDIR/scripts/utils/api.sh"
. "$MODDIR/scripts/utils/catalog.sh"

SUB_LOCK_DIR=""

#######################################
# 初始化 Catalog 事务目录
# 参数: 无
# 返回: 成功返回 0，否则返回 1
#######################################
initialize_catalog_storage() {
  mkdir -p "$CATALOG_DIR/default" "$CATALOG_STAGING_DIR/locks" "$SUB_RUNTIME_DIR" \
    "$MODDIR/logs" || return 1
  chmod 0700 "$CATALOG_DIR" "$CATALOG_STAGING_DIR" "$CATALOG_STAGING_DIR/locks" \
    "$SUB_RUNTIME_DIR" 2> /dev/null || true
  cleanup_stale_catalog_transactions

  acquire_catalog_lock "default" || return 1
  if ! "$NETPROXY_NATIVE_BIN" catalog group-ensure --root "$CATALOG_DIR" \
    --group default --name "本地配置" --type local > /dev/null 2> /dev/null; then
    release_catalog_lock
    return 1
  fi
  release_catalog_lock
}

#######################################
# 检查用户文本不含换行或控制分隔符
# 参数:
#   $1  文本
# 返回: 合法返回 0，否则返回 1
#######################################
validate_user_text() {
  local value="$1"

  case "$value" in
    *"$NL"* | *"$CR"* | *"$TAB"*) return 1 ;;
    *) return 0 ;;
  esac
}

#######################################
# 从订阅 URL 提取主机名，作为订阅名兜底
# 参数:
#   $1  订阅 URL
# 返回: 标准输出打印主机名；无法提取时无输出
#######################################
subscription_host_from_url() {
  local url="$1"
  local rest

  rest="${url#*://}"      # 去协议
  rest="${rest%%/*}"      # 去路径
  rest="${rest%%\?*}"     # 去查询串
  rest="${rest##*@}"      # 去 user:pass@
  rest="${rest%%:*}"      # 去端口
  case "$rest" in
    "" | *[!A-Za-z0-9.-]*) return 0 ;;
  esac
  printf "%s" "$rest"
}

#######################################
# 订阅名留空时按优先级自动取名
# 优先级: Profile-Title > Content-Disposition 文件名 > URL 主机名 > 默认名
# 参数:
#   $1  订阅 URL
# 返回: 标准输出打印订阅名
#######################################
resolve_subscription_display_name() {
  local url="$1"
  local candidate

  for candidate in "${SUB_PROFILE_TITLE:-}" "${SUB_FILE_NAME:-}" \
    "$(subscription_host_from_url "$url")"; do
    candidate="$(printf "%s" "$candidate" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
    [ -n "$candidate" ] || continue
    validate_user_text "$candidate" || continue
    printf "%s" "$candidate"
    return 0
  done
  printf "%s" "订阅"
}

# 清理异常退出遗留的 Catalog 事务
# 参数: 无
# 返回: 无
#######################################
cleanup_stale_catalog_transactions() {
  local lock_dir group_id stage

  for lock_dir in "$CATALOG_STAGING_DIR/locks"/*.lock; do
    [ -d "$lock_dir" ] || continue
    lock_owner_alive "$lock_dir" && continue
    stage="$(sed -n '1p' "$lock_dir/stage" 2> /dev/null || true)"
    case "$stage" in "$CATALOG_STAGING_DIR"/*) rm -rf "$stage" 2> /dev/null || true ;; esac
    group_id="${lock_dir##*/}"
    group_id="${group_id%.lock}"
    rm -f "$SUB_RUNTIME_DIR/$group_id.progress.json" "$SUB_RUNTIME_DIR/$group_id.cancel" 2> /dev/null || true
    rm -rf "$lock_dir" 2> /dev/null || true
  done
}

#######################################
# 获取分组事务锁
# 参数:
#   $1  分组 ID
# 返回: 成功返回 0，已有有效任务返回 1
#######################################
acquire_catalog_lock() {
  local group_id="$1"
  local lock_dir

  catalog_validate_group_id "$group_id" || return 1
  lock_dir="$CATALOG_STAGING_DIR/locks/$group_id.lock"
  if ! mkdir "$lock_dir" 2> /dev/null; then
    # 已有存活任务持锁则放弃，否则清理残锁后接管
    lock_owner_alive "$lock_dir" && return 1
    rm -rf "$lock_dir" 2> /dev/null || return 1
    mkdir "$lock_dir" 2> /dev/null || return 1
  fi
  lock_write_owner "$lock_dir"
  chmod 0700 "$lock_dir" 2> /dev/null || true
  SUB_LOCK_DIR="$lock_dir"
}

#######################################
# 释放当前分组事务锁与 staging
# 参数: 无
# 返回: 无
#######################################
release_catalog_lock() {
  [ -z "$SUB_LOCK_DIR" ] || rm -rf "$SUB_LOCK_DIR" 2> /dev/null || true
  SUB_LOCK_DIR=""
}

#######################################
# 清理订阅任务运行时进度
# 参数:
#   $1  分组 ID
# 返回: 无
#######################################
clear_subscription_progress() {
  rm -f "$SUB_RUNTIME_DIR/$1.progress.json"
}

#######################################
# 将指定分组设为活动组
# 参数:
#   $1  分组 ID；空值表示清空活动状态
# 返回: 成功返回 0，否则返回 1
#######################################
set_active_catalog_group() {
  local target="$1"

  if [ -n "$target" ]; then
    catalog_validate_group_id "$target" || return 1
    catalog_group_has_nodes "$target" || return 1
  fi

  set_conf_values "$MODULE_CONF" \
    "ACTIVE_GROUP_ID" "$(quote_conf "$target")" \
    "SELECTOR_MODE" "urltest" \
    "SELECTED_NODE_REF" '""'
}

#######################################
# 在当前没有有效活动组时启用指定非空分组
# 参数:
#   $1  分组 ID
# 返回: 本次发生启用返回 0，否则返回 1
#######################################
activate_group_if_needed() {
  local candidate="$1"
  local current

  current="$(read_conf "$MODULE_CONF" "ACTIVE_GROUP_ID" "")"
  if [ -n "$current" ] && catalog_group_has_nodes "$current"; then
    return 1
  fi
  set_active_catalog_group "$candidate" || return 1
}

#######################################
# 在运行中重新投影 Catalog 分组结构
# 参数: 无
# 返回: 始终返回 0；重新加载失败仅记录警告，不回滚已提交的 Catalog
#######################################
reload_catalog_structure_if_running() {
  [ -n "$(get_pid "$SING_BOX_BIN")" ] || return 0
  log "INFO" "Catalog 分组结构已变化，重新加载 sing-box 配置"
  if ! sh "$SERVICE_SCRIPT" reload > /dev/null 2>&1; then
    log "WARN" "Catalog 已保存，但运行时结构重新加载失败"
  fi
  return 0
}

#######################################
# 在后台重新投影 Catalog 分组结构
# 删除分组已经完成后无需阻塞调用方等待核心 reload；子进程忽略 HUP，
# 即使 Android/WebUI 关闭命令通道也会继续完成运行时同步。
# 参数: 无
# 返回: 始终返回 0
#######################################
reload_catalog_structure_async_if_running() {
  [ -n "$(get_pid "$SING_BOX_BIN")" ] || return 0
  (
    trap '' HUP
    sh "$SERVICE_SCRIPT" reload > /dev/null 2>&1
  ) < /dev/null > /dev/null 2>&1 &
  return 0
}

#######################################
# 手动节点消失时回退当前分组 Auto
# 参数:
#   $1  发生变更的分组 ID
# 返回: 始终返回 0；运行时切换失败仅记录警告
#######################################
fallback_missing_selected_node() {
  local group_id="$1"
  local selector selected selected_group selected_tag runtime_tag

  selector="$(read_conf "$MODULE_CONF" "SELECTOR_MODE" "urltest")"
  selected="$(read_conf "$MODULE_CONF" "SELECTED_NODE_REF" "")"
  [ "$selector" = "manual" ] && [ -n "$selected" ] || return 0
  selected_group="${selected%%/*}"
  selected_tag="${selected#*/}"
  [ "$selected_group" = "$group_id" ] || return 0
  catalog_group_contains_tag "$group_id" "$selected_tag" && return 0

  runtime_tag="$(catalog_runtime_group_tag "$group_id" 2> /dev/null || printf "%s" "$group_id")"
  log "WARN" "手动节点已从 Provider 移除，回退到 Auto/$runtime_tag"
  set_conf_values "$MODULE_CONF" \
    "SELECTOR_MODE" "urltest" \
    "SELECTED_NODE_REF" '""'
  [ -n "$(get_pid "$SING_BOX_BIN")" ] || return 0
  if ! sh "$SWITCH_SCRIPT" node auto "$group_id" > /dev/null 2>&1; then
    log "WARN" "选择状态已回退到 Auto/$runtime_tag，但运行实例同步失败"
  fi
  return 0
}

#######################################
# 取消正在进行的订阅更新
# 参数:
#   $1  分组 ID
# 返回: 成功标记返回 0
#######################################
cancel_subscription_update() {
  local group_id="$1"
  local pid_file="$SUB_RUNTIME_DIR/$group_id.child.pid"
  local pid

  catalog_validate_group_id "$group_id" || return 1
  mkdir -p "$SUB_RUNTIME_DIR" 2> /dev/null || return 1
  : > "$SUB_RUNTIME_DIR/$group_id.cancel"
  pid="$(sed -n '1p' "$pid_file" 2> /dev/null || true)"
  [ -z "$pid" ] || kill "$pid" 2> /dev/null || true
  clear_subscription_progress "$group_id"
}

#######################################
# 通过 Go 组件执行订阅更新事务
# 参数:
#   $1  分组 ID 或唯一名称
# 返回: 更新成功或 304 返回 0，失败返回 1
#######################################
update_subscription() {
  local query="$1"
  local group_id result error_file key value node_count=0 structure_changed=false

  initialize_catalog_storage || return 1
  group_id="$(catalog_resolve_group "$query")" || return $?

  rm -f "$SUB_RUNTIME_DIR/$group_id.cancel"
  error_file="$SUB_RUNTIME_DIR/$group_id.native.log"
  set -- "$NETPROXY_NATIVE_BIN" subscription update \
    --root "$CATALOG_DIR" \
    --group "$group_id" \
    --progress-dir "$SUB_RUNTIME_DIR" \
    --format kv

  result="$("$@" 2> "$error_file")" || {
    log "ERROR" "订阅更新失败: $group_id"
    rm -f "$SUB_RUNTIME_DIR/$group_id.progress.json" \
      "$SUB_RUNTIME_DIR/$group_id.child.pid"
    rm -f "$error_file"
    return 1
  }
  rm -f "$error_file"

  while IFS="=" read -r key value; do
    case "$key" in
      node_count) node_count="$value" ;;
      structure_changed) structure_changed="$value" ;;
    esac
  done << EOF
$result
EOF

  activate_group_if_needed "$group_id" || true
  fallback_missing_selected_node "$group_id"
  if [ "$structure_changed" = "true" ]; then
    reload_catalog_structure_if_running
  fi
  log "INFO" "订阅更新完成: $group_id，节点数: $node_count"
  return 0
}

#######################################
# 顺序更新全部 URL 订阅
# 参数: 无
# 返回: 全部成功返回 0，任一失败返回 1
#######################################
update_all_subscriptions() {
  local group_ids group_id failed=0

  initialize_catalog_storage || return 1
  group_ids="$("$NETPROXY_NATIVE_BIN" catalog ids \
    --root "$CATALOG_DIR" --type subscription --format raw)" || return 1
  while IFS= read -r group_id; do
    [ -n "$group_id" ] || continue
    update_subscription "$group_id" || failed=1
  done << EOF
$group_ids
EOF
  return "$failed"
}

#######################################
# 添加 URL 订阅并立即验证
# 参数:
#   $1  名称
#   $2  URL
# 返回: 标准输出打印新分组 ID
#######################################
add_subscription() {
  local name="$1"
  local url="$2"
  local group_id group_dir headers_file resolved_name

  validate_user_text "$name" && validate_user_text "$url" || return 1
  # name 允许留空：首次更新取得响应头后由 resolve_subscription_display_name 回填
  [ -n "$url" ] || return 1
  initialize_catalog_storage || return 1
  group_id="$(catalog_new_group_id subscription)" || return 1
  group_dir="$CATALOG_DIR/$group_id"
  headers_file="$CATALOG_STAGING_DIR/headers.$$.json"
  printf "%s\n" "${SUB_ADD_CUSTOM_HEADERS:-{}}" > "$headers_file" || return 1
  chmod 0600 "$headers_file" 2> /dev/null || true
  acquire_catalog_lock "$group_id" || { rm -f "$headers_file"; return 1; }
  set -- "$NETPROXY_NATIVE_BIN" catalog group-init \
    --root "$CATALOG_DIR" --group "$group_id" --type subscription \
    --name "$name" --url "$url" --user-agent "${SUB_ADD_USER_AGENT:-}" \
    --hwid "${SUB_ADD_HWID:-}" --headers-file "$headers_file" \
    --auto-update "${SUB_ADD_AUTO_UPDATE:-true}" \
    --update-interval "${SUB_ADD_UPDATE_INTERVAL:-86400}" \
    --interval-source user --update-via-proxy "${SUB_ADD_UPDATE_VIA_PROXY:-auto}" \
    --include "${SUB_ADD_INCLUDE:-}" --exclude "${SUB_ADD_EXCLUDE:-}" \
    --timeout "${SUB_ADD_TIMEOUT:-60}"
  [ "${SUB_ADD_ALLOW_INSECURE:-false}" = "true" ] && set -- "$@" --allow-insecure
  if ! "$@" > /dev/null 2> /dev/null; then
    rm -f "$headers_file"
    release_catalog_lock
    return 1
  fi
  rm -f "$headers_file"
  release_catalog_lock

  if ! update_subscription "$group_id"; then
    # 首次更新失败时仍需保证有可读名称，否则列表出现无名订阅
    resolved_name="$(resolve_subscription_display_name "$url")"
    if [ -n "$resolved_name" ] && acquire_catalog_lock "$group_id"; then
      "$NETPROXY_NATIVE_BIN" catalog group-name \
        --root "$CATALOG_DIR" --group "$group_id" --name "$resolved_name" \
        --now "$(date +%s)" > /dev/null 2>&1 || true
      release_catalog_lock
    fi
    printf "%s\n" "$group_id"
    return 1
  fi
  printf "%s\n" "$group_id"
}

#######################################
# 从本地文件创建独立本地分组
# 参数:
#   $1  输入文件
#   $2  显示名称 (可选)
# 返回: 标准输出打印新分组 ID
#######################################
import_local_file_group() {
  local input="$1"
  local display_name="${2:-${input##*/}}"
  local group_id

  [ -f "$input" ] || return 1
  validate_user_text "$display_name" || return 1
  initialize_catalog_storage || return 1
  group_id="$(catalog_new_group_id file "$input")" || return 1
  acquire_catalog_lock "$group_id" || return 1
  if ! "$NETPROXY_NATIVE_BIN" catalog group-import --root "$CATALOG_DIR" --group "$group_id" \
    --name "$display_name" --input "$input" > /dev/null 2> /dev/null; then
    release_catalog_lock
    return 1
  fi
  activate_group_if_needed "$group_id" || true
  release_catalog_lock
  reload_catalog_structure_if_running
  printf "%s\n" "$group_id"
}

#######################################
# 向本地分组追加节点链接或文件
# 参数:
#   $1  分组 ID
#   $2  节点链接或文件
# 返回: 成功返回 0
#######################################
append_local_node() {
  local group_id="$1"
  local input="$2"
  local group_dir mutation

  [ "$group_id" = "default" ] || {
    [ "$(catalog_group_type "$group_id")" != "subscription" ] || return 1
  }
  group_dir="$CATALOG_DIR/$group_id"
  [ -d "$group_dir" ] || return 1
  acquire_catalog_lock "$group_id" || return 1
  if ! mutation="$("$NETPROXY_NATIVE_BIN" catalog node-append --group-dir "$group_dir" --group "$group_id" \
    --input "$input" --format kv 2> /dev/null)"; then
    release_catalog_lock
    return 1
  fi
  activate_group_if_needed "$group_id" || true
  release_catalog_lock
  case "$mutation" in
    *structure_changed=true*) reload_catalog_structure_if_running ;;
  esac
}

#######################################
# 从 Catalog 分组删除节点
# 参数:
#   $1  节点引用 (<group-id>/<tag>)
# 返回: 成功返回 0
#######################################
remove_catalog_node() {
  local node_ref="$1"
  local group_id tag group_dir mutation

  group_id="${node_ref%%/*}"
  tag="${node_ref#*/}"
  [ "$group_id" != "$node_ref" ] && [ -n "$tag" ] || return 1
  group_dir="$CATALOG_DIR/$group_id"
  [ -d "$group_dir" ] || return 1
  acquire_catalog_lock "$group_id" || return 1
  if ! mutation="$("$NETPROXY_NATIVE_BIN" catalog node-remove --group-dir "$group_dir" --group "$group_id" \
    --tag "$tag" --format kv 2> /dev/null)"; then
    release_catalog_lock
    return 1
  fi
  release_catalog_lock
  fallback_missing_selected_node "$group_id"
  case "$mutation" in
    *structure_changed=true*) reload_catalog_structure_if_running ;;
  esac
}

#######################################
# 原子替换 Catalog 分组中的一个节点
# 参数:
#   $1  旧节点引用 (<group-id>/<tag>)
#   $2  新节点链接或文件
# 返回: 成功返回 0
#######################################
edit_catalog_node() {
  local node_ref="$1"
  local input="$2"
  local group_id tag group_dir

  group_id="${node_ref%%/*}"
  tag="${node_ref#*/}"
  [ "$group_id" != "$node_ref" ] && [ -n "$tag" ] || return 1
  group_dir="$CATALOG_DIR/$group_id"
  [ -d "$group_dir" ] || return 1
  acquire_catalog_lock "$group_id" || return 1
  if ! "$NETPROXY_NATIVE_BIN" catalog node-edit --group-dir "$group_dir" --group "$group_id" \
    --tag "$tag" --input "$input" > /dev/null 2> /dev/null; then
    release_catalog_lock
    return 1
  fi
  release_catalog_lock
  fallback_missing_selected_node "$group_id"
}

#######################################
# 删除订阅分组
# 参数:
#   $1  分组 ID 或唯一名称
#   $2  替代活动组 ID (可选)
# 返回: 成功返回 0
#######################################
remove_subscription() {
  local query="$1"
  local replacement="${2:-}"
  local group_id current

  group_id="$(catalog_resolve_group "$query")" || return $?
  [ "$(catalog_group_type "$group_id")" = "subscription" ] || return 1
  current="$(read_conf "$MODULE_CONF" "ACTIVE_GROUP_ID" "")"

  if [ "$current" = "$group_id" ]; then
    if [ -n "$replacement" ]; then
      replacement="$(catalog_resolve_group "$replacement")" || return 1
      [ "$replacement" != "$group_id" ] || return 1
      catalog_group_has_nodes "$replacement" || return 1
    else
      replacement="$(catalog_first_nonempty_group "$group_id")"
    fi
    set_active_catalog_group "$replacement" || return 1
    if [ -z "$replacement" ]; then
      if [ -n "$(get_pid "$SING_BOX_BIN")" ]; then
        sh "$SERVICE_SCRIPT" stop > /dev/null 2>&1 || true
      fi
    fi
  fi

  cancel_subscription_update "$group_id" 2> /dev/null || true
  acquire_catalog_lock "$group_id" || return 1
  if ! "$NETPROXY_NATIVE_BIN" catalog group-delete --root "$CATALOG_DIR" --group "$group_id" > /dev/null 2>&1; then
    release_catalog_lock
    return 1
  fi
  release_catalog_lock
  reload_catalog_structure_async_if_running
}

#######################################
# 显示低层脚本用法
# 参数: 无
# 返回: 无
#######################################
show_subscription_usage() {
  cat << EOF
用法:
  $(basename "$0") update <订阅 ID|名称>
  $(basename "$0") update-all
  $(basename "$0") cancel <订阅 ID>
EOF
}

#######################################
# 低层脚本入口
#######################################
subscription_main() {
  case "${1:-}" in
    update) [ -n "${2:-}" ] && update_subscription "$2" ;;
    update-all) update_all_subscriptions ;;
    cancel) [ -n "${2:-}" ] && cancel_subscription_update "$2" ;;
    help | -h | --help) show_subscription_usage ;;
    *) show_subscription_usage; return 1 ;;
  esac
}

if [ "${SUBSCRIPTION_LIBRARY_ONLY:-0}" != "1" ]; then
  subscription_main "$@"
fi
