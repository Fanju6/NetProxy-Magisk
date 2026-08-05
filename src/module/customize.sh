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
readonly LEGACY_CORE_NAME="x""ray"                  # 旧版内核名 (用于停止旧进程)
readonly LEGACY_WEB_DIR_NAME="web""root"            # 旧版 WebUI 目录名

# 全局状态: 安装前代理服务是否处于运行状态
PROXY_WAS_RUNNING=false
RESET_LEGACY_CONFIG=false

# 需要保留的配置文件/目录 (相对于 config/)
readonly PRESERVE_CONFIGS="
    module.conf
    ebpf/ebpf.conf
    catalog/
    singbox/source/direct.json
    singbox/source/proxy.json
    singbox/source/block.json
"

# 需要设置可执行权限的文件
readonly EXECUTABLE_FILES="
    bin/sing-box
    bin/netproxy-native
    action.sh
    netproxyctl
    uninstall.sh
    scripts/netproxyctl
    scripts/core/service.sh
    scripts/core/ebpf.sh
    scripts/core/switch.sh
    scripts/network/netmon.sh
    scripts/core/subscription.sh
    scripts/core/subworker.sh
    scripts/utils/state.sh
    scripts/utils/metadata.sh
    scripts/utils/gms_fix.sh
    scripts/utils/catalog.sh
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
backup_config() {
  print_step "检查现有配置..."

  # 配置目录为空视为全新安装，无需备份
  if ! dir_not_empty "$CONFIG_DIR"; then
    print_ok "全新安装，无需备份"
    return 0
  fi

  # 8.x Catalog 存在时按正常升级处理，只暂存当前版本仍受支持的配置。
  if [ ! -f "$CONFIG_DIR/catalog/default/meta.json" ] \
    || [ ! -f "$CONFIG_DIR/catalog/default/provider.json" ]; then
    RESET_LEGACY_CONFIG=true
    print_warn "检测到非 Catalog 配置，将直接初始化全新配置"
    return 0
  fi

  print_step "备份当前 Catalog 配置..."
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

  # 订阅 worker 与代理核心独立运行，安装前单独停止旧实例。
  if [ -f "$LIVE_DIR/scripts/core/subworker.sh" ]; then
    sh "$LIVE_DIR/scripts/core/subworker.sh" stop > /dev/null 2>&1 || true
  elif [ -f "$LIVE_DIR/scripts/core/subsched.sh" ]; then
    sh "$LIVE_DIR/scripts/core/subsched.sh" stop > /dev/null 2>&1 || true
  fi

  # 检测当前或旧版内核进程
  if pidof -s "$LIVE_DIR/bin/sing-box" > /dev/null 2>&1 || pidof -s "$LIVE_DIR/bin/$LEGACY_CORE_NAME" > /dev/null 2>&1; then
    PROXY_WAS_RUNNING=true
    print_step "检测到代理服务正在运行，停止服务..."
    sh "$LIVE_DIR/scripts/core/service.sh" stop > /dev/null 2>&1
    print_ok "服务已停止"
  fi

  # 即使核心已经异常退出，也让旧脚本清理可能残留的防火墙与策略路由
  if [ -f "$LIVE_DIR/scripts/network/tproxy.sh" ] && [ -d "$LIVE_DIR/config/tproxy" ]; then
    sh "$LIVE_DIR/scripts/network/tproxy.sh" stop -d "$LIVE_DIR/config/tproxy" > /dev/null 2>&1 || true
  fi

  return 0
}

#######################################
# 清理旧版 TPROXY 与 IPSET 文件
# 参数: 无
# 返回: 0
#######################################
cleanup_legacy_dataplane() {
  print_step "清理旧版透明代理组件..."

  rm -rf "$LIVE_DIR/config/tproxy" \
    "$LIVE_DIR/bin/IPSET-LKM" \
    "/data/adb/netfilter" \
    2> /dev/null || true
  rm -f "$LIVE_DIR/scripts/network/tproxy.sh" \
    "$LIVE_DIR/scripts/utils/ipset.sh" \
    "$LIVE_DIR/scripts/core/subsched.sh" \
    "$LIVE_DIR/post-fs-data.sh" \
    "/data/adb/ksu/bin/ipset" \
    "/data/adb/ap/bin/ipset" \
    2> /dev/null || true

  print_ok "旧版透明代理组件已清理"
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

  # API 地址与密钥已固定在 sing-box 配置中，不再保留旧凭据目录。
  rm -rf "$LIVE_DIR/config/api" 2> /dev/null || true

  # 非 Catalog 配置不迁移，运行目录直接改用全新配置。
  if [ "$RESET_LEGACY_CONFIG" = true ]; then
    rm -rf "$LIVE_DIR/config" 2> /dev/null || true
    if cp -r "$MODPATH/config" "$LIVE_DIR/config" 2> /dev/null; then
      print_ok "已初始化全新 Catalog 配置"
    else
      print_error "初始化 Catalog 配置失败"
      return 1
    fi
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
  # 热更新安装无需等待重启设备，先拉起新版独立订阅 worker。
  if [ -f "$LIVE_DIR/scripts/core/subworker.sh" ]; then
    sh "$LIVE_DIR/scripts/core/subworker.sh" start > /dev/null 2>&1 || true
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

  # 为可执行文件设置 0755 (同时同步运行目录中的同名文件)
  local file
  for file in $EXECUTABLE_FILES; do
    local path="$MODPATH/$file"
    if [ -e "$path" ]; then
      chmod 0755 "$path" 2> /dev/null
      [ -e "$LIVE_DIR/$file" ] && chmod 0755 "$LIVE_DIR/$file" 2> /dev/null
    fi
  done

  # 递归设置整个模块目录的默认属主与权限
  set_perm_recursive "$MODPATH" 0 0 0755 0755

  # Catalog 中包含节点凭据、订阅地址与自定义请求头，仅允许 root 读取。
  [ ! -d "$MODPATH/config/catalog" ] \
    || set_perm_recursive "$MODPATH/config/catalog" 0 0 0700 0600
  [ ! -d "$LIVE_DIR/config/catalog" ] \
    || set_perm_recursive "$LIVE_DIR/config/catalog" 0 0 0700 0600
  [ ! -d "$MODPATH/config/runtime" ] \
    || set_perm_recursive "$MODPATH/config/runtime" 0 0 0700 0600
  [ ! -d "$LIVE_DIR/config/runtime" ] \
    || set_perm_recursive "$LIVE_DIR/config/runtime" 0 0 0700 0600

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
  && cleanup_legacy_dataplane \
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
