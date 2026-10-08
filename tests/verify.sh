#!/usr/bin/env sh
# 文件: tests/verify.sh
# 功能: 按范围编排 NetProxy 本地开发与发布前验证。
# 用法: sh tests/verify.sh <quick|webui|android|docs|full>
# 依赖: 对应范围的 Go、Node.js、npm、Gradle、POSIX sh、7z、file、grep、sed

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
RUN_QUICK=0
RUN_WEBUI=0
RUN_ANDROID=0
RUN_DOCS=0

usage() {
  cat <<'EOF'
用法: sh tests/verify.sh <范围>...

范围:
  quick    Go、Shell 契约与工作流脚本检查
  webui    WebUI 类型检查、单测与构建
  android  Android 单测、编辑器 Host/Desktop 测试、Lint 与 Debug 构建
  docs     文档内容检查、单测与构建
  full     执行全部范围

EOF
}

run_quick() {
  printf '%s\n' '==> 执行核心验证'
  sh "$ROOT/tests/ci_verify.sh"
}

run_webui() {
  printf '%s\n' '==> 验证 WebUI'
  (
    cd "$ROOT/src/webui"
    npm ci --no-audit --no-fund
    npm run build
  )
}

run_android() {
  printf '%s\n' '==> 验证 Android 管理器'
  (
    cd "$ROOT/src/android"
    ./gradlew testDebugUnitTest :scripta:editor:testAndroidHostTest :scripta:editor:desktopTest lintDebug :app:assembleDebug --build-cache --parallel --no-daemon
  )
}

run_docs() {
  printf '%s\n' '==> 验证文档站'
  (
    cd "$ROOT/docs"
    npm ci --no-audit --no-fund
    npm run build
  )
}

[ "$#" -gt 0 ] || {
  usage >&2
  exit 2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    quick) RUN_QUICK=1 ;;
    webui) RUN_WEBUI=1 ;;
    android) RUN_ANDROID=1 ;;
    docs) RUN_DOCS=1 ;;
    full|all)
      RUN_QUICK=1
      RUN_WEBUI=1
      RUN_ANDROID=1
      RUN_DOCS=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf '%s\n' "未知验证范围: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

[ "$RUN_QUICK" = 0 ] || run_quick
[ "$RUN_WEBUI" = 0 ] || run_webui
[ "$RUN_ANDROID" = 0 ] || run_android
[ "$RUN_DOCS" = 0 ] || run_docs

git -C "$ROOT" diff --check
printf '%s\n' '验证通过'
