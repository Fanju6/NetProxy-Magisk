#!/system/bin/sh
#######################################
# 文件: subscription.sh
# 功能: 为旧生命周期入口保留最薄的订阅适配，不持有订阅业务状态。
# 用法: subscription.sh {update|update-all|cancel}；测试与内部脚本可调用同名适配函数。
# 依赖: common.sh、catalog.sh、netproxy-native。
#######################################

set -u

if [ -z "${MODDIR:-}" ]; then
  MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
fi
[ -n "${MODULE_CONF:-}" ] || MODULE_CONF="$MODDIR/config/module.conf"
[ -n "${CATALOG_DIR:-}" ] || CATALOG_DIR="$MODDIR/config/catalog"
[ -n "${NETPROXY_NATIVE_BIN:-}" ] || NETPROXY_NATIVE_BIN="$MODDIR/bin/netproxy-native"
[ -n "${SERVICE_SCRIPT:-}" ] || SERVICE_SCRIPT="$MODDIR/scripts/core/service.sh"
[ -n "${SING_BOX_BIN:-}" ] || SING_BOX_BIN="$MODDIR/bin/sing-box"
[ -n "${SUB_RUNTIME_DIR:-}" ] || SUB_RUNTIME_DIR="/dev/netproxy/subscriptions"
[ -n "${LOG_FILE:-}" ] || LOG_FILE="$MODDIR/logs/subscription.log"
[ -n "${LOG_TAG:-}" ] || LOG_TAG="subscription"
[ -n "${SINGBOX_DIR:-}" ] || SINGBOX_DIR="$MODDIR/config/singbox"
[ -n "${EBPF_CONF:-}" ] || EBPF_CONF="$MODDIR/config/ebpf/ebpf.conf"

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/catalog.sh"

#######################################
# 初始化 Catalog 根目录与固定本地组
# 参数: 无
# 返回: 成功返回 0，否则返回 1
#######################################
initialize_catalog_storage() {
  mkdir -p "$CATALOG_DIR" "$SUB_RUNTIME_DIR" "$MODDIR/logs" || return 1
  "$NETPROXY_NATIVE_BIN" catalog group-ensure \
    --root "$CATALOG_DIR" --group default --name "本地配置" --type local \
    > /dev/null 2>&1
}

#######################################
# 调用 Go 订阅 Worker 执行一次更新
# 参数: $1 分组 ID 或名称
# 返回: 更新成功或未变化返回 0，失败返回 1
#######################################
update_subscription() {
  local query="$1"
  local group_id

  initialize_catalog_storage || return 1
  group_id="$(catalog_resolve_group "$query")" || return 1
  rm -f "$SUB_RUNTIME_DIR/$group_id.cancel" "$SUB_RUNTIME_DIR/$group_id.progress.json"
  "$NETPROXY_NATIVE_BIN" subworker once \
    --root "$CATALOG_DIR" --group "$group_id" \
    --progress-dir "$SUB_RUNTIME_DIR" --pid-file "/dev/netproxy/subworker.pid" \
    --log-file "$LOG_FILE" --module-conf "$MODULE_CONF" \
    --reload-script "$SERVICE_SCRIPT" --sing-box "$SING_BOX_BIN" \
    --service-address "${SERVICE_API:-127.0.0.1:9090}" \
    --service-secret "${SERVICE_SECRET:-singbox}" --format kv \
    > /dev/null
}

#######################################
# 顺序更新全部订阅
# 参数: 无
# 返回: 全部成功返回 0，否则返回 1
#######################################
update_all_subscriptions() {
  "$NETPROXY_NATIVE_BIN" module sub update-all \
    --module-dir "$MODDIR" --catalog-root "$CATALOG_DIR" \
    --module-config "$MODULE_CONF" --ebpf-config "$EBPF_CONF" \
    --singbox-dir "$SINGBOX_DIR" --sing-box "$SING_BOX_BIN" \
    --runtime-dir "$SINGBOX_DIR/runtime" > /dev/null
}

#######################################
# 添加 URL 订阅并立即执行首次更新
# 参数: $1 名称；$2 URL
# 返回: 标准输出返回分组 ID，更新失败时仍返回分组 ID并以 1 退出
#######################################
add_subscription() {
  local name="$1"
  local url="$2"
  local group_id
  local headers_file="$CATALOG_DIR/staging/headers.$$.json"

  [ -n "$url" ] || return 1
  initialize_catalog_storage || return 1
  mkdir -p "${headers_file%/*}" || return 1
  printf '%s\n' "${SUB_ADD_CUSTOM_HEADERS:-{}}" > "$headers_file" || return 1
  chmod 0600 "$headers_file" 2> /dev/null || true
  group_id="$(catalog_new_group_id subscription "$url")" || {
    rm -f "$headers_file"
    return 1
  }
  if ! "$NETPROXY_NATIVE_BIN" catalog group-init \
    --root "$CATALOG_DIR" --group "$group_id" --type subscription \
    --name "$name" --url "$url" --user-agent "${SUB_ADD_USER_AGENT:-}" \
    --hwid "${SUB_ADD_HWID:-}" --headers-file "$headers_file" \
    --auto-update "${SUB_ADD_AUTO_UPDATE:-true}" \
    --update-interval "${SUB_ADD_UPDATE_INTERVAL:-86400}" \
    --interval-source user --update-via-proxy "${SUB_ADD_UPDATE_VIA_PROXY:-auto}" \
    --include "${SUB_ADD_INCLUDE:-}" --exclude "${SUB_ADD_EXCLUDE:-}" \
    --timeout "${SUB_ADD_TIMEOUT:-60}" > /dev/null 2>&1; then
    rm -f "$headers_file"
    return 1
  fi
  rm -f "$headers_file"

  if update_subscription "$group_id"; then
    printf '%s\n' "$group_id"
    return 0
  fi
  printf '%s\n' "$group_id"
  return 1
}

#######################################
# 导入本地节点文件，具体分组和运行时副作用交给 Go
# 参数: $1 文件；$2 显示名称 (可选)
# 返回: 标准输出返回分组 ID
#######################################
import_local_file_group() {
  local input="$1"
  local name="${2:-${input##*/}}"

  "$NETPROXY_NATIVE_BIN" module node import \
    --module-dir "$MODDIR" --catalog-root "$CATALOG_DIR" \
    --module-config "$MODULE_CONF" --ebpf-config "$EBPF_CONF" \
    --singbox-dir "$SINGBOX_DIR" --sing-box "$SING_BOX_BIN" \
    --runtime-dir "$SINGBOX_DIR/runtime" --name "$name" "$input"
}

#######################################
# 向本地或订阅分组追加节点
# 参数: $1 分组；$2 节点链接或文件
# 返回: 沿用 Go 命令退出码
#######################################
append_local_node() {
  "$NETPROXY_NATIVE_BIN" module node add \
    --module-dir "$MODDIR" --catalog-root "$CATALOG_DIR" \
    --module-config "$MODULE_CONF" --ebpf-config "$EBPF_CONF" \
    --singbox-dir "$SINGBOX_DIR" --sing-box "$SING_BOX_BIN" \
    --runtime-dir "$SINGBOX_DIR/runtime" "$2" "$1" > /dev/null
}

#######################################
# 删除 Catalog 节点并处理选择回退
# 参数: $1 节点引用 (<group-id>/<tag>)
# 返回: 沿用 Go 命令退出码
#######################################
remove_catalog_node() {
  "$NETPROXY_NATIVE_BIN" module node remove \
    --module-dir "$MODDIR" --catalog-root "$CATALOG_DIR" \
    --module-config "$MODULE_CONF" --ebpf-config "$EBPF_CONF" \
    --singbox-dir "$SINGBOX_DIR" --sing-box "$SING_BOX_BIN" \
    --runtime-dir "$SINGBOX_DIR/runtime" "$1" > /dev/null
}

#######################################
# 编辑 Catalog 节点
# 参数: $1 节点引用；$2 新节点链接或文件
# 返回: 沿用 Go 命令退出码
#######################################
edit_catalog_node() {
  "$NETPROXY_NATIVE_BIN" module node edit \
    --module-dir "$MODDIR" --catalog-root "$CATALOG_DIR" \
    --module-config "$MODULE_CONF" --ebpf-config "$EBPF_CONF" \
    --singbox-dir "$SINGBOX_DIR" --sing-box "$SING_BOX_BIN" \
    --runtime-dir "$SINGBOX_DIR/runtime" "$1" "$2" > /dev/null
}

#######################################
# 删除订阅并处理活动组替换
# 参数: $1 订阅 ID 或名称；$2 替代分组 (可选)
# 返回: 沿用 Go 命令退出码
#######################################
remove_subscription() {
  if [ -n "${2:-}" ]; then
    "$NETPROXY_NATIVE_BIN" module sub remove \
      --module-dir "$MODDIR" --catalog-root "$CATALOG_DIR" \
      --module-config "$MODULE_CONF" --ebpf-config "$EBPF_CONF" \
      --singbox-dir "$SINGBOX_DIR" --sing-box "$SING_BOX_BIN" \
      --runtime-dir "$SINGBOX_DIR/runtime" "$1" "$2" > /dev/null
  else
    "$NETPROXY_NATIVE_BIN" module sub remove \
      --module-dir "$MODDIR" --catalog-root "$CATALOG_DIR" \
      --module-config "$MODULE_CONF" --ebpf-config "$EBPF_CONF" \
      --singbox-dir "$SINGBOX_DIR" --sing-box "$SING_BOX_BIN" \
      --runtime-dir "$SINGBOX_DIR/runtime" "$1" > /dev/null
  fi
}

#######################################
# 请求取消订阅更新
# 参数: $1 订阅 ID 或名称
# 返回: 沿用 Go 命令退出码
#######################################
cancel_subscription_update() {
  "$NETPROXY_NATIVE_BIN" module sub cancel \
    --module-dir "$MODDIR" --catalog-root "$CATALOG_DIR" \
    --module-config "$MODULE_CONF" --ebpf-config "$EBPF_CONF" \
    --singbox-dir "$SINGBOX_DIR" --sing-box "$SING_BOX_BIN" \
    --runtime-dir "$SINGBOX_DIR/runtime" "$1" > /dev/null
}

#######################################
# 显示低层脚本用法
# 参数: 无
# 返回: 始终返回 0
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
# 参数: update、update-all 或 cancel
# 返回: 成功返回 0，参数错误返回 1
#######################################
subscription_main() {
  case "${1:-}" in
    update) [ -n "${2:-}" ] && update_subscription "$2" ;;
    update-all) update_all_subscriptions ;;
    cancel) [ -n "${2:-}" ] && cancel_subscription_update "$2" ;;
    help|-h|--help) show_subscription_usage ;;
    *) show_subscription_usage; return 1 ;;
  esac
}

if [ "${SUBSCRIPTION_LIBRARY_ONLY:-0}" != "1" ]; then
  subscription_main "$@"
fi
