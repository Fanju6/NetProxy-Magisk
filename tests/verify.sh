#!/usr/bin/env sh
# 文件: tests/verify.sh
# 功能: 按范围编排 NetProxy 本地开发与发布前验证。
# 用法: sh tests/verify.sh <quick|webui|android|docs|full> [--check-generated]
# 依赖: 对应范围的 Go、Node.js、npm、Gradle、POSIX sh、7z、file、grep、sed

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
RUN_QUICK=0
RUN_WEBUI=0
RUN_ANDROID=0
RUN_DOCS=0
CHECK_GENERATED="${NETPROXY_VERIFY_GENERATED:-0}"

usage() {
  cat <<'EOF'
用法: sh tests/verify.sh <范围>... [--check-generated]

范围:
  quick    Go、Shell 契约与工作流脚本检查
  webui    WebUI 类型检查、单测与构建
  android  Android 单测、Lint 与 Debug 构建
  docs     文档内容检查、单测与构建
  full     执行全部范围

选项:
  --check-generated  构建 WebUI 后确认模块内生成产物已同步到 Git 索引。
EOF
}

require_boolean() {
  case "$1" in
    0|1) ;;
    *) printf '%s\n' "无效布尔值: $1" >&2; exit 2 ;;
  esac
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

  if [ "$CHECK_GENERATED" = 1 ] && ! git -C "$ROOT" diff --quiet -- src/module/webroot/netproxy; then
    printf '%s\n' 'WebUI 生成产物尚未同步到 Git 索引，请检查并暂存 src/module/webroot/netproxy 后重试。' >&2
    return 1
  fi
}

run_android() {
  printf '%s\n' '==> 验证 Android 管理器'
  (
    cd "$ROOT/src/android"
    ./gradlew testDebugUnitTest lintDebug :app:assembleDebug --build-cache --parallel --no-daemon
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
    --check-generated) CHECK_GENERATED=1 ;;
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

require_boolean "$CHECK_GENERATED"

[ "$RUN_QUICK" = 0 ] || run_quick
[ "$RUN_WEBUI" = 0 ] || run_webui
[ "$RUN_ANDROID" = 0 ] || run_android
[ "$RUN_DOCS" = 0 ] || run_docs

git -C "$ROOT" diff --check
printf '%s\n' '验证通过'
