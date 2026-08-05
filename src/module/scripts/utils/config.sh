#!/system/bin/sh
#######################################
# 文件: config.sh
# 功能: 配置文件读写辅助函数，提供键值读取、原子写入、
#       引号处理，以及空格分隔列表的增删查操作。
# 用法: 由其他脚本通过 . "$MODDIR/scripts/utils/config.sh" 引入。
#       依赖 common.sh 提供的 CR 常量与 require_file/die。
#######################################

#######################################
# 去除配置值首尾的引号与结尾回车
# 参数:
#   $1  原始配置值
# 返回: 标准输出打印处理后的值
#######################################
strip_quotes() {
  local value="${1:-}"

  value="${value%"$CR"}"  # 去除结尾回车 (兼容 CRLF 文件)
  value="${value#\"}"     # 去除首部双引号
  value="${value%\"}"     # 去除尾部双引号
  printf "%s" "$value"
}

#######################################
# 读取配置值
# 参数:
#   $1  配置文件路径
#   $2  配置键名
#   $3  默认值 (键不存在时返回，可选)
# 返回: 标准输出打印配置值或默认值
#######################################
read_conf() {
  local file="$1"
  local key="$2"
  local default="${3:-}"
  local line value

  # 配置文件很小，使用 Shell 内建逐行读取可避免每个键都启动 grep 进程。
  if [ -f "$file" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
      case "$line" in
        "$key"=*)
          value="${line#*=}"
          strip_quotes "$value"
          return 0
          ;;
      esac
    done < "$file"
  fi

  # 未命中则返回默认值
  printf "%s" "$default"
}

#######################################
# 获取配置文件写锁
# 参数:
#   $1  配置文件路径
# 返回: 成功返回 0，超时返回 1
#######################################
acquire_conf_lock() {
  local file="$1"
  local lock="${file}.lock"
  local pid owner_start current_start attempts=0

  while [ "$attempts" -lt 50 ]; do
    if mkdir "$lock" 2> /dev/null; then
      printf '%s\n' "$$" > "$lock/pid"
      awk '{print $22}' "/proc/$$/stat" > "$lock/start" 2> /dev/null || true
      CONF_LOCK_DIR="$lock"
      return 0
    fi
    pid="$(sed -n '1p' "$lock/pid" 2> /dev/null || true)"
    owner_start="$(sed -n '1p' "$lock/start" 2> /dev/null || true)"
    current_start="$(awk '{print $22}' "/proc/$pid/stat" 2> /dev/null || true)"
    if [ -z "$pid" ] || [ -z "$owner_start" ] || [ "$owner_start" != "$current_start" ] \
      || ! kill -0 "$pid" 2> /dev/null; then
      rm -rf "$lock" 2> /dev/null || true
      continue
    fi
    sleep 0.1
    attempts=$((attempts + 1))
  done
  return 1
}

#######################################
# 释放配置文件写锁
# 参数: 无
# 返回: 无
#######################################
release_conf_lock() {
  [ -z "${CONF_LOCK_DIR:-}" ] || rm -rf "$CONF_LOCK_DIR" 2> /dev/null || true
  CONF_LOCK_DIR=""
}

#######################################
# 批量写入配置值
# 参数:
#   $1      配置文件路径
#   $2...   键和值交替排列
# 返回: 成功返回 0，写入失败则退出
# 说明: 一次锁定、一次重写和一次 rename，避免客户端读到部分状态。
#######################################
set_conf_values() {
  local file="$1"
  local updates tmp key value

  shift
  require_file "$file" "配置文件不存在: $file"
  [ "$#" -gt 0 ] && [ $(( $# % 2 )) -eq 0 ] || die "批量配置参数不完整"
  acquire_conf_lock "$file" || die "配置文件正忙: $file"

  updates="$file.updates.$$"
  tmp="$file.tmp.$$"
  : > "$updates" || { release_conf_lock; die "无法创建配置更新文件: $file"; }
  while [ "$#" -gt 0 ]; do
    key="$1"
    value="$2"
    shift 2
    case "$key" in
      "" | *[!A-Z0-9_]*)
        rm -f "$updates"
        release_conf_lock
        die "配置键名非法: $key"
        ;;
    esac
    case "$value" in
      *"$NL"* | *"$CR"* | *"$TAB"*)
        rm -f "$updates"
        release_conf_lock
        die "配置值不能包含换行或制表符: $key"
        ;;
    esac
    printf '%s\t%s\n' "$key" "$value" >> "$updates" || {
      rm -f "$updates"
      release_conf_lock
      die "无法写入配置更新文件: $file"
    }
  done

  if awk -F '\t' '
    NR == FNR {
      key = $1
      value = $0
      sub(/^[^\t]*\t/, "", value)
      if (!(key in values)) order[++count] = key
      values[key] = value
      next
    }
    {
      position = index($0, "=")
      key = position > 1 ? substr($0, 1, position - 1) : ""
      if (key in values) {
        if (!written[key]) print key "=" values[key]
        written[key] = 1
      } else {
        print
      }
    }
    END {
      for (index_value = 1; index_value <= count; index_value++) {
        key = order[index_value]
        if (!written[key]) print key "=" values[key]
      }
    }
  ' "$updates" "$file" > "$tmp"; then
    chmod 0600 "$tmp" 2> /dev/null || true
    if ! mv -f "$tmp" "$file"; then
      rm -f "$tmp" "$updates"
      release_conf_lock
      die "写入配置失败: $file"
    fi
  else
    rm -f "$tmp" "$updates"
    release_conf_lock
    die "写入配置失败: $file"
  fi
  rm -f "$updates"
  release_conf_lock
}

#######################################
# 写入单个配置值
# 参数:
#   $1  配置文件路径
#   $2  配置键名
#   $3  配置值
# 返回: 成功返回 0，写入失败则退出
#######################################
set_conf() {
  set_conf_values "$1" "$2" "$3"
}

#######################################
# 为配置值补上双引号
# 参数:
#   $1  原始值
# 返回: 标准输出打印加引号后的值
#######################################
quote_conf() {
  printf '"%s"' "$1"
}

#######################################
# 判断空格分隔列表是否包含指定值
# 参数:
#   $1  列表 (空格分隔)
#   $2  待查找的值
# 返回: 0=包含，非 0=不包含
#######################################
list_contains() {
  local list="$1"
  local item="$2"
  local value

  # 逐项比较
  for value in $list; do
    [ "$value" = "$item" ] && return 0
  done

  return 1
}

#######################################
# 向空格分隔列表追加值 (去重)
# 参数:
#   $1  原列表
#   $2  待追加的值
# 返回: 标准输出打印追加后的列表
#######################################
list_add() {
  local list="$1"
  local item="$2"

  # 已存在则原样返回；列表非空则空格拼接；否则直接作为首项
  if list_contains "$list" "$item"; then
    printf "%s" "$list"
  elif [ -n "$list" ]; then
    printf "%s %s" "$list" "$item"
  else
    printf "%s" "$item"
  fi
}

#######################################
# 从空格分隔列表移除指定值
# 参数:
#   $1  原列表
#   $2  待移除的值
# 返回: 标准输出打印移除后的列表
#######################################
list_remove() {
  local list="$1"
  local item="$2"
  local value output=""

  # 跳过待移除项，其余重新拼接
  for value in $list; do
    [ "$value" = "$item" ] && continue
    if [ -n "$output" ]; then
      output="$output $value"
    else
      output="$value"
    fi
  done

  printf "%s" "$output"
}

#######################################
# 校验 Android 应用包名列表
# 参数:
#   $1  空格分隔的包名
#   $2  配置键名
# 返回: 合法返回 0，否则返回非 0
#######################################
validate_android_package_list() {
  local values="${1:-}"
  local key="${2:-应用列表}"
  local value

  for value in $values; do
    case "$value" in
      "" | *[!A-Za-z0-9._]*) return 1 ;;
    esac
  done

  return 0
}

#######################################
# 校验 Android 用户 ID 列表
# 参数:
#   $1  空格分隔的用户 ID
#   $2  配置键名
# 返回: 合法返回 0，否则返回非 0
#######################################
validate_android_user_list() {
  local values="${1:-}"
  local key="${2:-Android 用户列表}"
  local value

  for value in $values; do
    case "$value" in
      "" | *[!0-9]*) return 1 ;;
    esac
  done

  return 0
}
