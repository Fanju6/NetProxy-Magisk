#!/usr/bin/env sh
# 文件: tests/module_packaging_test.sh
# 功能: 验证两种模块包的内容差异以及构建、发布契约。
# 用法: sh tests/module_packaging_test.sh
# 依赖: POSIX sh、7z、grep、cmp、mktemp

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BUILD_ACTION="$ROOT/.github/actions/build-module/action.yml"
RELEASE_WORKFLOW="$ROOT/.github/workflows/release.yml"
CI_WORKFLOW="$ROOT/.github/workflows/ci.yml"
VERIFY_SCRIPT="$ROOT/tests/ci_verify.sh"

assert_contains() {
  grep -Fq -- "$2" "$1" || {
    printf '缺少发行契约: %s\n' "$2" >&2
    return 1
  }
}

assert_not_contains() {
  if grep -Eq -- "$2" "$1"; then
    printf '发现已废弃的发行命名: %s\n' "$2" >&2
    return 1
  fi
}

assert_contains "$BUILD_ACTION" 'standard_name=NetProxy_${VERSION}_${COMMIT_COUNT}.zip'
assert_contains "$BUILD_ACTION" 'manager_name=NetProxy_${VERSION}_${COMMIT_COUNT}_with-manager.zip'
assert_contains "$BUILD_ACTION" 'sh .github/scripts/package-module.sh'
assert_contains "$BUILD_ACTION" 'sh tests/ci_verify.sh'
assert_contains "$BUILD_ACTION" 'install -m 0755 "$NETPROXY_CI_BUILD_DIR/netproxyctl-android" src/module/bin/netproxyctl'
assert_contains "$VERIFY_SCRIPT" './cmd/netproxyctl'
assert_contains "$VERIFY_SCRIPT" "-ldflags='-s -w -buildid='"
assert_not_contains "$BUILD_ACTION" 'full_name|lite_name|_lite'
assert_not_contains "$BUILD_ACTION" 'netproxy-native|cmd/netproxy-native'
[ ! -e "$ROOT/src/module/bin/netproxy-native" ] || {
  printf '%s\n' '模块目录仍包含已删除的 netproxy-native' >&2
  exit 1
}

assert_contains "$RELEASE_WORKFLOW" 'STANDARD_NAME: ${{ steps.pack.outputs.standard_name }}'
assert_contains "$RELEASE_WORKFLOW" '${{ steps.pack.outputs.manager_name }}'
assert_contains "$RELEASE_WORKFLOW" 'update.json'
assert_contains "$RELEASE_WORKFLOW" 'extract-release-notes.mjs'
assert_contains "$RELEASE_WORKFLOW" 'body_path: ${{ runner.temp }}/release-notes.md'
assert_contains "$RELEASE_WORKFLOW" '[标准包]'
assert_contains "$RELEASE_WORKFLOW" '[含管理器包]'
assert_not_contains "$RELEASE_WORKFLOW" 'body_path:[[:space:]]*docs/changelog\.md'
assert_not_contains "$RELEASE_WORKFLOW" 'full_name|lite_name|FULL_NAME|LITE_NAME'

assert_contains "$CI_WORKFLOW" '${{ steps.pack.outputs.standard_name }}'
assert_contains "$CI_WORKFLOW" '${{ steps.pack.outputs.manager_name }}'
assert_contains "$CI_WORKFLOW" 'needs: [module, android]'
assert_contains "$CI_WORKFLOW" "needs.module.result == 'success'"
assert_contains "$CI_WORKFLOW" "needs.android.result == 'success' || needs.android.result == 'skipped'"
assert_contains "$CI_WORKFLOW" 'compression-level: 0'
assert_contains "$CI_WORKFLOW" '--target "$GITHUB_SHA"'
assert_not_contains "$CI_WORKFLOW" 'full_name|lite_name'

TEMP="$(mktemp -d)"
trap 'rm -rf "$TEMP"' EXIT HUP INT TERM
mkdir -p "$TEMP/module/config/singbox" "$TEMP/module/runtime" "$TEMP/module/bin"
printf 'id=netproxy\nversion=test\n' > "$TEMP/module/module.prop"
printf 'binary fixture\n' > "$TEMP/module/bin/netproxyctl"
printf '{}\n' > "$TEMP/module/config/singbox/config.json"
printf 'manager fixture\n' > "$TEMP/module/NetProxy.apk"
: > "$TEMP/module/runtime/.gitkeep"

sh "$ROOT/.github/scripts/package-module.sh" "$TEMP/module" "$TEMP/output" standard.zip manager.zip > "$TEMP/package.log"
for name in standard manager; do
  7z l -slt "$TEMP/output/$name.zip" | tr '\\' '/' > "$TEMP/$name.list"
  assert_contains "$TEMP/$name.list" 'Path = module.prop'
  assert_contains "$TEMP/$name.list" 'Path = runtime/.gitkeep'
  for file in module.prop bin/netproxyctl config/singbox/config.json; do
    7z x -so "$TEMP/output/$name.zip" "$file" > "$TEMP/extracted"
    cmp "$TEMP/module/$file" "$TEMP/extracted"
  done
done
assert_not_contains "$TEMP/standard.list" '^Path = NetProxy[.]apk$'
assert_contains "$TEMP/manager.list" 'Path = NetProxy.apk'
7z x -so "$TEMP/output/manager.zip" NetProxy.apk > "$TEMP/extracted"
cmp "$TEMP/module/NetProxy.apk" "$TEMP/extracted"
if sh "$ROOT/.github/scripts/package-module.sh" "$TEMP/module" "$TEMP/output" standard.zip manager.zip >/dev/null 2>&1; then
  printf '%s\n' '打包程序不应复用已有输出归档' >&2
  exit 1
fi

printf '%s\n' 'module packaging test passed'
