#!/usr/bin/env sh
# 文件: .github/scripts/build-metadata.sh
# 功能: 从当前提交生成模块与管理器共用的构建元信息
# 用法: 在仓库根目录执行 sh .github/scripts/build-metadata.sh
# 依赖: POSIX sh、git、sed、tr、cut

set -eu

test "$(git rev-parse --is-shallow-repository)" = false
COMMIT_COUNT="$(git rev-list --count HEAD)"
VERSION="$(sed -n 's/^version=//p' src/module/module.prop | tr -d '\r')"
test -n "$VERSION"
SHORT_SHA="$(git rev-parse HEAD | cut -c1-7)"

printf 'version=%s\nmanager_version=%s\nshort_sha=%s\ncommit_count=%s\n' \
  "$VERSION" "${VERSION#v}" "$SHORT_SHA" "$COMMIT_COUNT"
printf 'module_name=NetProxy_%s_%s.zip\n' "$VERSION" "$COMMIT_COUNT"
