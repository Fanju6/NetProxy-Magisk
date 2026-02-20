#!/system/bin/sh
set -e

readonly MODDIR="$(cd "$(dirname "$0")/../.." && pwd)"
readonly BIN_DIR="$MODDIR/bin"
readonly TEMP_DIR="$BIN_DIR/.tmp"
readonly LOG_FILE="$MODDIR/logs/service.log"
readonly REPO="XTLS/Xray-core"
readonly ARCH="android-arm64-v8a"
readonly MIRROR="https://ghfast.top/?q="
# 导入工具库
. "$MODDIR/scripts/utils/log.sh"

# 清理临时文件
cleanup() {
  [ -d "$TEMP_DIR" ] && rm -rf "$TEMP_DIR"
}
trap cleanup EXIT

# 确保目录存在
mkdir -p "$TEMP_DIR" "${LOG_FILE%/*}"

log "========== 开始更新 Xray =========="

# 获取最新版本
log "获取最新版本..."
VERSION=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"\([^"]*\)"$/\1/')

if [ -z "$VERSION" ]; then
  log "错误: 无法获取版本信息"
  exit 1
fi

log "最新版本: $VERSION"

# 检查本地版本
if [ -x "$BIN_DIR/xray" ]; then
  CURRENT=$("$BIN_DIR/xray" version 2> /dev/null | head -1 | sed 's/Xray \([0-9.]*\).*/\1/')
  if [ -n "$CURRENT" ]; then
    log "本地版本: $CURRENT"

    # 比较版本号（去掉 v 前缀）
    LATEST_NUM="${VERSION#v}"
    if [ "$CURRENT" = "$LATEST_NUM" ]; then
      log "已是最新版本，无需更新"
      exit 0
    fi

    log "准备更新: $CURRENT -> $LATEST_NUM"
  fi
else
  log "未检测到本地安装，开始首次安装"
fi

# 构建下载 URL
FILENAME="Xray-${ARCH}.zip"
GITHUB_URL="https://github.com/$REPO/releases/download/$VERSION/$FILENAME"

# URL 编码（简化版）
ENCODED_URL=$(echo "$GITHUB_URL" | sed 's/:/%3A/g; s/\//%2F/g')
DOWNLOAD_URL="${MIRROR}${ENCODED_URL}"

log "开始下载..."
ZIP_FILE="$TEMP_DIR/$FILENAME"

# 下载文件
if ! curl -L -o "$ZIP_FILE" "$DOWNLOAD_URL"; then
  log "错误: 下载失败"
  exit 1
fi

# 验证文件大小
FILE_SIZE=$(stat -c%s "$ZIP_FILE" 2> /dev/null || stat -f%z "$ZIP_FILE" 2> /dev/null || echo "0")
if [ "$FILE_SIZE" -lt 1000000 ]; then
  log "错误: 文件大小异常"
  exit 1
fi

log "下载完成，大小: $(echo "$FILE_SIZE" | awk '{printf "%.1f MB", $1/1024/1024}')"

# 备份旧版本
[ -f "$BIN_DIR/xray" ] && mv "$BIN_DIR/xray" "$BIN_DIR/xray.bak"

# 解压
log "解压文件..."
if ! unzip -oq "$ZIP_FILE" -d "$TEMP_DIR"; then
  log "错误: 解压失败"
  [ -f "$BIN_DIR/xray.bak" ] && mv "$BIN_DIR/xray.bak" "$BIN_DIR/xray"
  exit 1
fi

# 安装
mkdir -p "$BIN_DIR"
cp "$TEMP_DIR/xray" "$BIN_DIR/xray"
chmod +x "$BIN_DIR/xray"

# 复制数据文件
[ -f "$TEMP_DIR/geoip.dat" ] && cp "$TEMP_DIR/geoip.dat" "$BIN_DIR/"
[ -f "$TEMP_DIR/geosite.dat" ] && cp "$TEMP_DIR/geosite.dat" "$BIN_DIR/"

# 删除备份
rm -f "$BIN_DIR/xray.bak"

# 验证安装
log "========== 更新成功 =========="
log "下载版本: $VERSION"

if [ -x "$BIN_DIR/xray" ]; then
  # 提取实际安装的版本号 (格式: Xray 25.12.8 ...)
  INSTALLED=$("$BIN_DIR/xray" version 2> /dev/null | head -1 | sed 's/Xray \([0-9.]*\).*/\1/')
  if [ -n "$INSTALLED" ]; then
    log "安装版本: $INSTALLED"
  fi
else
  log "错误: 安装失败"
  exit 1
fi
