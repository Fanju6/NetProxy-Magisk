#!/system/bin/sh
#######################################
# 文件: service.sh
# 功能: Magisk service 阶段入口，在系统启动完成后执行：记录运行环境、
#       加载模块配置，按需开机自启 sing-box 服务并执行设备兼容性修复。
# 用法: 由 Magisk/KernelSU/APatch 在 service 阶段自动调用。
# 依赖: common.sh、scripts/core/service.sh、scripts/utils/gms_fix.sh。
#######################################

set -e  # 命令失败立即退出

# 模块根目录与关键路径
readonly MODDIR="${0%/*}"                          # 模块根目录 (脚本所在目录)
readonly MODULE_CONF="$MODDIR/config/module.conf"  # 模块配置
readonly SUBWORKER_SCRIPT="$MODDIR/scripts/core/subworker.sh"  # 订阅更新 worker
readonly LOG_FILE="$MODDIR/logs/service.log"       # 服务日志
readonly LOG_TAG="boot"                            # 日志组件标签

. "$MODDIR/scripts/utils/common.sh"

#######################################
# 加载模块配置
# 先设默认值，再用配置文件覆盖 (文件不存在时沿用默认)。
# 参数: 无
# 全局: 设置 AUTO_START / GMS_FIX
# 返回: 无
#######################################
load_module_config() {
  # 开机服务相关默认值
  AUTO_START=1
  GMS_FIX=1

  if [ -f "$MODULE_CONF" ]; then
    . "$MODULE_CONF"
    log "INFO" "模块配置已加载"
  else
    log "WARN" "模块配置文件不存在，使用默认值"
  fi
}

#######################################
# 等待系统启动完成
# 阻塞至系统开机完成且外部存储挂载就绪。
# 参数: 无
# 返回: 无
#######################################
wait_for_boot() {
  log "INFO" "等待系统启动完成..."

  # 等待系统开机完成属性置位
  resetprop -w sys.boot_completed
  log "INFO" "系统启动完成"

  # 等待外部存储挂载完成
  while [ ! -d "/sdcard/Android" ]; do
    sleep 1
  done
  log "INFO" "存储挂载完成"
}

#######################################
# 执行设备兼容性修复
# 参数: 无
# 全局: GMS_FIX 为 1 时才执行
# 返回: 无
#######################################
check_device_specific() {
  # 启用时执行设备兼容性修复脚本
  if [ "$GMS_FIX" = "1" ]; then
    log "INFO" "GMS 修复已启用，执行修复脚本"
    sh "$MODDIR/scripts/utils/gms_fix.sh"
  fi
}

#######################################
# 启动独立订阅更新 worker
# worker 不依赖 sing-box 是否启用，启动失败也不阻断代理服务。
# 参数: 无
# 返回: 无
#######################################
start_subscription_worker() {
  if sh "$SUBWORKER_SCRIPT" start; then
    log "DEBUG" "订阅自动更新 worker 已就绪"
  else
    log "WARN" "订阅自动更新 worker 启动失败，可稍后手动重试"
  fi
}

#######################################
# 记录运行环境信息 (Root 方案、版本、模块版本等)
# 参数: 无
# 返回: 无
#######################################
log_env_info() {
  log "DEBUG" "环境信息检测"

  # KernelSU 环境
  if [ "$KSU" = "true" ]; then
    log "DEBUG" "环境: KernelSU"
    log "DEBUG" "KernelSU 版本: ${KSU_VER:-未知}"
    log "DEBUG" "KernelSU 版本号: ${KSU_VER_CODE:-未知}"
    log "DEBUG" "KernelSU 内核版本号: ${KSU_KERNEL_VER_CODE:-未知}"
  fi

  # APatch / KernelPatch 环境
  if [ "$APATCH" = "true" ] || [ "$KERNELPATCH" = "true" ]; then
    log "DEBUG" "环境: APatch / KernelPatch"
    log "DEBUG" "APatch 版本: ${APATCH_VER:-未知}"
    log "DEBUG" "APatch 版本号: ${APATCH_VER_CODE:-未知}"
    log "DEBUG" "内核版本: ${KERNEL_VERSION:-未知}"
    log "DEBUG" "KernelPatch 版本: ${KERNELPATCH_VERSION:-未知}"
  fi

  # Magisk 环境
  if [ -n "$MAGISK_VER" ]; then
    log "DEBUG" "环境: Magisk"
    log "DEBUG" "Magisk 版本: $MAGISK_VER"
    log "DEBUG" "Magisk 版本号: $MAGISK_VER_CODE"
  fi

  # 模块版本信息 (从 module.prop 提取，保留为单行 INFO)
  if [ -f "$MODDIR/module.prop" ]; then
    local version line versionCode
    line=$(grep "^version=" "$MODDIR/module.prop")
    version="${line#*=}"
    line=$(grep "^versionCode=" "$MODDIR/module.prop")
    versionCode="${line#*=}"
    log "INFO" "模块版本: ${version:-未知} (${versionCode:-未知})"
  fi
}

# 确保日志目录存在 (须在首次写日志前完成)
mkdir -p "$MODDIR/logs"

# ========== 主流程 ==========
log "INFO" "NetProxy 开机服务启动 (service 阶段)"
log_env_info
load_module_config

wait_for_boot

# 订阅调度与代理核心解耦，即使禁用开机代理也保持订阅按期更新。
start_subscription_worker

# 按配置决定是否开机自启服务
if [ "$AUTO_START" = "1" ]; then
  su -c "sh \"$MODDIR/scripts/core/service.sh\" start"
else
  log "INFO" "开机自启已禁用，跳过启动"
fi

# 执行设备兼容性修复
check_device_specific

log "INFO" "开机服务流程结束"
