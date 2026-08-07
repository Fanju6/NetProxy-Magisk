#!/system/bin/sh
#######################################
# 文件: subscription.sh
# 功能: Catalog 节点编排与订阅入口，负责本地节点变更、运行时 reload、
#       Go 订阅事务调用与取消控制。
# 用法: 由 netproxyctl 与 subworker.sh 引入；也可执行 update/update-all/cancel。
# 依赖: common.sh、config.sh、catalog.sh、metadata.sh 与 netproxy-native。
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
. "$MODDIR/scripts/utils/metadata.sh"

SUB_LOCK_DIR=""
SUB_STAGE_DIR=""

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

  if [ ! -f "$CATALOG_DIR/default/provider.json" ]; then
    printf '{\n  "outbounds": []\n}\n' > "$CATALOG_DIR/default/provider.json" || return 1
    chmod 0600 "$CATALOG_DIR/default/provider.json" 2> /dev/null || true
  fi
  if [ ! -f "$CATALOG_DIR/default/meta.json" ]; then
    initialize_local_meta "default" "本地配置" "local"
    write_catalog_meta "$CATALOG_DIR/default/meta.json" || return 1
  fi
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

#######################################
# 生成随机稳定的订阅分组 UUID
# 参数: 无
# 返回: 标准输出打印 UUID
#######################################
new_subscription_id() {
  local value attempt=0

  while [ "$attempt" -lt 8 ]; do
    if [ -r /proc/sys/kernel/random/uuid ]; then
      value="$(sed -n '1p' /proc/sys/kernel/random/uuid 2> /dev/null)"
    else
      value="$(printf '%08x-%04x-%04x-%04x-%012x' \
        "$(date +%s)" "$$" "${RANDOM:-0}" "$attempt" "$(date +%s)" 2> /dev/null)"
    fi
    if catalog_validate_group_id "$value" && [ ! -e "$CATALOG_DIR/$value" ]; then
      printf "%s\n" "$value"
      return 0
    fi
    attempt=$((attempt + 1))
  done
  return 1
}

#######################################
# 将文件名转换为本地分组 ID
# 参数:
#   $1  文件路径
# 返回: 标准输出打印不冲突的分组 ID
#######################################
local_group_id_from_file() {
  local name base candidate suffix=2

  name="${1##*/}"
  base="${name%.*}"
  base="$(printf "%s" "$base" | tr '[:upper:]' '[:lower:]' \
    | sed 's/[^a-z0-9._-]/-/g; s/--*/-/g; s/^[.-]*//; s/[.-]*$//')"
  [ -n "$base" ] || base="$(date +%s)"
  candidate="local-$base"
  while [ -e "$CATALOG_DIR/$candidate" ]; do
    candidate="local-$base-$suffix"
    suffix=$((suffix + 1))
  done
  printf "%s\n" "$candidate"
}

#######################################
# 按 ID 或唯一名称解析分组
# 参数:
#   $1  分组 ID 或显示名称
# 返回: 标准输出打印分组 ID；多重匹配返回 2
#######################################
resolve_catalog_group() {
  local query="$1"
  local group_dir group_id name match="" count=0

  if catalog_validate_group_id "$query" && [ -d "$CATALOG_DIR/$query" ]; then
    printf "%s\n" "$query"
    return 0
  fi

  for group_dir in "$CATALOG_DIR"/*; do
    [ -d "$group_dir" ] || continue
    group_id="${group_dir##*/}"
    [ "$group_id" != "staging" ] || continue
    name="$(meta_get_string "$group_dir/meta.json" "name" "")"
    [ "$name" = "$query" ] || continue
    match="$group_id"
    count=$((count + 1))
  done
  [ "$count" -eq 1 ] || { [ "$count" -gt 1 ] && return 2; return 1; }
  printf "%s\n" "$match"
}

#######################################
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
  [ -z "$SUB_STAGE_DIR" ] || rm -rf "$SUB_STAGE_DIR" 2> /dev/null || true
  [ -z "$SUB_LOCK_DIR" ] || rm -rf "$SUB_LOCK_DIR" 2> /dev/null || true
  SUB_STAGE_DIR=""
  SUB_LOCK_DIR=""
}

#######################################
# 创建当前事务 staging 目录
# 参数:
#   $1  分组 ID
# 返回: 标准输出打印目录路径
#######################################
create_catalog_stage() {
  local group_id="$1"

  SUB_STAGE_DIR="$CATALOG_STAGING_DIR/$group_id.$$.$(date +%s)"
  mkdir -p "$SUB_STAGE_DIR" || return 1
  [ -z "$SUB_LOCK_DIR" ] || printf '%s\n' "$SUB_STAGE_DIR" > "$SUB_LOCK_DIR/stage"
  chmod 0700 "$SUB_STAGE_DIR" 2> /dev/null || true
  printf "%s\n" "$SUB_STAGE_DIR"
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
    catalog_provider_has_nodes "$CATALOG_DIR/$target/provider.json" || return 1
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
  local current current_provider

  current="$(read_conf "$MODULE_CONF" "ACTIVE_GROUP_ID" "")"
  current_provider="$(catalog_provider_path "$current" 2> /dev/null || true)"
  if [ -n "$current" ] && catalog_provider_has_nodes "$current_provider"; then
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
  local selector selected selected_group selected_tag provider_file runtime_tag

  selector="$(read_conf "$MODULE_CONF" "SELECTOR_MODE" "urltest")"
  selected="$(read_conf "$MODULE_CONF" "SELECTED_NODE_REF" "")"
  [ "$selector" = "manual" ] && [ -n "$selected" ] || return 0
  selected_group="${selected%%/*}"
  selected_tag="${selected#*/}"
  [ "$selected_group" = "$group_id" ] || return 0
  provider_file="$CATALOG_DIR/$group_id/provider.json"
  catalog_provider_contains_tag "$provider_file" "$selected_tag" && return 0

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
# 判断当前订阅任务是否收到取消请求
# 参数:
#   $1  分组 ID
# 返回: 已取消返回 0，否则返回 1
#######################################
subscription_cancel_requested() {
  [ -f "$SUB_RUNTIME_DIR/$1.cancel" ]
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
# 根据订阅配置决定下载代理地址
# 参数: 无
# 全局: 读取 SUB_UPDATE_VIA_PROXY
# 返回: 标准输出打印代理 URL；直连时为空
#######################################
subscription_proxy_url() {
  case "$SUB_UPDATE_VIA_PROXY" in
    always) printf "%s" "http://127.0.0.1:7080" ;;
    never) printf "%s" "" ;;
    auto)
      if [ -n "$(get_pid "$SING_BOX_BIN")" ] && service_api_is_ready; then
        printf "%s" "http://127.0.0.1:7080"
      fi
      ;;
    *) printf "%s" "" ;;
  esac
}

#######################################
# 通过 Go 组件执行订阅更新事务
# 参数:
#   $1  分组 ID 或唯一名称
# 返回: 更新成功或 304 返回 0，失败返回 1
#######################################
update_subscription() {
  local query="$1"
  local group_id group_dir meta_file proxy_url result error_file node_count

  initialize_catalog_storage || return 1
  group_id="$(resolve_catalog_group "$query")" || return $?
  group_dir="$CATALOG_DIR/$group_id"
  meta_file="$group_dir/meta.json"
  load_catalog_meta "$meta_file" || return 1
  [ "$SUB_TYPE" = "subscription" ] || return 1
  [ -n "$SUB_URL" ] || return 1

  rm -f "$SUB_RUNTIME_DIR/$group_id.cancel"
  proxy_url="$(subscription_proxy_url)"
  error_file="$SUB_RUNTIME_DIR/$group_id.native.log"
  set -- "$NETPROXY_NATIVE_BIN" subscription update \
    --root "$CATALOG_DIR" \
    --group "$group_id" \
    --progress-dir "$SUB_RUNTIME_DIR"
  [ -z "$proxy_url" ] || set -- "$@" --proxy "$proxy_url"
  if [ "$SUB_UPDATE_VIA_PROXY" = "auto" ] && [ -n "$proxy_url" ]; then
    set -- "$@" --fallback-direct
  fi

  result="$("$@" 2> "$error_file")" || {
    log "ERROR" "订阅更新失败: $group_id"
    rm -f "$SUB_RUNTIME_DIR/$group_id.progress.json" \
      "$SUB_RUNTIME_DIR/$group_id.child.pid"
    rm -f "$error_file"
    return 1
  }
  rm -f "$error_file"

  activate_group_if_needed "$group_id" || true
  fallback_missing_selected_node "$group_id"
  if printf "%s" "$result" | grep -q '"structure_changed"[[:space:]]*:[[:space:]]*true'; then
    reload_catalog_structure_if_running
  fi
  node_count="$(meta_get_string "$meta_file" "node_count" "0")"
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
  local group_id group_dir

  validate_user_text "$name" && validate_user_text "$url" || return 1
  # name 允许留空：首次更新取得响应头后由 resolve_subscription_display_name 回填
  [ -n "$url" ] || return 1
  initialize_catalog_storage || return 1
  group_id="$(new_subscription_id)" || return 1
  group_dir="$CATALOG_DIR/$group_id"
  mkdir -p "$group_dir" || return 1
  chmod 0700 "$group_dir" 2> /dev/null || true
  initialize_subscription_meta "$group_id" "$name" "$url"
  [ -z "${SUB_ADD_USER_AGENT:-}" ] || SUB_USER_AGENT="$SUB_ADD_USER_AGENT"
  [ -z "${SUB_ADD_HWID:-}" ] || SUB_HWID="$SUB_ADD_HWID"
  [ -z "${SUB_ADD_CUSTOM_HEADERS:-}" ] || SUB_CUSTOM_HEADERS="$SUB_ADD_CUSTOM_HEADERS"
  [ -z "${SUB_ADD_UPDATE_INTERVAL:-}" ] || {
    SUB_UPDATE_INTERVAL="$SUB_ADD_UPDATE_INTERVAL"
    SUB_INTERVAL_SOURCE="user"
    schedule_next_update
  }
  [ -z "${SUB_ADD_UPDATE_VIA_PROXY:-}" ] || SUB_UPDATE_VIA_PROXY="$SUB_ADD_UPDATE_VIA_PROXY"
  [ -z "${SUB_ADD_INCLUDE:-}" ] || SUB_INCLUDE="$SUB_ADD_INCLUDE"
  [ -z "${SUB_ADD_EXCLUDE:-}" ] || SUB_EXCLUDE="$SUB_ADD_EXCLUDE"
  [ -z "${SUB_ADD_ALLOW_INSECURE:-}" ] || SUB_ALLOW_INSECURE="$SUB_ADD_ALLOW_INSECURE"
  [ -z "${SUB_ADD_TIMEOUT:-}" ] || SUB_TIMEOUT="$SUB_ADD_TIMEOUT"
  [ -z "${SUB_ADD_AUTO_UPDATE:-}" ] || SUB_AUTO_UPDATE="$SUB_ADD_AUTO_UPDATE"
  write_catalog_meta "$group_dir/meta.json" || { rm -rf "$group_dir"; return 1; }
  printf '{\n  "outbounds": []\n}\n' > "$group_dir/provider.json"
  chmod 0600 "$group_dir/provider.json" 2> /dev/null || true

  if ! update_subscription "$group_id"; then
    # 首次更新失败时仍需保证有可读名称，否则列表出现无名订阅
    if load_catalog_meta "$group_dir/meta.json" && [ -z "${SUB_NAME:-}" ]; then
      SUB_NAME="$(resolve_subscription_display_name "$url")"
      write_catalog_meta "$group_dir/meta.json" || true
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
  local group_id group_dir stage node_count now

  [ -f "$input" ] || return 1
  validate_user_text "$display_name" || return 1
  initialize_catalog_storage || return 1
  group_id="$(local_group_id_from_file "$input")" || return 1
  acquire_catalog_lock "$group_id" || return 1
  create_catalog_stage "$group_id" > /dev/null || { release_catalog_lock; return 1; }
  stage="$SUB_STAGE_DIR"
  if ! "$NETPROXY_NATIVE_BIN" convert file --input "$input" --output "$stage/provider.json" \
    > "$stage/result.json" 2> "$stage/error.json"; then
    release_catalog_lock
    return 1
  fi
  node_count="$(sed -n 's/.*"node_count":\([0-9][0-9]*\).*/\1/p' "$stage/result.json")"
  [ "$node_count" -gt 0 ] 2> /dev/null || { release_catalog_lock; return 1; }

  group_dir="$CATALOG_DIR/$group_id"
  mkdir -p "$group_dir" || { release_catalog_lock; return 1; }
  initialize_local_meta "$group_id" "$display_name" "file"
  SUB_NODE_COUNT="$node_count"
  SUB_REVISION=1
  now="$(date +%s)"
  SUB_UPDATED_AT="$(format_epoch_utc "$now")"
  write_catalog_meta "$stage/meta.json" || { release_catalog_lock; return 1; }
  mv -f "$stage/provider.json" "$group_dir/provider.json" || { release_catalog_lock; return 1; }
  mv -f "$stage/meta.json" "$group_dir/meta.json" || { release_catalog_lock; return 1; }
  chmod 0700 "$group_dir" 2> /dev/null || true
  chmod 0600 "$group_dir"/*.json 2> /dev/null || true
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
  local group_dir provider_file stage node_count now had_nodes=0

  [ "$group_id" = "default" ] || {
    [ "$(meta_get_string "$CATALOG_DIR/$group_id/meta.json" "type" "")" != "subscription" ] || return 1
  }
  group_dir="$CATALOG_DIR/$group_id"
  provider_file="$group_dir/provider.json"
  [ -f "$provider_file" ] || return 1
  catalog_provider_has_nodes "$provider_file" && had_nodes=1
  acquire_catalog_lock "$group_id" || return 1
  create_catalog_stage "$group_id" > /dev/null || { release_catalog_lock; return 1; }
  stage="$SUB_STAGE_DIR"
  cp "$provider_file" "$stage/provider.json" || { release_catalog_lock; return 1; }
  if ! "$NETPROXY_NATIVE_BIN" provider append --target "$stage/provider.json" --input "$input" \
    > "$stage/result.json" 2> "$stage/error.json"; then
    release_catalog_lock
    return 1
  fi
  node_count="$(grep -o '"protocol"' "$stage/result.json" | wc -l | tr -d '[:space:]')"
  load_catalog_meta "$group_dir/meta.json" || initialize_local_meta "$group_id" "$group_id" "local"
  SUB_NODE_COUNT="$node_count"
  SUB_REVISION=$((SUB_REVISION + 1))
  now="$(date +%s)"
  SUB_UPDATED_AT="$(format_epoch_utc "$now")"
  write_catalog_meta "$stage/meta.json" || { release_catalog_lock; return 1; }
  mv -f "$stage/provider.json" "$provider_file" || { release_catalog_lock; return 1; }
  mv -f "$stage/meta.json" "$group_dir/meta.json" || { release_catalog_lock; return 1; }
  activate_group_if_needed "$group_id" || true
  release_catalog_lock
  [ "$had_nodes" = "1" ] || reload_catalog_structure_if_running
}

#######################################
# 从 Catalog 分组删除节点
# 参数:
#   $1  节点引用 (<group-id>/<tag>)
# 返回: 成功返回 0
#######################################
remove_catalog_node() {
  local node_ref="$1"
  local group_id tag group_dir provider_file stage node_count now

  group_id="${node_ref%%/*}"
  tag="${node_ref#*/}"
  [ "$group_id" != "$node_ref" ] && [ -n "$tag" ] || return 1
  group_dir="$CATALOG_DIR/$group_id"
  provider_file="$group_dir/provider.json"
  catalog_provider_contains_tag "$provider_file" "$tag" || return 1
  acquire_catalog_lock "$group_id" || return 1
  create_catalog_stage "$group_id" > /dev/null || { release_catalog_lock; return 1; }
  stage="$SUB_STAGE_DIR"
  cp "$provider_file" "$stage/provider.json" || { release_catalog_lock; return 1; }
  if ! "$NETPROXY_NATIVE_BIN" provider remove --target "$stage/provider.json" --tag "$tag" \
    > "$stage/result.json" 2> "$stage/error.json"; then
    release_catalog_lock
    return 1
  fi
  node_count="$(grep -o '"protocol"' "$stage/result.json" | wc -l | tr -d '[:space:]')"
  node_count="${node_count:-0}"
  load_catalog_meta "$group_dir/meta.json" || { release_catalog_lock; return 1; }
  SUB_NODE_COUNT="$node_count"
  SUB_REVISION=$((SUB_REVISION + 1))
  now="$(date +%s)"
  SUB_UPDATED_AT="$(format_epoch_utc "$now")"
  write_catalog_meta "$stage/meta.json" || { release_catalog_lock; return 1; }
  mv -f "$stage/provider.json" "$provider_file" || { release_catalog_lock; return 1; }
  mv -f "$stage/meta.json" "$group_dir/meta.json" || { release_catalog_lock; return 1; }

  release_catalog_lock
  fallback_missing_selected_node "$group_id"
  [ "$node_count" -gt 0 ] 2> /dev/null || reload_catalog_structure_if_running
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
  local group_id tag group_dir provider_file stage node_count now

  group_id="${node_ref%%/*}"
  tag="${node_ref#*/}"
  [ "$group_id" != "$node_ref" ] && [ -n "$tag" ] || return 1
  group_dir="$CATALOG_DIR/$group_id"
  provider_file="$group_dir/provider.json"
  catalog_provider_contains_tag "$provider_file" "$tag" || return 1
  acquire_catalog_lock "$group_id" || return 1
  create_catalog_stage "$group_id" > /dev/null || { release_catalog_lock; return 1; }
  stage="$SUB_STAGE_DIR"
  cp "$provider_file" "$stage/provider.json" || { release_catalog_lock; return 1; }
  "$NETPROXY_NATIVE_BIN" provider remove --target "$stage/provider.json" --tag "$tag" \
    > "$stage/remove-result.json" 2> "$stage/error.json" \
    || { release_catalog_lock; return 1; }
  "$NETPROXY_NATIVE_BIN" provider append --target "$stage/provider.json" --input "$input" \
    > "$stage/append-result.json" 2> "$stage/error.json" \
    || { release_catalog_lock; return 1; }
  node_count="$(grep -o '"protocol"' "$stage/append-result.json" | wc -l | tr -d '[:space:]')"
  load_catalog_meta "$group_dir/meta.json" || { release_catalog_lock; return 1; }
  SUB_NODE_COUNT="$node_count"
  SUB_REVISION=$((SUB_REVISION + 1))
  now="$(date +%s)"
  SUB_UPDATED_AT="$(format_epoch_utc "$now")"
  write_catalog_meta "$stage/meta.json" || { release_catalog_lock; return 1; }
  mv -f "$stage/provider.json" "$provider_file" || { release_catalog_lock; return 1; }
  mv -f "$stage/meta.json" "$group_dir/meta.json" || { release_catalog_lock; return 1; }

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
  local group_id group_dir current candidate candidate_dir

  group_id="$(resolve_catalog_group "$query")" || return $?
  group_dir="$CATALOG_DIR/$group_id"
  [ "$(meta_get_string "$group_dir/meta.json" "type" "")" = "subscription" ] || return 1
  current="$(read_conf "$MODULE_CONF" "ACTIVE_GROUP_ID" "")"

  if [ "$current" = "$group_id" ]; then
    if [ -n "$replacement" ]; then
      replacement="$(resolve_catalog_group "$replacement")" || return 1
      [ "$replacement" != "$group_id" ] || return 1
      catalog_provider_has_nodes "$CATALOG_DIR/$replacement/provider.json" || return 1
    else
      for candidate_dir in "$CATALOG_DIR"/*; do
        [ -d "$candidate_dir" ] || continue
        candidate="${candidate_dir##*/}"
        [ "$candidate" != "staging" ] && [ "$candidate" != "$group_id" ] || continue
        if catalog_provider_has_nodes "$candidate_dir/provider.json"; then
          replacement="$candidate"
          break
        fi
      done
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
  rm -rf "$group_dir" || { release_catalog_lock; return 1; }
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
