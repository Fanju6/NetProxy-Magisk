#!/system/bin/sh

SKIPUNZIP=1

readonly CONFIG_DIR="/data/adb/modules/netproxy/config"

#######################################
# 备份并恢复配置文件
# Returns:
#   0 成功, 1 失败
#######################################
backup_and_restore_config() {
    if [ -d "$CONFIG_DIR" ] && [ "$(ls -A "$CONFIG_DIR" 2>/dev/null)" ]; then
        ui_print "检测到现有配置，开始备份..."
        
        # 备份整个 config 目录
        if ! cp -r "$CONFIG_DIR" "$TMPDIR/config_backup" >/dev/null 2>&1; then
            ui_print "警告: 配置备份失败"
            return 1
        fi
        
        # 解压新文件（排除整个配置目录）
        ui_print "解压模块文件（保留现有配置）..."
        if ! unzip -o "$ZIPFILE" -x "config/*" -d "$MODPATH" >/dev/null 2>&1; then
            ui_print "错误: 解压失败"
            return 1
        fi
        
        # 创建 config 目录（如果不存在）
        mkdir -p "$MODPATH/config" >/dev/null 2>&1
        
        # 恢复整个 config 目录
        ui_print "恢复配置文件..."
        if ! cp -r "$TMPDIR/config_backup"/* "$MODPATH/config/" >/dev/null 2>&1; then
            ui_print "警告: 配置恢复失败"
            return 1
        fi
        
        ui_print "配置文件已保留"
    else
        ui_print "全新安装，解压完整模块..."
        if ! unzip -o "$ZIPFILE" -d "$MODPATH" >/dev/null 2>&1; then
            ui_print "错误: 解压失败"
            return 1
        fi
    fi
    
    return 0
}

#######################################
# 设置文件权限
#######################################
set_permissions() {
    ui_print "设置文件权限..."
    
    set_perm_recursive "$MODPATH/bin/xray" 0 0 0755 0755
    set_perm_recursive "$MODPATH/scripts/core/start.sh" 0 0 0755 0755
    set_perm_recursive "$MODPATH/scripts/core/stop.sh" 0 0 0755 0755
    set_perm_recursive "$MODPATH/scripts/network/tproxy.sh" 0 0 0755 0755
    set_perm_recursive "$MODPATH/scripts/core/switch-config.sh" 0 0 0755 0755
    set_perm_recursive "$MODPATH/scripts/utils/update-xray.sh" 0 0 0755 0755
    set_perm_recursive "$MODPATH/scripts/config/url2json.sh" 0 0 0755 0755
    set_perm_recursive "$MODPATH/scripts/config/subscription.sh" 0 0 0755 0755
    set_perm_recursive "$MODPATH/scripts/utils/oneplus_a16_fix.sh" 0 0 0755 0755
    set_perm_recursive "$MODPATH/scripts/cli" 0 0 0755 0755
    set_perm_recursive "$MODPATH/action.sh" 0 0 0755 0755
}

#######################################
# 根据 routing.conf 初始化 03_routing.json
#######################################
init_routing_config() {
    local routing_conf="$MODPATH/config/routing.conf"
    local routing_json="$MODPATH/config/xray/confdir/03_routing.json"
    local routing_global="$MODPATH/config/routing_xray_global.json"
    local routing_rules="$MODPATH/config/routing_xray_rules.json"
    
    if [ -f "$routing_conf" ]; then
        local mode=$(grep "^MODE=" "$routing_conf" 2>/dev/null | cut -d'=' -f2)
        case "$mode" in
            global)
                if [ -f "$routing_global" ]; then
                    ui_print "初始化路由配置: 全局模式"
                    cp "$routing_global" "$routing_json"
                fi
                ;;
            rules)
                if [ -f "$routing_rules" ]; then
                    ui_print "初始化路由配置: 规则模式"
                    cp "$routing_rules" "$routing_json"
                fi
                ;;
        esac
    fi
}

# 主流程
ui_print "========================================="
ui_print "   NetProxy - Xray 透明代理模块"
ui_print "========================================="

if backup_and_restore_config && set_permissions; then
    init_routing_config
    ui_print "安装成功！"
    ui_print "请重启设备以使模块生效"
else
    ui_print "安装过程中出现错误"
    ui_print "请检查日志并重试"
    exit 1
fi