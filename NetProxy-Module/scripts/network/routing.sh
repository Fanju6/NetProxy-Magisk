#!/system/bin/sh

#############################################################################
# 路由规则管理脚本
# 功能: 读取/设置路由开关，生成 routing.json
# 支持两种路由模式:
#   - global: 全局代理模式，所有流量走代理（内网除外）
#   - rules:  规则模式，按分流规则处理（广告阻断、中国直连等）
#############################################################################

set -e
set -u

readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly SETTINGS_FILE="$MODDIR/config/routing.conf"
readonly ROUTING_FILE="$MODDIR/config/xray/confdir/03_routing.json"

#######################################
# 获取当前路由模式
# Returns: global 或 rules
#######################################
get_mode() {
    local mode
    mode=$(grep "^MODE=" "$SETTINGS_FILE" 2>/dev/null | cut -d'=' -f2)
    case "$mode" in
        global|rules) echo "$mode" ;;
        *) echo "rules" ;;
    esac
}

#######################################
# 设置路由模式
# Arguments:
#   $1 - mode (global/rules)
#######################################
cmd_mode() {
    local mode="${1:-}"
    
    if [ -z "$mode" ]; then
        echo "当前路由模式: $(get_mode)"
        return 0
    fi
    
    case "$mode" in
        global|rules)
            if grep -q "^MODE=" "$SETTINGS_FILE" 2>/dev/null; then
                sed -i "s/^MODE=.*/MODE=${mode}/" "$SETTINGS_FILE"
            else
                echo "MODE=${mode}" >> "$SETTINGS_FILE"
            fi
            echo "已切换到 $mode 模式"
            cmd_apply
            # 重启 Xray 使配置生效
            if pgrep -f "xray" >/dev/null 2>&1; then
                echo "正在重启服务..."
                sh "$MODDIR/scripts/core/stop.sh" >/dev/null 2>&1
                sh "$MODDIR/scripts/core/start.sh" >/dev/null 2>&1
                echo "服务已重启"
            fi
            ;;
        *)
            echo "错误: 无效的模式 '$mode'"
            echo "可用模式: global (全局代理), rules (规则分流)"
            return 1
            ;;
    esac
}

#######################################
# 读取设置 (返回 JSON 格式给 WebUI)
#######################################
cmd_get() {
    if [ ! -f "$SETTINGS_FILE" ]; then
        echo '{}'
        return
    fi
    
    echo '{'
    local first=true
    while IFS='=' read -r key value; do
        # 跳过注释和空行
        case "$key" in
            \#*|"") continue ;;
        esac
        key=$(echo "$key" | tr '[:upper:]' '[:lower:]')
        # MODE 是字符串，其他是布尔值
        if [ "$key" = "mode" ]; then
            [ "$first" = "true" ] && first=false || echo ','
            printf '    "%s": "%s"' "$key" "$value"
        else
            [ "$value" = "1" ] && value="true" || value="false"
            [ "$first" = "true" ] && first=false || echo ','
            printf '    "%s": %s' "$key" "$value"
        fi
    done < "$SETTINGS_FILE"
    echo
    echo '}'
}

#######################################
# 设置某项
# Arguments:
#   $1 - key (小写)
#   $2 - value (true/false)
#######################################
cmd_set() {
    local key="$1"
    local value="$2"
    local upper_key
    upper_key=$(echo "$key" | tr '[:lower:]' '[:upper:]')
    
    # 转换 true/false 为 1/0
    [ "$value" = "true" ] && value="1" || value="0"
    
    if grep -q "^${upper_key}=" "$SETTINGS_FILE" 2>/dev/null; then
        sed -i "s/^${upper_key}=.*/${upper_key}=${value}/" "$SETTINGS_FILE"
    else
        echo "${upper_key}=${value}" >> "$SETTINGS_FILE"
    fi
    
    echo "已设置 $key = $value"
}

#######################################
# 读取设置值
# Arguments:
#   $1 - key (小写)
# Returns:
#   true/false
#######################################
get_setting() {
    local key="$1"
    local upper_key
    upper_key=$(echo "$key" | tr '[:lower:]' '[:upper:]')
    local value
    value=$(grep "^${upper_key}=" "$SETTINGS_FILE" 2>/dev/null | cut -d'=' -f2)
    [ "$value" = "1" ] && echo "true" || echo "false"
}

#######################################
# 生成全局模式路由配置
#######################################
generate_global_routing() {
    cat > "$ROUTING_FILE" << 'EOF'
{
    "routing": {
        "domainStrategy": "AsIs",
        "rules": [
            {
                "type": "field",
                "ip": [
                    "geoip:private"
                ],
                "outboundTag": "direct"
            },
            {
                "type": "field",
                "domain": [
                    "geosite:private"
                ],
                "outboundTag": "direct"
            },
            {
                "type": "field",
                "port": "0-65535",
                "outboundTag": "proxy"
            }
        ]
    }
}
EOF
}

#######################################
# 生成规则模式路由配置
#######################################
generate_rules_routing() {
    # 开始 JSON
    cat > "$ROUTING_FILE" << 'EOF'
{
    "routing": {
        "domainStrategy": "AsIs",
        "rules": [
EOF

    local first=true
    
    # Google CN
    if [ "$(get_setting google_cn)" = "true" ]; then
        [ "$first" = "false" ] && echo "," >> "$ROUTING_FILE"
        first=false
        cat >> "$ROUTING_FILE" << 'EOF'
            {
                "type": "field",
                "domain": [
                    "domain:googleapis.cn",
                    "domain:gstatic.com"
                ],
                "outboundTag": "proxy"
            }
EOF
    fi
    
    # 阻断 UDP 443
    if [ "$(get_setting block_udp443)" = "true" ]; then
        [ "$first" = "false" ] && echo "," >> "$ROUTING_FILE"
        first=false
        cat >> "$ROUTING_FILE" << 'EOF'
            {
                "type": "field",
                "network": "udp",
                "port": "443",
                "outboundTag": "block"
            }
EOF
    fi
    
    # 阻断广告
    if [ "$(get_setting block_ads)" = "true" ]; then
        [ "$first" = "false" ] && echo "," >> "$ROUTING_FILE"
        first=false
        cat >> "$ROUTING_FILE" << 'EOF'
            {
                "type": "field",
                "domain": [
                    "geosite:category-ads-all"
                ],
                "outboundTag": "block"
            }
EOF
    fi
    
    # 绕过局域网 IP
    if [ "$(get_setting bypass_lan_ip)" = "true" ]; then
        [ "$first" = "false" ] && echo "," >> "$ROUTING_FILE"
        first=false
        cat >> "$ROUTING_FILE" << 'EOF'
            {
                "type": "field",
                "ip": [
                    "geoip:private"
                ],
                "outboundTag": "direct"
            }
EOF
    fi
    
    # 绕过局域网域名
    if [ "$(get_setting bypass_lan_domain)" = "true" ]; then
        [ "$first" = "false" ] && echo "," >> "$ROUTING_FILE"
        first=false
        cat >> "$ROUTING_FILE" << 'EOF'
            {
                "type": "field",
                "domain": [
                    "geosite:private"
                ],
                "outboundTag": "direct"
            }
EOF
    fi
    
    # 绕过中国公共 DNS IP
    if [ "$(get_setting bypass_cn_dns_ip)" = "true" ]; then
        [ "$first" = "false" ] && echo "," >> "$ROUTING_FILE"
        first=false
        cat >> "$ROUTING_FILE" << 'EOF'
            {
                "type": "field",
                "ip": [
                    "223.5.5.5",
                    "223.6.6.6",
                    "2400:3200::1",
                    "2400:3200:baba::1",
                    "119.29.29.29",
                    "1.12.12.12",
                    "120.53.53.53",
                    "2402:4e00::",
                    "2402:4e00:1::",
                    "180.76.76.76",
                    "2400:da00::6666",
                    "114.114.114.114",
                    "114.114.115.115",
                    "114.114.114.119",
                    "114.114.115.119",
                    "114.114.114.110",
                    "114.114.115.110",
                    "180.184.1.1",
                    "180.184.2.2",
                    "101.226.4.6",
                    "218.30.118.6",
                    "123.125.81.6",
                    "140.207.198.6",
                    "1.2.4.8",
                    "210.2.4.8",
                    "52.80.66.66",
                    "117.50.22.22",
                    "2400:7fc0:849e:200::4",
                    "2404:c2c0:85d8:901::4",
                    "117.50.10.10",
                    "52.80.52.52",
                    "2400:7fc0:849e:200::8",
                    "2404:c2c0:85d8:901::8",
                    "117.50.60.30",
                    "52.80.60.30"
                ],
                "outboundTag": "direct"
            }
EOF
    fi
    
    # 绕过中国公共 DNS 域名
    if [ "$(get_setting bypass_cn_dns_domain)" = "true" ]; then
        [ "$first" = "false" ] && echo "," >> "$ROUTING_FILE"
        first=false
        cat >> "$ROUTING_FILE" << 'EOF'
            {
                "type": "field",
                "domain": [
                    "domain:alidns.com",
                    "domain:doh.pub",
                    "domain:dot.pub",
                    "domain:360.cn",
                    "domain:onedns.net"
                ],
                "outboundTag": "direct"
            }
EOF
    fi
    
    # 绕过中国 IP
    if [ "$(get_setting bypass_cn_ip)" = "true" ]; then
        [ "$first" = "false" ] && echo "," >> "$ROUTING_FILE"
        first=false
        cat >> "$ROUTING_FILE" << 'EOF'
            {
                "type": "field",
                "ip": [
                    "geoip:cn"
                ],
                "outboundTag": "direct"
            }
EOF
    fi
    
    # 绕过中国域名
    if [ "$(get_setting bypass_cn_domain)" = "true" ]; then
        [ "$first" = "false" ] && echo "," >> "$ROUTING_FILE"
        first=false
        cat >> "$ROUTING_FILE" << 'EOF'
            {
                "type": "field",
                "domain": [
                    "geosite:cn"
                ],
                "outboundTag": "direct"
            }
EOF
    fi
    
    # 最终代理
    if [ "$(get_setting final_proxy)" = "true" ]; then
        [ "$first" = "false" ] && echo "," >> "$ROUTING_FILE"
        first=false
        cat >> "$ROUTING_FILE" << 'EOF'
            {
                "type": "field",
                "port": "0-65535",
                "outboundTag": "proxy"
            }
EOF
    fi
    
    # 固定的内部 DNS 规则（始终添加）
    [ "$first" = "false" ] && echo "," >> "$ROUTING_FILE"
    cat >> "$ROUTING_FILE" << 'EOF'
            {
                "type": "field",
                "inboundTag": [
                    "domestic-dns"
                ],
                "outboundTag": "direct"
            },
            {
                "type": "field",
                "inboundTag": [
                    "dns-module"
                ],
                "outboundTag": "proxy"
            }
        ]
    }
}
EOF
}

#######################################
# 生成路由配置（根据当前模式）
#######################################
cmd_apply() {
    local mode="$(get_mode)"
    echo "正在生成路由配置 (模式: $mode)..."
    
    case "$mode" in
        global)
            generate_global_routing
            ;;
        rules)
            generate_rules_routing
            ;;
        *)
            echo "警告: 未知模式 '$mode'，使用规则模式"
            generate_rules_routing
            ;;
    esac
    
    echo "路由配置已生成: $ROUTING_FILE"
}

#######################################
# 主程序
#######################################
case "${1:-}" in
    mode)
        cmd_mode "${2:-}"
        ;;
    get)
        cmd_get
        ;;
    set)
        # set 命令只在 rules 模式下生效
        if [ "$(get_mode)" != "rules" ]; then
            echo "错误: set 命令仅在 rules 模式下有效"
            echo "当前模式: $(get_mode)"
            echo "请先执行: $0 mode rules"
            exit 1
        fi
        cmd_set "$2" "$3"
        cmd_apply
        ;;
    apply)
        cmd_apply
        ;;
    *)
        echo "用法: $0 {mode|get|set|apply}"
        echo ""
        echo "命令:"
        echo "  mode             查看当前路由模式"
        echo "  mode <模式>      切换路由模式 (global/rules)"
        echo "  get              获取当前设置"
        echo "  set <key> <val>  设置某项 (true/false，仅规则模式)"
        echo "  apply            应用设置生成 routing.json"
        echo ""
        echo "路由模式:"
        echo "  global  全局代理，所有流量走代理（内网除外）"
        echo "  rules   规则分流，按配置的规则处理"
        exit 1
        ;;
esac
