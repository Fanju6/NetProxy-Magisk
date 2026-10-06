#!/usr/bin/env sh
# 文件: .github/scripts/complete-module.sh
# 功能: 向已压缩模块内容追加管理器 APK，完成唯一发行包
# 用法: sh .github/scripts/complete-module.sh <模块包> <NetProxy.apk>
# 依赖: POSIX sh、7z、dirname、basename

set -eu

test "$#" -eq 2
module="$(CDPATH= cd -- "$(dirname -- "$1")" && pwd)/$(basename -- "$1")"
apk_dir="$(CDPATH= cd -- "$(dirname -- "$2")" && pwd)"
test "$(basename -- "$2")" = NetProxy.apk
test -s "$module"
test -s "$apk_dir/NetProxy.apk"
(
  cd "$apk_dir"
  7z a -tzip -mx=0 "$module" NetProxy.apk
)
7z t "$module"
