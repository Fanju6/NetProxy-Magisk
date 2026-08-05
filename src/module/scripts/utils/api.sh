#!/system/bin/sh
#######################################
# 文件: api.sh
# 功能: 统一 Service API 与 Clash API 调用。核心控制优先使用固定 proto 的
#       Service API；Clash API 仅保留兼容面板、连接与延迟查询能力。
# 用法: 由其他脚本通过 . "$MODDIR/scripts/utils/api.sh" 引入。
#       依赖 common.sh，并允许调用方覆盖 SERVICE_API、SERVICE_SECRET、
#       CLASH_API、CLASH_SECRET、SELECTOR_GROUP 与 DELAY_URL。
#######################################

# busybox 路径：引入时探测一次并缓存 (沿用调用方已有的 BUSYBOX)，
# 供 api_request 反复调用 nc 时复用，避免每次请求重复探测
API_BUSYBOX="${BUSYBOX:-$(detect_busybox)}"
SERVICE_API="${SERVICE_API:-127.0.0.1:9090}"
SERVICE_SECRET="${SERVICE_SECRET:-singbox}"
CLASH_API="${CLASH_API:-127.0.0.1:9999}"
CLASH_SECRET="${CLASH_SECRET:-singbox}"

#######################################
# 读取控制器地址 (host:port)
# 参数: 无
# 返回: 标准输出打印控制器地址 (缺省 127.0.0.1:9999)
#######################################
api_controller() {
  printf "%s" "$CLASH_API"
}

#######################################
# 读取控制器访问密钥
# 参数: 无
# 返回: 标准输出打印固定的 Clash API 密钥
#######################################
api_secret() {
  printf "%s" "$CLASH_SECRET"
}

#######################################
# 调用 reF1nd Service API
# 参数:
#   $@  service 子命令及参数
# 返回: 成功打印 NetProxy 原生组件统一 JSON，失败返回非 0
#######################################
service_api_call() {
  local action="$1"
  shift

  "${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}" service "$action" \
    --address "$SERVICE_API" \
    --secret "$SERVICE_SECRET" \
    "$@"
}

#######################################
# 判断 Service API 与核心实例是否完整就绪
# 参数: 无
# 返回: 0=就绪，非 0=未就绪
#######################################
service_api_is_ready() {
  service_api_call ready --timeout 2s > /dev/null 2>&1
}

#######################################
# 轮询等待 Service API 就绪
# 参数:
#   $1  最大重试次数 (默认 30)
#   $2  间隔秒数 (默认 1)
# 返回: 就绪返回 0，超时返回 1
#######################################
service_api_wait_ready() {
  local retries="${1:-30}"
  local delay="${2:-1}"
  local count=0

  while [ "$count" -lt "$retries" ]; do
    service_api_is_ready && return 0
    sleep "$delay"
    count=$((count + 1))
  done
  return 1
}

#######################################
# 读取 Service API 的核心启动时间 (毫秒)
# 参数: 无
# 返回: 标准输出打印 Unix 毫秒时间戳
#######################################
service_api_started_at() {
  local result

  result="$(service_api_call started-at --timeout 2s 2> /dev/null)" || return 1
  printf "%s" "$result" | sed -n 's/.*"unix_milli"[^0-9]*\([0-9][0-9]*\).*/\1/p'
}

#######################################
# 将 Unix 毫秒时间戳转换为秒
# 参数:
#   $1  Unix 毫秒时间戳
# 返回: 标准输出打印 Unix 秒时间戳
# 说明: 使用字符串截断，避免 Android mksh 的 32 位整数溢出。
#######################################
unix_millis_to_seconds() {
  local value="${1:-}"

  case "$value" in "" | *[!0-9]*) return 1 ;; esac
  [ "${#value}" -gt 3 ] || { printf "0\n"; return 0; }
  printf "%s\n" "${value%???}"
}

#######################################
# 获取 Service API 节点组快照
# 参数: 无
# 返回: 标准输出打印统一 JSON
#######################################
service_api_groups() {
  service_api_call groups --timeout 5s
}

#######################################
# 获取核心状态与主选择器快照
# 参数:
#   $1  选择器标签 (默认 Proxy)
# 返回: 标准输出打印统一 JSON
#######################################
service_api_snapshot() {
  service_api_call snapshot --group "${1:-Proxy}" --timeout 3s
}

#######################################
# 读取指定 selector 当前出站
# 参数:
#   $1  selector 标签
# 返回: 标准输出打印当前出站标签
#######################################
service_api_selected() {
  local result

  result="$(service_api_call selected --group "$1" --timeout 5s 2> /dev/null)" || return 1
  printf "%s" "$result" | sed -n 's/.*"outbound":"\([^"]*\)".*/\1/p'
}

#######################################
# 通过 Service API 切换 selector
# 参数:
#   $1  selector 标签
#   $2  出站标签
# 返回: 成功返回 0，否则返回非 0
#######################################
service_api_select() {
  service_api_call select --group "$1" --outbound "$2" --timeout 5s > /dev/null 2>&1
}

#######################################
# 通过 Service API 设置出站模式
# 参数:
#   $1  module.conf 模式名
# 返回: 成功返回 0，否则返回非 0
#######################################
service_api_set_mode() {
  local mode

  mode="$(module_mode_to_clash_mode "$1")" || return 1
  service_api_call mode --mode "$mode" --timeout 5s > /dev/null 2>&1
}

#######################################
# 读取 Service API 当前出站模式
# 参数: 无
# 返回: 标准输出打印模式名称
#######################################
service_api_get_mode() {
  local result

  result="$(service_api_call mode --timeout 5s 2> /dev/null)" || return 1
  printf "%s" "$result" | sed -n 's/.*"current":"\([^"]*\)".*/\1/p'
}

#######################################
# 通过 Service API 触发节点或组测速
# 参数:
#   $1  出站标签
# 返回: 请求受理返回 0，否则返回非 0
#######################################
service_api_url_test() {
  service_api_call urltest --outbound "$1" --timeout 8s > /dev/null 2>&1
}

#######################################
# 读取主选择器组名
# 参数: 无
# 返回: 标准输出打印组名 (缺省 Proxy)
#######################################
api_selector_group() {
  printf "%s" "${SELECTOR_GROUP:-Proxy}"
}

#######################################
# 读取延迟测试地址
# 参数: 无
# 返回: 标准输出打印测速 URL
#######################################
api_delay_url() {
  printf "%s" "${DELAY_URL:-https://www.gstatic.com/generate_204}"
}

#######################################
# 对字符串做简单 URL 编码
# 参数:
#   $1  原始字符串
# 返回: 标准输出打印编码后的字符串 (转义常见特殊字符)
#######################################
url_encode_simple() {
  local decoded
  decoded="$(printf '%b' "$1")"
  printf "%s" "$decoded" | sed 's/%/%25/g; s/ /%20/g; s/'"$(printf '\t')"'/%09/g; s/#/%23/g; s/?/%3F/g; s/&/%26/g; s/\//%2F/g; s/+/%2B/g'
}

#######################################
# 从 JSON 文本中提取指定字符串字段的值
# 参数:
#   $1  JSON 文本
#   $2  字段名
# 返回: 标准输出打印首个匹配字段的值
#######################################
json_get_string() {
  local text="$1"
  local key="$2"

  # 按 ,{} 拆行后定位字段并提取引号内的值
  printf "%s" "$text" | tr ',{}' '\n' | grep -m 1 "\"$key\"" | sed -n 's/.*"'"$key"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
}

#######################################
# 发起一次 Clash API 请求 (基于 busybox nc 手写 HTTP)
# 参数:
#   $1  方法 (GET/POST/PUT/PATCH/DELETE)
#   $2  请求路径
#   $3  请求体 (可选，非空时按 JSON 发送)
# 返回: GET 时打印响应体；非 2xx 或连接失败返回非 0
#######################################
api_request() {
  local method="$1"
  local path="$2"
  local data="${3:-}"
  local controller secret host port

  # 拆分控制器地址为 host 与 port
  controller="$(api_controller)"
  secret="$(api_secret)"
  host="${controller%%:*}"
  port="${controller##*:}"

  local response status_line status
  # 手工拼装 HTTP 请求行与头部，经 nc 发送并读取响应
  response=$( {
    printf "%s %s HTTP/1.1\r\n" "$method" "$path"
    printf "Host: %s\r\n" "$controller"
    printf "Authorization: Bearer %s\r\n" "$secret"
    # 有请求体时补充 Content-Type 与 Content-Length
    if [ -n "$data" ]; then
      printf "Content-Type: application/json\r\n"
      printf "Content-Length: %d\r\n" "${#data}"
    fi
    printf "Connection: close\r\n"
    printf "\r\n"
    [ -n "$data" ] && printf "%s" "$data"
  } | "$API_BUSYBOX" nc "$host" "$port" 2>/dev/null )
  status=$?

  # 取首行状态行用于判断结果
  status_line=$(printf "%s" "$response" | head -n 1)

  if [ "$method" = "GET" ]; then
    # GET：剥离头部后输出响应体
    printf "%s" "$response" | sed '1,/^\r$/d'
    # 连接失败或非 2xx 状态视为失败
    if [ $status -ne 0 ] || ! printf "%s" "$status_line" | grep -q -E '^HTTP/[0-9.]+ 2[0-9][0-9]'; then
      return 1
    fi
    return 0
  else
    # 非 GET：仅校验状态码；失败记 DEBUG (是否严重由调用方判定)
    if [ $status -ne 0 ] || ! printf "%s" "$status_line" | grep -q -E '^HTTP/[0-9.]+ 2[0-9][0-9]'; then
      log "DEBUG" "[控制接口] 请求失败: $method $path，状态行: $status_line"
      return 1
    fi
    return 0
  fi
}

#######################################
# 判断控制接口是否可用
# 参数: 无
# 返回: 0=可用，非 0=不可用
#######################################
api_is_available() {
  api_request GET "/configs" > /dev/null 2>&1
}

#######################################
# 轮询等待控制接口就绪
# 参数:
#   $1  最大重试次数 (缺省 5)
#   $2  每次间隔秒数 (缺省 1)
# 返回: 就绪返回 0，超时返回 1
#######################################
api_wait_available() {
  local retries="${1:-5}"
  local delay="${2:-1}"
  local i=0

  # 循环探测直到可用或达到重试上限
  while [ "$i" -lt "$retries" ]; do
    api_is_available && return 0
    sleep "$delay"
    i=$((i + 1))
  done

  return 1
}

#######################################
# 将模块出站模式转换为 Clash 模式名
# 参数:
#   $1  模块模式 (rule/global/direct/AllowAds)
# 返回: 标准输出打印 Clash 模式名；未知模式返回非 0
#######################################
module_mode_to_clash_mode() {
  case "$1" in
    rule) printf "%s" "Rule" ;;
    global) printf "%s" "Global" ;;
    direct) printf "%s" "Direct" ;;
    AllowAds) printf "%s" "AllowAds" ;;
    *) return 1 ;;
  esac
}

#######################################
# 读取控制器当前运行模式
# 参数: 无
# 返回: 标准输出打印当前模式；请求失败返回非 0
#######################################
api_get_mode() {
  local result

  result="$(api_request GET "/configs" 2> /dev/null)" || return 1
  json_get_string "$result" "mode"
}

#######################################
# 设置控制器运行模式
# 参数:
#   $1  模块模式 (rule/global/direct/AllowAds)
# 返回: 成功返回 0，模式非法或请求失败返回非 0
#######################################
api_set_mode() {
  local mode="$1"
  local clash_mode payload

  # 转换为 Clash 模式名并组装请求体
  clash_mode="$(module_mode_to_clash_mode "$mode")" || return 1
  payload="{\"mode\":\"$(json_escape "$clash_mode")\"}"

  api_request PATCH "/configs" "$payload" > /dev/null 2>&1
}

#######################################
# 获取全部代理组数据
# 参数: 无
# 返回: 标准输出打印 /proxies 响应
#######################################
api_get_proxies() {
  api_request GET "/proxies"
}

#######################################
# 从代理数据中提取指定组的原始片段
# 参数:
#   $1  /proxies 响应文本
#   $2  组名 (缺省主选择器组)
# 返回: 标准输出打印该组花括号内的原始内容
#######################################
api_selector_block() {
  local text="$1"
  local group="${2:-$(api_selector_group)}"
  local compact

  # 去除换行后按组名截取其对象内容
  compact="$(printf "%s" "$text" | tr -d '\n')"
  printf "%s" "$compact" | sed -n 's/.*"'"$group"'":{\([^}]*\)}.*/\1/p'
}

#######################################
# 读取指定代理组当前选中的节点
# 参数:
#   $1  /proxies 响应文本
#   $2  组名 (缺省主选择器组)
# 返回: 标准输出打印当前节点名；无数据返回非 0
#######################################
api_selector_current() {
  local text="$1"
  local group="${2:-$(api_selector_group)}"
  local block

  block="$(api_selector_block "$text" "$group")"
  [ -n "$block" ] || return 1
  # 从片段中提取 now 字段
  printf "%s" "$block" | sed -n 's/.*"now":"\([^"]*\)".*/\1/p'
}

#######################################
# 列出指定代理组的可选节点
# 参数:
#   $1  /proxies 响应文本
#   $2  组名 (缺省主选择器组)
# 返回: 标准输出每行一个节点名；无数据返回非 0
#######################################
api_selector_options() {
  local text="$1"
  local group="${2:-$(api_selector_group)}"
  local block list

  block="$(api_selector_block "$text" "$group")"
  [ -n "$block" ] || return 1

  # 提取 all 数组内容
  list="$(printf "%s" "$block" | sed -n 's/.*"all":\[\([^]]*\)\].*/\1/p')"
  [ -n "$list" ] || return 1

  # 去除首尾引号并按逗号拆分为多行
  printf "%s" "$list" | sed 's/^"//; s/"$//; s/","/\n/g'
}

#######################################
# 通过控制接口切换代理组选中的节点
# 参数:
#   $1  目标节点标签
#   $2  组名 (缺省主选择器组)
# 返回: 成功返回 0，失败返回非 0
#######################################
api_select_proxy() {
  local tag="$1"
  local group="${2:-$(api_selector_group)}"
  local payload

  # 组装请求体并对组名做 URL 编码
  payload="{\"name\":\"$(json_escape "$tag")\"}"
  api_request PUT "/proxies/$(url_encode_simple "$group")" "$payload" > /dev/null 2>&1
}

#######################################
# 测试指定节点的延迟
# 参数:
#   $1  节点标签
#   $2  测速地址 (缺省 DELAY_URL)
#   $3  超时毫秒数 (缺省 5000)
# 返回: 标准输出打印延迟测试响应
#######################################
api_test_delay() {
  local tag="$1"
  local url="${2:-$(api_delay_url)}"
  local timeout="${3:-5000}"

  api_request GET "/proxies/$(url_encode_simple "$tag")/delay?timeout=$timeout&url=$(url_encode_simple "$url")"
}

#######################################
# 获取全部连接信息
# 参数: 无
# 返回: 标准输出打印 /connections 响应
#######################################
api_get_connections() {
  api_request GET "/connections"
}

#######################################
# 关闭指定连接
# 参数:
#   $1  连接 ID
# 返回: 成功返回 0，失败返回非 0
#######################################
api_close_connection() {
  local connection_id="$1"

  api_request DELETE "/connections/$(url_encode_simple "$connection_id")" > /dev/null 2>&1
}

#######################################
# 关闭全部连接
# 参数: 无
# 返回: 成功返回 0，失败返回非 0
#######################################
api_close_all_connections() {
  api_request DELETE "/connections" > /dev/null 2>&1
}
