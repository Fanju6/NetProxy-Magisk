#!/system/bin/sh
#######################################
# 文件: runtime.sh
# 功能: 为 service.sh 提供运行时文件路径和 Go 运行时准备适配
# 用法: 由 service.sh 通过 . "$MODDIR/scripts/core/runtime.sh" 引入
# 依赖: common.sh、netproxy-native
#######################################

RUNTIME_PROVIDERS_FILE=""
RUNTIME_OUTBOUNDS_FILE=""
RUNTIME_EBPF_FILE=""
RUNTIME_CATALOG_STATE_FILE=""
RUNTIME_BUILD_ERROR=""

#######################################
# 初始化运行时文件路径
#######################################
initialize_runtime_context() {
  RUNTIME_PROVIDERS_FILE="$RUNTIME_DIR/providers.json"
  RUNTIME_OUTBOUNDS_FILE="$RUNTIME_DIR/outbounds.json"
  RUNTIME_EBPF_FILE="$RUNTIME_DIR/ebpf.json"
  RUNTIME_CATALOG_STATE_FILE="$RUNTIME_DIR/catalog.state"
}

#######################################
# 调用 Go 生成 Catalog、Provider、选择器和 eBPF 入站
# 参数: strict 或 allow-empty
#######################################
build_runtime_catalog() {
  local mode="${1:-strict}"
  local runtime_output_dir="${RUNTIME_PROVIDERS_FILE%/*}"
  RUNTIME_BUILD_ERROR=""
  if [ "$mode" = "allow-empty" ]; then
    if ! "$NETPROXY_NATIVE_BIN" module prepare \
      --module-dir "$MODDIR" \
      --catalog-root "$CATALOG_DIR" \
      --module-config "$MODULE_CONF" \
      --ebpf-config "$EBPF_CONF" \
      --singbox-dir "$SINGBOX_DIR" \
      --runtime-dir "$runtime_output_dir" \
      --allow-empty > /dev/null; then
      RUNTIME_BUILD_ERROR="运行时配置生成失败"
      return 1
    fi
  elif ! "$NETPROXY_NATIVE_BIN" module prepare \
    --module-dir "$MODDIR" \
    --catalog-root "$CATALOG_DIR" \
    --module-config "$MODULE_CONF" \
    --ebpf-config "$EBPF_CONF" \
    --singbox-dir "$SINGBOX_DIR" \
    --runtime-dir "$runtime_output_dir" > /dev/null; then
    RUNTIME_BUILD_ERROR="运行时配置生成失败"
    return 1
  fi
  [ -s "$RUNTIME_PROVIDERS_FILE" ] \
    && [ -s "$RUNTIME_OUTBOUNDS_FILE" ] \
    && [ -s "$RUNTIME_EBPF_FILE" ] \
    || {
      RUNTIME_BUILD_ERROR="运行时配置文件不完整"
      return 1
    }
}

#######################################
# 保留 service.sh 使用的旧函数名，实际只调用 Go 运行时准备
#######################################
scan_catalog_groups() {
  build_runtime_catalog "$@"
}

#######################################
# 返回运行时 Provider 文件路径
#######################################
write_runtime_providers() {
  [ -n "$RUNTIME_PROVIDERS_FILE" ] || initialize_runtime_context
  [ -s "$RUNTIME_PROVIDERS_FILE" ] || build_runtime_catalog
  printf '%s\n' "$RUNTIME_PROVIDERS_FILE"
}

#######################################
# 返回运行时出站文件路径
#######################################
write_runtime_outbounds() {
  [ -n "$RUNTIME_OUTBOUNDS_FILE" ] || initialize_runtime_context
  [ -s "$RUNTIME_OUTBOUNDS_FILE" ] || build_runtime_catalog
  printf '%s\n' "$RUNTIME_OUTBOUNDS_FILE"
}
