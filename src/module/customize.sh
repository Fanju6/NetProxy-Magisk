#!/system/bin/sh
#######################################
# 文件: customize.sh
# 功能: NetProxy 模块安装脚本，由 Magisk/KernelSU/APatch 在刷入模块时执行：
#       备份/恢复配置、解压模块、清理旧数据面、同步到运行时目录、
#       设置权限，并按需安装配套应用。
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
    singbox/source/direct.json
    singbox/source/proxy.json
    singbox/source/block.json
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
    scripts/core/service.sh
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

  # 停止 Go Worker。
  pkill -f "netproxy-native.*subworker" 2> /dev/null || true

  # 检测当前 sing-box 进程。
  if pidof -s "$LIVE_DIR/bin/sing-box" > /dev/null 2>&1; then
    PROXY_WAS_RUNNING=true
    print_step "检测到代理服务正在运行，停止服务..."
    sh "$LIVE_DIR/scripts/core/service.sh" stop > /dev/null 2>&1
    print_ok "服务已停止"
  fi

  return 0
}

#######################################
# 同步新文件到运行目录 (支持热更新)
# 参数: 无
# 全局: 读取 MODPATH / LIVE_DIR
# 返回: 0 (首次安装时跳过)
#######################################
sync_to_live() {
  print_step "同步到运行时目录..."

  # 运行目录不存在 (首次安装) 则无需同步
  if [ ! -d "$LIVE_DIR" ]; then
    print_ok "首次安装，跳过同步"
    return 0
  fi

  # 同步程序文件与脚本，以及需要更新的内置资源 (整目录/文件覆盖)
  local sync_dirs="bin scripts netproxyctl action.sh service.sh uninstall.sh module.prop config/ebpf config/singbox/confdir config/singbox/source"

  for item in $sync_dirs; do
    local src="$MODPATH/$item"
    local dst="$LIVE_DIR/$item"

    if [ -e "$src" ]; then
      rm -rf "$dst" 2> /dev/null
      if cp -r "$src" "$dst" 2> /dev/null; then
        print_ok "已同步: $item"
      else
        print_warn "同步失败: $item"
      fi
    fi
  done

  # 增量更新配置目录中的新文件 (不覆盖已存在的)
  if [ -d "$MODPATH/config" ]; then
    print_step "增量更新配置..."

    cp -rn "$MODPATH/config/"* "$LIVE_DIR/config/" 2> /dev/null
    print_ok "配置目录已增量更新"
  fi

  return 0
}

#######################################
# 安装前若服务在运行，安装后重新启动
# 参数: 无
# 全局: 读取 PROXY_WAS_RUNNING
# 返回: 0
#######################################
restart_proxy_if_needed() {
  # 热更新安装无需等待重启设备，先拉起新版 Go 订阅 Worker。
  if [ -x "$LIVE_DIR/bin/netproxy-native" ]; then
    "$LIVE_DIR/bin/netproxy-native" subworker start \
      --root "$LIVE_DIR/data/catalog" \
      --progress-dir "/dev/netproxy/subscriptions" \
      --pid-file "/dev/netproxy/subworker.pid" \
      --log-file "$LIVE_DIR/logs/service.log" \
      --module-conf "$LIVE_DIR/config/module.conf" \
      --reload-script "$LIVE_DIR/scripts/core/service.sh" \
      --sing-box "$LIVE_DIR/bin/sing-box" > /dev/null 2>&1 || true
  fi
  if [ "$PROXY_WAS_RUNNING" = true ]; then
    print_step "重新启动代理服务..."
    # su 包裹：经管理器刷入时让 sing-box 迁出冻结 cgroup，避免切后台断网
    if su -c "sh \"$LIVE_DIR/scripts/core/service.sh\" start" > /dev/null 2>&1; then
      print_ok "服务已启动"
    else
      print_warn "服务未启动，请先导入可用节点"
    fi
  fi

  return 0
}

#######################################
# 设置模块文件权限
# 参数: 无
# 全局: 读取 EXECUTABLE_FILES / MODPATH / LIVE_DIR
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
      [ -e "$LIVE_DIR/$file" ] && chmod 0755 "$LIVE_DIR/$file" 2> /dev/null
    fi
  done

  # 用户配置与 Catalog 包含节点凭据、订阅地址和应用名单，仅允许 root 读取。
  [ ! -f "$MODPATH/config/module.conf" ] || chmod 0600 "$MODPATH/config/module.conf" 2> /dev/null
  [ ! -f "$MODPATH/config/ebpf/ebpf.conf" ] || chmod 0600 "$MODPATH/config/ebpf/ebpf.conf" 2> /dev/null
  [ ! -f "$LIVE_DIR/config/module.conf" ] || chmod 0600 "$LIVE_DIR/config/module.conf" 2> /dev/null
  [ ! -f "$LIVE_DIR/config/ebpf/ebpf.conf" ] || chmod 0600 "$LIVE_DIR/config/ebpf/ebpf.conf" 2> /dev/null
  [ ! -d "$MODPATH/data/catalog" ] \
    || set_perm_recursive "$MODPATH/data/catalog" 0 0 0700 0600
  [ ! -d "$LIVE_DIR/data/catalog" ] \
    || set_perm_recursive "$LIVE_DIR/data/catalog" 0 0 0700 0600
  [ ! -d "$MODPATH/runtime" ] \
    || set_perm_recursive "$MODPATH/runtime" 0 0 0700 0600
  [ ! -d "$LIVE_DIR/runtime" ] \
    || set_perm_recursive "$LIVE_DIR/runtime" 0 0 0700 0600

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
  && stop_proxy_if_running \
  && sync_to_live \
  && set_permissions \
  && restart_proxy_if_needed; then

  cleanup

  print_title "安装完成，请重启设备"

  # 询问是否安装配套应用
  ask_install_app
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
