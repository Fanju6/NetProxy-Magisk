#!/system/bin/sh
#######################################
# 文件: catalog.sh
# 功能: Catalog 节点目录辅助函数，负责校验分组 ID、读取 Provider
#       安全摘要，以及检查节点引用是否存在。
# 用法: 由 core 与 CLI 脚本通过 . "$MODDIR/scripts/utils/catalog.sh" 引入。
# 依赖: common.sh、metadata.sh；调用方可定义 CATALOG_DIR 与 NETPROXY_NATIVE_BIN。
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
# 返回 Catalog 分组目录
# 参数:
#   $1  分组 ID
# 返回: 标准输出打印分组目录；非法 ID 返回 1
#######################################
catalog_group_dir() {
  local group_id="$1"

  catalog_validate_group_id "$group_id" || return 1
  printf "%s/%s\n" "${CATALOG_DIR:?CATALOG_DIR 未定义}" "$group_id"
}

#######################################
# 返回分组 Provider 文件路径
# 参数:
#   $1  分组 ID
# 返回: 标准输出打印 provider.json 路径
#######################################
catalog_provider_path() {
  local group_dir

  group_dir="$(catalog_group_dir "$1")" || return 1
  printf "%s/provider.json\n" "$group_dir"
}

#######################################
# 返回分组的运行时显示标签
# 参数:
#   $1  分组 ID
# 返回: 标准输出打印分组名称；同名时追加稳定 ID 防止 tag 冲突
#######################################
catalog_runtime_group_tag() {
  local group_id="$1"
  local group_dir name other_dir other_id other_name duplicate_count=0

  group_dir="$(catalog_group_dir "$group_id")" || return 1
  [ -d "$group_dir" ] || return 1
  name="$(meta_get_string "$group_dir/meta.json" "name" "$group_id")" || name="$group_id"
  [ -n "$name" ] || name="$group_id"

  for other_dir in "$CATALOG_DIR"/*; do
    [ -d "$other_dir" ] || continue
    other_id="${other_dir##*/}"
    [ "$other_id" != "staging" ] || continue
    other_name="$(meta_get_string "$other_dir/meta.json" "name" "$other_id")" || other_name="$other_id"
    [ -n "$other_name" ] || other_name="$other_id"
    [ "$other_name" = "$name" ] && duplicate_count=$((duplicate_count + 1))
  done

  if [ "$duplicate_count" -gt 1 ]; then
    printf "%s [%s]\n" "$name" "$group_id"
  else
    printf "%s\n" "$name"
  fi
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

#######################################
# 判断 Provider 是否包含节点
# 参数:
#   $1  provider.json 路径
# 返回: 包含节点返回 0，空 Provider 或文件不存在返回 1
#######################################
catalog_provider_has_nodes() {
  local provider_file="$1"
  local line

  [ -f "$provider_file" ] || return 1
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      *\"type\"*:*) return 0 ;;
    esac
  done < "$provider_file"
  return 1
}

#######################################
# 获取 Provider 安全摘要
# 参数:
#   $1  provider.json 路径
# 返回: 标准输出打印原生组件 JSON 摘要
#######################################
catalog_provider_inspect() {
  local provider_file="$1"
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"

  [ -x "$native" ] || return 1
  "$native" provider inspect --input "$provider_file" --format json
}

#######################################
# 校验 Provider 文档
# 参数:
#   $1  provider.json 路径
# 返回: 配置有效返回 0，否则返回 1
#######################################
catalog_provider_validate() {
  local provider_file="$1"
  local native="${NETPROXY_NATIVE_BIN:-$MODDIR/bin/netproxy-native}"

  [ -x "$native" ] || return 1
  "$native" provider validate --input "$provider_file" > /dev/null
}

#######################################
# 统计 Provider 节点数
# 参数:
#   $1  provider.json 路径
# 返回: 标准输出打印节点数
#######################################
catalog_provider_node_count() {
  local summary

  summary="$(catalog_provider_inspect "$1")" || return 1
  printf "%s" "$summary" | grep -o '"protocol"' | wc -l | tr -d '[:space:]'
}

#######################################
# 获取 Provider 第一个节点标签
# 参数:
#   $1  provider.json 路径
# 返回: 标准输出打印第一个节点标签
#######################################
catalog_provider_first_tag() {
  local summary

  summary="$(catalog_provider_inspect "$1")" || return 1
  printf "%s" "$summary" | sed -n 's/.*"data":\[{"tag":"\([^"]*\)".*/\1/p'
}

#######################################
# 判断 Provider 是否包含指定节点标签
# 参数:
#   $1  provider.json 路径
#   $2  原始节点标签，不含 group-id 前缀
# 返回: 存在返回 0，否则返回 1
#######################################
catalog_provider_contains_tag() {
  local provider_file="$1"
  local tag="$2"
  local summary escaped

  summary="$(catalog_provider_inspect "$provider_file")" || return 1
  escaped="$(json_escape "$tag")"
  printf "%s" "$summary" | grep -F -q "\"tag\":\"$escaped\""
}

#######################################
# 判断换行分隔的分组列表是否包含指定 ID
# 参数:
#   $1  分组列表
#   $2  分组 ID
# 返回: 包含返回 0，否则返回 1
#######################################
catalog_group_list_contains() {
  local groups="$1"
  local expected="$2"
  local group_id

  while IFS= read -r group_id; do
    [ "$group_id" = "$expected" ] && return 0
  done << EOF
$groups
EOF
  return 1
}
