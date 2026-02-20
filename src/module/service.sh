#!/system/bin/sh
set -e

readonly MAX_WAIT=60
readonly MODDIR="${0%/*}"
readonly MODULE_CONF="$MODDIR/config/module.conf"
readonly LOG_FILE="$MODDIR/logs/service.log"

. "$MODDIR/scripts/utils/log.sh"

#######################################
# 加载模块配置
#######################################
load_module_config() {
  # 默认值
  AUTO_START=1
  ONEPLUS_A16_FIX=1

  if [ -f "$MODULE_CONF" ]; then
    . "$MODULE_CONF"
    log "INFO" "模块配置已加载"
  else
    log "WARN" "模块配置文件不存在，使用默认值"
  fi
}

#######################################
# 等待系统启动完成
# Returns:
#   0 成功, 1 超时
#######################################
wait_for_boot() {
  local count=0

  log "INFO" "等待系统启动完成..."

  # 等待系统开机完成
  while [ "$(getprop sys.boot_completed)" != "1" ]; do
    sleep 1
    count=$((count + 1))
    [ "$count" -ge "$MAX_WAIT" ] && return 1
  done
  log "INFO" "系统启动完成 (耗时 ${count}s)"

  # 等待存储挂载完成
  count=0
  while [ ! -d "/sdcard/Android" ]; do
    sleep 1
    count=$((count + 1))
    [ "$count" -ge "$MAX_WAIT" ] && return 1
  done
  log "INFO" "存储挂载完成"

  return 0
}

#######################################
# 执行设备特定修复脚本
#######################################
check_device_specific() {
  # 如果启用 OnePlus A16 修复，直接执行
  if [ "$ONEPLUS_A16_FIX" = "1" ]; then
    log "INFO" "OnePlus A16 修复已启用，执行修复脚本"
    sh "$MODDIR/scripts/utils/oneplus_a16_fix.sh"
  fi
}

# 确保日志目录存在
mkdir -p "$MODDIR/logs"

#######################################
# 记录环境信息
#######################################
log_env_info() {
  log "INFO" "========== 环境信息检测 =========="

  # KernelSU
  if [ "$KSU" = "true" ]; then
    log "INFO" "环境: KernelSU"
    log "INFO" "KSU_VER: ${KSU_VER:-unknown}"
    log "INFO" "KSU_VER_CODE: ${KSU_VER_CODE:-unknown}"
    log "INFO" "KSU_KERNEL_VER_CODE: ${KSU_KERNEL_VER_CODE:-unknown}"
  fi

  # APatch
  if [ "$APATCH" = "true" ] || [ "$KERNELPATCH" = "true" ]; then
    log "INFO" "环境: APatch / KernelPatch"
    log "INFO" "APATCH_VER: ${APATCH_VER:-unknown}"
    log "INFO" "APATCH_VER_CODE: ${APATCH_VER_CODE:-unknown}"
    log "INFO" "KERNEL_VERSION: ${KERNEL_VERSION:-unknown}"
    log "INFO" "KERNELPATCH_VERSION: ${KERNELPATCH_VERSION:-unknown}"
  fi

  # Magisk
  if [ -n "$MAGISK_VER" ]; then
    log "INFO" "环境: Magisk"
    log "INFO" "MAGISK_VER: $MAGISK_VER"
    log "INFO" "MAGISK_VER_CODE: $MAGISK_VER_CODE"
  fi

  # Module Info
  if [ -f "$MODDIR/module.prop" ]; then
    local version=$(grep "^version=" "$MODDIR/module.prop" | cut -d= -f2)
    local versionCode=$(grep "^versionCode=" "$MODDIR/module.prop" | cut -d= -f2)
    log "INFO" "VERSION: ${version:-unknown}"
    log "INFO" "VERSION_CODE: ${versionCode:-unknown}"
  fi

  log "INFO" "=================================="
}

# 主流程
log "INFO" "========== NetProxy 服务启动 =========="
log_env_info
load_module_config

if wait_for_boot; then
  # 执行OnePlus A16修复
  check_device_specific

  # 检查是否启用开机自启
  if [ "$AUTO_START" = "1" ]; then
    log "INFO" "开始启动服务..."
    sh "$MODDIR/scripts/core/service.sh" start
    log "INFO" "服务启动完成"
  else
    log "INFO" "开机自启已禁用，跳过启动"
  fi

  log "INFO" "========== 服务启动流程结束 =========="
else
  log "ERROR" "系统启动超时，无法启动 NetProxy"
  exit 1
fi
