#!/system/bin/sh
# NetProxy Magisk Module 安装脚本

SKIPUNZIP=1

################################################################################
# 常量定义
################################################################################

readonly MODULE_ID="netproxy"
readonly LIVE_DIR="/data/adb/modules/$MODULE_ID"
readonly CONFIG_DIR="$LIVE_DIR/config"
readonly BACKUP_DIR="$TMPDIR/netproxy_backup"

# 全局状态: Xray 是否在运行
XRAY_WAS_RUNNING=false

# 需要保留的配置文件/目录 (相对于 config/)
readonly PRESERVE_CONFIGS="
    module.conf
    routing_rules.json
    tproxy.conf
    xray/outbounds
    xray/confdir/02_dns.json
    xray/confdir/03_routing.json
"

# 需要设置可执行权限的文件
readonly EXECUTABLE_FILES="
    bin/xray
    bin/proxylink
    action.sh
    scripts/cli
    scripts/core/service.sh
    scripts/core/switch-config.sh
    scripts/core/switch-mode.sh
    scripts/network/tproxy.sh
    scripts/config/subscription.sh
    scripts/utils/update-xray.sh
    scripts/utils/oneplus_a16_fix.sh
"

################################################################################
# 工具函数
################################################################################

# 打印带分隔线的标题
print_title() {
  ui_print ""
  ui_print "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  ui_print "  $1"
  ui_print "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# 打印步骤
print_step() {
  ui_print "▶ $1"
}

# 打印成功
print_ok() {
  ui_print "  ✓ $1"
}

# 打印警告
print_warn() {
  ui_print "  ⚠ $1"
}

# 打印错误
print_error() {
  ui_print "  ✗ $1"
}

# 检查目录是否非空
dir_not_empty() {
  [ -d "$1" ] && [ "$(ls -A "$1" 2> /dev/null)" ]
}

################################################################################
# 核心函数
################################################################################

# 备份现有配置
backup_config() {
  print_step "检查现有配置..."

  if ! dir_not_empty "$CONFIG_DIR"; then
    print_ok "全新安装，无需备份"
    return 0
  fi

  print_step "备份现有配置..."
  mkdir -p "$BACKUP_DIR"

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

# 解压模块文件
extract_module() {
  print_step "解压模块文件..."

  # 解压到 $MODPATH (Magisk 临时目录，重启后会复制到 $LIVE_DIR)
  if ! unzip -o "$ZIPFILE" -d "$MODPATH" > /dev/null 2>&1; then
    print_error "解压失败"
    return 1
  fi

  print_ok "模块文件已解压"
  return 0
}

# 恢复配置文件
restore_config() {
  if ! dir_not_empty "$BACKUP_DIR"; then
    return 0
  fi

  print_step "恢复配置文件..."

  local config_item
  for config_item in $PRESERVE_CONFIGS; do
    local src="$BACKUP_DIR/$config_item"
    local dst="$MODPATH/config/$config_item"

    if [ -e "$src" ]; then
      # 创建父目录
      mkdir -p "$(dirname "$dst")"
      # 删除目标 (防止目录嵌套)
      rm -rf "$dst" 2> /dev/null
      # 复制
      if cp -r "$src" "$dst" 2> /dev/null; then
        print_ok "已恢复: $config_item"
      else
        print_warn "恢复失败: $config_item"
      fi
    fi
  done

  return 0
}

# 停止 Xray 服务 (如果运行中)
stop_xray_if_running() {
  # 如果 LIVE_DIR 不存在，无需停止
  if [ ! -d "$LIVE_DIR" ]; then
    return 0
  fi

  if pidof -s "$LIVE_DIR/bin/xray" > /dev/null 2>&1; then
    XRAY_WAS_RUNNING=true
    print_step "检测到 Xray 正在运行，停止服务..."
    sh "$LIVE_DIR/scripts/core/service.sh" stop > /dev/null 2>&1
    print_ok "服务已停止"
  fi

  return 0
}

# 同步到运行时目录 (热更新支持)
sync_to_live() {
  print_step "同步到运行时目录..."

  # 如果 LIVE_DIR 不存在，首次安装无需同步
  if [ ! -d "$LIVE_DIR" ]; then
    print_ok "首次安装，跳过同步"
    return 0
  fi

  # 同步非配置文件 (bin, scripts, webroot 等)
  local sync_dirs="bin scripts webroot action.sh service.sh module.prop"

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

  # 同步配置目录中的新文件 (增量更新)
  if [ -d "$MODPATH/config" ]; then
    print_step "增量更新配置..."

    # 复制新增的配置文件 (不覆盖已存在的)
    cp -rn "$MODPATH/config/"* "$LIVE_DIR/config/" 2> /dev/null
    print_ok "配置目录已增量更新"
  fi

  return 0
}

# 重新启动 Xray 服务 (如果之前在运行)
restart_xray_if_needed() {
  if [ "$XRAY_WAS_RUNNING" = true ]; then
    print_step "重新启动 Xray 服务..."
    sh "$LIVE_DIR/scripts/core/service.sh" start > /dev/null 2>&1
    print_ok "服务已启动"
  fi

  return 0
}

# 设置文件权限
set_permissions() {
  print_step "设置文件权限..."

  local file
  for file in $EXECUTABLE_FILES; do
    local path="$MODPATH/$file"
    if [ -e "$path" ]; then
      chmod 0755 "$path" 2> /dev/null
      # 同时设置 LIVE_DIR 的权限
      [ -e "$LIVE_DIR/$file" ] && chmod 0755 "$LIVE_DIR/$file" 2> /dev/null
    fi
  done

  # 设置目录权限
  set_perm_recursive "$MODPATH" 0 0 0755 0755

  print_ok "权限设置完成"
  return 0
}

# 询问用户是否安装配套应用
ask_install_app() {
  ui_print ""
  ui_print "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  ui_print "  是否安装 NetProxy 配套应用？"
  ui_print "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  ui_print ""
  ui_print "  [音量+] 安装 (打开 Google Play)"
  ui_print "  [音量-] 跳过"
  ui_print ""

  local timeout=10
  local choice=""

  while [ $timeout -gt 0 ]; do
    # 读取音量键
    local key=$(getevent -lqc 1 2> /dev/null | grep -E "KEY_VOLUME(UP|DOWN)" | head -1)

    if echo "$key" | grep -q "VOLUMEUP"; then
      choice="install"
      break
    elif echo "$key" | grep -q "VOLUMEDOWN"; then
      choice="skip"
      break
    fi

    sleep 1
    timeout=$((timeout - 1))
  done

  if [ "$choice" = "install" ]; then
    print_step "正在打开 Google Play..."
    am start -a android.intent.action.VIEW -d "https://play.google.com/store/apps/details?id=www.netproxy.web.ui" > /dev/null 2>&1
    print_ok "已打开 Google Play"
  else
    print_step "已跳过安装"
  fi

  return 0
}

# 清理临时文件
cleanup() {
  rm -rf "$BACKUP_DIR" 2> /dev/null
}

################################################################################
# 主流程
################################################################################

print_title "NetProxy - Xray 透明代理"
ui_print "  版本: $(grep_prop version "$TMPDIR/module.prop" 2> /dev/null || echo "unknown")"

# 解压 module.prop 读取版本
unzip -o "$ZIPFILE" "module.prop" -d "$TMPDIR" > /dev/null 2>&1

# 执行安装步骤
if backup_config \
  && extract_module \
  && restore_config \
  && stop_xray_if_running \
  && sync_to_live \
  && set_permissions \
  && restart_xray_if_needed; then

  cleanup

  print_title "安装完成，无需重启设备"

  # 询问是否安装配套应用
  ask_install_app
else
  cleanup
  print_title "安装失败"
  ui_print ""
  ui_print "  请检查上述错误信息"
  ui_print "  并在 GitHub Issues 反馈"
  ui_print ""
  exit 1
fi
