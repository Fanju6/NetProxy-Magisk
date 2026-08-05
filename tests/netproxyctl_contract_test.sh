#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REAL_NETPROXY_NATIVE="${1:-$ROOT/src/module/bin/netproxy-native}"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

MODULE="$TMP_ROOT/module"
mkdir -p "$MODULE/bin" "$MODULE/logs"
cp -R "$ROOT/src/module/scripts" "$MODULE/"
cp -R "$ROOT/src/module/config" "$MODULE/"
cp "$ROOT/src/module/netproxyctl" "$MODULE/netproxyctl"
cp "$REAL_NETPROXY_NATIVE" "$MODULE/bin/netproxy-native"
chmod +x "$MODULE/bin/netproxy-native" "$MODULE/netproxyctl" "$MODULE/scripts/netproxyctl"
SUB_RUNTIME_DIR="$TMP_ROOT/runtime/subscriptions"
export SUB_RUNTIME_DIR
mkdir -p "$SUB_RUNTIME_DIR"
sed -i \
  "s|readonly SUB_RUNTIME_DIR=\"/dev/netproxy/subscriptions\"|readonly SUB_RUNTIME_DIR=\"$SUB_RUNTIME_DIR\"|" \
  "$MODULE/scripts/netproxyctl"

run_json() {
  output="$1"
  printf '%s' "$output" | grep -q '^{' || return 1
  printf '%s' "$output" | grep -q '"schema":1' || return 1
  printf '%s' "$output" | grep -q '"ok":true' || return 1
  if command -v python3 > /dev/null 2>&1; then
    printf '%s' "$output" | python3 -m json.tool > /dev/null
  fi
}

result="$(sh "$MODULE/scripts/netproxyctl" --json catalog list)"
run_json "$result"
printf '%s' "$result" | grep -q '"id":"default"'

result="$(sh "$MODULE/scripts/netproxyctl" --json mode AllowAds)"
run_json "$result"
printf '%s' "$result" | grep -q '"mode":"AllowAds"'
grep -q '^OUTBOUND_MODE=AllowAds$' "$MODULE/config/module.conf"
result="$(sh "$MODULE/scripts/netproxyctl" --json mode rule)"
run_json "$result"

result="$(sh "$MODULE/scripts/netproxyctl" --json node add 'socks://example.com:1080#CLI')"
run_json "$result"
printf '%s' "$result" | grep -q '"code":"node.added"'

result="$(sh "$MODULE/scripts/netproxyctl" --json node list default)"
run_json "$result"
printf '%s' "$result" | grep -q '"tag":"CLI"'
printf '%s' "$result" | grep -q '"protocol":"socks"'

result="$(sh "$MODULE/scripts/netproxyctl" --json node snapshot)"
run_json "$result"
printf '%s' "$result" | grep -q '"code":"node.snapshot"'
printf '%s' "$result" | grep -q '"groups":\['
printf '%s' "$result" | grep -q '"selection":{"active_group_id":"default"'

result="$(sh "$MODULE/scripts/netproxyctl" --json node remove 'default/CLI')"
run_json "$result"

result="$(sh "$MODULE/scripts/netproxyctl" --json sub list)"
run_json "$result"

result="$(sh "$MODULE/scripts/netproxyctl" --json config list)"
run_json "$result"
printf '%s' "$result" | grep -q '"id":"singbox/confdir/03_dns.json"'
printf '%s' "$result" | grep -q '"id":"singbox/source/direct.json"'

result="$(sh "$MODULE/scripts/netproxyctl" --json config read singbox/confdir/03_dns.json)"
run_json "$result"
printf '%s' "$result" | grep -q '"target":"singbox/confdir/03_dns.json"'

mkdir -p "$MODULE/config/catalog/test-sub"
cp "$MODULE/config/catalog/default/meta.json" "$MODULE/config/catalog/test-sub/meta.json"
cp "$MODULE/config/catalog/default/provider.json" "$MODULE/config/catalog/test-sub/provider.json"
sed -i \
  -e 's/"id": "default"/"id": "test-sub"/' \
  -e 's/"name": "本地配置"/"name": "契约订阅"/' \
  -e 's/"type": "local"/"type": "subscription"/' \
  -e 's#"url": ""#"url": "https://example.test/sub?token=secret"#' \
  "$MODULE/config/catalog/test-sub/meta.json"
result="$(sh "$MODULE/scripts/netproxyctl" --json sub show test-sub --private)"
run_json "$result"
printf '%s' "$result" | grep -q '"url":"https://example.test/sub?token=secret"'

printf '%s\n' \
  '{"schema":1,"group_id":"test-sub","stage":"done","message":"订阅更新完成"}' \
  > "$SUB_RUNTIME_DIR/test-sub.progress.json"
result="$(sh "$MODULE/scripts/netproxyctl" --json sub list)"
run_json "$result"
printf '%s' "$result" | grep -q '"id":"test-sub"'
printf '%s' "$result" | grep -q '"progress":null'

printf '%s\n' \
  '{"schema":1,"group_id":"test-sub","stage":"download","message":"正在下载订阅"}' \
  > "$SUB_RUNTIME_DIR/test-sub.progress.json"
result="$(sh "$MODULE/scripts/netproxyctl" --json sub list)"
run_json "$result"
printf '%s' "$result" | grep -q '"progress":{"schema":1,"group_id":"test-sub","stage":"download"'

result="$(sh "$MODULE/scripts/netproxyctl" --json service status)"
run_json "$result"
printf '%s' "$result" | grep -q '"state":"stopped"'
printf '%s' "$result" | grep -q '"active_group_node_count":0'
printf '%s' "$result" | grep -q '"process_cpu_ticks":0'
printf '%s' "$result" | grep -q '"system_cpu_ticks":'
printf '%s' "$result" | grep -q '"cpu_count":'

result="$(sh "$MODULE/scripts/netproxyctl" --json api bootstrap)"
run_json "$result"
printf '%s' "$result" | grep -q '"service_api":{"url":"http://127.0.0.1:9090"'
printf '%s' "$result" | grep -q '"clash_api":{"url":"http://127.0.0.1:9999"'
printf '%s' "$result" | grep -q '"secret":"singbox"'

if result="$(sh "$MODULE/scripts/netproxyctl" --json unknown 2> /dev/null)"; then
  printf '%s\n' 'unknown command should fail' >&2
  exit 1
fi
printf '%s' "$result" | grep -q '"ok":false'

printf '%s\n' "netproxyctl contract test passed"
