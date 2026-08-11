#!/usr/bin/env sh
# 文件: tests/module_packaging_test.sh
# 功能: 检查标准包与含管理器包的构建、发布和自更新命名契约。
# 用法: sh tests/module_packaging_test.sh
# 依赖: POSIX sh、grep

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BUILD_ACTION="$ROOT/.github/actions/build-module/action.yml"
RELEASE_WORKFLOW="$ROOT/.github/workflows/release.yml"
CI_WORKFLOW="$ROOT/.github/workflows/ci.yml"

assert_contains() {
  grep -Fq "$2" "$1" || {
    printf '缺少发行契约: %s\n' "$2" >&2
    return 1
  }
}

assert_not_contains() {
  if grep -Eq "$2" "$1"; then
    printf '发现已废弃的发行命名: %s\n' "$2" >&2
    return 1
  fi
}

assert_contains "$BUILD_ACTION" 'standard_name=NetProxy_${VERSION}_${COMMIT_COUNT}.zip'
assert_contains "$BUILD_ACTION" 'manager_name=NetProxy_${VERSION}_${COMMIT_COUNT}_with-manager.zip'
assert_contains "$BUILD_ACTION" '7z a -tzip -mx=9 "../../$STANDARD_NAME" . -x!"NetProxy.apk"'
assert_contains "$BUILD_ACTION" '7z a -tzip -mx=9 "../../$MANAGER_NAME" .'
assert_not_contains "$BUILD_ACTION" 'full_name|lite_name|_lite'

assert_contains "$RELEASE_WORKFLOW" 'STANDARD_NAME: ${{ steps.pack.outputs.standard_name }}'
assert_contains "$RELEASE_WORKFLOW" '${{ steps.pack.outputs.manager_name }}'
assert_contains "$RELEASE_WORKFLOW" '[标准包]'
assert_contains "$RELEASE_WORKFLOW" '[含管理器包]'
assert_not_contains "$RELEASE_WORKFLOW" 'full_name|lite_name|FULL_NAME|LITE_NAME'

assert_contains "$CI_WORKFLOW" '${{ steps.pack.outputs.standard_name }}'
assert_contains "$CI_WORKFLOW" '${{ steps.pack.outputs.manager_name }}'
assert_not_contains "$CI_WORKFLOW" 'full_name|lite_name'

printf '%s\n' 'module packaging test passed'
