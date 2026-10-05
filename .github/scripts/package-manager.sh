#!/usr/bin/env sh
# 文件: .github/scripts/package-manager.sh
# 功能: 复用已验证标准包的压缩数据，以 Store 追加管理器 APK
# 用法: sh .github/scripts/package-manager.sh <标准包> <NetProxy.apk> <含管理器包>
# 依赖: POSIX sh、7z、cp、dirname、basename

set -eu

test "$#" -eq 3
standard="$(CDPATH= cd -- "$(dirname -- "$1")" && pwd)/$(basename -- "$1")"
apk_dir="$(CDPATH= cd -- "$(dirname -- "$2")" && pwd)"
test "$(basename -- "$2")" = NetProxy.apk
test -s "$standard"
test -s "$apk_dir/NetProxy.apk"
mkdir -p "$(dirname -- "$3")"
manager="$(CDPATH= cd -- "$(dirname -- "$3")" && pwd)/$(basename -- "$3")"
test ! -e "$manager"

cp "$standard" "$manager"
(
  cd "$apk_dir"
  7z a -tzip -mx=0 "$manager" NetProxy.apk
)
7z t "$manager"
