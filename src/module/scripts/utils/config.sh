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
# 写入配置值 (原子写)
# 存在同名键则整行替换，否则追加到文件末尾。
# 参数:
#   $1  配置文件路径
#   $2  配置键名
#   $3  配置值
# 返回: 成功返回 0，写入失败则退出
# 说明: 通过临时文件 + rename 完成原子替换，键值经环境变量传入。
#######################################
set_conf() {
  local file="$1"
  local key="$2"
  local value="$3"
  local tmp

  require_file "$file" "配置文件不存在: $file"

  # 临时文件带 PID 后缀，避免并发写入互相覆盖
  tmp="$file.tmp.$$"
  # 用 awk 重写整个文件：命中键则替换，文件末尾补齐缺失键
  if SC_KEY="$key" SC_VAL="$value" awk '
    BEGIN { key = ENVIRON["SC_KEY"]; val = ENVIRON["SC_VAL"]; found = 0 }
    {
      # 行首匹配到目标键则整行替换
      if (index($0, key "=") == 1) {
        print key "=" val
        found = 1
      } else {
        print
      }
    }
    # 全文未出现该键则追加
    END { if (!found) print key "=" val }
  ' "$file" > "$tmp"; then
    mv -f "$tmp" "$file"   # 原子覆盖原文件
  else
    rm -f "$tmp"           # 失败时清理临时文件
    die "写入配置失败: $file"
  fi
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
