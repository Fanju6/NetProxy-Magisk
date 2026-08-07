#!/system/bin/sh
#######################################
# 文件: metadata.sh
# 功能: 通过 netproxy-native 读取、初始化和原子保存 Catalog 元数据。
# 用法: 由 subscription.sh、subworker.sh 与 netproxyctl 引入。
# 依赖: common.sh、netproxy-native。
#######################################

METADATA_SEPARATOR="$(printf '\037')"

#######################################
# 调用 Go 元数据实现
# 参数: $@ 传递给 netproxy-native catalog
# 返回: 透传 Go 组件退出码
#######################################
metadata_native() {
  local binary="${METADATA_NATIVE_BIN:-${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}}"
  "$binary" catalog "$@"
}

#######################################
# 读取 JSON 字段原始值
# 参数: $1 元数据文件；$2 字段名；$3 默认值
# 返回: 找到字段返回 0，否则打印默认值并返回 1
#######################################
meta_get_raw() {
  local file="$1"
  local key="$2"
  local default="${3:-}"
  local value

  value="$(metadata_native meta-get --input "$file" --field "$key" --format raw 2> /dev/null)" || {
    printf "%s" "$default"
    return 1
  }
  [ -n "$value" ] || {
    printf "%s" "$default"
    return 1
  }
  printf "%s" "$value"
}

#######################################
# 读取 JSON 字符串字段
# 参数: $1 元数据文件；$2 字段名；$3 默认值
# 返回: 找到字段返回 0，否则打印默认值并返回 1
#######################################
meta_get_string() {
  local file="$1"
  local key="$2"
  local default="${3:-}"
  local value

  value="$(metadata_native meta-get --input "$file" --field "$key" --format string 2> /dev/null)" || {
    printf "%s" "$default"
    return 1
  }
  printf "%s" "$value"
}

#######################################
# 将时长文本转换为秒
# 参数: $1 15m、4h、1d 或纯秒数
# 返回: 标准输出打印秒数；非法输入返回 1
#######################################
duration_to_seconds() {
  metadata_native duration --value "$1" --format raw
}

#######################################
# 将 Unix 秒转换为 UTC 时间
# 参数: $1 Unix 秒
# 返回: 标准输出打印 RFC3339 时间
#######################################
format_epoch_utc() {
  metadata_native time --now "$1" --format raw
}

#######################################
# 计算并保存下一次订阅更新时间
# 参数: $1 可选的基准 epoch
# 全局: 读取 SUB_UPDATE_INTERVAL，设置 SUB_NEXT_UPDATE_*
# 返回: 成功返回 0，Go 组件不可用或输入无效返回 1
#######################################
schedule_next_update() {
  local now="${1:-$(date +%s)}"
  local record key value

  record="$(metadata_native schedule-next --value "$SUB_UPDATE_INTERVAL" \
    --now "$now" --format tsv 2> /dev/null)" || return 1
  while IFS="$TAB" read -r key value; do
    case "$key" in
      interval) SUB_UPDATE_INTERVAL="$value" ;;
      next_update_epoch) SUB_NEXT_UPDATE_EPOCH="$value" ;;
      next_update_at) SUB_NEXT_UPDATE_AT="$value" ;;
    esac
  done << EOF
$record
EOF
  [ -n "${SUB_NEXT_UPDATE_EPOCH:-}" ] && [ -n "${SUB_NEXT_UPDATE_AT:-}" ]
}

#######################################
# 加载元数据到 SUB_* 临时变量
# 参数: $1 meta.json 路径
# 返回: 文件有效返回 0，否则返回 1
#######################################
load_catalog_meta() {
  local file="$1"
  local record old_ifs

  [ -f "$file" ] || return 1
  record="$(metadata_native meta-get --input "$file" --format tsv 2> /dev/null)" || return 1
  old_ifs="$IFS"
  IFS="$METADATA_SEPARATOR"
  read -r SUB_SCHEMA SUB_ID SUB_NAME SUB_TYPE SUB_URL SUB_USER_AGENT SUB_HWID \
    SUB_CUSTOM_HEADERS SUB_AUTO_UPDATE SUB_UPDATE_INTERVAL SUB_INTERVAL_SOURCE \
    SUB_UPDATE_VIA_PROXY SUB_INCLUDE SUB_EXCLUDE SUB_ALLOW_INSECURE SUB_TIMEOUT \
    SUB_USAGE SUB_NODE_COUNT SUB_REVISION SUB_ETAG SUB_LAST_MODIFIED \
    SUB_PROFILE_TITLE SUB_PROFILE_WEB_PAGE_URL SUB_CONTENT_DISPOSITION SUB_FILE_NAME \
    SUB_LAST_STATUS_CODE SUB_LAST_DIAGNOSTICS SUB_LAST_ATTEMPT_AT SUB_LAST_SUCCESS_AT \
    SUB_NEXT_UPDATE_AT SUB_NEXT_UPDATE_EPOCH SUB_LAST_ERROR SUB_CREATED_AT SUB_UPDATED_AT << EOF
$record
EOF
  IFS="$old_ifs"
}

#######################################
# 原子保存当前 SUB_* 临时变量
# 参数: $1 meta.json 路径
# 返回: 成功返回 0，否则返回 1
#######################################
write_catalog_meta() {
  local file="$1"
  local state="${file}.state.$$"
  local custom_headers="${SUB_CUSTOM_HEADERS:-}"

  mkdir -p "${file%/*}" || return 1
  [ -n "$custom_headers" ] || custom_headers="{}"
  printf '%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\n' \
    "${SUB_SCHEMA:-1}" "${SUB_ID:-}" "${SUB_NAME:-}" "${SUB_TYPE:-local}" \
    "${SUB_URL:-}" "${SUB_USER_AGENT:-}" "${SUB_HWID:-}" "$custom_headers" \
    "${SUB_AUTO_UPDATE:-false}" "${SUB_UPDATE_INTERVAL:-86400}" "${SUB_INTERVAL_SOURCE:-default}" \
    "${SUB_UPDATE_VIA_PROXY:-auto}" "${SUB_INCLUDE:-}" "${SUB_EXCLUDE:-}" \
    "${SUB_ALLOW_INSECURE:-false}" "${SUB_TIMEOUT:-60}" "${SUB_USAGE:-null}" \
    "${SUB_NODE_COUNT:-0}" "${SUB_REVISION:-0}" "${SUB_ETAG:-}" "${SUB_LAST_MODIFIED:-}" \
    "${SUB_PROFILE_TITLE:-}" "${SUB_PROFILE_WEB_PAGE_URL:-}" "${SUB_CONTENT_DISPOSITION:-}" \
    "${SUB_FILE_NAME:-}" "${SUB_LAST_STATUS_CODE:-0}" "${SUB_LAST_DIAGNOSTICS:-[]}" \
    "${SUB_LAST_ATTEMPT_AT:-}" "${SUB_LAST_SUCCESS_AT:-}" "${SUB_NEXT_UPDATE_AT:-}" \
    "${SUB_NEXT_UPDATE_EPOCH:-0}" "${SUB_LAST_ERROR:-}" "${SUB_CREATED_AT:-}$(printf '\037')${SUB_UPDATED_AT:-}" > "$state" || {
      rm -f "$state"
      return 1
    }
  if metadata_native meta-write --input "$file" --state-file "$state" > /dev/null 2> /dev/null; then
    rm -f "$state"
    return 0
  fi
  rm -f "$state"
  return 1
}

#######################################
# 初始化 Catalog 元数据临时变量
# 参数: $1 分组 ID；$2 显示名称；$3 类型
# 返回: 始终返回 0
#######################################
initialize_catalog_meta() {
  local now now_text

  now="$(date +%s)"
  now_text="$(format_epoch_utc "$now")" || return 1
  SUB_SCHEMA=1
  SUB_ID="$1"
  SUB_NAME="$2"
  SUB_TYPE="${3:-local}"
  SUB_URL=""
  SUB_USER_AGENT=""
  SUB_HWID=""
  SUB_CUSTOM_HEADERS="{}"
  SUB_AUTO_UPDATE=false
  SUB_UPDATE_INTERVAL=86400
  SUB_INTERVAL_SOURCE="default"
  SUB_UPDATE_VIA_PROXY="auto"
  SUB_INCLUDE=""
  SUB_EXCLUDE=""
  SUB_ALLOW_INSECURE=false
  SUB_TIMEOUT=60
  SUB_USAGE=null
  SUB_NODE_COUNT=0
  SUB_REVISION=0
  SUB_ETAG=""
  SUB_LAST_MODIFIED=""
  SUB_PROFILE_TITLE=""
  SUB_PROFILE_WEB_PAGE_URL=""
  SUB_CONTENT_DISPOSITION=""
  SUB_FILE_NAME=""
  SUB_LAST_STATUS_CODE=0
  SUB_LAST_DIAGNOSTICS="[]"
  SUB_LAST_ATTEMPT_AT=""
  SUB_LAST_SUCCESS_AT=""
  SUB_NEXT_UPDATE_AT=""
  SUB_NEXT_UPDATE_EPOCH=0
  SUB_LAST_ERROR=""
  SUB_CREATED_AT="$now_text"
  SUB_UPDATED_AT="$now_text"
}

#######################################
# 初始化本地分组元数据
# 参数: $1 分组 ID；$2 显示名称；$3 类型
# 返回: 始终返回 0
#######################################
initialize_local_meta() {
  initialize_catalog_meta "$1" "$2" "${3:-local}"
}

#######################################
# 初始化 URL 订阅元数据
# 参数: $1 分组 ID；$2 显示名称；$3 订阅地址
# 返回: 成功返回 0，调度计算失败返回 1
#######################################
initialize_subscription_meta() {
  initialize_catalog_meta "$1" "$2" "subscription" || return 1
  SUB_URL="$3"
  SUB_AUTO_UPDATE=true
  schedule_next_update || return 1
}
