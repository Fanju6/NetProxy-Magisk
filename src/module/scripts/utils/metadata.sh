#!/system/bin/sh
#######################################
# 文件: metadata.sh
# 功能: 读取和原子写入 Catalog meta.json，并提供订阅周期、时间与
#       NetProxy 原生组件 HTTP 元数据解析函数。
# 用法: 由 subscription.sh、subworker.sh 与 netproxyctl 引入。
# 依赖: common.sh。
#######################################

#######################################
# 读取格式化 JSON 的顶层字段原始值
# 参数:
#   $1  JSON 文件路径
#   $2  字段名
#   $3  默认值 (可选)
# 返回: 标准输出打印原始 JSON 值
#######################################
meta_get_raw() {
  local file="$1"
  local key="$2"
  local default="${3:-}"
  local line normalized remainder value

  [ -f "$file" ] || { printf "%s" "$default"; return 1; }
  while IFS= read -r line || [ -n "$line" ]; do
    normalized="$line"
    while :; do
      case "$normalized" in
        [[:space:]]*) normalized="${normalized#?}" ;;
        *) break ;;
      esac
    done
    case "$normalized" in
      \"$key\"*)
        remainder="${normalized#\"$key\"}"
        while :; do
          case "$remainder" in
            [[:space:]]*) remainder="${remainder#?}" ;;
            *) break ;;
          esac
        done
        case "$remainder" in :*) ;; *) continue ;; esac
        value="${remainder#:}"
        while :; do
          case "$value" in
            [[:space:]]*) value="${value#?}" ;;
            *) break ;;
          esac
        done
        while :; do
          case "$value" in
            *[[:space:]]) value="${value%?}" ;;
            *) break ;;
          esac
        done
        value="${value%,}"
        while :; do
          case "$value" in
            *[[:space:]]) value="${value%?}" ;;
            *) break ;;
          esac
        done
        printf "%s" "$value"
        return 0
        ;;
    esac
  done < "$file"
  printf "%s" "$default"
  return 1
}

#######################################
# 读取紧凑 JSON 的顶层字段原始值
# 参数:
#   $1  JSON 文件路径
#   $2  字段名
#   $3  默认值 (可选)
# 返回: 标准输出打印原始 JSON 值
#######################################
compact_json_get_raw() {
  local file="$1"
  local key="$2"
  local default="${3:-}"
  local value

  [ -f "$file" ] || { printf "%s" "$default"; return 1; }
  value="$(awk -v wanted="$key" '
    { text = text $0 "\n" }
    END {
      marker = "\"" wanted "\""
      position = index(text, marker)
      if (!position) exit 1
      position += length(marker)
      while (substr(text, position, 1) ~ /[[:space:]]/) position++
      if (substr(text, position, 1) != ":") exit 1
      position++
      while (substr(text, position, 1) ~ /[[:space:]]/) position++
      start = position
      first = substr(text, position, 1)

      if (first == "\"") {
        escaped = 0
        for (position++; position <= length(text); position++) {
          char = substr(text, position, 1)
          if (escaped) { escaped = 0; continue }
          if (char == "\\") { escaped = 1; continue }
          if (char == "\"") { print substr(text, start, position - start + 1); exit }
        }
        exit 1
      }

      if (first == "{" || first == "[") {
        open = first
        closing = first == "{" ? "}" : "]"
        depth = 0
        quoted = 0
        escaped = 0
        for (; position <= length(text); position++) {
          char = substr(text, position, 1)
          if (quoted) {
            if (escaped) escaped = 0
            else if (char == "\\") escaped = 1
            else if (char == "\"") quoted = 0
            continue
          }
          if (char == "\"") { quoted = 1; continue }
          if (char == open) depth++
          else if (char == closing) {
            depth--
            if (depth == 0) { print substr(text, start, position - start + 1); exit }
          }
        }
        exit 1
      }

      for (; position <= length(text); position++) {
        char = substr(text, position, 1)
        if (char == "," || char == "}" || char ~ /[[:space:]]/) break
      }
      value = substr(text, start, position - start)
      if (value == "") exit 1
      print value
    }
  ' "$file" 2> /dev/null)" || true
  [ -n "$value" ] || { printf "%s" "$default"; return 1; }
  printf "%s" "$value"
}

#######################################
# 解码 JSON 字符串的常用转义
# 参数:
#   $1  带双引号的 JSON 字符串
# 返回: 标准输出打印解码后的字符串
#######################################
json_raw_to_string() {
  local raw="$1"

  [ "$raw" != "null" ] || return 1
  raw="${raw#\"}"
  raw="${raw%\"}"
  printf "%s" "$raw" | sed 's/\\"/"/g; s/\\\\/\\/g'
}

#######################################
# 读取格式化 JSON 的顶层字符串字段
# 参数:
#   $1  JSON 文件路径
#   $2  字段名
#   $3  默认值 (可选)
# 返回: 标准输出打印解码后的字符串
#######################################
meta_get_string() {
  local raw

  raw="$(meta_get_raw "$1" "$2" "")" || true
  if [ -z "$raw" ] || [ "$raw" = "null" ]; then
    printf "%s" "${3:-}"
    return 1
  fi
  json_raw_to_string "$raw"
}

#######################################
# 读取紧凑 JSON 的顶层字符串字段
# 参数:
#   $1  JSON 文件路径
#   $2  字段名
#   $3  默认值 (可选)
# 返回: 标准输出打印解码后的字符串
#######################################
compact_json_get_string() {
  local raw

  raw="$(compact_json_get_raw "$1" "$2" "")" || true
  if [ -z "$raw" ] || [ "$raw" = "null" ]; then
    printf "%s" "${3:-}"
    return 1
  fi
  json_raw_to_string "$raw"
}

#######################################
# 将时长文本转换为秒数
# 参数:
#   $1  15m、24h、7d 或纯秒数
# 返回: 标准输出打印秒数；非法返回 1
#######################################
duration_to_seconds() {
  local value="$1"
  local number multiplier

  case "$value" in
    *m) number="${value%m}"; multiplier=60 ;;
    *h) number="${value%h}"; multiplier=3600 ;;
    *d) number="${value%d}"; multiplier=86400 ;;
    *) number="$value"; multiplier=1 ;;
  esac
  case "$number" in
    "" | *[!0-9]*) return 1 ;;
  esac
  [ "$number" -gt 0 ] 2> /dev/null || return 1
  printf "%s\n" "$((number * multiplier))"
}

#######################################
# 将 epoch 秒转换为 UTC 时间
# 参数:
#   $1  epoch 秒
# 返回: 标准输出打印 RFC3339；平台不支持时打印 epoch
#######################################
format_epoch_utc() {
  local epoch="$1"
  local formatted

  formatted="$(date -u -d "@$epoch" '+%Y-%m-%dT%H:%M:%SZ' 2> /dev/null || true)"
  if [ -z "$formatted" ]; then
    formatted="$(date -u -r "$epoch" '+%Y-%m-%dT%H:%M:%SZ' 2> /dev/null || true)"
  fi
  [ -n "$formatted" ] && printf "%s\n" "$formatted" || printf "%s\n" "$epoch"
}

#######################################
# 计算并保存下一次订阅更新时间
# 参数:
#   $1  基准 epoch 秒 (可选，默认当前时间)
# 全局: 读取 SUB_UPDATE_INTERVAL，设置 SUB_NEXT_UPDATE_*
# 返回: 无
#######################################
schedule_next_update() {
  local now="${1:-$(date +%s)}"
  local interval="$SUB_UPDATE_INTERVAL"

  case "$interval" in
    "" | *[!0-9]*) interval=86400 ;;
  esac
  [ "$interval" -ge 900 ] 2> /dev/null || interval=900
  SUB_UPDATE_INTERVAL="$interval"
  SUB_NEXT_UPDATE_EPOCH=$((now + interval))
  SUB_NEXT_UPDATE_AT="$(format_epoch_utc "$SUB_NEXT_UPDATE_EPOCH")"
}

#######################################
# 加载分组元数据到 SUB_* 全局变量
# 参数:
#   $1  meta.json 路径
# 返回: 文件存在返回 0，否则返回 1
#######################################
load_catalog_meta() {
  local file="$1"
  local record old_ifs separator

  [ -f "$file" ] || return 1
  separator="$(printf '\037')"
  record="$(awk '
    function decode(value, output, i, char, next_char) {
      if (value == "null") return ""
      if (substr(value, 1, 1) != "\"") return value
      value = substr(value, 2, length(value) - 2)
      output = ""
      for (i = 1; i <= length(value); i++) {
        char = substr(value, i, 1)
        if (char != "\\" || i == length(value)) {
          output = output char
          continue
        }
        next_char = substr(value, ++i, 1)
        if (next_char == "n" || next_char == "r" || next_char == "t") output = output " "
        else output = output next_char
      }
      return output
    }
    BEGIN {
      count = split("id name type active url user_agent hwid custom_headers auto_update update_interval interval_source update_via_proxy include exclude allow_insecure timeout usage node_count revision etag last_modified profile_title profile_web_page_url content_disposition file_name last_status_code last_diagnostics last_attempt_at last_success_at next_update_at next_update_epoch last_error created_at updated_at", order, " ")
      string_fields = " id name type url user_agent hwid interval_source update_via_proxy include exclude etag last_modified profile_title profile_web_page_url content_disposition file_name last_attempt_at last_success_at next_update_at last_error created_at updated_at "
      defaults["type"] = "\"local\""
      defaults["active"] = "false"
      defaults["custom_headers"] = "{}"
      defaults["auto_update"] = "false"
      defaults["update_interval"] = "86400"
      defaults["interval_source"] = "\"default\""
      defaults["update_via_proxy"] = "\"auto\""
      defaults["allow_insecure"] = "false"
      defaults["timeout"] = "60"
      defaults["usage"] = "null"
      defaults["node_count"] = "0"
      defaults["revision"] = "0"
      defaults["last_status_code"] = "0"
      defaults["last_diagnostics"] = "[]"
      defaults["next_update_epoch"] = "0"
    }
    /^[[:space:]]*"[^"]+"[[:space:]]*:/ {
      line = $0
      sub(/^[[:space:]]*"/, "", line)
      key = line
      sub(/".*/, "", key)
      value = $0
      sub(/^[^:]*:[[:space:]]*/, "", value)
      sub(/,[[:space:]]*$/, "", value)
      values[key] = value
    }
    END {
      for (i = 1; i <= count; i++) {
        key = order[i]
        value = key in values ? values[key] : defaults[key]
        if (i > 1) printf "%c", 31
        if (index(string_fields, " " key " ")) printf "%s", decode(value)
        else printf "%s", value
      }
      printf "\n"
    }
  ' "$file")" || return 1

  old_ifs="$IFS"
  IFS="$separator"
  read -r SUB_ID SUB_NAME SUB_TYPE SUB_ACTIVE SUB_URL SUB_USER_AGENT SUB_HWID \
    SUB_CUSTOM_HEADERS SUB_AUTO_UPDATE SUB_UPDATE_INTERVAL SUB_INTERVAL_SOURCE \
    SUB_UPDATE_VIA_PROXY SUB_INCLUDE SUB_EXCLUDE SUB_ALLOW_INSECURE SUB_TIMEOUT \
    SUB_USAGE SUB_NODE_COUNT SUB_REVISION SUB_ETAG SUB_LAST_MODIFIED \
    SUB_PROFILE_TITLE SUB_PROFILE_WEB_PAGE_URL SUB_CONTENT_DISPOSITION SUB_FILE_NAME \
    SUB_LAST_STATUS_CODE SUB_LAST_DIAGNOSTICS SUB_LAST_ATTEMPT_AT SUB_LAST_SUCCESS_AT \
    SUB_NEXT_UPDATE_AT SUB_NEXT_UPDATE_EPOCH SUB_LAST_ERROR SUB_CREATED_AT SUB_UPDATED_AT << EOF
$record
EOF
  IFS="$old_ifs"
  [ -n "$SUB_NAME" ] || SUB_NAME="$SUB_ID"
}

#######################################
# 原子写入当前 SUB_* 元数据
# 参数:
#   $1  meta.json 路径
# 返回: 成功返回 0，否则返回 1
#######################################
write_catalog_meta() {
  local file="$1"
  local tmp="$file.tmp.$$"

  mkdir -p "${file%/*}" || return 1
  cat > "$tmp" << EOF
{
  "schema": 1,
  "id": "$(json_escape "$SUB_ID")",
  "name": "$(json_escape "$SUB_NAME")",
  "type": "$(json_escape "$SUB_TYPE")",
  "active": $SUB_ACTIVE,
  "url": "$(json_escape "$SUB_URL")",
  "user_agent": "$(json_escape "$SUB_USER_AGENT")",
  "hwid": "$(json_escape "$SUB_HWID")",
  "custom_headers": $SUB_CUSTOM_HEADERS,
  "auto_update": $SUB_AUTO_UPDATE,
  "update_interval": $SUB_UPDATE_INTERVAL,
  "interval_source": "$(json_escape "$SUB_INTERVAL_SOURCE")",
  "update_via_proxy": "$(json_escape "$SUB_UPDATE_VIA_PROXY")",
  "include": "$(json_escape "$SUB_INCLUDE")",
  "exclude": "$(json_escape "$SUB_EXCLUDE")",
  "allow_insecure": $SUB_ALLOW_INSECURE,
  "timeout": $SUB_TIMEOUT,
  "usage": $SUB_USAGE,
  "node_count": $SUB_NODE_COUNT,
  "revision": $SUB_REVISION,
  "etag": "$(json_escape "$SUB_ETAG")",
  "last_modified": "$(json_escape "$SUB_LAST_MODIFIED")",
  "profile_title": "$(json_escape "$SUB_PROFILE_TITLE")",
  "profile_web_page_url": "$(json_escape "$SUB_PROFILE_WEB_PAGE_URL")",
  "content_disposition": "$(json_escape "$SUB_CONTENT_DISPOSITION")",
  "file_name": "$(json_escape "$SUB_FILE_NAME")",
  "last_status_code": $SUB_LAST_STATUS_CODE,
  "last_diagnostics": $SUB_LAST_DIAGNOSTICS,
  "last_attempt_at": "$(json_escape "$SUB_LAST_ATTEMPT_AT")",
  "last_success_at": "$(json_escape "$SUB_LAST_SUCCESS_AT")",
  "next_update_at": "$(json_escape "$SUB_NEXT_UPDATE_AT")",
  "next_update_epoch": $SUB_NEXT_UPDATE_EPOCH,
  "last_error": "$(json_escape "$SUB_LAST_ERROR")",
  "created_at": "$(json_escape "$SUB_CREATED_AT")",
  "updated_at": "$(json_escape "$SUB_UPDATED_AT")"
}
EOF
  chmod 0600 "$tmp" 2> /dev/null || true
  mv -f "$tmp" "$file"
}

#######################################
# 初始化全部元数据变量
# 参数:
#   $1  分组 ID
#   $2  显示名称
#   $3  类型 (local/file/subscription)
# 返回: 无
#######################################
initialize_catalog_meta() {
  local now now_text

  now="$(date +%s)"
  now_text="$(format_epoch_utc "$now")"
  SUB_ID="$1"
  SUB_NAME="$2"
  SUB_TYPE="${3:-local}"
  SUB_ACTIVE=false
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
# 初始化本地分组元数据变量
# 参数:
#   $1  分组 ID
#   $2  显示名称
#   $3  类型 (local/file)
# 返回: 无
#######################################
initialize_local_meta() {
  initialize_catalog_meta "$1" "$2" "${3:-local}"
}

#######################################
# 初始化 URL 订阅元数据变量
# 参数:
#   $1  分组 ID
#   $2  显示名称
#   $3  订阅地址
# 返回: 无
#######################################
initialize_subscription_meta() {
  initialize_catalog_meta "$1" "$2" "subscription"
  SUB_URL="$3"
  SUB_AUTO_UPDATE=true
  schedule_next_update
}
