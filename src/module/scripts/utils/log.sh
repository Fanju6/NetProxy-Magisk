#!/system/bin/sh

#######################################
# 标准日志函数
# 用法:
#   log <消息内容>         -> 默认使用 INFO 级别
#   log <级别> <消息内容>  -> 指定级别 (INFO, WARN, ERROR, DEBUG)
# 全局变量:
#   LOG_FILE - 日志文件路径 (可选)
# 输出:
#   如果定义了 LOG_FILE，将格式化后的日志追加到该文件
#   同时将格式化后的日志输出到标准错误 (stderr)
#######################################
log() {
  local level="INFO"
  local message="$1"

  # 如果提供了两个或更多参数，第一个参数作为级别，第二个参数作为消息
  if [ $# -ge 2 ]; then
    level="$1"
    message="$2"
  fi

  local timestamp
  timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
  local log_content="[$timestamp] [$level] $message"

  # 如果 LOG_FILE 变量非空，则追加到日志文件
  [ -n "${LOG_FILE:-}" ] && echo "$log_content" >> "$LOG_FILE"

  # 将日志内容输出到标准错误 (stderr) 这允许在控制台或父进程中捕获日志输出
  echo "$log_content" >&2
}
