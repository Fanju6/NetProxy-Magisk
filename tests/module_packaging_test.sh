#!/usr/bin/env sh
# 文件: tests/module_packaging_test.sh
# 功能: 验证两种模块包的内容差异以及构建、发布契约。
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
assert_contains "$ANDROID_ACTION" 'keytool -genkeypair -noprompt'
assert_contains "$ANDROID_ACTION" 'GITHUB_RUN_ATTEMPT'
assert_contains "$ANDROID_ACTION" 'openssl rand -hex 24'
assert_contains "$ANDROID_ACTION" '-storetype PKCS12'
assert_contains "$ANDROID_ACTION" 'versionName='\''${MANAGER_VERSION}-ci.${BUILD_ID}'\'''
assert_contains "$ANDROID_ACTION" '--v4-signing-enabled false'
assert_contains "$ANDROID_ACTION" 'apksigner" verify --verbose'
assert_contains "$ANDROID_ACTION" 'install -m 0644 "$SIGNED_APK" src/module/NetProxy.apk'
assert_contains "$ROOT/src/android/app/build.gradle.kts" 'versionName = if (ciManagerBuild)'
assert_contains "$ROOT/src/android/app/src/main/java/com/fanjv/netproxy/core/app/AppSignature.kt" 'GOOGLE_PLAY_APP_SIGNING_SHA256'
assert_contains "$ROOT/src/android/app/src/main/java/com/fanjv/netproxy/core/app/AppSignature.kt" 'signingCertificateHistory'
assert_contains "$ROOT/.gitignore" 'src/module/NetProxy.apk'
assert_contains "$VERIFY_SCRIPT" './cmd/netproxyctl'
assert_contains "$VERIFY_SCRIPT" "-ldflags='-s -w -buildid='"
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

assert_contains "$RELEASE_WORKFLOW" 'STANDARD_NAME: ${{ needs.build.outputs.standard_name }}'
assert_contains "$RELEASE_WORKFLOW" '${{ env.MANAGER_NAME }}'
assert_contains "$RELEASE_WORKFLOW" 'needs: build'
assert_contains "$RELEASE_WORKFLOW" 'sh .github/scripts/package-manager.sh'
assert_contains "$RELEASE_WORKFLOW" 'update.json'
assert_contains "$RELEASE_WORKFLOW" 'extract-release-notes.mjs'
assert_contains "$RELEASE_WORKFLOW" 'body_path: ${{ runner.temp }}/release-notes.md'
assert_contains "$RELEASE_WORKFLOW" '[标准包]'
assert_contains "$RELEASE_WORKFLOW" '[含管理器包]'
assert_not_contains "$RELEASE_WORKFLOW" 'body_path:[[:space:]]*docs/changelog\.md'
assert_not_contains "$RELEASE_WORKFLOW" 'full_name|lite_name|FULL_NAME|LITE_NAME'

assert_contains "$SHARED_WORKFLOW" '${{ steps.pack.outputs.standard_name }}'
assert_contains "$CI_WORKFLOW" 'needs: build'
assert_contains "$CI_WORKFLOW" "needs.build.result == 'success'"
assert_contains "$CI_WORKFLOW" "verify-android: \${{ needs.changes.outputs.android == 'true' }}"
assert_contains "$CI_WORKFLOW" 'sh .github/scripts/package-manager.sh'
assert_contains "$SHARED_WORKFLOW" 'compression-level: 0'
assert_contains "$SHARED_WORKFLOW" '            src/module/config/singbox'
assert_contains "$CI_WORKFLOW" '--target "$GITHUB_SHA"'
assert_not_contains "$CI_WORKFLOW" 'full_name|lite_name'
for workflow in "$CI_WORKFLOW" "$RELEASE_WORKFLOW"; do
  assert_contains "$workflow" 'uses: ./.github/workflows/build-module.yml'
  assert_contains "$workflow" 'name: module-standard'
  assert_contains "$workflow" 'name: manager-apk'
done
assert_contains "$SHARED_WORKFLOW" "build-manager: 'true'"
assert_contains "$SHARED_WORKFLOW" 'verify: ${{ inputs.verify-android }}'
assert_contains "$SHARED_WORKFLOW" 'overwrite: true'
assert_not_contains "$SHARED_WORKFLOW" '^[[:space:]]+needs:'

TEMP="$(mktemp -d)"
trap 'rm -rf "$TEMP"' EXIT HUP INT TERM
(
  cd "$ROOT"
  sh "$METADATA_SCRIPT"
) > "$TEMP/metadata"
version="$(sed -n 's/^version=//p' "$ROOT/src/module/module.prop" | tr -d '\r')"
commit_count="$(git -C "$ROOT" rev-list --count HEAD)"
short_sha="$(git -C "$ROOT" rev-parse HEAD | cut -c1-7)"
assert_contains "$TEMP/metadata" "standard_name=NetProxy_${version}_${commit_count}.zip"
assert_contains "$TEMP/metadata" "manager_name=NetProxy_${version}_${commit_count}_with-manager.zip"
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

sh "$ROOT/.github/scripts/package-module.sh" "$TEMP/module" "$TEMP/output" standard.zip > "$TEMP/package.log"
if sh "$ROOT/.github/scripts/package-manager.sh" "$TEMP/output/standard.zip" "$TEMP/module/NetProxy.apk" "$TEMP/output/missing-apk.zip" >/dev/null 2>&1; then
  printf '%s\n' '缺少 APK 时不应生成含管理器包' >&2
  exit 1
fi
[ ! -e "$TEMP/output/missing-apk.zip" ]
cp "$TEMP/module/bin/netproxyctl" "$TEMP/module/NetProxy.apk"
sh "$ROOT/.github/scripts/package-manager.sh" "$TEMP/output/standard.zip" "$TEMP/module/NetProxy.apk" "$TEMP/output/manager.zip" >> "$TEMP/package.log"
for name in standard manager; do
  7z l -slt "$TEMP/output/$name.zip" | tr '\\' '/' > "$TEMP/$name.list"
  assert_contains "$TEMP/$name.list" 'Path = module.prop'
  assert_contains "$TEMP/$name.list" 'Path = runtime/.gitkeep'
  7z l -slt "$TEMP/output/$name.zip" bin/netproxyctl > "$TEMP/core.list"
  grep -Eiq '^Method = xz$' "$TEMP/core.list" || {
    printf '%s\n' '模块核心未使用 XZ 压缩' >&2
    exit 1
  }
  grep -E '^(Packed Size|CRC|Method) = ' "$TEMP/core.list" > "$TEMP/$name.core"
  for file in module.prop bin/netproxyctl config/singbox/config.json; do
    7z x -so "$TEMP/output/$name.zip" "$file" > "$TEMP/extracted"
    cmp "$TEMP/module/$file" "$TEMP/extracted"
  done
done
cmp "$TEMP/standard.core" "$TEMP/manager.core"
assert_not_contains "$TEMP/standard.list" '^Path = NetProxy[.]apk$'
assert_contains "$TEMP/manager.list" 'Path = NetProxy.apk'
7z l -slt "$TEMP/output/manager.zip" NetProxy.apk > "$TEMP/apk.list"
assert_contains "$TEMP/apk.list" 'Method = Store'
7z x -so "$TEMP/output/manager.zip" NetProxy.apk > "$TEMP/extracted"
cmp "$TEMP/module/NetProxy.apk" "$TEMP/extracted"
sh "$ROOT/.github/scripts/package-module.sh" "$TEMP/module" "$TEMP/output" with-apk.zip >> "$TEMP/package.log"
7z l -slt "$TEMP/output/with-apk.zip" > "$TEMP/with-apk.list"
assert_not_contains "$TEMP/with-apk.list" '^Path = NetProxy[.]apk$'
if sh "$ROOT/.github/scripts/package-module.sh" "$TEMP/module" "$TEMP/output" standard.zip >/dev/null 2>&1; then
  printf '%s\n' '打包程序不应复用已有输出归档' >&2
  exit 1
fi
if sh "$ROOT/.github/scripts/package-manager.sh" "$TEMP/output/standard.zip" "$TEMP/module/NetProxy.apk" "$TEMP/output/manager.zip" >/dev/null 2>&1; then
  printf '%s\n' '含管理器包不应覆盖已有输出归档' >&2
  exit 1
fi

printf '%s\n' 'module packaging test passed'
