#!/system/bin/sh

#############################################################################
# V2Ray Multi-Protocol URL to JSON Converter
# 支持协议: VLESS, VMess, Trojan, Shadowsocks, SOCKS, HTTP
# 基于 v2rayNG 的解析逻辑实现
#############################################################################

set -e

# 路径配置
readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly ADDCONFIG_FILE="$MODDIR/config/xray/outbounds/"

# 显示帮助信息
show_help() {
    cat << EOF
用法: $0 [选项] <PROXY_URL>

将代理 URL 转换为 Xray 出站配置文件（仅 outbounds）

支持的协议:
    - VLESS   (vless://...)
    - VMess   (vmess://...)
    - Trojan  (trojan://...)
    - Shadowsocks (ss://...)
    - SOCKS   (socks://...)
    - HTTP    (http://... 或 https://...)

选项:
    -o, --output <file>     输出文件路径 (默认: outbounds/<备注名>.json)
    -d, --dir <directory>   输出目录 (用于订阅分组)
    -h, --help              显示此帮助信息

示例:
    $0 "vless://uuid@server:443?type=xhttp&security=tls#name"
    $0 "vmess://base64encodedJSON"
    $0 "trojan://password@server:443?security=tls#name"
    $0 "ss://method:password@server:port#name"
    $0 -o my_config.json "vless://..."

EOF
    exit 0
}

# URL 解码函数 (支持 UTF-8 多字节字符)
url_decode() {
    local url_encoded="$1"
    
    # 将 + 替换为空格
    url_encoded="${url_encoded//+/ }"
    
    # 使用 sed 和 printf 正确解码 UTF-8
    # 将 %XX 转换为实际字符
    echo -e "$(echo "$url_encoded" | sed 's/%\([0-9A-Fa-f][0-9A-Fa-f]\)/\\x\1/g')"
}

# Base64 解码函数（兼容多种实现）
base64_decode() {
    local input="$1"
    
    # 尝试使用不同的 base64 命令
    if command -v base64 &> /dev/null; then
        if base64 --help 2>&1 | grep -q "\-\-decode"; then
            echo "$input" | base64 --decode 2>/dev/null || echo "$input" | base64 -d 2>/dev/null
        else
            echo "$input" | base64 -d 2>/dev/null || echo "$input" | base64 -D 2>/dev/null
        fi
    else
        echo -e "${RED}错误: 未找到 base64 命令${NC}" >&2
        exit 1
    fi
}

# 自动检测协议类型
detect_protocol() {
    url="$1"
    
    case "$url" in
        vless://*)
            PROTOCOL="VLESS"
            ;;
        vmess://*)
            PROTOCOL="VMESS"
            ;;
        trojan://*)
            PROTOCOL="TROJAN"
            ;;
        ss://*)
            PROTOCOL="SHADOWSOCKS"
            ;;
        socks://*)
            PROTOCOL="SOCKS"
            ;;
        http://*|https://*)
            PROTOCOL="HTTP"
            ;;
        *)
            printf '%b' "${RED}错误: 不支持的协议${NC}\n" >&2
            echo "URL: $url"
            exit 1
            ;;
    esac
}

#############################################################################
# VLESS 协议解析
#############################################################################

parse_vless() {
    url="$1"
    url="${url#vless://}"
    
    # 提取备注
    case "$url" in
        *"#"*)
            REMARK=$(url_decode "${url##*#}")
            url="${url%#*}"
            ;;
        *)
            REMARK="VLESS Server"
            ;;
    esac
    
    # 提取查询参数
    case "$url" in
        *"?"*)
            QUERY="${url##*\?}"
            url="${url%\?*}"
            ;;
        *)
            printf '%b' "${RED}错误: VLESS URL 缺少查询参数${NC}\n" >&2
            return 1
            ;;
    esac
    
    # 提取 UUID@SERVER:PORT
    UUID=$(echo "$url" | sed -n 's/^\([^@]*\)@.*/\1/p')
    SERVER=$(echo "$url" | sed -n 's/^[^@]*@\([^:]*\):.*/\1/p')
    PORT=$(echo "$url" | sed -n 's/^[^@]*@[^:]*:\([0-9]*\)$/\1/p')
    
    if [ -z "$UUID" ] || [ -z "$SERVER" ] || [ -z "$PORT" ]; then
        printf '%b' "${RED}错误: 无法解析 VLESS 服务器信息${NC}\n" >&2
        return 1
    fi
    
    # 解析查询参数
    parse_query_params "$QUERY"
    
    # VLESS 特定参数
    ENCRYPTION="${ENCRYPTION:-none}"
}

#############################################################################
# VMess 协议解析（纯 Shell 实现，无需 jq）
#############################################################################

# JSON 字段提取函数（纯 sh 实现）
extract_json_field() {
    json="$1"
    field="$2"
    default="${3:-}"
    
    # 匹配 "field":"value" 或 "field":value（数字）
    value=$(echo "$json" | grep -o "\"$field\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | head -1 | sed 's/.*:"\(.*\)".*/\1/')
    
    # 如果没找到，尝试匹配数字值
    if [ -z "$value" ]; then
        value=$(echo "$json" | grep -o "\"$field\"[[:space:]]*:[[:space:]]*[0-9]*" | head -1 | sed 's/.*:[[:space:]]*\([0-9]*\).*/\1/')
    fi
    
    # 如果还是没找到，使用默认值
    if [ -z "$value" ]; then
        value="$default"
    fi
    
    echo "$value"
}

parse_vmess() {
    url="$1"
    url="${url#vmess://}"
    
    # VMess 使用 Base64 编码的 JSON
    decoded=$(base64_decode "$url")
    
    if [ -z "$decoded" ]; then
        printf '%b' "${RED}错误: VMess Base64 解码失败${NC}\\n" >&2
        return 1
    fi
    
    # 纯 Shell 解析 JSON
    REMARK=$(extract_json_field "$decoded" "ps" "VMess Server")
    SERVER=$(extract_json_field "$decoded" "add")
    PORT=$(extract_json_field "$decoded" "port")
    UUID=$(extract_json_field "$decoded" "id")
    SECURITY_METHOD=$(extract_json_field "$decoded" "scy" "auto")
    NETWORK=$(extract_json_field "$decoded" "net" "tcp")
    HEADER_TYPE=$(extract_json_field "$decoded" "type" "none")
    HOST=$(extract_json_field "$decoded" "host" "")
    PATH_VALUE=$(extract_json_field "$decoded" "path" "")
    SECURITY=$(extract_json_field "$decoded" "tls" "")
    SNI=$(extract_json_field "$decoded" "sni" "")
    ALPN=$(extract_json_field "$decoded" "alpn" "")
    FINGERPRINT=$(extract_json_field "$decoded" "fp" "")
    
    insecure=$(extract_json_field "$decoded" "insecure" "0")
    if [ "$insecure" = "1" ]; then
        ALLOW_INSECURE="true"
    else
        ALLOW_INSECURE="false"
    fi
    
    # gRPC 特殊处理
    if [ "$NETWORK" = "grpc" ]; then
        MODE=$(extract_json_field "$decoded" "type" "gun")
        SERVICE_NAME=$(extract_json_field "$decoded" "path" "")
        AUTHORITY=$(extract_json_field "$decoded" "host" "")
    elif [ "$NETWORK" = "kcp" ]; then
        SEED=$(extract_json_field "$decoded" "path" "")
    fi
    
    # 验证必要字段
    if [ -z "$SERVER" ] || [ -z "$PORT" ] || [ -z "$UUID" ]; then
        printf '%b' "${RED}错误: VMess JSON 缺少必要字段 (add/port/id)${NC}\\n" >&2
        return 1
    fi
}

#############################################################################
# Trojan 协议解析
#############################################################################

parse_trojan() {
    url="$1"
    url="${url#trojan://}"
    
    # 提取备注
    case "$url" in
        *"#"*)
            REMARK=$(url_decode "${url##*#}")
            url="${url%#*}"
            ;;
        *)
            REMARK="Trojan Server"
            ;;
    esac
    
    # Trojan 协议默认值（在解析参数前预设，parse_query_params 只会覆盖明确指定的参数）
    NETWORK="tcp"
    SECURITY="tls"
    ALLOW_INSECURE="false"
    
    # 提取查询参数
    case "$url" in
        *"?"*)
            QUERY="${url##*\?}"
            url="${url%\?*}"
            parse_query_params "$QUERY"
            ;;
    esac
    
    # 提取 PASSWORD@SERVER:PORT
    PASSWORD=$(echo "$url" | sed -n 's/^\([^@]*\)@.*/\1/p')
    SERVER=$(echo "$url" | sed -n 's/^[^@]*@\([^:]*\):.*/\1/p')
    PORT=$(echo "$url" | sed -n 's/^[^@]*@[^:]*:\([0-9]*\).*/\1/p')
    
    if [ -z "$PASSWORD" ] || [ -z "$SERVER" ] || [ -z "$PORT" ]; then
        printf '%b' "${RED}错误: 无法解析 Trojan 服务器信息${NC}\n" >&2
        return 1
    fi
}

#############################################################################
# Shadowsocks 协议解析
#############################################################################

parse_shadowsocks() {
    url="$1"
    url="${url#ss://}"
    
    # 提取备注
    case "$url" in
        *"#"*)
            REMARK=$(url_decode "${url##*#}")
            url="${url%#*}"
            ;;
        *)
            REMARK="Shadowsocks Server"
            ;;
    esac
    
    # SIP002 格式: ss://base64(method:password)@server:port
    # 或者 ss://method:password@server:port
    
    # 检查是否有 @
    case "$url" in
        *"@"*)
            # SIP002 格式
            userinfo="${url%%@*}"
            serverinfo="${url#*@}"
            
            # 解析服务器:端口
            SERVER=$(echo "$serverinfo" | sed -n 's/^\([^:]*\):.*/\1/p')
            PORT=$(echo "$serverinfo" | sed -n 's/^[^:]*:\([0-9]*\).*/\1/p')
            
            if [ -z "$SERVER" ] || [ -z "$PORT" ]; then
                printf '%b' "${RED}错误: 无法解析 Shadowsocks 服务器信息${NC}\n" >&2
                return 1
            fi
            
            # 解析 userinfo (可能是 base64 编码的)
            case "$userinfo" in
                *":"*)
                    # 已经是明文 method:password
                    SS_METHOD="${userinfo%%:*}"
                    PASSWORD="${userinfo#*:}"
                    ;;
                *)
                    # Base64 编码的
                    decoded=$(base64_decode "$userinfo")
                    SS_METHOD=$(echo "$decoded" | sed -n 's/^\([^:]*\):.*/\1/p')
                    PASSWORD=$(echo "$decoded" | sed -n 's/^[^:]*:\(.*\)$/\1/p')
                    
                    if [ -z "$SS_METHOD" ] || [ -z "$PASSWORD" ]; then
                        printf '%b' "${RED}错误: 无法解析 Shadowsocks 加密方法和密码${NC}\n" >&2
                        return 1
                    fi
                    ;;
            esac
            ;;
        *)
            # Legacy 格式: 整个 URL 是 base64 编码的
            decoded=$(base64_decode "$url")
            SS_METHOD=$(echo "$decoded" | sed -n 's/^\([^:]*\):.*/\1/p')
            PASSWORD=$(echo "$decoded" | sed -n 's/^[^:]*:\([^@]*\)@.*/\1/p')
            SERVER=$(echo "$decoded" | sed -n 's/^[^@]*@\([^:]*\):.*/\1/p')
            PORT=$(echo "$decoded" | sed -n 's/^[^@]*@[^:]*:\([0-9]*\)$/\1/p')
            
            if [ -z "$SS_METHOD" ] || [ -z "$PASSWORD" ] || [ -z "$SERVER" ] || [ -z "$PORT" ]; then
                printf '%b' "${RED}错误: 无法解析 Shadowsocks 配置${NC}\n" >&2
                return 1
            fi
            ;;
    esac
}

#############################################################################
# SOCKS 协议解析
#############################################################################

parse_socks() {
    url="$1"
    url="${url#socks://}"
    
    # 提取备注
    case "$url" in
        *"#"*)
            REMARK=$(url_decode "${url##*#}")
            url="${url%#*}"
            ;;
        *)
            REMARK="SOCKS Server"
            ;;
    esac
    
    # SOCKS URL 格式: socks://[user:pass@]server:port
    case "$url" in
        *"@"*)
            # 有认证
            userinfo="${url%%@*}"
            serverinfo="${url#*@}"
            
            # 可能是 base64 编码的
            case "$userinfo" in
                *":"*)
                    ;; # 已经是明文
                *)
                    userinfo=$(base64_decode "$userinfo")
                    ;;
            esac
            
            USERNAME=$(echo "$userinfo" | sed -n 's/^\([^:]*\):.*/\1/p')
            PASSWORD=$(echo "$userinfo" | sed -n 's/^[^:]*:\(.*\)$/\1/p')
            
            url="$serverinfo"
            ;;
    esac
    
    # 解析服务器:端口
    SERVER=$(echo "$url" | sed -n 's/^\([^:]*\):.*/\1/p')
    PORT=$(echo "$url" | sed -n 's/^[^:]*:\([0-9]*\)$/\1/p')
    
    if [ -z "$SERVER" ] || [ -z "$PORT" ]; then
        printf '%b' "${RED}错误: 无法解析 SOCKS 服务器信息${NC}\n" >&2
        return 1
    fi
}

#############################################################################
# HTTP 协议解析  
#############################################################################

parse_http() {
    url="$1"
    
    # 移除协议前缀
    url="${url#http://}"
    url="${url#https://}"
    
    # 提取备注
    case "$url" in
        *"#"*)
            REMARK=$(url_decode "${url##*#}")
            url="${url%#*}"
            ;;
        *)
            REMARK="HTTP Proxy"
            ;;
    esac
    
    # HTTP URL 格式: http://[user:pass@]server:port
    case "$url" in
        *"@"*)
            userinfo="${url%%@*}"
            serverinfo="${url#*@}"
            
            USERNAME=$(echo "$userinfo" | sed -n 's/^\([^:]*\):.*/\1/p')
            PASSWORD=$(echo "$userinfo" | sed -n 's/^[^:]*:\(.*\)$/\1/p')
            
            url="$serverinfo"
            ;;
    esac
    
    # 解析服务器:端口
    SERVER=$(echo "$url" | sed -n 's/^\([^:]*\):.*/\1/p')
    PORT=$(echo "$url" | sed -n 's/^[^:]*:\([0-9]*\)$/\1/p')
    
    if [ -z "$SERVER" ] || [ -z "$PORT" ]; then
        printf '%b' "${RED}错误: 无法解析 HTTP 代理信息${NC}\n" >&2
        return 1
    fi
}

#############################################################################
# 查询参数解析（通用）
#############################################################################

parse_query_params() {
    query="$1"
    
    # 保存旧的 IFS
    OLD_IFS="$IFS"
    IFS='&'
    
    # 使用 for 循环而不是管道（避免子shell问题）
    for param in $query; do
        key="${param%%=*}"
        value="${param#*=}"
        value=$(url_decode "$value")
        
        case "$key" in
            type) NETWORK="$value" ;;
            security) SECURITY="$value" ;;
            encryption) ENCRYPTION="$value" ;;
            path) PATH_VALUE="$value" ;;
            host) HOST="$value" ;;
            sni|peer) SNI="$value" ;;  # peer 等同于 sni
            alpn) ALPN="$value" ;;
            fp|fingerprint) FINGERPRINT="$value" ;;
            pbk|publicKey) PUBLIC_KEY="$value" ;;
            sid|shortId) SHORT_ID="$value" ;;
            mode) MODE="$value" ;;
            serviceName) SERVICE_NAME="$value" ;;
            authority) AUTHORITY="$value" ;;
            headerType) HEADER_TYPE="$value" ;;
            seed) SEED="$value" ;;
            flow) FLOW="$value" ;;
            spx) SPIDER_X="$value" ;;
            insecure|allowInsecure) 
                if [ "$value" = "1" ]; then
                    ALLOW_INSECURE="true"
                else
                    ALLOW_INSECURE="false"
                fi
                ;;
        esac
    done
    
    # 恢复 IFS
    IFS="$OLD_IFS"
    
    # 设置默认值（仅当变量未定义时，不覆盖协议解析器预设的值）
    : "${NETWORK:=tcp}"
    : "${SECURITY:=none}"
    : "${ALLOW_INSECURE:=false}"
}

#############################################################################
# 生成 streamSettings 配置（通用）
#############################################################################

generate_stream_settings() {
    # 构建 TLS settings（如果需要）
    local tls_settings=""
    if [ "$SECURITY" = "tls" ]; then
        tls_settings="\"allowInsecure\": $ALLOW_INSECURE"
        if [ -n "$SNI" ]; then
            tls_settings="$tls_settings,
          \"serverName\": \"$SNI\""
        fi
        tls_settings="$tls_settings,
          \"show\": false"
    fi
    
    # 开始构建 streamSettings
    stream_settings="\"network\": \"$NETWORK\""
    
    # TLS 配置
    if [ "$SECURITY" = "tls" ]; then
        stream_settings="$stream_settings,
        \"security\": \"tls\",
        \"sockopt\": {
          \"domainStrategy\": \"UseIP\",
          \"happyEyeballs\": {
            \"interleave\": 2,
            \"maxConcurrentTry\": 4,
            \"prioritizeIPv6\": false,
            \"tryDelayMs\": 250
          }
        }"
    elif [ "$SECURITY" = "reality" ]; then
        # Reality 配置
        stream_settings="$stream_settings,
      \"realitySettings\": {
        \"allowInsecure\": $ALLOW_INSECURE"
        
        if [ -n "$FINGERPRINT" ]; then
            stream_settings="$stream_settings,
        \"fingerprint\": \"$FINGERPRINT\""
        fi
        
        # publicKey - Reality 必需字段
        if [ -n "$PUBLIC_KEY" ]; then
            stream_settings="$stream_settings,
        \"publicKey\": \"$PUBLIC_KEY\""
        fi
        
        if [ -n "$SNI" ]; then
            stream_settings="$stream_settings,
        \"serverName\": \"$SNI\""
        fi
        
        # shortId - Reality 必需字段
        if [ -n "$SHORT_ID" ]; then
            stream_settings="$stream_settings,
        \"shortId\": \"$SHORT_ID\""
        fi
        
        stream_settings="$stream_settings,
        \"show\": false"
        
        # spiderX
        if [ -n "$SPIDER_X" ]; then
            stream_settings="$stream_settings,
        \"spiderX\": \"$SPIDER_X\""
        fi
        
        stream_settings="$stream_settings
      },
      \"security\": \"reality\""
        
        # 添加 sockopt 配置 (用于 Reality 协议优化)
        stream_settings="$stream_settings,
      \"sockopt\": {
        \"domainStrategy\": \"UseIP\",
        \"happyEyeballs\": {
          \"interleave\": 2,
          \"maxConcurrentTry\": 4,
          \"prioritizeIPv6\": false,
          \"tryDelayMs\": 250
        }
      }"
    fi
    
    # 传输协议配置
    case "$NETWORK" in
        tcp)
            # tcpSettings
            stream_settings="$stream_settings,
        \"tcpSettings\": {
          \"header\": {
            \"type\": \"${HEADER_TYPE:-none}\""
            
            # 如果有自定义 header 类型且不是 none，添加额外配置
            if [ -n "$HEADER_TYPE" ] && [ "$HEADER_TYPE" != "none" ]; then
                if [ -n "$HOST" ] || [ -n "$PATH_VALUE" ]; then
                    stream_settings="$stream_settings,
            \"request\": {
              \"headers\": {
                \"Host\": [\"${HOST:-}\"]
              },
              \"path\": [\"${PATH_VALUE:-/}\"]
            }"
                fi
            fi
            
            stream_settings="$stream_settings
          }
        }"
            
            # TLS 需要添加 tlsSettings
            if [ "$SECURITY" = "tls" ]; then
                stream_settings="$stream_settings,
        \"tlsSettings\": {
          $tls_settings
        }"
            fi
            ;;
        
        ws)
            stream_settings="$stream_settings,
      \"wsSettings\": {
        \"path\": \"${PATH_VALUE:-/}\""
            
            if [ -n "$HOST" ]; then
                stream_settings="$stream_settings,
        \"headers\": {
          \"Host\": \"$HOST\"
        }"
            fi
            
            stream_settings="$stream_settings
      }"
            ;;
        
        xhttp)
            stream_settings="$stream_settings,
      \"xhttpSettings\": {
        \"host\": \"${HOST:-}\",
        \"mode\": \"${MODE:-auto}\",
        \"path\": \"${PATH_VALUE:-/}\"
      }"
            ;;
        
        grpc)
            stream_settings="$stream_settings,
      \"grpcSettings\": {"
            
            if [ -n "$SERVICE_NAME" ]; then
                stream_settings="$stream_settings
        \"serviceName\": \"$SERVICE_NAME\""
            fi
            
            if [ -n "$MODE" ]; then
                stream_settings="$stream_settings,
        \"mode\": \"$MODE\""
            fi
            
            if [ -n "$AUTHORITY" ]; then
                stream_settings="$stream_settings,
        \"authority\": \"$AUTHORITY\""
            fi
            
            stream_settings="$stream_settings
      }"
            ;;
        
        kcp)
            stream_settings="$stream_settings,
      \"kcpSettings\": {
        \"header\": {
          \"type\": \"${HEADER_TYPE:-none}\"
        }"
            
            if [ -n "$SEED" ]; then
                stream_settings="$stream_settings,
        \"seed\": \"$SEED\""
            fi
            
            stream_settings="$stream_settings
      }"
            ;;
        
        h2|http)
            stream_settings="$stream_settings,
      \"httpSettings\": {
        \"path\": \"${PATH_VALUE:-/}\""
            
            if [ -n "$HOST" ]; then
                stream_settings="$stream_settings,
        \"host\": [\"$HOST\"]"
            fi
            
            stream_settings="$stream_settings
      }"
            ;;
    esac
    
    echo "$stream_settings"
}

#############################################################################
# 生成不同协议的 Outbound 配置
#############################################################################

generate_outbound() {
    case "$PROTOCOL" in
        VLESS)
            # 构建 flow 字段（如果存在）
            local flow_field=""
            if [ -n "$FLOW" ]; then
                flow_field="\"flow\": \"$FLOW\","
            fi
            
            cat << EOF
    {
      "mux": {
        "concurrency": -1,
        "enabled": false
      },
      "protocol": "vless",
      "settings": {
        "vnext": [
          {
            "address": "$SERVER",
            "port": $PORT,
            "users": [
              {
                "encryption": "$ENCRYPTION",
                ${flow_field}
                "id": "$UUID",
                "level": 8
              }
            ]
          }
        ]
      },
      "streamSettings": {
        $(generate_stream_settings)
      },
      "tag": "proxy"
    }
EOF
            ;;
        
        VMESS)
            cat << EOF
    {
      "protocol": "vmess",
      "tag": "proxy",
      "settings": {
        "vnext": [
          {
            "address": "$SERVER",
            "port": $PORT,
            "users": [
              {
                "id": "$UUID",
                "security": "${SECURITY_METHOD:-auto}",
                "alterId": 0,
                "level": 8
              }
            ]
          }
        ]
      },
      "streamSettings": {
        $(generate_stream_settings)
      }
    }
EOF
            ;;
        
        TROJAN)
            cat << EOF
    {
      "mux": {
        "concurrency": -1,
        "enabled": false
      },
      "protocol": "trojan",
      "settings": {
        "servers": [
          {
            "address": "$SERVER",
            "port": $PORT,
            "ota": false,
            "password": "$PASSWORD",
            "level": 8$(if [ -n "$FLOW" ]; then echo ",
            \"flow\": \"$FLOW\""; fi)
          }
        ]
      },
      "streamSettings": {
        $(generate_stream_settings)
      },
      "tag": "proxy"
    }
EOF
            ;;
        
        SHADOWSOCKS)
            cat << EOF
    {
      "protocol": "shadowsocks",
      "tag": "proxy",
      "settings": {
        "servers": [
          {
            "address": "$SERVER",
            "port": $PORT,
            "method": "$SS_METHOD",
            "password": "$PASSWORD",
            "level": 8
          }
        ]
      }
    }
EOF
            ;;
        
        SOCKS)
            cat << EOF
    {
      "protocol": "socks",
      "tag": "proxy",
      "settings": {
        "servers": [
          {
            "address": "$SERVER",
            "port": $PORT$(if [ -n "$USERNAME" ]; then echo ",
            \"users\": [
              {
                \"user\": \"$USERNAME\",
                \"pass\": \"$PASSWORD\"
              }
            ]"; fi)
          }
        ]
      }
    }
EOF
            ;;
        
        HTTP)
            cat << EOF
    {
      "protocol": "http",
      "tag": "proxy",
      "settings": {
        "servers": [
          {
            "address": "$SERVER",
            "port": $PORT$(if [ -n "$USERNAME" ]; then echo ",
            \"users\": [
              {
                \"user\": \"$USERNAME\",
                \"pass\": \"$PASSWORD\"
              }
            ]"; fi)
          }
        ]
      }
    }
EOF
            ;;
    esac
}

#############################################################################
# 生成出站配置（仅 outbounds）
#############################################################################

generate_outbounds_config() {
    local output_file="$1"
    
    cat > "$output_file" << EOF
{
  "outbounds": [
$(generate_outbound),
    {
      "protocol": "freedom",
      "settings": {
        "domainStrategy": "UseIP"
      },
      "tag": "direct"
    },
    {
      "protocol": "blackhole",
      "settings": {
        "response": {
          "type": "http"
        }
      },
      "tag": "block"
    }
  ]
}
EOF
}

#############################################################################
# 主程序
#############################################################################

# 默认参数
OUTPUT_FILE="config.json"
OUTPUT_DIR=""
PROXY_URL=""

# 解析命令行参数
while [ $# -gt 0 ]; do
    case $1 in
        -o|--output)
            OUTPUT_FILE="$2"
            shift 2
            ;;
        -d|--dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -h|--help)
            show_help
            ;;
        *)
            if [ -z "$PROXY_URL" ]; then
                PROXY_URL="$1"
            else
                printf '%b' "${RED}错误: 未知参数 $1${NC}\n" >&2
                echo "使用 -h 或 --help 查看帮助信息"
                exit 1
            fi
            shift
            ;;
    esac
done

# 检查是否提供了 URL
if [ -z "$PROXY_URL" ]; then
    printf '%b' "${RED}错误: 请提供代理 URL${NC}\n" >&2
    echo "使用 -h 或 --help 查看帮助信息"
    exit 1
fi

# 检测协议
printf '%b' "${YELLOW}正在检测协议类型...${NC}\n"
detect_protocol "$PROXY_URL"
printf '%b' "${GREEN}检测到协议: $PROTOCOL${NC}\n"

# 解析 URL
printf '%b' "${YELLOW}正在解析 $PROTOCOL URL...${NC}\n"
case "$PROTOCOL" in
    VLESS)
        parse_vless "$PROXY_URL"
        ;;
    VMESS)
        parse_vmess "$PROXY_URL"
        ;;
    TROJAN)
        parse_trojan "$PROXY_URL"
        ;;
    SHADOWSOCKS)
        parse_shadowsocks "$PROXY_URL"
        ;;
    SOCKS)
        parse_socks "$PROXY_URL"
        ;;
    HTTP)
        parse_http "$PROXY_URL"
        ;;
esac

# 如果没有通过 -o 参数指定输出文件，则使用 REMARK 作为文件名
if [ "$OUTPUT_FILE" = "config.json" ]; then
    # 清理文件名中的文件系统不安全字符、空格和回车符
    # 移除: / \ : * ? " < > | \r 并将空格转为下划线
    SAFE_REMARK=$(echo "$REMARK" | tr -d '\r' | sed 's/[\/\\:*?"<>| ]/_/g')
    
    # 使用指定目录或默认目录
    if [ -n "$OUTPUT_DIR" ]; then
        mkdir -p "$OUTPUT_DIR"
        OUTPUT_FILE="${OUTPUT_DIR}/${SAFE_REMARK}.json"
    else
        mkdir -p "$ADDCONFIG_FILE"
        OUTPUT_FILE="${ADDCONFIG_FILE}${SAFE_REMARK}.json"
    fi
fi

# 显示解析结果
printf '%b' "${GREEN}解析成功:${NC}\n"
echo "  协议: $PROTOCOL"
echo "  备注: $REMARK"
echo "  服务器: $SERVER:$PORT"

case "$PROTOCOL" in
    VLESS|VMESS)
        echo "  UUID: $UUID"
        if [ -n "$NETWORK" ]; then echo "  传输协议: $NETWORK"; fi
        if [ -n "$SECURITY" ]; then echo "  安全传输: $SECURITY"; fi
        if [ -n "$SNI" ]; then echo "  SNI: $SNI"; fi
        ;;
    TROJAN)
        echo "  密码: ${PASSWORD:0:8}***"
        if [ -n "$SECURITY" ]; then echo "  安全传输: $SECURITY"; fi
        ;;
    SHADOWSOCKS)
        echo "  加密方法: $SS_METHOD"
        echo "  密码: ${PASSWORD:0:8}***"
        ;;
    SOCKS|HTTP)
        if [ -n "$USERNAME" ]; then
            echo "  用户名: $USERNAME"
            echo "  密码: ${PASSWORD:0:4}***"
        fi
        ;;
esac

# 生成配置文件
printf '%b' "${YELLOW}正在生成出站配置...${NC}\n"
generate_outbounds_config "$OUTPUT_FILE"
printf '%b' "${GREEN}出站配置已生成: $OUTPUT_FILE${NC}\n"
