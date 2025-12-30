#!/system/bin/sh

# 脚本目录
_SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
readonly SCRIPT_DIR="$_SCRIPT_DIR"

# 配置文件路径
readonly CONFIG_FILE="${TPROXY_CONFIG:-$SCRIPT_DIR/../../config/tproxy.conf}"
readonly LOG_FILE="$SCRIPT_DIR/../../logs/tproxy.log"


log() {
    local level="$1"
    local message="$2"
    local timestamp
    local color_code

    timestamp="$(date +"%Y-%m-%d %H:%M:%S")"

    case "$level" in
        Debug) color_code="\033[0;36m" ;;  # 青色
        Info) color_code="\033[1;32m" ;;   # 绿色
        Warn) color_code="\033[1;33m" ;;   # 黄色
        Error) color_code="\033[1;31m" ;;  # 红色
        *)
            level="Unknown"
            color_code="\033[0m"
            ;;
    esac

    # 输出到日志文件
    printf "%s\n" "${timestamp} [${level}]: ${message}" >> "$LOG_FILE"

    # 输出到终端
    if [ -t 1 ]; then
        printf "%b\n" "${color_code}${timestamp} [${level}]: ${message}\033[0m"
    else
        printf "%s\n" "${timestamp} [${level}]: ${message}"
    fi
}

load_config() {
    # 加载配置文件
    if [ ! -f "$CONFIG_FILE" ]; then
        echo "错误：未找到配置文件：$CONFIG_FILE" >&2
        exit 1
    fi
    
    # shellcheck source=/dev/null
    . "$CONFIG_FILE"
    
    log Info "已加载配置文件：$CONFIG_FILE"
    log Info "端口=$PROXY_TCP_PORT 模式=$APP_PROXY_MODE"
}

validate_config() {
    log Debug "正在验证配置..."

    if ! echo "$PROXY_TCP_PORT" | grep -E '^[0-9]+$' > /dev/null || [ "$PROXY_TCP_PORT" -lt 1 ] || [ "$PROXY_TCP_PORT" -gt 65535 ]; then
        log Error "无效的 PROXY_TCP_PORT (TCP代理端口)：$PROXY_TCP_PORT"
        return 1
    fi

    if ! echo "$PROXY_UDP_PORT" | grep -E '^[0-9]+$' > /dev/null || [ "$PROXY_UDP_PORT" -lt 1 ] || [ "$PROXY_UDP_PORT" -gt 65535 ]; then
        log Error "无效的 PROXY_UDP_PORT (UDP代理端口)：$PROXY_UDP_PORT"
        return 1
    fi

    if ! echo "$PROXY_MODE" | grep -E '^[0-2]$' > /dev/null; then
        log Error "无效的 PROXY_MODE (代理模式)：$PROXY_MODE (必须是 0=自动, 1=强制TPROXY, 2=强制REDIRECT)"
        return 1
    fi

    if ! echo "$DNS_HIJACK_ENABLE" | grep -E '^[0-2]$' > /dev/null; then
        log Error "无效的 DNS_HIJACK_ENABLE (DNS劫持开关)：$DNS_HIJACK_ENABLE (必须是 0=禁用, 1=tproxy, 2=redirect)"
        return 1
    fi

    if ! echo "$DNS_PORT" | grep -E '^[0-9]+$' > /dev/null || [ "$DNS_PORT" -lt 1 ] || [ "$DNS_PORT" -gt 65535 ]; then
        log Error "无效的 DNS_PORT (DNS端口)：$DNS_PORT"
        return 1
    fi

    if ! echo "$MARK_VALUE" | grep -E '^[0-9]+$' > /dev/null || [ "$MARK_VALUE" -lt 1 ] || [ "$MARK_VALUE" -gt 2147483647 ]; then
        log Error "无效的 MARK_VALUE (IPv4标记值)：$MARK_VALUE"
        return 1
    fi

    if ! echo "$MARK_VALUE6" | grep -E '^[0-9]+$' > /dev/null || [ "$MARK_VALUE6" -lt 1 ] || [ "$MARK_VALUE6" -gt 2147483647 ]; then
        log Error "无效的 MARK_VALUE6 (IPv6标记值)：$MARK_VALUE6"
        return 1
    fi

    if ! echo "$TABLE_ID" | grep -E '^[0-9]+$' > /dev/null || [ "$TABLE_ID" -lt 1 ] || [ "$TABLE_ID" -gt 65535 ]; then
        log Error "无效的 TABLE_ID (路由表ID)：$TABLE_ID"
        return 1
    fi

    case "$CORE_USER_GROUP" in
        *:*)
            CORE_USER=$(echo "$CORE_USER_GROUP" | cut -d: -f1)
            CORE_GROUP=$(echo "$CORE_USER_GROUP" | cut -d: -f2)
            log Debug "解析用户:组为 '$CORE_USER:$CORE_GROUP'"
            ;;
        *)
            CORE_USER="root"
            CORE_GROUP="net_admin"
            log Debug "使用默认用户:组 '$CORE_USER:$CORE_GROUP'"
            ;;
    esac

    if [ -z "$CORE_USER" ] || [ -z "$CORE_GROUP" ]; then
        log Warn "检测到用户或组为空，使用默认值"
        CORE_USER="root"
        CORE_GROUP="net_admin"
    fi

    log Info "最终用户:组配置：'$CORE_USER:$CORE_GROUP'"

    case "$APP_PROXY_MODE" in
        blacklist | whitelist) ;;
        *)
            log Error "无效的 APP_PROXY_MODE (应用代理模式)：$APP_PROXY_MODE"
            return 1
            ;;
    esac

    case "$MAC_PROXY_MODE" in
        blacklist | whitelist) ;;
        *)
            log Error "无效的 MAC_PROXY_MODE (MAC代理模式)：$MAC_PROXY_MODE"
            return 1
            ;;
    esac

    log Debug "配置验证通过"
    return 0
}

check_root() {
    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] 跳过 Root 检查"
        return 0
    fi
    if [ "$(id -u 2> /dev/null || echo 1)" != "0" ]; then
        log Error "必须以 Root 权限运行"
        exit 1
    fi
}

check_dependencies() {
    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] 跳过依赖检查"
        return 0
    fi

    export PATH="$PATH:/data/data/com.termux/files/usr/bin"

    local missing=""
    local required_commands="ip iptables curl"
    local cmd

    for cmd in $required_commands; do
        if ! command -v "$cmd" > /dev/null 2>&1; then
            missing="$missing $cmd"
        fi
    done

    if [ -n "$missing" ]; then
        log Error "缺少必要的命令：$missing"
        log Info "检查 PATH 环境变量：$PATH"
        exit 1
    fi
}

check_kernel_feature() {
    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] 跳过内核特性检查：$1"
        return 0
    fi

    local feature="$1"
    local config_name="CONFIG_${feature}"

    if [ -f /proc/config.gz ]; then
        if zcat /proc/config.gz 2> /dev/null | grep -qE "^${config_name}=[ym]$"; then
            log Debug "内核特性 $feature 已启用"
            return 0
        else
            log Debug "内核特性 $feature 已禁用或未找到"
            return 1
        fi
    else
        log Debug "无法检查内核特性 $feature：/proc/config.gz 不可用"
        return 1
    fi
}

check_tproxy_support() {
    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] 跳过 TPROXY 支持检查"
        return 0
    fi

    if check_kernel_feature "NETFILTER_XT_TARGET_TPROXY"; then
        log Debug "内核 TPROXY 支持已确认"
        return 0
    else
        log Debug "内核 TPROXY 支持不可用"
        return 1
    fi
}

# 统一命令包装函数
run_ipt_command() {
    local cmd="$1"
    shift
    local args="$*"

    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] $cmd $args"
        return 0
    else
        command $cmd -w 100 $args
    fi
}

iptables() {
    run_ipt_command iptables "$@"
}

ip6tables() {
    run_ipt_command ip6tables "$@"
}

ip_rule() {
    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] ip rule $*"
        return 0
    else
        command ip rule "$@"
    fi
}

ip6_rule() {
    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] ip -6 rule $*"
        return 0
    else
        command ip -6 rule "$@"
    fi
}

ip_route() {
    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] ip route $*"
        return 0
    else
        command ip route "$@"
    fi
}

ip6_route() {
    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] ip -6 route $*"
        return 0
    else
        command ip -6 route "$@"
    fi
}

get_package_uid() {
    local pkg="$1"
    local line
    local uid
    if [ ! -r /data/system/packages.list ]; then
        log Debug "无法读取 /data/system/packages.list"
        return 1
    fi
    line=$(grep -m1 "^${pkg}[[:space:]]" /data/system/packages.list 2> /dev/null || true)
    if [ -z "$line" ]; then
        log Debug "在 packages.list 中未找到包名：$pkg"
        return 1
    fi

    uid=$(echo "$line" | awk '{print $2}' 2> /dev/null || true)
    case "$uid" in
        '' | *[!0-9]*)
            uid=$(echo "$line" | awk '{print $(NF-1)}' 2> /dev/null || true)
            ;;
    esac
    case "$uid" in
        '' | *[!0-9]*)
            log Debug "包名的 UID 格式无效：$pkg"
            return 1
            ;;
        *)
            echo "$uid"
            return 0
            ;;
    esac
}

find_packages_uid() {
    local out
    local token
    local uid_base
    local final_uid
    # shellcheck disable=SC2048
    for token in $*; do
        local user_prefix=0
        local package="$token"
        case "$token" in
            *:*)
                user_prefix=$(echo "$token" | cut -d: -f1)
                package=$(echo "$token" | cut -d: -f2-)
                case "$user_prefix" in
                    '' | *[!0-9]*)
                        log Warn "令牌中的用户前缀无效：$token，使用 0"
                        user_prefix=0
                        ;;
                esac
                ;;
        esac
        if uid_base=$(get_package_uid "$package" 2> /dev/null); then
            final_uid=$((user_prefix * 100000 + uid_base))
            out="$out $final_uid"
            log Debug "已解析包 $token 为 UID $final_uid"
        else
            log Warn "无法解析包的 UID：$package"
        fi
    done
    echo "$out" | awk '{$1=$1;print}'
}

safe_chain_exists() {
    local family="$1"
    local table="$2"
    local chain="$3"
    local cmd="iptables"

    if [ "$family" = "6" ]; then
        cmd="ip6tables"
    fi

    if $cmd -t "$table" -L "$chain" > /dev/null 2>&1; then
        return 0
    fi

    return 1
}

safe_chain_create() {
    local family="$1"
    local table="$2"
    local chain="$3"
    local cmd="iptables"

    if [ "$family" = "6" ]; then
        cmd="ip6tables"
    fi

    if [ "$DRY_RUN" -eq 1 ] || ! safe_chain_exists "$family" "$table" "$chain"; then
        $cmd -t "$table" -N "$chain"
    fi

    $cmd -t "$table" -F "$chain"
}

download_cn_ip_list() {
    if [ "$BYPASS_CN_IP" -eq 0 ]; then
        log Debug "中国 IP 绕过已禁用，跳过下载"
        return 0
    fi

    log Info "正在检查/下载中国大陆 IP 列表至 $CN_IP_FILE"

    # 如果文件不存在或超过7天则重新下载
    if [ ! -f "$CN_IP_FILE" ] || [ "$(find "$CN_IP_FILE" -mtime +7 2> /dev/null)" ]; then
        log Info "正在获取最新的中国 IP 列表自 $CN_IP_URL"
        if [ "$DRY_RUN" -eq 1 ]; then
            log Debug "[DRY-RUN] curl -fsSL --connect-timeout 10 --retry 3 $CN_IP_URL -o $CN_IP_FILE.tmp"
        else
            if ! curl -fsSL --connect-timeout 10 --retry 3 \
                "$CN_IP_URL" \
                -o "$CN_IP_FILE.tmp"; then
                log Error "下载中国 IP 列表失败"
                rm -f "$CN_IP_FILE.tmp"
                return 1
            fi
        fi
        if [ "$DRY_RUN" -eq 0 ]; then
            mv "$CN_IP_FILE.tmp" "$CN_IP_FILE"
        fi
        log Info "中国 IP 列表已保存至 $CN_IP_FILE"
    else
        log Debug "使用现有的中国 IP 列表：$CN_IP_FILE"
    fi

    if [ "$PROXY_IPV6" -eq 1 ]; then
        log Info "正在检查/下载中国大陆 IPv6 列表至 $CN_IPV6_FILE"

        if [ ! -f "$CN_IPV6_FILE" ] || [ "$(find "$CN_IPV6_FILE" -mtime +7 2> /dev/null)" ]; then
            log Info "正在获取最新的中国 IPv6 列表自 $CN_IPV6_URL"
            if [ "$DRY_RUN" -eq 1 ]; then
                log Debug "[DRY-RUN] curl -fsSL --connect-timeout 10 --retry 3 $CN_IPV6_URL -o $CN_IPV6_FILE.tmp"
            else
                if ! curl -fsSL --connect-timeout 10 --retry 3 \
                    "$CN_IPV6_URL" \
                    -o "$CN_IPV6_FILE.tmp"; then
                    log Error "下载中国 IPv6 列表失败"
                    rm -f "$CN_IPV6_FILE.tmp"
                    return 1
                fi
            fi
            if [ "$DRY_RUN" -eq 0 ]; then
                mv "$CN_IPV6_FILE.tmp" "$CN_IPV6_FILE"
            fi
            log Info "中国 IPv6 列表已保存至 $CN_IPV6_FILE"
        else
            log Debug "使用现有的中国 IPv6 列表：$CN_IPV6_FILE"
        fi
    fi
}

setup_cn_ipset() {
    if [ "$BYPASS_CN_IP" -eq 0 ]; then
        log Debug "中国 IP 绕过已禁用，跳过 ipset 设置"
        return 0
    fi

    if ! command -v ipset > /dev/null 2>&1; then
        log Error "未找到 ipset 命令。无法绕过中国 IP"
        return 1
    fi

    log Info "正在设置中国大陆 IP 的 ipset"

    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] ipset destroy cnip"
        log Debug "[DRY-RUN] ipset destroy cnip6"
    else
        ipset destroy cnip 2> /dev/null || true
        ipset destroy cnip6 2> /dev/null || true
    fi

    if [ -f "$CN_IP_FILE" ]; then
        log Debug "正在加载 IPv4 CIDR 自 $CN_IP_FILE"
        ipv4_count=$(wc -l < "$CN_IP_FILE" 2> /dev/null || echo "0")

        if [ "$DRY_RUN" -eq 1 ]; then
            log Debug "[DRY-RUN] 将通过 ipset restore 加载 $ipv4_count 条 IPv4 CIDR 记录"
            log Debug "[DRY-RUN] ipset create cnip hash:net family inet hashsize 8192 maxelem 65536"
        else
            temp_file=$(mktemp)
            {
                echo "create cnip hash:net family inet hashsize 8192 maxelem 65536"
                awk '!/^[[:space:]]*#/ && NF > 0 {printf "add cnip %s\n", $0}' "$CN_IP_FILE"
            } > "$temp_file"

            if ipset restore -f "$temp_file" 2> /dev/null; then
                log Info "成功加载 $ipv4_count 条 IPv4 CIDR 记录"
            else
                log Error "无法创建 ipset 'cnip' 或加载 IPv4 CIDR 记录"
                rm -f "$temp_file"
                return 1
            fi
            rm -f "$temp_file"
        fi
    else
        log Warn "未找到 CN IP 文件：$CN_IP_FILE"
        return 1
    fi

    log Info "ipset 'cnip' 已加载中国大陆 IP"

    if [ "$PROXY_IPV6" -eq 1 ]; then
        if [ -f "$CN_IPV6_FILE" ]; then
            log Debug "正在加载 IPv6 CIDR 自 $CN_IPV6_FILE"
            ipv6_count=$(wc -l < "$CN_IPV6_FILE" 2> /dev/null || echo "0")

            if [ "$DRY_RUN" -eq 1 ]; then
                log Debug "[DRY-RUN] 将通过 ipset restore 加载 $ipv6_count 条 IPv6 CIDR 记录"
                log Debug "[DRY-RUN] ipset create cnip6 hash:net family inet6 hashsize 8192 maxelem 65536"
            else
                temp_file6=$(mktemp)
                {
                    echo "create cnip6 hash:net family inet6 hashsize 8192 maxelem 65536"
                    awk '!/^[[:space:]]*#/ && NF > 0 {printf "add cnip6 %s\n", $0}' "$CN_IPV6_FILE"
                } > "$temp_file6"

                if ipset restore -f "$temp_file6" 2> /dev/null; then
                    log Info "成功加载 $ipv6_count 条 IPv6 CIDR 记录"
                else
                    log Error "无法创建 ipset 'cnip6' 或加载 IPv6 CIDR 记录"
                    rm -f "$temp_file6"
                    return 1
                fi
                rm -f "$temp_file6"
            fi
            log Info "ipset 'cnip6' 已加载中国大陆 IPv6 IP"
        else
            log Warn "未找到 CN IPv6 文件：$CN_IPV6_FILE"
        fi
    fi

    return 0
}

# IPv4 和 IPv6 统一设置函数
setup_proxy_chain() {
    local family="$1"
    local mode="$2" # tproxy 或 redirect
    local suffix=""
    local mark="$MARK_VALUE"
    local cmd="iptables"

    if [ "$family" = "6" ]; then
        suffix="6"
        mark="$MARK_VALUE6"
        cmd="ip6tables"
    fi

    # 设置模式名称用于日志
    local mode_name="$mode"
    if [ "$mode" = "tproxy" ]; then
        mode_name="TPROXY"
    else
        mode_name="REDIRECT"
    fi

    log Info "正在设置 IPv${family} 的 $mode_name 链"

    # 根据 IP 版本定义链
    local chains=""
    if [ "$family" = "6" ]; then
        chains="PROXY_PREROUTING6 PROXY_OUTPUT6 BYPASS_IP6 BYPASS_INTERFACE6 PROXY_INTERFACE6 DNS_HIJACK_PRE6 DNS_HIJACK_OUT6 APP_CHAIN6 MAC_CHAIN6"
    else
        chains="PROXY_PREROUTING PROXY_OUTPUT BYPASS_IP BYPASS_INTERFACE PROXY_INTERFACE DNS_HIJACK_PRE DNS_HIJACK_OUT APP_CHAIN MAC_CHAIN"
    fi

    local table="mangle"
    if [ "$mode" = "redirect" ]; then
        table="nat"
    fi

    # 创建链
    for c in $chains; do
        safe_chain_create "$family" "$table" "$c"
    done

    $cmd -t "$table" -A "PROXY_PREROUTING$suffix" -j "BYPASS_IP$suffix"
    $cmd -t "$table" -A "PROXY_PREROUTING$suffix" -j "PROXY_INTERFACE$suffix"
    $cmd -t "$table" -A "PROXY_PREROUTING$suffix" -j "MAC_CHAIN$suffix"
    $cmd -t "$table" -A "PROXY_PREROUTING$suffix" -j "DNS_HIJACK_PRE$suffix"

    $cmd -t "$table" -A "PROXY_OUTPUT$suffix" -j "BYPASS_IP$suffix"
    $cmd -t "$table" -A "PROXY_OUTPUT$suffix" -j "BYPASS_INTERFACE$suffix"
    $cmd -t "$table" -A "PROXY_OUTPUT$suffix" -j "APP_CHAIN$suffix"
    $cmd -t "$table" -A "PROXY_OUTPUT$suffix" -j "DNS_HIJACK_OUT$suffix"

    if check_kernel_feature "NETFILTER_XT_MATCH_ADDRTYPE"; then
        $cmd -t "$table" -A "BYPASS_IP$suffix" -m addrtype --dst-type LOCAL -p udp ! --dport 53 -j ACCEPT
        $cmd -t "$table" -A "BYPASS_IP$suffix" -m addrtype --dst-type LOCAL ! -p udp -j ACCEPT
        log Info "已添加本地地址类型绕过"
    fi

    if check_kernel_feature "NETFILTER_XT_MATCH_CONNTRACK"; then
        $cmd -t "$table" -A "BYPASS_IP$suffix" -m conntrack --ctdir REPLY -j ACCEPT
        log Info "已添加回复连接方向绕过"
    fi

    # 添加私有 IP 段绕过
    if [ "$family" = "6" ]; then
        for subnet6 in ::/128 ::1/128 ::ffff:0:0/96 \
            100::/64 64:ff9b::/96 2001::/32 2001:10::/28 \
            2001:20::/28 2001:db8::/32 \
            2002::/16 fe80::/10 ff00::/8; do
            $cmd -t "$table" -A "BYPASS_IP$suffix" -d "$subnet6" -p udp ! --dport 53 -j ACCEPT
            $cmd -t "$table" -A "BYPASS_IP$suffix" -d "$subnet6" ! -p udp -j ACCEPT
        done
    else
        for subnet4 in 0.0.0.0/8 10.0.0.0/8 100.0.0.0/8 127.0.0.0/8 \
            169.254.0.0/16 172.16.0.0/12 192.0.0.0/24 192.0.2.0/24 192.88.99.0/24 \
            192.168.0.0/16 198.51.100.0/24 203.0.113.0/24 \
            224.0.0.0/4 240.0.0.0/4 255.255.255.255/32; do
            $cmd -t "$table" -A "BYPASS_IP$suffix" -d "$subnet4" -p udp ! --dport 53 -j ACCEPT
            $cmd -t "$table" -A "BYPASS_IP$suffix" -d "$subnet4" ! -p udp -j ACCEPT
        done
    fi
    log Info "已添加私有 IP 段绕过规则"

    if [ "$BYPASS_CN_IP" -eq 1 ]; then
        ipset_name="cnip"
        if [ "$family" = "6" ]; then
            ipset_name="cnip6"
        fi
        if command -v ipset > /dev/null 2>&1 && ipset list "$ipset_name" > /dev/null 2>&1; then
            $cmd -t "$table" -A "BYPASS_IP$suffix" -m set --match-set "$ipset_name" dst -p udp ! --dport 53 -j ACCEPT
            $cmd -t "$table" -A "BYPASS_IP$suffix" -m set --match-set "$ipset_name" dst ! -p udp -j ACCEPT
            log Info "已添加基于 ipset 的中国 IP 绕过规则"
        else
            log Warn "ipset '$ipset_name' 不可用，跳过中国 IP 绕过"
        fi
    fi

    log Info "正在配置接口代理规则"
    $cmd -t "$table" -A "PROXY_INTERFACE$suffix" -i lo -j RETURN
    if [ "$PROXY_MOBILE" -eq 1 ]; then
        $cmd -t "$table" -A "PROXY_INTERFACE$suffix" -i "$MOBILE_INTERFACE" -j RETURN
        log Info "移动数据接口 $MOBILE_INTERFACE 将被代理"
    else
        $cmd -t "$table" -A "PROXY_INTERFACE$suffix" -i "$MOBILE_INTERFACE" -j ACCEPT
        $cmd -t "$table" -A "BYPASS_INTERFACE$suffix" -o "$MOBILE_INTERFACE" -j ACCEPT
        log Info "移动数据接口 $MOBILE_INTERFACE 将被绕过"
    fi
    if [ "$PROXY_WIFI" -eq 1 ]; then
        $cmd -t "$table" -A "PROXY_INTERFACE$suffix" -i "$WIFI_INTERFACE" -j RETURN
        log Info "WiFi 接口 $WIFI_INTERFACE 将被代理"
    else
        $cmd -t "$table" -A "PROXY_INTERFACE$suffix" -i "$WIFI_INTERFACE" -j ACCEPT
        $cmd -t "$table" -A "BYPASS_INTERFACE$suffix" -o "$WIFI_INTERFACE" -j ACCEPT
        log Info "WiFi 接口 $WIFI_INTERFACE 将被绕过"
    fi
    if [ "$PROXY_HOTSPOT" -eq 1 ]; then
        if [ "$HOTSPOT_INTERFACE" = "$WIFI_INTERFACE" ]; then
            local subnet=""
            if [ "$family" = "6" ]; then
                subnet="fe80::/10"
            else
                subnet="192.168.43.0/24"
            fi
            $cmd -t "$table" -A "PROXY_INTERFACE$suffix" -i "$WIFI_INTERFACE" ! -s "$subnet" -j RETURN
            log Info "热点接口 $WIFI_INTERFACE 将被代理"
        else
            $cmd -t "$table" -A "PROXY_INTERFACE$suffix" -i "$HOTSPOT_INTERFACE" -j RETURN
            log Info "热点接口 $HOTSPOT_INTERFACE 将被代理"
        fi
    else
        $cmd -t "$table" -A "BYPASS_INTERFACE$suffix" -o "$HOTSPOT_INTERFACE" -j ACCEPT
        log Info "热点接口 $HOTSPOT_INTERFACE 将被绕过"
    fi
    if [ "$PROXY_USB" -eq 1 ]; then
        $cmd -t "$table" -A "PROXY_INTERFACE$suffix" -i "$USB_INTERFACE" -j RETURN
        log Info "USB 接口 $USB_INTERFACE 将被代理"
    else
        $cmd -t "$table" -A "PROXY_INTERFACE$suffix" -i "$USB_INTERFACE" -j ACCEPT
        $cmd -t "$table" -A "BYPASS_INTERFACE$suffix" -o "$USB_INTERFACE" -j ACCEPT
        log Info "USB 接口 $USB_INTERFACE 将被绕过"
    fi
    $cmd -t "$table" -A "PROXY_INTERFACE$suffix" -j ACCEPT
    log Info "接口代理规则配置完成"

    if [ "$MAC_FILTER_ENABLE" -eq 1 ] && [ "$PROXY_HOTSPOT" -eq 1 ] && [ -n "$HOTSPOT_INTERFACE" ]; then
        if check_kernel_feature "NETFILTER_XT_MATCH_MAC"; then
            log Info "正在设置 MAC 地址过滤规则，接口：$HOTSPOT_INTERFACE"
            case "$MAC_PROXY_MODE" in
                blacklist)
                    if [ -n "$BYPASS_MACS_LIST" ]; then
                        for mac in $BYPASS_MACS_LIST; do
                            if [ -n "$mac" ]; then
                                $cmd -t "$table" -A "MAC_CHAIN$suffix" -m mac --mac-source "$mac" -i "$HOTSPOT_INTERFACE" -j ACCEPT
                                log Info "已添加 MAC 绕过规则：$mac"
                            fi
                        done
                    else
                        log Warn "MAC 黑名单模式已启用但未配置绕过 MAC"
                    fi
                    $cmd -t "$table" -A "MAC_CHAIN$suffix" -i "$HOTSPOT_INTERFACE" -j RETURN
                    ;;
                whitelist)
                    if [ -n "$PROXY_MACS_LIST" ]; then
                        for mac in $PROXY_MACS_LIST; do
                            if [ -n "$mac" ]; then
                                $cmd -t "$table" -A "MAC_CHAIN$suffix" -m mac --mac-source "$mac" -i "$HOTSPOT_INTERFACE" -j RETURN
                                log Info "已添加 MAC 代理规则：$mac"
                            fi
                        done
                    else
                        log Warn "MAC 白名单模式已启用但未配置代理 MAC"
                    fi
                    $cmd -t "$table" -A "MAC_CHAIN$suffix" -i "$HOTSPOT_INTERFACE" -j ACCEPT
                    ;;
            esac
        else
            log Warn "MAC 过滤需要 NETFILTER_XT_MATCH_MAC 内核特性，该特性不可用"
        fi
    fi

    if check_kernel_feature "NETFILTER_XT_MATCH_OWNER"; then
        $cmd -t "$table" -A "APP_CHAIN$suffix" -m owner --uid-owner "$CORE_USER" --gid-owner "$CORE_GROUP" -j ACCEPT
        log Info "已添加核心用户 $CORE_USER:$CORE_GROUP 绕过"
    elif check_kernel_feature "NETFILTER_XT_MATCH_MARK" && [ -n "$ROUTING_MARK" ]; then
        $cmd -t "$table" -A "APP_CHAIN$suffix" -m mark --mark "$ROUTING_MARK" -j ACCEPT
        log Info "已添加标记流量绕过，核心标记 $ROUTING_MARK"
    else
        log Warn "核心流量绕过未配置，可能导致流量死循环"
    fi

    if [ "$APP_PROXY_ENABLE" -eq 1 ]; then
        if check_kernel_feature "NETFILTER_XT_MATCH_OWNER"; then
            log Info "正在设置应用过滤规则，模式：$APP_PROXY_MODE"
            case "$APP_PROXY_MODE" in
                blacklist)
                    if [ -n "$BYPASS_APPS_LIST" ]; then
                        uids=$(find_packages_uid "$BYPASS_APPS_LIST" || true)
                        for uid in $uids; do
                            if [ -n "$uid" ]; then
                                $cmd -t "$table" -A "APP_CHAIN$suffix" -m owner --uid-owner "$uid" -j ACCEPT
                                log Info "已添加 UID 绕过：$uid"
                            fi
                        done
                    else
                        log Warn "应用黑名单模式已启用但未配置绕过应用"
                    fi
                    $cmd -t "$table" -A "APP_CHAIN$suffix" -j RETURN
                    ;;
                whitelist)
                    if [ -n "$PROXY_APPS_LIST" ]; then
                        uids=$(find_packages_uid "$PROXY_APPS_LIST" || true)
                        for uid in $uids; do
                            if [ -n "$uid" ]; then
                                $cmd -t "$table" -A "APP_CHAIN$suffix" -m owner --uid-owner "$uid" -j RETURN
                                log Info "已添加 UID 代理：$uid"
                            fi
                        done
                    else
                        log Warn "应用白名单模式已启用但未配置代理应用"
                    fi
                    $cmd -t "$table" -A "APP_CHAIN$suffix" -j ACCEPT
                    ;;
            esac
        else
            log Warn "应用过滤需要 NETFILTER_XT_MATCH_OWNER 内核特性，该特性不可用"
        fi
    fi

    if [ "$DNS_HIJACK_ENABLE" -ne 0 ]; then
        if [ "$mode" = "redirect" ]; then
            setup_dns_hijack "$family" "redirect"
        else
            if [ "$DNS_HIJACK_ENABLE" -eq 2 ]; then
                setup_dns_hijack "$family" "redirect2"
            else
                setup_dns_hijack "$family" "tproxy"
            fi
        fi
    fi

    if [ "$mode" = "tproxy" ]; then
        $cmd -t "$table" -A "PROXY_PREROUTING$suffix" -p tcp -j TPROXY --on-port "$PROXY_TCP_PORT" --tproxy-mark "$mark"
        $cmd -t "$table" -A "PROXY_PREROUTING$suffix" -p udp -j TPROXY --on-port "$PROXY_UDP_PORT" --tproxy-mark "$mark"
        $cmd -t "$table" -A "PROXY_OUTPUT$suffix" -j MARK --set-mark "$mark"
        log Info "TPROXY 模式规则已添加"
    else
        $cmd -t "$table" -A "PROXY_PREROUTING$suffix" -j REDIRECT --to-ports "$PROXY_TCP_PORT"
        $cmd -t "$table" -A "PROXY_OUTPUT$suffix" -j REDIRECT --to-ports "$PROXY_TCP_PORT"
        log Info "REDIRECT 模式规则已添加"
    fi

    # 添加规则到主链
    if [ "$PROXY_UDP" -eq 1 ] || [ "$mode" = "redirect" ]; then
        $cmd -t "$table" -I PREROUTING -p udp -j "PROXY_PREROUTING$suffix"
        $cmd -t "$table" -I OUTPUT -p udp -j "PROXY_OUTPUT$suffix"
        log Info "已添加 UDP 规则到 PREROUTING 和 OUTPUT 链"
    fi
    if [ "$PROXY_TCP" -eq 1 ]; then
        $cmd -t "$table" -I PREROUTING -p tcp -j "PROXY_PREROUTING$suffix"
        $cmd -t "$table" -I OUTPUT -p tcp -j "PROXY_OUTPUT$suffix"
        log Info "已添加 TCP 规则到 PREROUTING 和 OUTPUT 链"
    fi

    log Info "IPv${family} 的 $mode_name 链设置完成"
}

setup_dns_hijack() {
    local family="$1"
    local mode="$2"
    local suffix=""
    local mark="$MARK_VALUE"
    local cmd="iptables"

    if [ "$family" = "6" ]; then
        suffix="6"
        mark="$MARK_VALUE6"
        cmd="ip6tables"
    fi

    case "$mode" in
        tproxy)
            # 在 PREROUTING 链处理来自接口的 DNS (DNS_HIJACK_PRE)
            $cmd -t mangle -A "DNS_HIJACK_PRE$suffix" -j RETURN
            # 在 OUTPUT 链处理本地 DNS 劫持 (DNS_HIJACK_OUT)
            $cmd -t mangle -A "DNS_HIJACK_OUT$suffix" -j RETURN

            log Info "已使用 TPROXY 模式启用 DNS 劫持"
            ;;
        redirect)
            # 使用 REDIRECT 方法处理 DNS
            $cmd -t nat -A "DNS_HIJACK_PRE$suffix" -p tcp --dport 53 -j REDIRECT --to-ports "$DNS_PORT"
            $cmd -t nat -A "DNS_HIJACK_PRE$suffix" -p udp --dport 53 -j REDIRECT --to-ports "$DNS_PORT"
            $cmd -t nat -A "DNS_HIJACK_OUT$suffix" -p tcp --dport 53 -j REDIRECT --to-ports "$DNS_PORT"
            $cmd -t nat -A "DNS_HIJACK_OUT$suffix" -p udp --dport 53 -j REDIRECT --to-ports "$DNS_PORT"

            log Info "已使用 REDIRECT 模式启用 DNS 劫持至端口 $DNS_PORT"
            ;;
        redirect2)
            # 使用 REDIRECT 方法处理 DNS
            if [ "$family" = "6" ] && {
                ! check_kernel_feature "IP6_NF_NAT" || ! check_kernel_feature "IP6_NF_TARGET_REDIRECT"
            }; then
                log Warn "IPv6: 内核不支持 IPv6 NAT 或 REDIRECT，跳过 IPv6 DNS 劫持"
                return 0
            fi
            safe_chain_create "$family" "nat" "NAT_DNS_HIJACK$suffix"
            $cmd -t nat -A "NAT_DNS_HIJACK$suffix" -p tcp --dport 53 -j REDIRECT --to-ports "$DNS_PORT"
            $cmd -t nat -A "NAT_DNS_HIJACK$suffix" -p udp --dport 53 -j REDIRECT --to-ports "$DNS_PORT"

            [ "$PROXY_MOBILE" -eq 1 ] && $cmd -t nat -A PREROUTING -i "$MOBILE_INTERFACE" -j "NAT_DNS_HIJACK$suffix"
            [ "$PROXY_WIFI" -eq 1 ] && $cmd -t nat -A PREROUTING -i "$WIFI_INTERFACE" -j "NAT_DNS_HIJACK$suffix"
            [ "$PROXY_USB" -eq 1 ] && $cmd -t nat -A PREROUTING -i "$USB_INTERFACE" -j "NAT_DNS_HIJACK$suffix"

            $cmd -t nat -A OUTPUT -p udp --dport 53 -m owner --uid-owner "$CORE_USER" --gid-owner "$CORE_GROUP" -j ACCEPT
            $cmd -t nat -A OUTPUT -p tcp --dport 53 -m owner --uid-owner "$CORE_USER" --gid-owner "$CORE_GROUP" -j ACCEPT
            $cmd -t nat -A OUTPUT -j "NAT_DNS_HIJACK$suffix"

            log Info "已使用 REDIRECT 模式启用 DNS 劫持至端口 $DNS_PORT"
            ;;
    esac
}

setup_tproxy_chain4() {
    setup_proxy_chain 4 "tproxy"
}

setup_redirect_chain4() {
    log Warn "REDIRECT 模式仅支持 TCP"
    setup_proxy_chain 4 "redirect"
}

setup_tproxy_chain6() {
    setup_proxy_chain 6 "tproxy"
}

setup_redirect_chain6() {
    if ! check_kernel_feature "IP6_NF_NAT" || ! check_kernel_feature "IP6_NF_TARGET_REDIRECT"; then
        log Warn "IPv6: 内核不支持 IPv6 NAT 或 REDIRECT，跳过 IPv6 代理设置"
        return 0
    fi
    log Warn "REDIRECT 模式仅支持 TCP"
    setup_proxy_chain 6 "redirect"
}

setup_routing4() {
    log Info "正在设置 IPv4 路由规则"

    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] ip rule add fwmark $MARK_VALUE lookup $TABLE_ID"
        log Debug "[DRY-RUN] ip route add local 0.0.0.0/0 dev lo table $TABLE_ID"
        log Debug "[DRY-RUN] echo 1 > /proc/sys/net/ipv4/ip_forward"
    else
        ip_rule del fwmark "$MARK_VALUE" lookup "$TABLE_ID" 2> /dev/null || true
        ip_route del local 0.0.0.0/0 dev lo table "$TABLE_ID" 2> /dev/null || true

        if ! ip_rule add fwmark "$MARK_VALUE" table "$TABLE_ID" pref "$TABLE_ID"; then
            log Error "添加 IPv4 路由规则失败"
            return 1
        fi

        if ! ip_route add local 0.0.0.0/0 dev lo table "$TABLE_ID"; then
            log Error "添加 IPv4 路由失败"
            ip_rule del fwmark "$MARK_VALUE" table "$TABLE_ID" pref "$TABLE_ID" 2> /dev/null || true
            return 1
        fi

        echo 1 > /proc/sys/net/ipv4/ip_forward
    fi

    log Info "IPv4 路由设置完成"
}

setup_routing6() {
    log Info "正在设置 IPv6 路由规则"

    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] ip -6 rule add fwmark $MARK_VALUE6 lookup $TABLE_ID"
        log Debug "[DRY-RUN] ip -6 route add local ::/0 dev lo table $TABLE_ID"
        log Debug "[DRY-RUN] echo 1 > /proc/sys/net/ipv6/conf/all/forwarding"
    else
        ip6_rule del fwmark "$MARK_VALUE6" table "$TABLE_ID" pref "$TABLE_ID" 2> /dev/null || true
        ip6_route del local ::/0 dev lo table "$TABLE_ID" 2> /dev/null || true

        if ! ip6_rule add fwmark "$MARK_VALUE6" table "$TABLE_ID" pref "$TABLE_ID"; then
            log Error "添加 IPv6 路由规则失败"
            return 1
        fi

        if ! ip6_route add local ::/0 dev lo table "$TABLE_ID"; then
            log Error "添加 IPv6 路由失败"
            ip6_rule del fwmark "$MARK_VALUE6" table "$TABLE_ID" pref "$TABLE_ID" 2> /dev/null || true
            return 1
        fi

        echo 1 > /proc/sys/net/ipv6/conf/all/forwarding
    fi

    log Info "IPv6 路由设置完成"
}

# 统一清理函数
cleanup_chain() {
    local family="$1"
    local mode="$2"
    local suffix=""
    local cmd="iptables"

    if [ "$family" = "6" ]; then
        suffix="6"
        cmd="ip6tables"
    fi

    # 设置模式名称用于日志
    local mode_name="$mode"
    if [ "$mode" = "tproxy" ]; then
        mode_name="TPROXY"
    else
        mode_name="REDIRECT"
    fi

    log Info "正在清理 IPv${family} 的 $mode_name 链"

    local table="mangle"
    if [ "$mode" = "redirect" ]; then
        table="nat"
    fi

    # 从主链移除规则
    $cmd -t "$table" -D "PROXY_PREROUTING$suffix" -j "BYPASS_IP$suffix" 2> /dev/null || true
    $cmd -t "$table" -D "PROXY_PREROUTING$suffix" -j "PROXY_INTERFACE$suffix" 2> /dev/null || true
    $cmd -t "$table" -D "PROXY_PREROUTING$suffix" -j "MAC_CHAIN$suffix" 2> /dev/null || true
    $cmd -t "$table" -D "PROXY_PREROUTING$suffix" -j "DNS_HIJACK_PRE$suffix" 2> /dev/null || true

    $cmd -t "$table" -D "PROXY_OUTPUT$suffix" -j "BYPASS_IP$suffix" 2> /dev/null || true
    $cmd -t "$table" -D "PROXY_OUTPUT$suffix" -j "BYPASS_INTERFACE$suffix" 2> /dev/null || true
    $cmd -t "$table" -D "PROXY_OUTPUT$suffix" -j "APP_CHAIN$suffix" 2> /dev/null || true
    $cmd -t "$table" -D "PROXY_OUTPUT$suffix" -j "DNS_HIJACK_OUT$suffix" 2> /dev/null || true

    if [ "$PROXY_TCP" -eq 1 ]; then
        $cmd -t "$table" -D PREROUTING -p tcp -j "PROXY_PREROUTING$suffix" 2> /dev/null || true
        $cmd -t "$table" -D OUTPUT -p tcp -j "PROXY_OUTPUT$suffix" 2> /dev/null || true
    fi
    if [ "$PROXY_UDP" -eq 1 ]; then
        $cmd -t "$table" -D PREROUTING -p udp -j "PROXY_PREROUTING$suffix" 2> /dev/null || true
        $cmd -t "$table" -D OUTPUT -p udp -j "PROXY_OUTPUT$suffix" 2> /dev/null || true
    fi

    # 定义链
    local chains=""
    if [ "$family" = "6" ]; then
        chains="PROXY_PREROUTING6 PROXY_OUTPUT6 BYPASS_IP6 BYPASS_INTERFACE6 PROXY_INTERFACE6 DNS_HIJACK_PRE6 DNS_HIJACK_OUT6 APP_CHAIN6 MAC_CHAIN6"
    else
        chains="PROXY_PREROUTING PROXY_OUTPUT BYPASS_IP BYPASS_INTERFACE PROXY_INTERFACE DNS_HIJACK_PRE DNS_HIJACK_OUT APP_CHAIN MAC_CHAIN"
    fi

    # 清理链
    for c in $chains; do
        $cmd -t "$table" -F "$c" 2> /dev/null || true
        $cmd -t "$table" -X "$c" 2> /dev/null || true
    done

    # 移除 DNS 规则（如果适用）
    if [ "$mode" = "tproxy" ] && [ "$DNS_HIJACK_ENABLE" -eq 2 ]; then
        $cmd -t nat -D PREROUTING -i "$MOBILE_INTERFACE" -j "NAT_DNS_HIJACK$suffix" 2> /dev/null || true
        $cmd -t nat -D PREROUTING -i "$WIFI_INTERFACE" -j "NAT_DNS_HIJACK$suffix" 2> /dev/null || true
        $cmd -t nat -D PREROUTING -i "$USB_INTERFACE" -j "NAT_DNS_HIJACK$suffix" 2> /dev/null || true
        $cmd -t nat -D OUTPUT -p udp --dport 53 -m owner --uid-owner "$CORE_USER" --gid-owner "$CORE_GROUP" -j ACCEPT 2> /dev/null || true
        $cmd -t nat -D OUTPUT -p tcp --dport 53 -m owner --uid-owner "$CORE_USER" --gid-owner "$CORE_GROUP" -j ACCEPT 2> /dev/null || true
        $cmd -t nat -D OUTPUT -j "NAT_DNS_HIJACK$suffix" 2> /dev/null || true
        $cmd -t nat -F "NAT_DNS_HIJACK$suffix" 2> /dev/null || true
        $cmd -t nat -X "NAT_DNS_HIJACK$suffix" 2> /dev/null || true
    fi

    log Info "IPv${family} 的 $mode_name 链清理完成"
}

cleanup_tproxy_chain4() {
    cleanup_chain 4 "tproxy"
}

cleanup_tproxy_chain6() {
    cleanup_chain 6 "tproxy"
}

cleanup_redirect_chain4() {
    cleanup_chain 4 "redirect"
}

cleanup_redirect_chain6() {
    if ! check_kernel_feature "IP6_NF_NAT" || ! check_kernel_feature "IP6_NF_TARGET_REDIRECT"; then
        log Warn "IPv6: 内核不支持 IPv6 NAT 或 REDIRECT，跳过 IPv6 清理"
        return 0
    fi
    cleanup_chain 6 "redirect"
}

cleanup_routing4() {
    log Info "正在清理 IPv4 路由规则"

    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] ip rule del fwmark $MARK_VALUE table $TABLE_ID pref $TABLE_ID"
        log Debug "[DRY-RUN] ip route del local 0.0.0.0/0 dev lo table $TABLE_ID"
        log Debug "[DRY-RUN] echo 0 > /proc/sys/net/ipv4/ip_forward"
    else
        ip_rule del fwmark "$MARK_VALUE" table "$TABLE_ID" pref "$TABLE_ID" 2> /dev/null || true
        ip_route del local 0.0.0.0/0 dev lo table "$TABLE_ID" 2> /dev/null || true
        echo 0 > /proc/sys/net/ipv4/ip_forward 2> /dev/null || true
    fi

    log Info "IPv4 路由清理完成"
}

cleanup_routing6() {
    log Info "正在清理 IPv6 路由规则"

    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] ip -6 rule del fwmark $MARK_VALUE6 table $TABLE_ID pref $TABLE_ID"
        log Debug "[DRY-RUN] ip -6 route del local ::/0 dev lo table $TABLE_ID"
        log Debug "[DRY-RUN] echo 0 > /proc/sys/net/ipv6/conf/all/forwarding"
    else
        ip6_rule del fwmark "$MARK_VALUE6" table "$TABLE_ID" pref "$TABLE_ID" 2> /dev/null || true
        ip6_route del local ::/0 dev lo table "$TABLE_ID" 2> /dev/null || true
        echo 0 > /proc/sys/net/ipv6/conf/all/forwarding 2> /dev/null || true
    fi

    log Info "IPv6 路由清理完成"
}

cleanup_ipset() {
    if [ "$BYPASS_CN_IP" -eq 0 ]; then
        log Debug "中国 IP 绕过已禁用，跳过 ipset 清理"
        return 0
    fi

    if [ "$DRY_RUN" -eq 1 ]; then
        log Debug "[DRY-RUN] ipset destroy cnip"
        log Debug "[DRY-RUN] ipset destroy cnip6"
    else
        ipset destroy cnip 2> /dev/null || true
        ipset destroy cnip6 2> /dev/null || true
        log Info "ipset 'cnip' 和 'cnip6' 已销毁"
    fi
}

detect_proxy_mode() {
    USE_TPROXY=0
    case "$PROXY_MODE" in
        0)
            if check_tproxy_support; then
                USE_TPROXY=1
                log Info "内核支持 TPROXY，使用 TPROXY 模式 (自动)"
            else
                log Warn "内核不支持 TPROXY，回退至 REDIRECT 模式 (自动)"
            fi
            ;;
        1)
            if check_tproxy_support; then
                USE_TPROXY=1
                log Info "使用 TPROXY 模式 (配置强制)"
            else
                log Error "强制启用了 TPROXY 模式，但内核不支持 TPROXY"
                exit 1
            fi
            ;;
        2)
            log Info "使用 REDIRECT 模式 (配置强制)"
            ;;
    esac
}

start_proxy() {
    log Info "开始代理设置..."
    if [ "$BYPASS_CN_IP" -eq 1 ]; then
        if ! check_kernel_feature "IP_SET" || ! check_kernel_feature "NETFILTER_XT_SET"; then
            log Error "内核不支持 ipset (CONFIG_IP_SET, CONFIG_NETFILTER_XT_SET)，无法绕过中国 IP"
            BYPASS_CN_IP=0
        else
            download_cn_ip_list || log Warn "下载 CN IP 列表失败，继续运行但不包含该列表"
            if ! setup_cn_ipset; then
                log Error "设置 ipset 失败，CN 绕过已禁用"
                BYPASS_CN_IP=0
            fi
        fi
    fi

    if [ "$USE_TPROXY" -eq 1 ]; then
        setup_tproxy_chain4
        setup_routing4
        if [ "$PROXY_IPV6" -eq 1 ]; then
            setup_tproxy_chain6
            setup_routing6
        fi
    else
        setup_redirect_chain4
        if [ "$PROXY_IPV6" -eq 1 ]; then
            setup_redirect_chain6
        fi
    fi
    log Info "代理设置完成"
}

stop_proxy() {
    log Info "正在停止代理..."
    if [ "$USE_TPROXY" -eq 1 ]; then
        log Info "清理 TPROXY 链"
        cleanup_tproxy_chain4
        cleanup_routing4
        # 无条件清理 IPv6 规则，避免之前开启后关闭时残留规则
        cleanup_tproxy_chain6
        cleanup_routing6
    else
        log Info "清理 REDIRECT 链"
        cleanup_redirect_chain4
        # 无条件清理 IPv6 规则，避免之前开启后关闭时残留规则
        cleanup_redirect_chain6
    fi
    cleanup_ipset
    log Info "代理已停止"
}

show_usage() {
    cat << EOF
用法: $(basename "$0") {start|stop|restart} [--dry-run]

选项:
  --dry-run    空跑模式（仅显示将要执行的命令，不实际修改系统）
  -h, --help   显示此帮助信息
EOF
}

parse_args() {
    MAIN_CMD=""

    while [ $# -gt 0 ]; do
        case "$1" in
            start | stop | restart)
                if [ -n "$MAIN_CMD" ]; then
                    log Error "指定了多个命令。"
                    exit 1
                fi
                MAIN_CMD="$1"
                ;;
            --dry-run)
                DRY_RUN=1
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            *)
                log Error "无效参数：$1"
                show_usage
                exit 1
                ;;
        esac
        shift
    done

    if [ -z "$MAIN_CMD" ]; then
        log Error "未指定命令"
        show_usage
        exit 1
    fi
}

main() {
    load_config
    if ! validate_config; then
        log Error "配置验证失败"
        exit 1
    fi

    check_root
    check_dependencies

    detect_proxy_mode

    case "$MAIN_CMD" in
        start)
            start_proxy
            ;;
        stop)
            stop_proxy
            ;;
        restart)
            log Info "正在重启代理..."
            stop_proxy
            sleep 2
            start_proxy
            log Info "代理已重启"
            ;;
        *)
            log Error "无效命令：$MAIN_CMD"
            show_usage
            exit 1
            ;;
    esac
}

parse_args "$@"

main
