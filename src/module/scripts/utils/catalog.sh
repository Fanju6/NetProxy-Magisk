#!/system/bin/sh
#######################################
# 文件: catalog.sh
# 功能: Catalog 分组辅助函数，负责调用 native 读取分组状态、节点和历史。
# 用法: 由 core 与 CLI 脚本通过 . "$MODDIR/scripts/utils/catalog.sh" 引入。
# 依赖: common.sh 与 netproxy-native；调用方可定义 CATALOG_DIR 与 NETPROXY_NATIVE_BIN。
#######################################

#######################################
# 校验 Catalog 分组 ID
# 参数:
#   $1  分组 ID
# 返回: 合法返回 0，否则返回 1
#######################################
catalog_validate_group_id() {
  local group_id="$1"

  case "$group_id" in
    "" | staging | *[!A-Za-z0-9._-]* | .* | *..*) return 1 ;;
    *) return 0 ;;
  esac
}

#######################################
# 通过 Go 组件解析分组 ID 或唯一显示名称
# 参数:
#   $1  分组 ID 或显示名称
# 返回: 标准输出打印分组 ID；不存在或名称重复返回 1
#######################################
catalog_resolve_group() {
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"

  [ -x "$native" ] || return 1
  "$native" catalog resolve --root "$CATALOG_DIR" --group "$1" --format raw
}

#######################################
# 由 Go 组件生成不冲突的 Catalog 分组 ID
# 参数:
#   $1  subscription 或 file
#   $2  文件路径 (file 类型使用)
# 返回: 标准输出打印分组 ID
#######################################
catalog_new_group_id() {
  local kind="$1"
  local source="${2:-}"
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"

  [ -x "$native" ] || return 1
  "$native" catalog new-id --root "$CATALOG_DIR" --kind "$kind" \
    --input "$source" --format raw
}

#######################################
# 通过 Go 组件检查分组是否有节点
# 参数:
#   $1  分组 ID
# 返回: 有节点返回 0，否则返回 1
#######################################
catalog_group_has_nodes() {
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"

  [ -x "$native" ] || return 1
  [ "$("$native" catalog group-has-nodes --root "$CATALOG_DIR" --group "$1" --format raw 2> /dev/null)" = "true" ]
}

#######################################
# 通过 Go 组件读取分组首个节点标签
# 参数:
#   $1  分组 ID
# 返回: 标准输出打印节点标签
#######################################
catalog_group_first_tag() {
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"

  [ -x "$native" ] || return 1
  "$native" catalog group-first-tag --root "$CATALOG_DIR" --group "$1" --format raw
}

#######################################
# 通过 Go 组件检查分组是否包含节点标签
# 参数:
#   $1  分组 ID
#   $2  节点标签
# 返回: 存在返回 0，否则返回 1
#######################################
catalog_group_contains_tag() {
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"

  [ -x "$native" ] || return 1
  [ "$("$native" catalog group-contains-tag --root "$CATALOG_DIR" --group "$1" \
    --tag "$2" --format raw 2> /dev/null)" = "true" ]
}

#######################################
# 通过 Go 组件读取分组类型
# 参数:
#   $1  分组 ID
# 返回: 标准输出打印 local、file 或 subscription
#######################################
catalog_group_type() {
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"

  [ -x "$native" ] || return 1
  "$native" catalog group-type --root "$CATALOG_DIR" --group "$1" --format raw
}

#######################################
# 通过 Go 组件查找首个非空分组
# 参数:
#   $1  要排除的分组 ID，可为空
# 返回: 标准输出打印分组 ID；没有可用分组时无输出并返回 0
#######################################
catalog_first_nonempty_group() {
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"

  [ -x "$native" ] || return 1
  "$native" catalog first-nonempty --root "$CATALOG_DIR" --group "$1" --format raw
}

#######################################
# 通过 Go 组件读取分组私有元数据
# 参数:
#   $1  分组 ID
# 返回: 标准输出打印完整元数据 JSON
#######################################
catalog_group_private_json() {
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"
  local result

  [ -x "$native" ] || return 1
  result="$("$native" catalog group-private --root "$CATALOG_DIR" --group "$1")" || return 1
  extract_result_data "$result"
}

#######################################
# 通过 Go 组件解析订阅更新周期
# 参数:
#   $1  15m、4h、1d 或纯秒数
# 返回: 标准输出打印秒数；非法输入返回 1
#######################################
catalog_duration_to_seconds() {
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"

  [ -x "$native" ] || return 1
  "$native" catalog duration --value "$1" --format raw
}

#######################################
# 通过 Go 组件读取分组更新历史
# 参数:
#   $1  分组 ID
# 返回: 标准输出打印历史 JSON 数组
#######################################
catalog_history_json() {
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"
  local result

  [ -x "$native" ] || return 1
  result="$("$native" catalog history --root "$CATALOG_DIR" --group "$1")" || return 1
  extract_result_data "$result"
}

#######################################
# 返回分组的运行时显示标签
# 参数:
#   $1  分组 ID
# 返回: 标准输出打印分组名称；同名时追加稳定 ID 防止 tag 冲突
#######################################
catalog_runtime_group_tag() {
  local group_id="$1"
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"

  catalog_validate_group_id "$group_id" || return 1
  [ -x "$native" ] || return 1
  "$native" catalog tag --root "$CATALOG_DIR" --group "$group_id" --format raw
}

#######################################
# 将持久节点引用转换为 sing-box 运行时出站标签
# 参数:
#   $1  节点引用 (<group-id>/<tag>)
# 返回: 标准输出打印 <分组名称>/<tag>
#######################################
catalog_runtime_node_ref() {
  local node_ref="$1"
  local group_id tag runtime_group

  group_id="${node_ref%%/*}"
  tag="${node_ref#*/}"
  [ "$group_id" != "$node_ref" ] && [ -n "$tag" ] || return 1
  runtime_group="$(catalog_runtime_group_tag "$group_id")" || return 1
  printf "%s/%s\n" "$runtime_group" "$tag"
}
