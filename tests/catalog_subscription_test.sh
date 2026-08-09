#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NATIVE="${1:-$ROOT/src/module/bin/netproxy-native}"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

MODULE="$ROOT/src/module"
MODULE_CONF="$TMP_ROOT/module.conf"
EBPF_CONF="$TMP_ROOT/ebpf.conf"
CATALOG_ROOT="$TMP_ROOT/catalog"
mkdir -p "$CATALOG_ROOT"
cp "$MODULE/config/module.conf" "$MODULE_CONF"
cp "$MODULE/config/ebpf/ebpf.conf" "$EBPF_CONF"
cp -R "$MODULE/data/catalog/default" "$CATALOG_ROOT/default"

native_module_node() {
  action="$1"
  shift
  "$NATIVE" module node "$action" --module-dir "$MODULE" --catalog-root "$CATALOG_ROOT" --module-config "$MODULE_CONF" --ebpf-config "$EBPF_CONF" "$@"
}

native_module_node add 'socks://example.com:1080#SOCKS' > /dev/null
[ "$("$NATIVE" catalog groups --root "$CATALOG_ROOT" --type all --format json | grep -o '"node_count":[0-9][0-9]*' | head -1)" = '"node_count":1' ]

native_module_node edit 'default/SOCKS' 'http://example.org:8080#EDITED' > /dev/null
"$NATIVE" catalog group-contains-tag --root "$CATALOG_ROOT" --group default   --tag EDITED --format raw | grep -q '^true$'

native_module_node remove 'default/EDITED' > /dev/null
"$NATIVE" catalog group-contains-tag --root "$CATALOG_ROOT" --group default   --tag EDITED --format raw | grep -q '^false$'

printf '%s\n' "catalog native test passed"
