#!/system/bin/sh
#######################################
# 文件: subscription.sh
# 功能: Catalog 节点与订阅事务层，负责分组锁、staging、Provider 原子替换、
#       HTTP 元数据、更新历史与取消控制。
# 用法: 由 netproxyctl 与 subworker.sh 引入；也可执行 update/update-all/cancel。
# 依赖: common.sh、config.sh、catalog.sh、metadata.sh 与 NetProxy 原生组件。
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

  if [ ! -f "$CATALOG_DIR/default/provider.json" ]; then
    printf '{\n  "outbounds": []\n}\n' > "$CATALOG_DIR/default/provider.json" || return 1
    chmod 0600 "$CATALOG_DIR/default/provider.json" 2> /dev/null || true
  fi
  if [ ! -f "$CATALOG_DIR/default/meta.json" ]; then
    initialize_local_meta "default" "本地配置" "local"
    SUB_ACTIVE=true
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
# 获取分组事务锁
# 参数:
#   $1  分组 ID
# 返回: 成功返回 0，已有有效任务返回 1
#######################################
acquire_catalog_lock() {
  local group_id="$1"
  local lock_dir pid

  catalog_validate_group_id "$group_id" || return 1
  lock_dir="$CATALOG_STAGING_DIR/locks/$group_id.lock"
  if ! mkdir "$lock_dir" 2> /dev/null; then
    pid="$(sed -n '1p' "$lock_dir/pid" 2> /dev/null || true)"
    if [ -n "$pid" ] && kill -0 "$pid" 2> /dev/null; then
      return 1
    fi
    rm -rf "$lock_dir" 2> /dev/null || return 1
    mkdir "$lock_dir" 2> /dev/null || return 1
  fi
  printf '%s\n' "$$" > "$lock_dir/pid"
  printf '%s\n' "$(date +%s)" > "$lock_dir/created_at"
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
  chmod 0700 "$SUB_STAGE_DIR" 2> /dev/null || true
  printf "%s\n" "$SUB_STAGE_DIR"
}

#######################################
# 写入订阅任务进度
# 参数:
#   $1  分组 ID
#   $2  阶段 (download/convert/validate/apply)
#   $3  中文说明
# 返回: 无
#######################################
write_subscription_progress() {
  local group_id="$1"
  local stage="$2"
  local message="$3"
  local target="$SUB_RUNTIME_DIR/$group_id.progress.json"
  local tmp="$target.tmp.$$"

  mkdir -p "$SUB_RUNTIME_DIR" 2> /dev/null || return 0
  cat > "$tmp" << EOF
{"schema":1,"group_id":"$(json_escape "$group_id")","stage":"$(json_escape "$stage")","message":"$(json_escape "$message")","updated_at":"$(format_epoch_utc "$(date +%s)")"}
EOF
  chmod 0600 "$tmp" 2> /dev/null || true
  mv -f "$tmp" "$target" 2> /dev/null || true
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
# 读取 JSON 文件并压缩为单行
# 参数:
#   $1  文件路径
#   $2  默认 JSON
# 返回: 标准输出打印 JSON
#######################################
read_compact_json_file() {
  local file="$1"
  local default="${2:-null}"
  local value

  [ -f "$file" ] || { printf "%s" "$default"; return 1; }
  value="$(tr -d '\r\n' < "$file" 2> /dev/null)"
  [ -n "$value" ] || value="$default"
  printf "%s" "$value"
}

#######################################
# 追加一条脱敏更新历史并只保留最近 20 条
# 参数:
#   $1  分组目录
#   $2  单行 JSON 记录
# 返回: 无
#######################################
append_subscription_history() {
  local group_dir="$1"
  local record="$2"
  local history="$group_dir/history.jsonl"
  local tmp="$history.tmp.$$"

  if [ -f "$history" ]; then
    tail -n 19 "$history" > "$tmp" 2> /dev/null || : > "$tmp"
  else
    : > "$tmp"
  fi
  printf '%s\n' "$record" >> "$tmp"
  chmod 0600 "$tmp" 2> /dev/null || true
  mv -f "$tmp" "$history"
}

#######################################
# 从原生组件 HTTP 元数据更新当前 SUB_* 变量
# 参数:
#   $1  metadata.json 路径
#   $2  modified 或 not_modified
# 返回: 无
#######################################
apply_http_metadata() {
  local file="$1"
  local response_kind="$2"
  local raw value

  [ -f "$file" ] || return 0
  raw="$(compact_json_get_raw "$file" "status_code" "0")" || raw=0
  case "$raw" in "" | *[!0-9]*) raw=0 ;; esac
  SUB_LAST_STATUS_CODE="$raw"

  value="$(compact_json_get_string "$file" "etag" "")" || true
  [ -z "$value" ] || SUB_ETAG="$value"
  value="$(compact_json_get_string "$file" "last_modified" "")" || true
  [ -z "$value" ] || SUB_LAST_MODIFIED="$value"
  value="$(compact_json_get_string "$file" "profile_title" "")" || true
  [ -z "$value" ] || SUB_PROFILE_TITLE="$value"
  value="$(compact_json_get_string "$file" "profile_web_page_url" "")" || true
  [ -z "$value" ] || SUB_PROFILE_WEB_PAGE_URL="$value"
  value="$(compact_json_get_string "$file" "content_disposition" "")" || true
  [ -z "$value" ] || SUB_CONTENT_DISPOSITION="$value"
  value="$(compact_json_get_string "$file" "file_name" "")" || true
  [ -z "$value" ] || SUB_FILE_NAME="$value"

  raw="$(compact_json_get_raw "$file" "usage" "__missing__")" || true
  if [ "$raw" != "__missing__" ]; then
    SUB_USAGE="$raw"
  elif [ "$response_kind" = "modified" ]; then
    SUB_USAGE=null
  fi

  raw="$(compact_json_get_raw "$file" "update_interval_seconds" "")" || true
  case "$raw" in
    "" | *[!0-9]*) ;;
    *)
      case "$SUB_INTERVAL_SOURCE" in
        default | profile)
          if [ "$raw" -ge 900 ]; then
            SUB_UPDATE_INTERVAL="$raw"
            SUB_INTERVAL_SOURCE="profile"
          fi
          ;;
      esac
      ;;
  esac

  raw="$(compact_json_get_raw "$file" "diagnostics" "[]")" || raw="[]"
  SUB_LAST_DIAGNOSTICS="$raw"
}

#######################################
# 将指定分组设为活动组并同步各组 active 字段
# 参数:
#   $1  分组 ID；空值表示清空活动状态
# 返回: 成功返回 0，否则返回 1
#######################################
set_active_catalog_group() {
  local target="$1"
  local group_dir group_id meta_file

  if [ -n "$target" ]; then
    catalog_validate_group_id "$target" || return 1
    catalog_provider_has_nodes "$CATALOG_DIR/$target/provider.json" || return 1
  fi

  set_conf "$MODULE_CONF" "ACTIVE_GROUP_ID" "$(quote_conf "$target")"
  if [ -z "$target" ]; then
    set_conf "$MODULE_CONF" "SELECTOR_MODE" "urltest"
    set_conf "$MODULE_CONF" "SELECTED_NODE_REF" '""'
  fi

  for group_dir in "$CATALOG_DIR"/*; do
    [ -d "$group_dir" ] || continue
    group_id="${group_dir##*/}"
    [ "$group_id" != "staging" ] || continue
    meta_file="$group_dir/meta.json"
    load_catalog_meta "$meta_file" || continue
    if [ "$group_id" = "$target" ]; then
      SUB_ACTIVE=true
    else
      SUB_ACTIVE=false
    fi
    write_catalog_meta "$meta_file" || return 1
  done
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
  set_conf "$MODULE_CONF" "SELECTOR_MODE" "urltest"
  set_conf "$MODULE_CONF" "SELECTED_NODE_REF" '""'
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
  set_conf "$MODULE_CONF" "SELECTOR_MODE" "urltest"
  set_conf "$MODULE_CONF" "SELECTED_NODE_REF" '""'
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
# 运行一次原生组件订阅转换
# 参数:
#   $1  输出 Provider
#   $2  HTTP 元数据文件
#   $3  diagnostics 文件
#   $4  stdout 结果文件
#   $5  stderr 错误文件
# 返回: 原生组件退出码
#######################################
run_subscription_conversion() {
  local output="$1"
  local metadata_file="$2"
  local diagnostics_file="$3"
  local result_file="$4"
  local error_file="$5"
  local headers_file="$SUB_STAGE_DIR/headers.json"
  local proxy_url child_pid status

  printf '%s\n' "$SUB_CUSTOM_HEADERS" > "$headers_file"
  chmod 0600 "$headers_file" 2> /dev/null || true
  proxy_url="$(subscription_proxy_url)"

  set -- "$NETPROXY_NATIVE_BIN" convert subscription \
    --url "$SUB_URL" \
    --output "$output" \
    --metadata-output "$metadata_file" \
    --diagnostics-output "$diagnostics_file" \
    --headers-file "$headers_file" \
    --timeout "${SUB_TIMEOUT}s"
  [ -z "$SUB_USER_AGENT" ] || set -- "$@" --user-agent "$SUB_USER_AGENT"
  [ -z "$SUB_HWID" ] || set -- "$@" --hwid "$SUB_HWID"
  [ -z "$SUB_ETAG" ] || set -- "$@" --etag "$SUB_ETAG"
  [ -z "$SUB_LAST_MODIFIED" ] || set -- "$@" --last-modified "$SUB_LAST_MODIFIED"
  [ -z "$SUB_INCLUDE" ] || set -- "$@" --include "$SUB_INCLUDE"
  [ -z "$SUB_EXCLUDE" ] || set -- "$@" --exclude "$SUB_EXCLUDE"
  [ "$SUB_ALLOW_INSECURE" != "true" ] || set -- "$@" --allow-insecure
  [ -z "$proxy_url" ] || set -- "$@" --proxy "$proxy_url"

  "$@" > "$result_file" 2> "$error_file" &
  child_pid=$!
  printf '%s\n' "$child_pid" > "$SUB_RUNTIME_DIR/$SUB_ID.child.pid"
  wait "$child_pid"
  status=$?
  rm -f "$SUB_RUNTIME_DIR/$SUB_ID.child.pid"
  return "$status"
}

#######################################
# 记录订阅更新失败并保留上一版 Provider
# 参数:
#   $1  分组目录
#   $2  错误代码
#   $3  安全错误说明
#   $4  开始 epoch 秒
# 返回: 始终返回 1
#######################################
record_subscription_failure() {
  local group_dir="$1"
  local code="$2"
  local message="$3"
  local started_at="$4"
  local now now_text duration

  now="$(date +%s)"
  now_text="$(format_epoch_utc "$now")"
  duration=$((now - started_at))
  SUB_LAST_ATTEMPT_AT="$now_text"
  SUB_UPDATED_AT="$now_text"
  SUB_LAST_ERROR="$message"
  schedule_next_update "$now"
  write_catalog_meta "$group_dir/meta.json" || true
  append_subscription_history "$group_dir" \
    "{\"at\":\"$now_text\",\"ok\":false,\"code\":\"$(json_escape "$code")\",\"message\":\"$(json_escape "$message")\",\"duration_seconds\":$duration}"
  clear_subscription_progress "$SUB_ID"
  log "ERROR" "订阅更新失败: $SUB_ID ($code)"
  release_catalog_lock
  return 1
}

#######################################
# 更新指定 URL 订阅
# 参数:
#   $1  分组 ID 或唯一名称
# 返回: 更新成功或 304 返回 0，失败返回 1
#######################################
update_subscription() {
  local query="$1"
  local group_id group_dir meta_file provider_file
  local metadata_file diagnostics_file result_file error_file
  local started_at now now_text response_kind result_code node_count diagnostics duration
  local had_nodes=0 has_nodes=0

  initialize_catalog_storage || return 1
  group_id="$(resolve_catalog_group "$query")" || return $?
  group_dir="$CATALOG_DIR/$group_id"
  meta_file="$group_dir/meta.json"
  provider_file="$group_dir/provider.json"
  load_catalog_meta "$meta_file" || return 1
  [ "$SUB_TYPE" = "subscription" ] || return 1
  [ -n "$SUB_URL" ] || return 1
  catalog_provider_has_nodes "$provider_file" && had_nodes=1
  acquire_catalog_lock "$group_id" || { log "WARN" "订阅已有更新任务: $group_id"; return 1; }
  create_catalog_stage "$group_id" > /dev/null || { release_catalog_lock; return 1; }

  started_at="$(date +%s)"
  rm -f "$SUB_RUNTIME_DIR/$group_id.cancel"
  metadata_file="$SUB_STAGE_DIR/http-metadata.json"
  diagnostics_file="$SUB_STAGE_DIR/diagnostics.json"
  result_file="$SUB_STAGE_DIR/result.json"
  error_file="$SUB_STAGE_DIR/error.json"

  write_subscription_progress "$group_id" "download" "正在下载订阅"
  if ! run_subscription_conversion "$SUB_STAGE_DIR/provider.json" "$metadata_file" \
    "$diagnostics_file" "$result_file" "$error_file"; then
    if subscription_cancel_requested "$group_id"; then
      rm -f "$SUB_RUNTIME_DIR/$group_id.cancel"
      record_subscription_failure "$group_dir" "subscription.cancelled" "订阅更新已取消" "$started_at"
      return 1
    fi
    apply_http_metadata "$metadata_file" "modified"
    record_subscription_failure "$group_dir" "subscription.convert_failed" \
      "订阅下载、转换或校验失败" "$started_at"
    return 1
  fi

  result_code="$(compact_json_get_string "$result_file" "code" "")" || true
  if [ "$result_code" = "subscription.not_modified" ]; then
    response_kind="not_modified"
  else
    response_kind="modified"
  fi
  apply_http_metadata "$metadata_file" "$response_kind"
  diagnostics="$(read_compact_json_file "$diagnostics_file" "[]")" || diagnostics="[]"
  SUB_LAST_DIAGNOSTICS="$diagnostics"

  if subscription_cancel_requested "$group_id"; then
    rm -f "$SUB_RUNTIME_DIR/$group_id.cancel"
    record_subscription_failure "$group_dir" "subscription.cancelled" "订阅更新已取消" "$started_at"
    return 1
  fi

  if [ "$response_kind" = "modified" ]; then
    write_subscription_progress "$group_id" "validate" "正在校验节点配置"
    # 原生组件转换成功时已完成 Provider 校验和原子写入，直接复用转换结果。
    node_count="$(sed -n 's/.*"node_count":\([0-9][0-9]*\).*/\1/p' "$result_file")"
    case "$node_count" in
      "" | *[!0-9]* | 0)
        record_subscription_failure "$group_dir" "provider.empty" "订阅中没有可用节点" "$started_at"
        return 1
        ;;
    esac
    SUB_NODE_COUNT="$node_count"
    SUB_REVISION=$((SUB_REVISION + 1))
  fi

  # 从这里进入不可取消的提交阶段。
  write_subscription_progress "$group_id" "apply" "正在应用订阅更新"
  now="$(date +%s)"
  now_text="$(format_epoch_utc "$now")"
  duration=$((now - started_at))
  SUB_LAST_ATTEMPT_AT="$now_text"
  SUB_LAST_SUCCESS_AT="$now_text"
  SUB_UPDATED_AT="$now_text"
  SUB_LAST_ERROR=""
  schedule_next_update "$now"
  write_catalog_meta "$SUB_STAGE_DIR/meta.json" \
    || { record_subscription_failure "$group_dir" "metadata.write_failed" "订阅元数据写入失败" "$started_at"; return 1; }

  if [ "$response_kind" = "modified" ]; then
    chmod 0600 "$SUB_STAGE_DIR/provider.json" 2> /dev/null || true
    cp "$provider_file" "$SUB_STAGE_DIR/provider.previous.json" 2> /dev/null || true
    mv -f "$SUB_STAGE_DIR/provider.json" "$provider_file" \
      || { record_subscription_failure "$group_dir" "provider.commit_failed" "订阅 Provider 提交失败" "$started_at"; return 1; }
  fi
  if ! mv -f "$SUB_STAGE_DIR/meta.json" "$meta_file"; then
    if [ "$response_kind" = "modified" ] && [ -f "$SUB_STAGE_DIR/provider.previous.json" ]; then
      mv -f "$SUB_STAGE_DIR/provider.previous.json" "$provider_file" 2> /dev/null || true
    fi
    record_subscription_failure "$group_dir" "metadata.commit_failed" "订阅元数据提交失败" "$started_at"
    return 1
  fi

  append_subscription_history "$group_dir" \
    "{\"at\":\"$now_text\",\"ok\":true,\"code\":\"$result_code\",\"node_count\":$SUB_NODE_COUNT,\"revision\":$SUB_REVISION,\"duration_seconds\":$duration,\"diagnostics\":$SUB_LAST_DIAGNOSTICS}"
  activate_group_if_needed "$group_id" || true
  rm -f "$SUB_RUNTIME_DIR/$group_id.cancel"
  clear_subscription_progress "$group_id"
  log "INFO" "订阅更新完成: $group_id，节点: $SUB_NODE_COUNT"
  release_catalog_lock
  if [ "$response_kind" = "modified" ]; then
    fallback_missing_selected_node "$group_id"
    catalog_provider_has_nodes "$provider_file" && has_nodes=1
    if [ "$had_nodes" != "$has_nodes" ]; then
      reload_catalog_structure_if_running
    fi
  fi
  return 0
}

#######################################
# 顺序更新全部 URL 订阅
# 参数: 无
# 返回: 全部成功返回 0，任一失败返回 1
#######################################
update_all_subscriptions() {
  local group_dir group_id failed=0

  initialize_catalog_storage || return 1
  for group_dir in "$CATALOG_DIR"/*; do
    [ -d "$group_dir" ] || continue
    group_id="${group_dir##*/}"
    [ "$group_id" != "staging" ] || continue
    [ "$(meta_get_string "$group_dir/meta.json" "type" "")" = "subscription" ] || continue
    update_subscription "$group_id" || failed=1
  done
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
  [ -n "$name" ] && [ -n "$url" ] || return 1
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
# 从任意分组复制节点到本地分组
# 参数:
#   $1  源节点引用 (<group-id>/<tag>)
#   $2  目标本地分组 ID (默认 default)
# 返回: 成功返回 0
#######################################
copy_node_to_local() {
  local source_ref="$1"
  local target_group="${2:-default}"
  local source_group tag source_provider target_dir target_provider stage node_count now had_nodes=0

  source_group="${source_ref%%/*}"
  tag="${source_ref#*/}"
  [ "$source_group" != "$source_ref" ] && [ -n "$tag" ] || return 1
  source_provider="$(catalog_provider_path "$source_group")" || return 1
  catalog_provider_contains_tag "$source_provider" "$tag" || return 1
  [ "$(meta_get_string "$CATALOG_DIR/$target_group/meta.json" "type" "")" != "subscription" ] || return 1
  target_dir="$CATALOG_DIR/$target_group"
  target_provider="$target_dir/provider.json"
  [ -f "$target_provider" ] || return 1
  catalog_provider_has_nodes "$target_provider" && had_nodes=1

  acquire_catalog_lock "$target_group" || return 1
  create_catalog_stage "$target_group" > /dev/null || { release_catalog_lock; return 1; }
  stage="$SUB_STAGE_DIR"
  cp "$target_provider" "$stage/provider.json" || { release_catalog_lock; return 1; }
  if ! "$NETPROXY_NATIVE_BIN" provider append --target "$stage/provider.json" \
    --input "$source_provider" --tag "$tag" > "$stage/result.json" 2> "$stage/error.json"; then
    release_catalog_lock
    return 1
  fi
  node_count="$(grep -o '"protocol"' "$stage/result.json" | wc -l | tr -d '[:space:]')"
  load_catalog_meta "$target_dir/meta.json" || initialize_local_meta "$target_group" "$target_group" "local"
  SUB_NODE_COUNT="$node_count"
  SUB_REVISION=$((SUB_REVISION + 1))
  now="$(date +%s)"
  SUB_UPDATED_AT="$(format_epoch_utc "$now")"
  write_catalog_meta "$stage/meta.json" || { release_catalog_lock; return 1; }
  mv -f "$stage/provider.json" "$target_provider" || { release_catalog_lock; return 1; }
  mv -f "$stage/meta.json" "$target_dir/meta.json" || { release_catalog_lock; return 1; }
  release_catalog_lock
  [ "$had_nodes" = "1" ] || reload_catalog_structure_if_running
}

#######################################
# 从本地分组删除节点
# 参数:
#   $1  节点引用 (<group-id>/<tag>)
# 返回: 成功返回 0
#######################################
remove_local_node() {
  local node_ref="$1"
  local group_id tag group_dir provider_file stage node_count now

  group_id="${node_ref%%/*}"
  tag="${node_ref#*/}"
  [ "$group_id" != "$node_ref" ] && [ -n "$tag" ] || return 1
  group_dir="$CATALOG_DIR/$group_id"
  [ "$(meta_get_string "$group_dir/meta.json" "type" "")" != "subscription" ] || return 1
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
# 原子替换本地分组中的一个节点
# 参数:
#   $1  旧节点引用 (<group-id>/<tag>)
#   $2  新节点链接或文件
# 返回: 成功返回 0
#######################################
edit_local_node() {
  local node_ref="$1"
  local input="$2"
  local group_id tag group_dir provider_file stage node_count now

  group_id="${node_ref%%/*}"
  tag="${node_ref#*/}"
  [ "$group_id" != "$node_ref" ] && [ -n "$tag" ] || return 1
  group_dir="$CATALOG_DIR/$group_id"
  [ "$(meta_get_string "$group_dir/meta.json" "type" "")" != "subscription" ] || return 1
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
    set_active_catalog_group "$replacement" || [ -z "$replacement" ] || return 1
    if [ -z "$replacement" ]; then
      set_active_catalog_group "" || return 1
      if [ -n "$(get_pid "$SING_BOX_BIN")" ]; then
        sh "$SERVICE_SCRIPT" stop > /dev/null 2>&1 || true
      fi
    fi
  fi

  cancel_subscription_update "$group_id" 2> /dev/null || true
  acquire_catalog_lock "$group_id" || return 1
  rm -rf "$group_dir" || { release_catalog_lock; return 1; }
  release_catalog_lock
  reload_catalog_structure_if_running
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
