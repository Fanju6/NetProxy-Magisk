#!/system/bin/sh
set -e
set -u

#############################################################################
# OnePlus Android 16 兼容性修复脚本
# 功能: 清理 fw_INPUT/fw_OUTPUT 链中可能阻止代理工作的 REJECT 规则
# 说明: OnePlus Android 16 (ColorOS 16) 系统可能在 filter 表中添加 REJECT
#       规则，导致 TProxy 透明代理无法正常工作。此脚本用于清理这些规则。
#############################################################################

readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly LOG_FILE="$MODDIR/logs/service.log"

# 导入工具库
. "$MODDIR/scripts/utils/log.sh"

#######################################
# 清理指定链中的 REJECT 规则
# Arguments:
#   $1 - iptables 命令 (iptables / ip6tables)
#   $2 - 链名
#######################################
remove_reject_from_chain() {
  local cmd="$1"
  local chain="$2"
  local table="filter"

  # 一次性获取所有 REJECT 规则的行号（倒序，避免删除时行号错位）
  local line_numbers
  line_numbers=$(
    $cmd -t "$table" -nvL "$chain" --line-numbers 2> /dev/null \
      | awk '/REJECT/ {print $1}' \
      | sort -rn
  )

  if [ -z "$line_numbers" ]; then
    log "INFO" "$cmd: $chain 链中未发现 REJECT 规则"
    return 0
  fi

  # 统计删除数量
  local count=0

  # 逐行删除（已倒序，从大到小删除不会影响行号）
  for line_num in $line_numbers; do
    if $cmd -t "$table" -D "$chain" "$line_num" 2> /dev/null; then
      log "INFO" "已删除 ($cmd) $chain 第 $line_num 行 REJECT 规则"
      count=$((count + 1))
    else
      log "WARN" "删除失败 ($cmd) $chain 第 $line_num 行"
    fi
  done

  log "INFO" "$cmd: $chain 链共删除 $count 条 REJECT 规则"
}

#######################################
# 主清理函数
#######################################
remove_reject_rules() {
  local chains="fw_INPUT fw_OUTPUT"

  # 预检查命令是否存在
  local has_iptables=0
  local has_ip6tables=0

  command -v iptables > /dev/null 2>&1 && has_iptables=1
  command -v ip6tables > /dev/null 2>&1 && has_ip6tables=1

  if [ "$has_iptables" -eq 0 ] && [ "$has_ip6tables" -eq 0 ]; then
    log "ERROR" "iptables 和 ip6tables 命令均不存在"
    return 1
  fi

  for chain in $chains; do
    [ "$has_iptables" -eq 1 ] && remove_reject_from_chain "iptables" "$chain"
    [ "$has_ip6tables" -eq 1 ] && remove_reject_from_chain "ip6tables" "$chain"
  done
}

#######################################
# 主流程
#######################################
log "INFO" "========== OnePlus A16 兼容性修复：开始清理 REJECT 规则 =========="

remove_reject_rules

log "INFO" "========== OnePlus A16 兼容性修复：REJECT 规则清理完成 =========="
