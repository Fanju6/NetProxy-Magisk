#!/usr/bin/env sh
# 文件: tests/ci_verify.sh
# 功能: 构建原生组件并执行 Go、Shell 管理接口契约测试
# 用法: sh tests/ci_verify.sh
# 依赖: Go、Node.js、7z、POSIX sh、file、grep、sed

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BUILD_DIR="${NETPROXY_CI_BUILD_DIR:-$ROOT/.tmp/ci}"
NATIVE_DIR="$ROOT/src/native/netproxy"

mkdir -p "$BUILD_DIR"

#######################################
# 构建主机测试程序与 Android arm64 产物
#######################################
build_binaries() {
  printf '%s\n' '开始构建 netproxyctl'
  if [ "${NETPROXY_TELEMETRY_REQUIRED:-0}" = 1 ]; then
    case "${POSTHOG_PROJECT_TOKEN:-}" in phc_*) ;; *) printf '%s\n' '缺少 PostHog Project Token' >&2; return 1 ;; esac
    case "$POSTHOG_PROJECT_TOKEN" in *[!A-Za-z0-9_]*) printf '%s\n' '无效的 PostHog Project Token' >&2; return 1 ;; esac
    [ "${#POSTHOG_PROJECT_TOKEN}" -gt 4 ] && [ "${#POSTHOG_PROJECT_TOKEN}" -le 256 ] || return 1
    case "${POSTHOG_HOST:-}" in https://us.i.posthog.com|https://eu.i.posthog.com) ;; *) printf '%s\n' '无效的 PostHog ingestion host' >&2; return 1 ;; esac
  fi
  (
    cd "$NATIVE_DIR"
    go test ./...
    go vet ./...
    CGO_ENABLED=0 go build -trimpath -buildvcs=false -pgo=auto \
      -o "$BUILD_DIR/netproxyctl" ./cmd/netproxyctl
    CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build \
      -trimpath -buildvcs=false -pgo=auto \
      -ldflags="-s -w -buildid= -X github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/telemetry.ProjectToken=${POSTHOG_PROJECT_TOKEN:-} -X github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/telemetry.IngestionHost=${POSTHOG_HOST:-}" \
      -o "$BUILD_DIR/netproxyctl-android" \
      ./cmd/netproxyctl
    go version -m "$BUILD_DIR/netproxyctl-android" | grep -q -- '-pgo=default.pgo'
    go version -m "$BUILD_DIR/netproxyctl-android" | grep -q 'github.com/reF1nd/sing-box'
    file "$BUILD_DIR/netproxyctl-android" | grep -q 'ARM aarch64'
  )
}

#######################################
# 执行 Shell 契约测试
#######################################
run_shell_contracts() {
  printf '%s\n' '开始执行 Shell 契约测试'
  sh "$ROOT/tests/runtime_catalog_test.sh" "$BUILD_DIR/netproxyctl"
  sh "$ROOT/tests/module_scripts_test.sh"
  sh "$ROOT/tests/customize_hot_update_test.sh" "$BUILD_DIR/netproxyctl"
  sh "$ROOT/tests/module_packaging_test.sh"
  sh "$ROOT/tests/release_notes_test.sh"
}

build_binaries
run_shell_contracts
node --test "$ROOT"/.github/scripts/*.test.mjs
sh -n "$ROOT/tests/verify.sh"
sh "$ROOT/tests/verify.sh" --help >/dev/null
if sh "$ROOT/tests/verify.sh" invalid >/dev/null 2>&1; then
  printf '%s\n' '开发验证脚本不应接受未知范围' >&2
  exit 1
fi
printf '%s\n' 'Go 与 Shell 契约测试全部通过'
