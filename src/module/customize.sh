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
readonly LIVE_DIR="/data/adb/modules/$MODULE_ID"    # 已安装模块的运行目录
readonly CONFIG_DIR="$LIVE_DIR/config"              # 运行目录下的配置目录
readonly BACKUP_DIR="$TMPDIR/netproxy_backup"       # 配置备份临时目录

# 全局状态: 安装前代理服务是否处于运行状态
PROXY_WAS_RUNNING=false

# 需要保留的配置文件/目录 (相对于 config/)
readonly DATA_DIR="$LIVE_DIR/data"

readonly PRESERVE_CONFIGS="
    module.conf
    ebpf/ebpf.conf
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
# 返回: 无
#######################################
set_perm_recursive() {
  # 先设置所有子目录权限
  find "$1" -type d -print0 2>/dev/null | while IFS= read -r -d '' dir; do
    set_perm "$dir" "$2" "$3" "$4" "$6"
  done

  # 再设置所有文件与符号链接权限
  find "$1" \( -type f -o -type l \) -print0 2>/dev/null | while IFS= read -r -d '' file; do
    set_perm "$file" "$2" "$3" "$5" "$6"
  done
}

################################################################################
# 核心函数
################################################################################

#######################################
# 备份现有配置到临时目录
# 参数: 无
# 全局: 读取 CONFIG_DIR / PRESERVE_CONFIGS / BACKUP_DIR
# 返回: 0 (全新安装时跳过)
#######################################
backup_catalog_data() {
  [ -d "$DATA_DIR/catalog" ] || return 0
  mkdir -p "$BACKUP_DIR/data"
  rm -rf "$BACKUP_DIR/data/catalog" 2> /dev/null || true
  cp -r "$DATA_DIR/catalog" "$BACKUP_DIR/data/catalog" 2> /dev/null || print_warn "Catalog 数据备份失败"
}

restore_catalog_data() {
  [ -d "$BACKUP_DIR/data/catalog" ] || return 0
  mkdir -p "$MODPATH/data"
  rm -rf "$MODPATH/data/catalog" 2> /dev/null || true
  cp -r "$BACKUP_DIR/data/catalog" "$MODPATH/data/catalog" 2> /dev/null || print_warn "Catalog 数据恢复失败"
}

backup_config() {
  print_step "检查现有配置..."

  # 配置目录为空视为全新安装，无需备份
  if ! dir_not_empty "$CONFIG_DIR" && ! dir_not_empty "$DATA_DIR"; then
    print_ok "全新安装，无需备份"
    return 0
  fi

  backup_catalog_data
  print_step "备份当前配置..."
  mkdir -p "$BACKUP_DIR"

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
        print_warn "备份失败: $config_item"
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
# 全局: 读取 BACKUP_DIR / PRESERVE_CONFIGS / MODPATH
# 返回: 0 (无备份时跳过)
#######################################
restore_config() {
  restore_catalog_data

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
        print_warn "恢复失败: $config_item"
      fi
    fi
  done

  return 0
}

#######################################
# 安装前停止正在运行的代理服务
# 参数: 无
# 全局: 检测新旧内核进程，置 PROXY_WAS_RUNNING
# 返回: 0
#######################################
stop_proxy_if_running() {
  # 运行目录不存在 (首次安装) 则无需停止
  if [ ! -d "$LIVE_DIR" ]; then
    return 0
  fi

  # 通过 Worker PID 文件停止订阅调度，不按进程名误杀其他实例。
  if [ -x "$LIVE_DIR/bin/netproxy-native" ]; then
    "$LIVE_DIR/bin/netproxy-native" subworker stop \
      --module-dir "$LIVE_DIR" > /dev/null 2>&1 || true
  fi

  # 检测当前 sing-box 进程。
  if pidof -s "$LIVE_DIR/bin/sing-box" > /dev/null 2>&1; then
    PROXY_WAS_RUNNING=true
    print_step "检测到代理服务正在运行，停止服务..."
    "$LIVE_DIR/netproxyctl" service stop > /dev/null 2>&1
    print_ok "服务已停止"
  fi

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
# 全局: 读取 MODPATH / LIVE_DIR / PROXY_WAS_RUNNING / MODULE_ID
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
    launch_detached_root_shell -c "/system/bin/sh -s -- '$$' '$MODPATH' '$LIVE_DIR' '$PROXY_WAS_RUNNING' '$MODULE_ID'" <<'NETPROXY_HOT_UPDATE_WORKER'
# NETPROXY_HOT_UPDATE_WORKER_BEGIN
set -u

[ "$#" -eq 5 ] || exit 2
installer_pid="$1"
stage_dir="$2"
live_dir="$3"
restart_service="$4"
module_id="$5"
log_file="$live_dir/logs/service.log"

#######################################
# 写入后台热更新日志。
# 参数: $1 日志正文
# 返回: 始终返回 0，不影响更新回退。
#######################################
write_log() {
  mkdir -p "$(dirname "$log_file")" 2> /dev/null || return 0
  printf '[%s] [INFO] %s\n' "$(date '+%Y-%m-%d %H:%M:%S' 2> /dev/null || printf 'unknown-time')" "$1" >> "$log_file" 2> /dev/null || true
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
# 合并 live 目录在安装期间新增的用户状态。
# 参数: 无
# 返回: 0=成功，1=任一项复制失败。
#######################################
merge_live_state() {
  [ -d "$live_dir" ] || return 0
  copy_persistent_entry "$live_dir/data/catalog" "$stage_dir/data/catalog" || return 1

  for config_item in \
    module.conf \
    ebpf/ebpf.conf \
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
  su -c "\"$live_dir/bin/netproxy-native\" subworker start --module-dir \"$live_dir\"" > /dev/null 2>&1 || true
  su -c "\"$live_dir/netproxyctl\" service start" > /dev/null 2>&1 || true
}

#######################################
# 记录失败并保留管理器的下次开机更新路径。
# 参数: $1 失败原因
# 返回: 不返回，退出后台 Shell。
#######################################
fail_hot_update() {
  write_log "后台热更新未提交: $1；保留待更新目录，下次开机将由管理器完成更新"
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
write_log "后台热更新已完成，无需重启设备"

if [ -x "$live_dir/bin/netproxy-native" ]; then
  su -c "\"$live_dir/bin/netproxy-native\" subworker start --module-dir \"$live_dir\"" > /dev/null 2>&1 \
    || write_log "新版订阅 Worker 启动失败"
fi

if [ "$restart_service" = true ]; then
  if su -c "\"$live_dir/netproxyctl\" service start" > /dev/null 2>&1; then
    write_log "后台热更新后服务已恢复"
  else
    write_log "后台热更新后服务未启动，请在管理器中检查节点配置"
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
  set_perm_recursive "$MODPATH" 0 0 0755 0644

  local file
  for file in $EXECUTABLE_FILES; do
    local path="$MODPATH/$file"
    if [ -e "$path" ]; then
      chmod 0755 "$path" 2> /dev/null
    fi
  done

  # 用户配置与 Catalog 包含节点凭据、订阅地址和应用名单，仅允许 root 读取。
  [ ! -f "$MODPATH/config/module.conf" ] || chmod 0600 "$MODPATH/config/module.conf" 2> /dev/null
  [ ! -f "$MODPATH/config/ebpf/ebpf.conf" ] || chmod 0600 "$MODPATH/config/ebpf/ebpf.conf" 2> /dev/null
  [ ! -d "$MODPATH/data/catalog" ] \
    || set_perm_recursive "$MODPATH/data/catalog" 0 0 0700 0600
  [ ! -d "$MODPATH/runtime" ] \
    || set_perm_recursive "$MODPATH/runtime" 0 0 0700 0600

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
  local key

  # 每秒轮询一次按键事件，捕获到音量键即返回
  while [ "$timeout" -gt 0 ]; do
    key=$(getevent -lqc 1 2> /dev/null | grep -E "KEY_VOLUME(UP|DOWN)" | head -1)

    if echo "$key" | grep -q "VOLUMEUP"; then
      printf "up\n"
      return 0
    elif echo "$key" | grep -q "VOLUMEDOWN"; then
      printf "down\n"
      return 0
    fi

    sleep 1
    timeout=$((timeout - 1))
  done

  # 超时未按键
  printf "timeout\n"
}

#######################################
# 询问用户是否安装配套应用 (音量键交互)
# 参数: 无
# 返回: 0 (无论安装与否)
#######################################
ask_install_app() {
  print_title "是否安装 NetProxy 配套应用？"
  ui_print ""
  ui_print "  [音量+] 安装 (默认)"
  ui_print "  [音量-] 跳过"
  ui_print ""

  # 等待选择：音量- 跳过，音量+ 或超时则安装
  if [ "$(wait_volume_key 10)" = "down" ]; then
    print_step "已跳过安装"
    rm -f "$MODPATH/NetProxy.apk"
    return 0
  fi

  # 二次选择：模块内安装 还是 跳转 Google Play
  sleep 1

  print_title "选择安装来源"
  ui_print ""
  ui_print "  [音量+] 模块内安装 (默认，含广告)"
  ui_print "  [音量-] Google Play (无广告)"
  ui_print ""

  # 等待选择：音量- 选 Google Play，音量+ 或超时则模块内安装
  local source="module"
  [ "$(wait_volume_key 10)" = "down" ] && source="play"

  # 模块内安装：调用 pm 安装内置 APK
  if [ "$source" = "module" ] && [ -f "$MODPATH/NetProxy.apk" ]; then
    print_step "正在安装模块内应用..."
    if pm install -r "$MODPATH/NetProxy.apk" > /dev/null 2>&1; then
      print_ok "应用安装成功"
    else
      print_warn "应用安装失败，请手动安装"
    fi
  else
    # 否则跳转到 Google Play 页面
    print_step "正在打开 Google Play..."
    am start -a android.intent.action.VIEW -d "https://play.google.com/store/apps/details?id=com.fanjv.netproxy" > /dev/null 2>&1
    print_ok "已打开 Google Play"
  fi

  # 清理安装包以减小模块体积
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
ui_print "  版本: $(grep_prop version "$TMPDIR/module.prop" 2> /dev/null || echo "未知")"

# 按顺序执行安装步骤，任一失败则进入失败分支
if backup_config \
  && extract_module \
  && restore_config \
  && set_permissions; then

  cleanup

  # 询问是否安装配套应用
  ask_install_app

  if [ "${BOOTMODE:-false}" = true ]; then
    stop_proxy_if_running
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
