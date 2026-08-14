#!/system/bin/sh
#######################################
# 文件: customize.sh
# 功能: NetProxy 模块安装脚本，由 Magisk/KernelSU/APatch 在刷入模块时执行：
#       备份/恢复用户状态、解压和校验新模块；在已开机环境中等待管理器
#       写入更新标记后，以原子目录切换立即应用更新，无需重启设备。
# 用法: 由管理器在安装模块时自动调用 (SKIPUNZIP=1 表示自行解压)。
# 说明: 运行于管理器提供的 busybox 环境，依赖 ui_print/grep_prop 等管理器函数。
#######################################

SKIPUNZIP=1  # 跳过管理器自动解压，由本脚本手动控制解压流程

################################################################################
# 常量定义
################################################################################

readonly MODULE_ID="netproxy"                       # 模块 ID
readonly MANAGER_PACKAGE="com.fanjv.netproxy"       # Android 管理器包名
readonly LIVE_DIR="/data/adb/modules/$MODULE_ID"    # 已安装模块的运行目录
readonly CONFIG_DIR="$LIVE_DIR/config"              # 运行目录下的配置目录
readonly BACKUP_DIR="$TMPDIR/netproxy_backup"       # 配置备份临时目录

# 全局状态: 安装前代理服务是否处于运行状态
PROXY_WAS_RUNNING=false

# 安装方式: preserve=保留现有用户数据，fresh=使用包内默认数据。
INSTALL_MODE=fresh

# 需要保留的配置文件/目录 (相对于 config/)
readonly DATA_DIR="$LIVE_DIR/data"

readonly PRESERVE_CONFIGS="
    module.conf
    ebpf/ebpf.conf
    singbox/confdir
    singbox/rules/local/direct.json
    singbox/rules/local/proxy.json
    singbox/rules/local/block.json
"

# 需要设置可执行权限的文件
readonly EXECUTABLE_FILES="
    bin/sing-box
    bin/netproxy-native
    bin/netproxyctl
    action.sh
    netproxyctl
    service.sh
    uninstall.sh
    bin/bpftool
"

################################################################################
# 工具函数
################################################################################

# 打印带分隔线的标题。参数: $1 标题文本
print_title() {
  ui_print ""
  ui_print "━━━━━━━━━━━━━━━━━━━━━━━━━"
  ui_print "  $1"
  ui_print "━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# 打印步骤提示。参数: $1 文本
print_step() {
  ui_print "▶ $1"
}

# 打印成功提示。参数: $1 文本
print_ok() {
  ui_print "  ✓ $1"
}

# 打印警告提示。参数: $1 文本
print_warn() {
  ui_print "  ⚠ $1"
}

# 打印错误提示。参数: $1 文本
print_error() {
  ui_print "  ✗ $1"
}

# 判断目录是否存在且非空。参数: $1 目录；返回: 0=非空
dir_not_empty() {
  [ -d "$1" ] && [ "$(ls -A "$1" 2> /dev/null)" ]
}

#######################################
# 设置单个文件的属主、权限与 SELinux 上下文
# 参数:
#   $1 路径  $2 属主  $3 属组  $4 权限  $5 SELinux 上下文 (可选)
# 返回: 任一步失败返回 1
#######################################
set_perm() {
  chown "$2:$3" "$1" || return 1
  chmod "$4" "$1" || return 1
  local CON="$5"
  # 未指定上下文时使用默认系统文件上下文
  [ -z "$CON" ] && CON="u:object_r:system_file:s0"
  chcon "$CON" "$1" || return 1
}

#######################################
# 递归设置目录的属主、权限与上下文
# 参数:
#   $1 目录  $2 属主  $3 属组  $4 目录权限  $5 文件权限  $6 上下文 (可选)
# 返回: 0=完成，1=任一项设置失败。
#######################################
set_perm_recursive() {
  # 先设置所有子目录权限
  # 模块路径由安装包控制，不包含换行；使用 POSIX read，兼容 Android mksh。
  find "$1" -type d -print 2>/dev/null | while IFS= read -r dir; do
    set_perm "$dir" "$2" "$3" "$4" "$6" || exit 1
  done || return 1

  # 再设置所有文件与符号链接权限
  find "$1" \( -type f -o -type l \) -print 2>/dev/null | while IFS= read -r file; do
    set_perm "$file" "$2" "$3" "$5" "$6" || exit 1
  done || return 1
  return 0
}

################################################################################
# 核心函数
################################################################################

#######################################
# 判断是否存在可由用户保留的数据。
# 参数: 无
# 返回: 0=存在，1=不存在。
#######################################
has_existing_user_data() {
  [ -f "$CONFIG_DIR/module.conf" ] \
    || [ -f "$CONFIG_DIR/ebpf/ebpf.conf" ] \
    || [ -d "$CONFIG_DIR/singbox/confdir" ] \
    || [ -d "$DATA_DIR/catalog" ]
}

#######################################
# 选择安装方式。
# 参数: 无
# 全局: 写入 INSTALL_MODE
# 返回: 始终返回 0。
#######################################
choose_install_mode() {
  if ! has_existing_user_data; then
    INSTALL_MODE=fresh
    print_step "未发现现有用户数据，将执行全新安装"
    return 0
  fi

  print_title "选择安装方式"
  ui_print ""
  ui_print "  [音量+] 保留现有数据 (默认)"
  ui_print "  [音量-] 全新安装"
  ui_print ""

  if [ "$(wait_volume_key 10)" = "down" ]; then
    INSTALL_MODE=fresh
    print_step "已选择全新安装"
  else
    INSTALL_MODE=preserve
    print_step "已选择保留现有数据"
  fi
}

#######################################
# 复制持久 Catalog 状态，忽略事务 staging 目录。
# 参数: $1 源 Catalog 目录  $2 目标 Catalog 目录
# 返回: 0=成功，1=复制失败。
#######################################
copy_catalog_state() {
  local source_dir="$1"
  local target_dir="$2"
  local group_dir

  [ -d "$source_dir" ] || return 1
  rm -rf "$target_dir" 2> /dev/null || return 1
  mkdir -p "$target_dir" || return 1

  for group_dir in "$source_dir"/*; do
    [ -e "$group_dir" ] || continue
    [ "$(basename "$group_dir")" = staging ] && continue
    cp -r "$group_dir" "$target_dir/" 2> /dev/null || return 1
  done
  return 0
}

#######################################
# 备份现有配置到临时目录
# 参数: 无
# 全局: 读取 INSTALL_MODE / CONFIG_DIR / PRESERVE_CONFIGS / BACKUP_DIR
# 返回: 0=成功或全新安装跳过，1=失败。
#######################################
backup_catalog_data() {
  [ -d "$DATA_DIR/catalog" ] || return 0
  mkdir -p "$BACKUP_DIR/data" || return 1
  if copy_catalog_state "$DATA_DIR/catalog" "$BACKUP_DIR/data/catalog"; then
    return 0
  fi
  print_error "Catalog 数据备份失败"
  return 1
}

restore_catalog_data() {
  [ -d "$BACKUP_DIR/data/catalog" ] || return 0
  mkdir -p "$MODPATH/data" || return 1
  if copy_catalog_state "$BACKUP_DIR/data/catalog" "$MODPATH/data/catalog"; then
    return 0
  fi
  print_error "Catalog 数据恢复失败"
  return 1
}

backup_config() {
  if [ "$INSTALL_MODE" != "preserve" ]; then
    print_step "全新安装不保留现有数据"
    return 0
  fi

  print_step "备份现有用户数据..."

  mkdir -p "$BACKUP_DIR" || return 1
  backup_catalog_data || return 1

  # 逐项备份需保留的配置
  local config_item
  for config_item in $PRESERVE_CONFIGS; do
    local src="$CONFIG_DIR/$config_item"
    local dst="$BACKUP_DIR/$config_item"

    if [ -e "$src" ]; then
      mkdir -p "$(dirname "$dst")"
      if cp -r "$src" "$dst" 2> /dev/null; then
        print_ok "已备份: $config_item"
      else
        print_error "备份失败: $config_item"
        return 1
      fi
    fi
  done

  return 0
}

#######################################
# 解压模块文件到安装目录
# 参数: 无
# 全局: 读取 ZIPFILE / MODPATH
# 返回: 成功 0，失败 1
#######################################
extract_module() {
  print_step "解压模块文件..."

  # 解压到安装临时目录，排除 META-INF 目录
  if ! unzip -o "$ZIPFILE" -x "META-INF/*" -d "$MODPATH" > /dev/null 2>&1; then
    print_error "解压失败"
    return 1
  fi

  print_ok "模块文件已解压"
  return 0
}

#######################################
# 将备份的配置恢复到新解压的模块目录
# 参数: 无
# 全局: 读取 INSTALL_MODE / BACKUP_DIR / PRESERVE_CONFIGS / MODPATH
# 返回: 0=成功或无备份时跳过，1=失败。
#######################################
restore_config() {
  [ "$INSTALL_MODE" = "preserve" ] || return 0
  restore_catalog_data || return 1

  # 无备份则跳过
  if ! dir_not_empty "$BACKUP_DIR"; then
    return 0
  fi

  print_step "恢复配置文件..."

  # 逐项恢复，覆盖解压出的默认配置
  local config_item
  for config_item in $PRESERVE_CONFIGS; do
    local src="$BACKUP_DIR/$config_item"
    local dst="$MODPATH/config/$config_item"

    if [ -e "$src" ]; then
      # 创建父目录
      mkdir -p "$(dirname "$dst")"
      # 删除目标 (防止目录嵌套)
      rm -rf "$dst" 2> /dev/null
      # 复制回配置
      if cp -r "$src" "$dst" 2> /dev/null; then
        print_ok "已恢复: $config_item"
      else
        print_error "恢复失败: $config_item"
        return 1
      fi
    fi
  done

  return 0
}

#######################################
# 清理安装前残留的后台 Worker 状态。
# 参数: 无
# 返回: 始终返回 0；只处理 /dev/netproxy 下由本模块记录的 Worker PID。
#######################################
cleanup_worker_state() {
  [ -d /dev/netproxy ] || return 0

  # 统一处理 /dev/netproxy 下的 Worker PID 文件，避免状态残留。
  for pid_file in /dev/netproxy/*worker.pid; do
    [ -f "$pid_file" ] || continue
    pid="$(cat "$pid_file" 2> /dev/null || true)"
    case "$pid" in
      ''|*[!0-9]*) pid='' ;;
    esac

    if [ -n "$pid" ] && [ -r "/proc/$pid/cmdline" ] \
      && grep -q "$LIVE_DIR/bin/netproxy-native" "/proc/$pid/cmdline" 2> /dev/null; then
      kill -TERM "$pid" 2> /dev/null || true
      wait_count=0
      while [ -d "/proc/$pid" ] && [ "$wait_count" -lt 10 ]; do
        sleep 1
        wait_count=$((wait_count + 1))
      done
    fi

    rm -f "$pid_file" "$pid_file.lock" 2> /dev/null || true
  done

  rm -rf /dev/netproxy/*worker.pid.lock 2> /dev/null || true
  return 0
}

#######################################
# 安装前停止正在运行的代理服务
# 参数: 无
# 全局: 检测 sing-box 进程，置 PROXY_WAS_RUNNING
# 返回: 0
#######################################
stop_proxy_if_running() {
  # 运行目录不存在 (首次安装) 则无需停止
  if [ ! -d "$LIVE_DIR" ]; then
    return 0
  fi

  # 检测当前 sing-box 进程。
  if pidof -s "$LIVE_DIR/bin/sing-box" > /dev/null 2>&1; then
    PROXY_WAS_RUNNING=true
    print_step "检测到代理服务正在运行，停止服务..."
    if "$LIVE_DIR/netproxyctl" service stop > /dev/null 2>&1; then
      print_ok "服务已停止"
    else
      print_error "服务停止失败，取消本次安装"
      PROXY_WAS_RUNNING=false
      return 1
    fi
  fi

  # 通过 Worker PID 文件停止后台调度，不按进程名误杀其他实例。
  if [ -x "$LIVE_DIR/bin/netproxy-native" ]; then
    "$LIVE_DIR/bin/netproxy-native" worker stop \
      --module-dir "$LIVE_DIR" > /dev/null 2>&1 || true
  fi
  cleanup_worker_state

  return 0
}

#######################################
# 在新会话中以 root 执行后台 Shell。
# 参数: 透传给 su 的参数。
# 返回: 不返回，exec 到后台 root Shell。
#######################################
launch_detached_root_shell() {
  if command -v setsid > /dev/null 2>&1; then
    exec setsid nohup su "$@"
  fi
  exec nohup su "$@"
}

#######################################
# 在 KernelSU 写入 update 标记后提交暂存模块。
# 参数: 无
# 全局: 读取 MODPATH / LIVE_DIR / PROXY_WAS_RUNNING / INSTALL_MODE / MODULE_ID
# 返回: 0=已安排后台提交，1=无法安排，保留 KernelSU 下次开机更新
#######################################
schedule_hot_update() {
  if ! command -v su > /dev/null 2>&1; then
    print_warn "无法启动后台热更新，将在下次开机时由管理器完成更新"
    return 1
  fi

  # setsid 和 nohup 先脱离安装器会话，su 再迁出管理器 cgroup。Worker 从标准
  # 输入读取，避免 customize.sh 被安装器清理后发生脚本文件竞争；它只在安装器
  # 退出且 update 标记出现后提交。
  (
    launch_detached_root_shell -c "/system/bin/sh -s -- '$$' '$MODPATH' '$LIVE_DIR' '$PROXY_WAS_RUNNING' '$INSTALL_MODE' '$MODULE_ID'" <<'NETPROXY_HOT_UPDATE_WORKER'
# NETPROXY_HOT_UPDATE_WORKER_BEGIN
set -u

[ "$#" -eq 6 ] || exit 2
installer_pid="$1"
stage_dir="$2"
live_dir="$3"
restart_service="$4"
install_mode="$5"
module_id="$6"
log_file="$live_dir/logs/service.log"

case "$install_mode" in
  preserve|fresh) ;;
  *) exit 2 ;;
esac

#######################################
# 写入后台热更新日志。
# 参数: $1 日志级别，$2 结果，$3 错误码或 -，$4 日志正文
# 返回: 始终返回 0，不影响更新回退。
#######################################
write_log() {
  mkdir -p "$(dirname "$log_file")" 2> /dev/null || return 0
  printf '[%s] [%s] [module] [module.update] [%s] [%s] %s\n' \
    "$(date '+%Y-%m-%d %H:%M:%S' 2> /dev/null || printf 'unknown-time')" "$1" "$2" "$3" "$4" \
    >> "$log_file" 2> /dev/null || true
}

#######################################
# 校验待提交模块包含最小运行入口。
# 参数: 无
# 返回: 0=有效，1=无效。
#######################################
stage_is_valid() {
  [ -d "$stage_dir" ] \
    && [ -f "$stage_dir/module.prop" ] \
    && grep -qx "id=$module_id" "$stage_dir/module.prop" \
    && [ -f "$stage_dir/netproxyctl" ] \
    && [ -f "$stage_dir/bin/netproxy-native" ] \
    && [ -f "$stage_dir/bin/sing-box" ]
}

#######################################
# 原子替换前复制一项最新持久状态。
# 参数: $1 源路径  $2 目标路径
# 返回: 0=成功或源不存在，1=复制失败。
#######################################
copy_persistent_entry() {
  source_path="$1"
  target_path="$2"
  [ -e "$source_path" ] || return 0
  rm -rf "$target_path" 2> /dev/null || return 1
  mkdir -p "$(dirname "$target_path")" || return 1
  cp -af "$source_path" "$target_path"
}

#######################################
# 复制持久 Catalog 状态，忽略事务 staging 目录。
# 参数: $1 源 Catalog 目录  $2 目标 Catalog 目录
# 返回: 0=成功，1=复制失败。
#######################################
copy_catalog_state() {
  source_dir="$1"
  target_dir="$2"
  [ -d "$source_dir" ] || return 1
  rm -rf "$target_dir" 2> /dev/null || return 1
  mkdir -p "$target_dir" || return 1

  for group_dir in "$source_dir"/*; do
    [ -e "$group_dir" ] || continue
    [ "$(basename "$group_dir")" = staging ] && continue
    cp -r "$group_dir" "$target_dir/" 2> /dev/null || return 1
  done
  return 0
}

#######################################
# 合并 live 目录在安装期间新增的用户状态。
# 参数: 无
# 返回: 0=成功或全新安装跳过，1=任一项复制失败。
#######################################
merge_live_state() {
  [ "$install_mode" = "preserve" ] || return 0
  [ -d "$live_dir" ] || return 0
  if [ -d "$live_dir/data/catalog" ]; then
    copy_catalog_state "$live_dir/data/catalog" "$stage_dir/data/catalog" || return 1
  fi

  for config_item in \
    module.conf \
    ebpf/ebpf.conf \
    singbox/confdir \
    singbox/rules/local/direct.json \
    singbox/rules/local/proxy.json \
    singbox/rules/local/block.json; do
    copy_persistent_entry "$live_dir/config/$config_item" "$stage_dir/config/$config_item" || return 1
  done
}

#######################################
# 热提交失败时恢复更新前正在运行的服务。
# 参数: 无
# 返回: 始终返回 0，不覆盖原始失败原因。
#######################################
restore_live_service() {
  [ "$restart_service" = true ] || return 0
  [ -x "$live_dir/netproxyctl" ] || return 0
  su -c "\"$live_dir/bin/netproxy-native\" worker start --module-dir \"$live_dir\"" > /dev/null 2>&1 || true
  su -c "\"$live_dir/netproxyctl\" service start" > /dev/null 2>&1 || true
}

#######################################
# 记录失败并保留管理器的下次开机更新路径。
# 参数: $1 失败原因
# 返回: 不返回，退出后台 Shell。
#######################################
fail_hot_update() {
  write_log "WARN" "failed" "module.update_failed" "后台热更新未提交: $1；保留待更新目录，下次开机将由管理器完成更新"
  restore_live_service
  exit 0
}

# KernelSU 在 customize.sh 返回后才写 live/update 并完成自己的清理。
elapsed=0
while [ -d "/proc/$installer_pid" ]; do
  [ "$elapsed" -lt 30 ] || fail_hot_update "等待安装器退出超时"
  sleep 1
  elapsed=$((elapsed + 1))
done

elapsed=0
while [ ! -f "$live_dir/update" ]; do
  [ "$elapsed" -lt 30 ] || fail_hot_update "未检测到更新标记"
  sleep 1
  elapsed=$((elapsed + 1))
done

# 给管理器完成 module.prop 复制和暂存目录清理留出稳定窗口。
sleep 3
stage_is_valid || fail_hot_update "暂存模块校验失败"
[ -f "$live_dir/update" ] || fail_hot_update "更新标记已被撤销"
merge_live_state || fail_hot_update "合并最新用户数据失败"

module_parent="$(dirname "$live_dir")"
backup_dir="$module_parent/.${module_id}.hot-update.$$"
rm -rf "$backup_dir" 2> /dev/null || fail_hot_update "无法清理旧热更新备份"

if [ -e "$live_dir" ] && ! mv "$live_dir" "$backup_dir"; then
  fail_hot_update "无法备份当前模块"
fi

if ! mv "$stage_dir" "$live_dir"; then
  if [ -e "$backup_dir" ] && [ ! -e "$live_dir" ]; then
    mv "$backup_dir" "$live_dir" || true
  fi
  fail_hot_update "无法切换新模块，已尝试恢复旧模块"
fi

rm -f "$live_dir/update"
rm -rf "$backup_dir" 2> /dev/null || true
write_log "INFO" "success" "-" "后台热更新已完成，无需重启设备"

if [ -x "$live_dir/bin/netproxy-native" ]; then
  su -c "\"$live_dir/bin/netproxy-native\" worker start --module-dir \"$live_dir\"" > /dev/null 2>&1 \
    || write_log "WARN" "failed" "worker.start_failed" "新版后台 Worker 启动失败"
fi

if [ "$restart_service" = true ]; then
  if su -c "\"$live_dir/netproxyctl\" service start" > /dev/null 2>&1; then
    write_log "INFO" "success" "-" "后台热更新后服务已恢复"
  else
    write_log "WARN" "failed" "service.start_failed" "后台热更新后服务未启动，请在管理器中检查节点配置"
  fi
fi
# NETPROXY_HOT_UPDATE_WORKER_END
NETPROXY_HOT_UPDATE_WORKER
  ) > /dev/null 2>&1 &

  return 0
}

#######################################
# 设置模块文件权限
# 参数: 无
# 全局: 读取 EXECUTABLE_FILES / MODPATH
# 返回: 0
#######################################
set_permissions() {
  print_step "设置文件权限..."

  # 先设置默认权限，再单独放开真正需要执行的入口。
  set_perm_recursive "$MODPATH" 0 0 0755 0644 || return 1

  local file
  for file in $EXECUTABLE_FILES; do
    local path="$MODPATH/$file"
    if [ -e "$path" ]; then
      chmod 0755 "$path" 2> /dev/null || return 1
    fi
  done

  # 用户配置与 Catalog 包含节点凭据、订阅地址和应用名单，仅允许 root 读取。
  [ ! -f "$MODPATH/config/module.conf" ] || chmod 0600 "$MODPATH/config/module.conf" 2> /dev/null || return 1
  [ ! -f "$MODPATH/config/ebpf/ebpf.conf" ] || chmod 0600 "$MODPATH/config/ebpf/ebpf.conf" 2> /dev/null || return 1
  [ ! -d "$MODPATH/data/catalog" ] \
    || set_perm_recursive "$MODPATH/data/catalog" 0 0 0700 0600 || return 1
  [ ! -d "$MODPATH/runtime" ] \
    || set_perm_recursive "$MODPATH/runtime" 0 0 0700 0600 || return 1

  print_ok "权限设置完成"
  return 0
}

#######################################
# 在限定时间内等待用户按音量键
# 参数:
#   $1  超时秒数 (可选，默认 10)
# 返回: 标准输出打印 up / down / timeout
#######################################
wait_volume_key() {
  local timeout="${1:-10}"
  local key event_file event_pid

  event_file="${TMPDIR:-/data/local/tmp}/netproxy_volume_key.$$"

  # 每秒轮询一次按键事件，避免无按键时被 getevent 无限阻塞。
  while [ "$timeout" -gt 0 ]; do
    : > "$event_file" || break
    getevent -lqc 1 > "$event_file" 2> /dev/null &
    event_pid=$!
    sleep 1
    key=$(cat "$event_file" 2> /dev/null)
    kill "$event_pid" 2> /dev/null || true
    wait "$event_pid" 2> /dev/null || true
    rm -f "$event_file"
    key=$(printf '%s\n' "$key" | grep -E "KEY_VOLUME(UP|DOWN)" | head -1)

    if echo "$key" | grep -q "VOLUMEUP"; then
      printf "up\n"
      return 0
    elif echo "$key" | grep -q "VOLUMEDOWN"; then
      printf "down\n"
      return 0
    fi

    timeout=$((timeout - 1))
  done

  # 超时未按键
  printf "timeout\n"
}

#######################################
# 读取已安装管理器的版本信息。
# 参数: 无
# 返回: 0=标准输出版本信息，1=管理器未安装或无法读取。
#######################################
get_installed_manager_version() {
  local package_dump version_name version_code

  pm path "$MANAGER_PACKAGE" > /dev/null 2>&1 || return 1
  package_dump="$(dumpsys package "$MANAGER_PACKAGE" 2> /dev/null)" || return 1
  version_name="$(printf '%s\n' "$package_dump" | sed -n 's/^[[:space:]]*versionName=\([^[:space:]]*\).*/\1/p' | head -n 1)"
  version_code="$(printf '%s\n' "$package_dump" | sed -n 's/^[[:space:]]*versionCode=\([0-9][0-9]*\).*/\1/p' | head -n 1)"

  [ -n "$version_name" ] || version_name="未知"
  [ -n "$version_code" ] || version_code="未知"
  printf '%s (versionCode %s)\n' "$version_name" "$version_code"
}

#######################################
# 按安装包内容选择是否安装随附管理器。
# 参数: 无
# 全局: 读取 MODPATH
# 返回: 始终返回 0。
#######################################
install_bundled_manager() {
  local installed_version

  print_title "安装 NetProxy 管理器"
  ui_print ""

  if [ ! -f "$MODPATH/NetProxy.apk" ]; then
    ui_print "  本安装包未随附 NetProxy 管理器"
    ui_print "  可稍后从 Google Play 安装管理器"
    return 0
  fi

  if installed_version="$(get_installed_manager_version)"; then
    ui_print "  已安装 NetProxy 管理器"
    ui_print "  当前版本: $installed_version"
    ui_print "  为避免覆盖现有安装，跳过随附 APK"
    rm -f "$MODPATH/NetProxy.apk"
    return 0
  fi

  ui_print "  本包随附 NetProxy 管理器 APK"
  ui_print "  [音量+] 安装 (默认)"
  ui_print "  [音量-] 跳过"
  ui_print ""

  if [ "$(wait_volume_key 10)" = "down" ]; then
    print_step "已跳过管理器安装"
    rm -f "$MODPATH/NetProxy.apk"
    return 0
  fi

  print_step "正在安装随附管理器..."
  if pm install -r "$MODPATH/NetProxy.apk" > /dev/null 2>&1; then
    print_ok "管理器安装成功"
  else
    print_warn "管理器安装失败，可稍后手动安装或使用 Google Play"
  fi

  # 随附 APK 仅用于刷入时安装，成功或跳过后都不保留在模块目录。
  rm -f "$MODPATH/NetProxy.apk"

  return 0
}

# 清理安装过程产生的临时文件
cleanup() {
  rm -rf "$BACKUP_DIR" 2> /dev/null
}

################################################################################
# 主流程
################################################################################

# 预解压 module.prop 以读取版本号 (须在打印版本前完成)
unzip -o "$ZIPFILE" "module.prop" -d "$TMPDIR" > /dev/null 2>&1

print_title "NetProxy - sing-box 透明代理"
ui_print ""
ui_print "  版本: $(grep_prop version "$TMPDIR/module.prop" 2> /dev/null || echo "未知")"

# 先停止旧服务，再替换模块文件，避免运行中的进程继续使用旧文件。
choose_install_mode
if [ "${BOOTMODE:-false}" = true ] && ! stop_proxy_if_running; then
  print_title "安装失败"
  ui_print ""
  ui_print "  旧服务未能安全停止，已取消模块替换"
  ui_print ""
  exit 1
fi

# 按顺序执行安装步骤，任一失败则进入失败分支
if backup_config \
  && extract_module \
  && restore_config \
  && set_permissions; then

  cleanup

  install_bundled_manager

  if [ "${BOOTMODE:-false}" = true ]; then
    if schedule_hot_update; then
      print_title "安装完成"
      ui_print "  正在后台应用新版本，无需重启设备"
      ui_print "  接下来约 3 秒请不要重启；若现在重启，"
      ui_print "  KernelSU 将在开机时按标准流程继续更新"
    else
      print_title "安装完成，将在下次开机时应用更新"
    fi
  else
    print_title "安装完成，请重启设备"
  fi
else
  # 安装失败：清理并提示反馈
  cleanup
  print_title "安装失败"
  ui_print ""
  ui_print "  请检查上述错误信息"
  ui_print "  并在 GitHub Issues 反馈"
  ui_print ""
  exit 1
fi
