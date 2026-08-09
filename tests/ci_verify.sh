#!/usr/bin/env sh
# 文件: tests/ci_verify.sh
# 功能: 构建原生组件并执行 Go、Shell 管理接口契约测试
# 用法: sh tests/ci_verify.sh
# 依赖: Go、POSIX sh、file、grep、sed

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BUILD_DIR="${NETPROXY_CI_BUILD_DIR:-$ROOT/.tmp/ci}"
NATIVE_DIR="$ROOT/src/native/netproxy"

mkdir -p "$BUILD_DIR"

#######################################
# 构建主机测试程序与 Android arm64 产物
#######################################
build_binaries() {
  printf '%s\n' '开始构建 netproxy-native 与 netproxyctl'
  (
    cd "$NATIVE_DIR"
    go test ./...
    go vet ./...
    CGO_ENABLED=0 go build -trimpath -buildvcs=false \
      -o "$BUILD_DIR/netproxy-native" ./cmd/netproxy-native
    CGO_ENABLED=0 go build -trimpath -buildvcs=false \
      -o "$BUILD_DIR/netproxyctl" ./cmd/netproxyctl
    CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build \
      -trimpath -buildvcs=false -o "$BUILD_DIR/netproxy-native-android" \
      ./cmd/netproxy-native
    CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build \
      -trimpath -buildvcs=false -o "$BUILD_DIR/netproxyctl-android" \
      ./cmd/netproxyctl
  )
}

#######################################
# 执行 Shell 契约测试
#######################################
run_shell_contracts() {
  printf '%s\n' '开始执行 Shell 契约测试'
  sh "$ROOT/tests/catalog_subscription_test.sh" "$BUILD_DIR/netproxy-native"
  sh "$ROOT/tests/config_utils_test.sh" "$BUILD_DIR/netproxy-native"
  sh "$ROOT/tests/runtime_catalog_test.sh" "$BUILD_DIR/netproxy-native"
  sh "$ROOT/tests/service_state_test.sh" "$BUILD_DIR/netproxy-native"
  sh "$ROOT/tests/module_scripts_test.sh"
  sh "$ROOT/tests/netproxyctl_contract_test.sh" \
    "$BUILD_DIR/netproxyctl" "$BUILD_DIR/netproxy-native"
}

build_binaries
run_shell_contracts
printf '%s\n' 'Go 与 Shell 契约测试全部通过'
