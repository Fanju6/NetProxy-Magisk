#!/usr/bin/env sh
# 文件: .github/scripts/package-module.sh
# 功能: 使用 ZIP 容器与 Deflate 9 压缩模块内容，供汇合任务追加管理器
# 用法: sh .github/scripts/package-module.sh <模块目录> <输出目录> <模块包名>
# 依赖: POSIX sh、7z

set -eu

test "$#" -eq 3
module_dir="$(CDPATH= cd -- "$1" && pwd)"
mkdir -p "$2"
output_dir="$(CDPATH= cd -- "$2" && pwd)"
module="$output_dir/$3"

test -f "$module_dir/module.prop"
# Windows 检出的 CRLF 会让 customize.sh 的严格匹配失败，进而中断安装。
# grep 在 MSYS 环境会剥离 CR，必须用 od 做字节级检测。
if od -An -c "$module_dir/module.prop" | grep -q '\\r'; then
  printf '%s\n' 'module.prop 包含 CRLF 行尾，请先转换为 LF' >&2
  exit 1
fi
# 拒绝更新已有归档，避免上次打包的已删除文件混入新版本。
test ! -e "$module"

(
  cd "$module_dir"
  # KernelSU/SukiSU 环境常用 toybox unzip，不支持 XZ 压缩的 ZIP 条目。
  7z a -tzip -mm=Deflate -mx=9 "$module" . -x!NetProxy.apk -x!module.prop
  7z a -tzip -mx=0 "$module" module.prop
)

7z t "$module"
