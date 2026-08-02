#!/system/bin/sh
#######################################
# 文件: apps.sh
# 功能: Android 应用与 UID 转换辅助函数。
# 用法: 由 runtime.sh 等脚本引入。
#######################################

#######################################
# 将应用名单解析为 Android UID
# 参数:
#   $1  空格分隔的包名，支持 user:package 格式
# 返回: 标准输出打印空格分隔的 UID
#######################################
resolve_package_uids() {
  local packages="${1:-}"

  [ -n "$packages" ] || return 0
  [ -f /data/system/packages.list ] || return 0

  awk -v tokens="$packages" '
    BEGIN {
      count = split(tokens, items, " ")
      for (i = 1; i <= count; i++) {
        token = items[i]
        if (token ~ /:/) {
          split(token, parts, ":")
          users[i] = parts[1]
          names[i] = parts[2]
        } else {
          users[i] = 0
          names[i] = token
        }
        wanted[names[i]] = 1
      }
    }
    ($1 in wanted) {
      uid = ""
      if ($2 ~ /^[0-9]+$/) uid = $2
      else if ($(NF - 1) ~ /^[0-9]+$/) uid = $(NF - 1)
      if (uid != "") base_uid[$1] = uid
    }
    END {
      output = ""
      for (i = 1; i <= count; i++) {
        name = names[i]
        if (!(name in base_uid)) continue
        uid = users[i] * 100000 + base_uid[name]
        if (seen[uid]) continue
        seen[uid] = 1
        output = output == "" ? uid : output " " uid
      }
      printf "%s", output
    }
  ' /data/system/packages.list
}

#######################################
# 将空格分隔的数字转换为 JSON 数组片段
# 参数:
#   $1  数字列表
# 返回: 标准输出打印逗号分隔的数字
#######################################
uid_list_to_json() {
  local values="${1:-}"
  local value output=""

  for value in $values; do
    case "$value" in
      *[!0-9]* | "") continue ;;
    esac
    if [ -n "$output" ]; then
      output="$output, $value"
    else
      output="$value"
    fi
  done

  printf "%s" "$output"
}
