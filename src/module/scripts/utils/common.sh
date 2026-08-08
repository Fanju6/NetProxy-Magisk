#!/system/bin/sh
#######################################
# 文件: common.sh
# 功能: 通用辅助函数库，提供日志、错误处理、依赖检查、
#       文件/目录校验、进程与网络信息获取等基础能力。
# 用法: 由其他脚本通过 . "$MODDIR/scripts/utils/common.sh" 引入。
#######################################

# 常用空白字符常量，供全模块拼接字符串时复用
NL='
'                     # 换行符 (字面量；命令替换会剥除尾部换行，故不可用 $(printf))
TAB="$(printf '\t')"  # 制表符
CR="$(printf '\r')"   # 回车符

#######################################
# 将日志级别名映射为数字 severity
# 参数:
#   $1  级别名 (DEBUG/INFO/WARN/ERROR)
# 返回: 标准输出打印对应数字 (未知级别按 INFO=20 处理)
#######################################
log_level_value() {
  case "$1" in
    DEBUG) printf "10" ;;
    INFO) printf "20" ;;
    WARN) printf "30" ;;
    ERROR) printf "40" ;;
    *) printf "20" ;;
  esac
}

#######################################
# 写入标准日志
# 参数:
#   $1        日志级别 (传两个参数时)，或日志内容 (仅一个参数时)
#   $2        日志内容 (可选)
# 全局:
#   LOG_FILE   日志文件路径 (存在时追加写入)
#   LOG_STDERR 是否输出到 stderr (0=否，默认输出)
#   LOG_LEVEL  输出阈值级别 (默认 INFO)，低于该级别的消息被丢弃
#   LOG_TAG    来源组件标签 (缺省取脚本名)
# 返回: 无
#######################################
log() {
  local level="INFO"
  local message="$1"
  local timestamp tag

  # 传入两个参数时，第一个作为日志级别
  if [ $# -ge 2 ]; then
    level="$1"
    message="$2"
  fi

  # 低于阈值的消息直接丢弃 (文件与 stderr 均不写)
  [ "$(log_level_value "$level")" -ge "$(log_level_value "${LOG_LEVEL:-INFO}")" ] || return 0

  # 组装带时间戳与组件标签的日志行
  timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
  tag="${LOG_TAG:-${0##*/}}"
  local log_content="[$timestamp] [$level] [$tag] $message"

  # 写入日志文件 (若已配置)
  [ -n "${LOG_FILE:-}" ] && printf "%s\n" "$log_content" >> "$LOG_FILE"
  # 输出到 stderr (除非显式关闭)
  [ "${LOG_STDERR:-1}" = "0" ] || printf "%s\n" "$log_content" >&2
}

#######################################
# 记录错误日志并退出
# 参数:
#   $1  错误信息
#   $2  退出码 (可选，默认 1)
# 返回: 不返回，直接退出进程
#######################################
die() {
  log "ERROR" "$1"
  exit "${2:-1}"
}

#######################################
# 检测可用的 busybox 路径
# 参数: 无
# 返回: 标准输出打印 busybox 路径，找不到时回退为 "busybox"
#######################################
detect_busybox() {
  local path

  # 依次检查 KernelSU / APatch / Magisk 自带的 busybox
  for path in "/data/adb/ksu/bin/busybox" "/data/adb/ap/bin/busybox" "/data/adb/magisk/busybox"; do
    if [ -x "$path" ]; then
      printf "%s\n" "$path"
      return 0
    fi
  done

  # 未找到则回退到 PATH 中的 busybox
  printf "%s\n" "busybox"
}

#######################################
# 判断指定命令是否存在
# 参数:
#   $1  命令名
# 返回: 0=存在，非 0=不存在
#######################################
command_exists() {
  command -v "$1" > /dev/null 2>&1
}

#######################################
# 检查必需的外部命令是否齐备
# 参数:
#   $@  需要检查的命令名列表
# 返回: 全部存在返回 0；缺失任意一个则记录错误并退出
# 用法: require_cmds awk sed nc
#######################################
require_cmds() {
  local cmd missing=""

  # 收集所有缺失的命令
  for cmd in "$@"; do
    command_exists "$cmd" || missing="$missing $cmd"
  done

  # 存在缺失命令则终止
  [ -z "$missing" ] || die "缺少必需的命令:$missing"
}

#######################################
# 校验文件必须存在，否则退出
# 参数:
#   $1  文件路径
#   $2  自定义错误信息 (可选)
# 返回: 文件存在返回 0，否则退出
#######################################
require_file() {
  local file="$1"
  local message="${2:-文件不存在: $file}"

  [ -f "$file" ] || die "$message"
}

#######################################
# 校验目录必须存在，否则退出
# 参数:
#   $1  目录路径
#   $2  自定义错误信息 (可选)
# 返回: 目录存在返回 0，否则退出
#######################################
require_dir() {
  local dir="$1"
  local message="${2:-目录不存在: $dir}"

  [ -d "$dir" ] || die "$message"
}

#######################################
# 确保目录存在，不存在则创建
# 参数:
#   $1  目录路径
#   $2  创建失败时的错误信息 (可选)
# 返回: 成功返回 0，创建失败则退出
#######################################
ensure_dir() {
  local dir="$1"
  local message="${2:-无法创建目录: $dir}"

  [ -d "$dir" ] || mkdir -p "$dir" || die "$message"
}

#######################################
# 转义字符串中的 JSON 特殊字符 (反斜杠与双引号)
# 参数:
#   $1  原始字符串
# 返回: 标准输出打印转义后的字符串
#######################################
json_escape() {
  local value="$1"

  # Android mksh 与 BusyBox ash 均支持该替换语法，可避免为每个字段启动 sed。
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf "%s" "$value"
}

#######################################
# 从 netproxy-native 统一结果中提取 data
# 参数:
#   $1  单行结果 JSON
# 返回: 标准输出打印 data JSON；结果格式无效返回 1
#######################################
extract_result_data() {
  local result="$1"
  local data

  case "$result" in
    *'"data":'*) ;;
    *) return 1 ;;
  esac
  data="${result#*\"data\":}"
  data="${data%?}"
  printf "%s" "$data"
}

#######################################
# 获取指定二进制对应的进程 PID
# 参数:
#   $1  二进制路径
# 返回: 标准输出打印 PID；未运行时无输出
#######################################
get_pid() {
  local bin="$1"

  [ -n "$bin" ] || return 1
  # 优先用 pidof 精确匹配，失败再用 pgrep 按命令行匹配
  pidof -s "$bin" 2> /dev/null || pgrep -f "^$bin" 2> /dev/null | head -1 || true
}

#######################################
# 在锁目录写入当前进程的持有者标记
# 参数:
#   $1  锁目录
# 返回: 无
#######################################
lock_write_owner() {
  printf '%s\n' "$$" > "$1/pid"
  # 记录进程启动时刻 (/proc/pid/stat 第 22 字段)，用于区分 PID 回卷后的新进程
  awk '{print $22}' "/proc/$$/stat" > "$1/start" 2> /dev/null || true
}

#######################################
# 判断锁目录的持有者进程是否仍存活
# 参数:
#   $1  锁目录
# 返回: 持有者存活返回 0，锁已失效返回 1
#######################################
lock_owner_alive() {
  local lock_dir="$1"
  local pid owner_start current_start

  pid="$(sed -n '1p' "$lock_dir/pid" 2> /dev/null || true)"
  owner_start="$(sed -n '1p' "$lock_dir/start" 2> /dev/null || true)"
  current_start="$(awk '{print $22}' "/proc/$pid/stat" 2> /dev/null || true)"

  # PID 或启动时刻缺失、启动时刻不一致 (PID 被复用) 均视为锁已失效
  [ -n "$pid" ] && [ -n "$owner_start" ] && [ "$owner_start" = "$current_start" ] \
    && kill -0 "$pid" 2> /dev/null
}
