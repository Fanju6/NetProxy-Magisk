#!/usr/bin/env sh
# 文件: .github/scripts/package-module.sh
# 功能: 生成标准模块包，并复用压缩数据生成含管理器包
# 用法: sh .github/scripts/package-module.sh <模块目录> <输出目录> <标准包名> <含管理器包名>
# 依赖: POSIX sh、7z、cp

set -eu

module_dir="$(CDPATH= cd -- "$1" && pwd)"
mkdir -p "$2"
output_dir="$(CDPATH= cd -- "$2" && pwd)"
standard="$output_dir/$3"
manager="$output_dir/$4"

test -f "$module_dir/module.prop"
test -s "$module_dir/NetProxy.apk"
# 拒绝更新已有归档，避免上次打包的已删除文件混入新版本。
test ! -e "$standard"
test ! -e "$manager"

(
  cd "$module_dir"
  7z a -tzip -mx=5 "$standard" . -x!NetProxy.apk
  cp "$standard" "$manager"
  # APK 自身已压缩；追加时不重新压缩标准包内的核心和 Web 资源。
  7z a -tzip -mx=0 "$manager" NetProxy.apk
)

7z t "$standard"
7z t "$manager"
