#!/usr/bin/env sh
# 文件: .github/scripts/package-module.sh
# 功能: 使用 ZIP 容器与 XZ 9 生成不含管理器的标准模块包
# 用法: sh .github/scripts/package-module.sh <模块目录> <输出目录> <标准包名>
# 依赖: POSIX sh、7z

set -eu

test "$#" -eq 3
module_dir="$(CDPATH= cd -- "$1" && pwd)"
mkdir -p "$2"
output_dir="$(CDPATH= cd -- "$2" && pwd)"
standard="$output_dir/$3"

test -f "$module_dir/module.prop"
# 拒绝更新已有归档，避免上次打包的已删除文件混入新版本。
test ! -e "$standard"

(
  cd "$module_dir"
  7z a -tzip -mm=XZ -mx=9 "$standard" . -x!NetProxy.apk
)

7z t "$standard"
