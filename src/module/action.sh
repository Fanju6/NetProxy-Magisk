#!/system/bin/sh
#######################################
# 文件: action.sh
# 功能: 模块管理器中的操作按钮入口，交由 netproxyctl 切换服务状态。
# 用法: 由 Magisk/KernelSU/APatch 管理器点击模块操作按钮时调用。
# 依赖: netproxyctl、su
#######################################

readonly MODDIR="${0%/*}"
readonly NETPROXY_CTL="$MODDIR/netproxyctl"

[ -x "$NETPROXY_CTL" ] || {
  printf '%s\n' '缺少 netproxyctl，无法执行服务操作。' >&2
  exit 1
}

[ "$#" -eq 0 ] || {
  printf '%s\n' 'action.sh 仅供模块管理器调用。' >&2
  exit 2
}

printf '%s\n' '==================================='
printf '%s\n' ' NetProxy 服务操作'
printf '%s\n' '==================================='

# 服务状态与生命周期由 netproxyctl/Go 统一处理，Shell 不读取进程或 JSON 推断状态。
if command -v su > /dev/null 2>&1; then
  su -c "\"$NETPROXY_CTL\" service toggle" > /dev/null
  status=$?
else
  "$NETPROXY_CTL" service toggle > /dev/null
  status=$?
fi

if [ "$status" -eq 0 ]; then
  printf '%s\n' ' 操作结果: NetProxy 服务状态已切换'
else
  printf '%s\n' ' 操作结果: NetProxy 服务切换失败'
fi
printf '%s\n' '==================================='
exit "$status"
