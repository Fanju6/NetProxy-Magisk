#!/usr/bin/env sh
# 文件: tests/module_packaging_test.sh
# 功能: 验证唯一模块包的 APK、压缩方法以及构建、发布契约。
# 用法: sh tests/module_packaging_test.sh
# 依赖: POSIX sh、7z、git、grep、sed、tr、cut、cmp、cp、mktemp

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BUILD_ACTION="$ROOT/.github/actions/build-module/action.yml"
ANDROID_ACTION="$ROOT/.github/actions/verify-android/action.yml"
METADATA_SCRIPT="$ROOT/.github/scripts/build-metadata.sh"
RELEASE_WORKFLOW="$ROOT/.github/workflows/release.yml"
CI_WORKFLOW="$ROOT/.github/workflows/ci.yml"
SHARED_WORKFLOW="$ROOT/.github/workflows/build-module.yml"
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

assert_contains "$BUILD_ACTION" 'sh .github/scripts/build-metadata.sh'
assert_contains "$ANDROID_ACTION" 'sh .github/scripts/build-metadata.sh'
assert_contains "$BUILD_ACTION" 'sh .github/scripts/package-module.sh'
assert_contains "$ROOT/.github/scripts/package-module.sh" '7z a -tzip -mm=XZ -mx=9'
assert_contains "$BUILD_ACTION" 'sh tests/ci_verify.sh'
assert_contains "$BUILD_ACTION" 'install -m 0755 "$NETPROXY_CI_BUILD_DIR/netproxyctl-android" src/module/bin/netproxyctl'
assert_not_contains "$BUILD_ACTION" 'gradlew|setup-java|setup-gradle|keytool'
assert_contains "$ANDROID_ACTION" 'set -- :app:assembleRelease'
assert_contains "$ANDROID_ACTION" 'set -- "$@" testDebugUnitTest lintDebug'
assert_contains "$ANDROID_ACTION" './gradlew "$@" --build-cache --parallel --no-daemon'
assert_contains "$ANDROID_ACTION" '-PnetproxyManagerCi=true'
assert_contains "$ANDROID_ACTION" 'printf '\''%s'\'' "$SIGNING_KEYSTORE_BASE64" | base64 --decode > "$KEYSTORE"'
assert_contains "$ANDROID_ACTION" '--ks-key-alias "$SIGNING_KEY_ALIAS"'
assert_contains "$ANDROID_ACTION" '--ks-pass env:SIGNING_STORE_PASSWORD'
assert_contains "$ANDROID_ACTION" '--key-pass env:SIGNING_KEY_PASSWORD'
assert_contains "$ANDROID_ACTION" 'versionName='\''${MANAGER_VERSION}-ci.${BUILD_ID}'\'''
assert_contains "$ANDROID_ACTION" '--v4-signing-enabled false'
assert_contains "$ANDROID_ACTION" 'apksigner" verify --verbose'
assert_contains "$ANDROID_ACTION" 'install -m 0644 "$SIGNED_APK" src/module/NetProxy.apk'
assert_contains "$ROOT/src/android/app/build.gradle.kts" 'versionName = if (ciManagerBuild)'
assert_contains "$ROOT/.gitignore" 'src/module/NetProxy.apk'
assert_contains "$VERIFY_SCRIPT" './cmd/netproxyctl'
assert_contains "$VERIFY_SCRIPT" '-ldflags="-s -w -buildid='
assert_contains "$VERIFY_SCRIPT" 'internal/telemetry.ProjectToken=${POSTHOG_PROJECT_TOKEN:-}'
assert_contains "$VERIFY_SCRIPT" 'internal/telemetry.IngestionHost=${POSTHOG_HOST:-}'
assert_contains "$BUILD_ACTION" "NETPROXY_TELEMETRY_REQUIRED: '1'"
assert_contains "$BUILD_ACTION" 'POSTHOG_PROJECT_TOKEN: ${{ inputs.posthog-project-token }}'
assert_contains "$BUILD_ACTION" 'POSTHOG_HOST: ${{ inputs.posthog-host }}'
assert_contains "$SHARED_WORKFLOW" 'posthog-project-token: ${{ vars.POSTHOG_PROJECT_TOKEN }}'
assert_contains "$SHARED_WORKFLOW" 'posthog-host: ${{ vars.POSTHOG_HOST }}'
assert_not_contains "$BUILD_ACTION" '\$\{\{[[:space:]]*vars\.'
assert_not_contains "$BUILD_ACTION" 'full_name|lite_name|_lite'
assert_not_contains "$BUILD_ACTION" 'netproxy-native|cmd/netproxy-native'
[ ! -e "$ROOT/src/module/bin/netproxy-native" ] || {
  printf '%s\n' '模块目录仍包含已删除的 netproxy-native' >&2
  exit 1
}
find "$ROOT/src/module/webroot" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' |
  while IFS= read -r directory; do
    case "$directory" in
      netproxy|sing-box-dashboard) ;;
      *)
        printf '模块 Web 根目录包含未声明的面板资源: %s\n' "$directory" >&2
        exit 1
        ;;
    esac
  done
[ -d "$ROOT/src/module/webroot/sing-box-dashboard" ] || {
  printf '%s\n' '模块 Web 根目录缺少 sing-box Dashboard' >&2
  exit 1
}
! grep -q '"external_ui"' "$ROOT/src/module/config/singbox/config.json" || {
  printf '%s\n' '默认配置仍声明已删除的外部 UI' >&2
  exit 1
}
! grep -q '"githubRelease"' "$ROOT/.github/resources.json" || {
  printf '%s\n' '资源清单仍包含已删除的 GitHub Release 面板来源' >&2
  exit 1
}

assert_contains "$RELEASE_WORKFLOW" 'MODULE_NAME: ${{ needs.build.outputs.module_name }}'
assert_contains "$RELEASE_WORKFLOW" '${{ env.MODULE_NAME }}'
assert_contains "$RELEASE_WORKFLOW" 'needs: build'
assert_contains "$RELEASE_WORKFLOW" 'update.json'
assert_contains "$RELEASE_WORKFLOW" 'releases/download/${TAG}/${MODULE_NAME}'
assert_contains "$RELEASE_WORKFLOW" 'extract-release-notes.mjs'
assert_contains "$RELEASE_WORKFLOW" 'body_path: ${{ runner.temp }}/release-notes.md'
assert_contains "$RELEASE_WORKFLOW" '[模块包]'
assert_not_contains "$RELEASE_WORKFLOW" 'body_path:[[:space:]]*docs/changelog\.md'
assert_not_contains "$RELEASE_WORKFLOW" 'full_name|lite_name|FULL_NAME|LITE_NAME'

assert_contains "$SHARED_WORKFLOW" '${{ steps.pack.outputs.module_name }}'
assert_contains "$SHARED_WORKFLOW" 'needs: [module, android]'
assert_contains "$SHARED_WORKFLOW" 'sh .github/scripts/complete-module.sh "$MODULE_NAME" manager/NetProxy.apk'
assert_contains "$SHARED_WORKFLOW" 'name: module-content'
assert_contains "$SHARED_WORKFLOW" 'name: manager-apk'
assert_contains "$SHARED_WORKFLOW" 'name: module-package'
assert_contains "$CI_WORKFLOW" 'needs: build'
assert_contains "$CI_WORKFLOW" "needs.build.result == 'success'"
assert_contains "$CI_WORKFLOW" "verify-android: \${{ needs.changes.outputs.android == 'true' }}"
assert_contains "$CI_WORKFLOW" 'MODULE_NAME: ${{ needs.build.outputs.module_name }}'
assert_contains "$SHARED_WORKFLOW" 'compression-level: 0'
assert_contains "$SHARED_WORKFLOW" '            src/module/config/singbox'
assert_contains "$CI_WORKFLOW" '--target "$GITHUB_SHA"'
assert_not_contains "$CI_WORKFLOW" 'full_name|lite_name'
for workflow in "$CI_WORKFLOW" "$RELEASE_WORKFLOW"; do
  assert_contains "$workflow" 'uses: ./.github/workflows/build-module.yml'
  assert_contains "$workflow" 'name: module-package'
  for secret in ANDROID_KEYSTORE_BASE64 ANDROID_KEYSTORE_PASSWORD ANDROID_KEY_ALIAS ANDROID_KEY_PASSWORD; do
    assert_contains "$workflow" "$secret: \${{ secrets.$secret }}"
    assert_contains "$SHARED_WORKFLOW" "\${{ secrets.$secret }}"
  done
done
assert_contains "$SHARED_WORKFLOW" "build-manager: 'true'"
assert_contains "$SHARED_WORKFLOW" 'verify: ${{ inputs.verify-android }}'
assert_contains "$SHARED_WORKFLOW" 'overwrite: true'

TEMP="$(mktemp -d)"
trap 'rm -rf "$TEMP"' EXIT HUP INT TERM
(
  cd "$ROOT"
  sh "$METADATA_SCRIPT"
) > "$TEMP/metadata"
version="$(sed -n 's/^version=//p' "$ROOT/src/module/module.prop" | tr -d '\r')"
commit_count="$(git -C "$ROOT" rev-list --count HEAD)"
short_sha="$(git -C "$ROOT" rev-parse HEAD | cut -c1-7)"
assert_contains "$TEMP/metadata" "module_name=NetProxy_${version}_${commit_count}.zip"
assert_contains "$TEMP/metadata" "manager_version=${version#v}"
assert_contains "$TEMP/metadata" "short_sha=$short_sha"
assert_contains "$TEMP/metadata" "commit_count=$commit_count"
mkdir -p "$TEMP/module/config/singbox" "$TEMP/module/runtime" "$TEMP/module/bin"
printf 'id=netproxy\nversion=test\n' > "$TEMP/module/module.prop"
# 小文件可能自动使用 Store；可压缩内容才能验证实际压缩方法。
{
  count=0
  while [ "$count" -lt 128 ]; do
    printf '%s\n' 'anonymous module packaging fixture: core and manager compression verification'
    count=$((count + 1))
  done
} > "$TEMP/module/bin/netproxyctl"
printf '{}\n' > "$TEMP/module/config/singbox/config.json"
: > "$TEMP/module/runtime/.gitkeep"

sh "$ROOT/.github/scripts/package-module.sh" "$TEMP/module" "$TEMP/output" module.zip > "$TEMP/package.log"
cp "$TEMP/output/module.zip" "$TEMP/before.zip"
if sh "$ROOT/.github/scripts/complete-module.sh" "$TEMP/output/module.zip" "$TEMP/module/NetProxy.apk" >/dev/null 2>&1; then
  printf '%s\n' '缺少 APK 时不能完成模块包' >&2
  exit 1
fi
cmp "$TEMP/before.zip" "$TEMP/output/module.zip"
: > "$TEMP/module/NetProxy.apk"
if sh "$ROOT/.github/scripts/complete-module.sh" "$TEMP/output/module.zip" "$TEMP/module/NetProxy.apk" >/dev/null 2>&1; then
  printf '%s\n' '空 APK 时不能完成模块包' >&2
  exit 1
fi
cmp "$TEMP/before.zip" "$TEMP/output/module.zip"
cp "$TEMP/module/bin/netproxyctl" "$TEMP/module/NetProxy.apk"
7z l -slt "$TEMP/before.zip" bin/netproxyctl > "$TEMP/before.core"
sh "$ROOT/.github/scripts/complete-module.sh" "$TEMP/output/module.zip" "$TEMP/module/NetProxy.apk" >> "$TEMP/package.log"
7z l -slt "$TEMP/output/module.zip" | tr '\\' '/' > "$TEMP/module.list"
assert_contains "$TEMP/module.list" 'Path = module.prop'
assert_contains "$TEMP/module.list" 'Path = runtime/.gitkeep'
assert_contains "$TEMP/module.list" 'Path = NetProxy.apk'
[ "$(find "$TEMP/output" -type f | wc -l)" -eq 1 ]
7z l -slt "$TEMP/output/module.zip" bin/netproxyctl > "$TEMP/core.list"
grep -Eiq '^Method = xz$' "$TEMP/core.list" || {
  printf '%s\n' '模块核心未使用 XZ 压缩' >&2
  exit 1
}
grep -E '^(Packed Size|CRC|Method) = ' "$TEMP/before.core" > "$TEMP/before.compression"
grep -E '^(Packed Size|CRC|Method) = ' "$TEMP/core.list" > "$TEMP/after.compression"
cmp "$TEMP/before.compression" "$TEMP/after.compression"
for file in module.prop bin/netproxyctl config/singbox/config.json NetProxy.apk; do
  7z x -so "$TEMP/output/module.zip" "$file" > "$TEMP/extracted"
  cmp "$TEMP/module/$file" "$TEMP/extracted"
done
7z l -slt "$TEMP/output/module.zip" NetProxy.apk > "$TEMP/apk.list"
assert_contains "$TEMP/apk.list" 'Method = Store'
if sh "$ROOT/.github/scripts/package-module.sh" "$TEMP/module" "$TEMP/output" module.zip >/dev/null 2>&1; then
  printf '%s\n' '打包程序不应复用已有输出归档' >&2
  exit 1
fi
if sh "$ROOT/.github/scripts/complete-module.sh" "$TEMP/output/missing.zip" "$TEMP/module/NetProxy.apk" >/dev/null 2>&1; then
  printf '%s\n' '不能从缺失的模块内容生成发行包' >&2
  exit 1
fi
[ ! -e "$TEMP/output/missing.zip" ]

printf '%s\n' 'module packaging test passed'
