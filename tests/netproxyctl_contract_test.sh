#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REAL_NETPROXY_CTL="${1:-$ROOT/.tmp/netproxyctl}"
REAL_NETPROXY_NATIVE="${2:-$ROOT/.tmp/netproxy-native}"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

MODULE="$TMP_ROOT/module"
mkdir -p "$MODULE/bin" "$MODULE/logs"
cp -R "$ROOT/src/module/scripts" "$MODULE/"
cp -R "$ROOT/src/module/config" "$MODULE/"
cp -R "$ROOT/src/module/data" "$MODULE/"
mkdir -p "$MODULE/runtime"
# 契约测试在宿主机运行，使用同等行为的临时 POSIX 入口，不执行 Android AArch64 发行二进制。
printf '%s\n' \
  '#!/usr/bin/env sh' \
  'exec "${0%/*}/bin/netproxyctl" "$@"' \
  > "$MODULE/netproxyctl"
cp "$REAL_NETPROXY_CTL" "$MODULE/bin/netproxyctl"
cp "$REAL_NETPROXY_NATIVE" "$MODULE/bin/netproxy-native"
cp "$REAL_NETPROXY_NATIVE" "$MODULE/bin/netproxy-native.exe"
cat > "$MODULE/bin/sing-box" << 'EOF'
#!/usr/bin/env sh
[ "${1:-}" = "tools" ] && {
  printf '%s\n' 'Platform: kernel: 6.1.0; architecture: arm64;'
  printf '%s\n' 'Summary: PASS=8 WARN=0 FAIL=0 UNKNOWN=0'
  exit 0
}
[ "${1:-}" = "check" ]
EOF
chmod +x "$MODULE/bin/netproxyctl" "$MODULE/bin/netproxy-native" "$MODULE/bin/netproxy-native.exe" "$MODULE/bin/sing-box" "$MODULE/netproxyctl"
SUB_RUNTIME_DIR="$TMP_ROOT/runtime/subscriptions"
export SUB_RUNTIME_DIR
export NETPROXY_MODULE_DIR="$MODULE"
export NETPROXY_NATIVE_BIN="$MODULE/bin/netproxy-native.exe"
mkdir -p "$SUB_RUNTIME_DIR"

run_json() {
  output="$1"
  printf '%s' "$output" | grep -q '^{' || return 1
  printf '%s' "$output" | grep -q '"schema":1' || return 1
  printf '%s' "$output" | grep -q '"ok":true' || return 1
  if command -v python3 > /dev/null 2>&1; then
    printf '%s' "$output" | python3 -m json.tool > /dev/null
  fi
}

result="$(sh "$MODULE/netproxyctl" --json catalog list)"
run_json "$result"
printf '%s' "$result" | grep -q '"id":"default"'

result="$(sh "$MODULE/netproxyctl" --json mode AllowAds)"
run_json "$result"
printf '%s' "$result" | grep -q '"mode":"AllowAds"'
grep -q '^OUTBOUND_MODE=AllowAds$' "$MODULE/config/module.conf"
result="$(sh "$MODULE/netproxyctl" --json mode rule)"
run_json "$result"

result="$(sh "$MODULE/netproxyctl" --json app list)"
run_json "$result"
printf '%s' "$result" | grep -q '"android_users":""'

case "$(uname -s 2>/dev/null || printf unknown)" in
  MINGW* | MSYS* | CYGWIN*) : ;;
  *)
    result="$(sh "$MODULE/netproxyctl" --json ebpf status configured)"
    run_json "$result"
    printf '%s' "$result" | grep -q '"code":"ebpf.status"'
    printf '%s' "$result" | grep -q '通过: 8 项'
    ;;
esac

result="$(sh "$MODULE/netproxyctl" --json app users 0 999)"
run_json "$result"
grep -q '^APP_ANDROID_USERS="0 999"$' "$MODULE/config/ebpf/ebpf.conf"
result="$(sh "$MODULE/netproxyctl" --json app add com.android.chrome)"
run_json "$result"
grep -q '^BYPASS_APPS_LIST="com.android.chrome"$' "$MODULE/config/ebpf/ebpf.conf"
if result="$(sh "$MODULE/netproxyctl" --json app add 0:com.android.chrome 2> /dev/null)"; then
  printf '%s\n' 'legacy user:package syntax should fail' >&2
  exit 1
fi
printf '%s' "$result" | grep -q '"code":"app.package_invalid"'

cp "$MODULE/config/module.conf" "$TMP_ROOT/module.candidate"
original_auto_start="$(sed -n 's/^AUTO_START=//p' "$MODULE/config/module.conf")"
if [ "$original_auto_start" = "1" ]; then
  candidate_auto_start=0
else
  candidate_auto_start=1
fi
sed -i "s/^AUTO_START=.*/AUTO_START=$candidate_auto_start/" "$TMP_ROOT/module.candidate"
result="$(sh "$MODULE/netproxyctl" --json config validate module "$TMP_ROOT/module.candidate")"
run_json "$result"
grep -q "^AUTO_START=$original_auto_start$" "$MODULE/config/module.conf"
result="$(sh "$MODULE/netproxyctl" --json config apply module "$TMP_ROOT/module.candidate")"
run_json "$result"
grep -q "^AUTO_START=$candidate_auto_start$" "$MODULE/config/module.conf"

result="$(sh "$MODULE/netproxyctl" --json node add 'socks://example.com:1080#CLI')"
run_json "$result"
printf '%s' "$result" | grep -q '"code":"node.added"'

result="$(sh "$MODULE/netproxyctl" --json node list default)"
run_json "$result"
printf '%s' "$result" | grep -q '"tag":"CLI"'
printf '%s' "$result" | grep -q '"protocol":"socks"'

result="$(sh "$MODULE/netproxyctl" --json node get 'default/CLI')"
run_json "$result"
printf '%s' "$result" | grep -q '"code":"node.loaded"'
printf '%s' "$result" | grep -q '"outbounds":\['

result="$(sh "$MODULE/netproxyctl" --json node export 'default/CLI')"
run_json "$result"
printf '%s' "$result" | grep -q '"code":"node.exported"'
printf '%s' "$result" | grep -q '"link":"socks://example.com:1080#CLI"'

printf '%s\n' '{"outbounds":[{"type":"socks","tag":"CLI-JSON","server":"example.org","server_port":1081}]}' \
  > "$TMP_ROOT/node-edit.json"
result="$(sh "$MODULE/netproxyctl" --json node edit 'default/CLI' "$TMP_ROOT/node-edit.json")"
run_json "$result"
printf '%s' "$result" | grep -q '"code":"node.edited"'
result="$(sh "$MODULE/netproxyctl" --json node export 'default/CLI-JSON')"
run_json "$result"
printf '%s' "$result" | grep -q '"link":"socks://example.org:1081#CLI-JSON"'

result="$(sh "$MODULE/netproxyctl" --json node snapshot)"
run_json "$result"
printf '%s' "$result" | grep -q '"code":"node.snapshot"'
printf '%s' "$result" | grep -q '"groups":\['
printf '%s' "$result" | grep -q '"selection":{"active_group_id":"default"'
printf '%s' "$result" | grep -q '"selected":"Auto/本地配置"'

stderr_file="$TMP_ROOT/node-current.stderr"
result="$(sh "$MODULE/netproxyctl" --json node current 2> "$stderr_file")"
run_json "$result"
printf '%s' "$result" | grep -q '"code":"node.current"'
printf '%s' "$result" | grep -q '"selected":"Auto/本地配置"'
[ ! -s "$stderr_file" ]

stderr_file="$TMP_ROOT/mode.stderr"
result="$(sh "$MODULE/netproxyctl" --json mode 2> "$stderr_file")"
run_json "$result"
printf '%s' "$result" | grep -q '"code":"mode.current"'
printf '%s' "$result" | grep -q '"mode":"rule"'
[ ! -s "$stderr_file" ]

result="$(sh "$MODULE/netproxyctl" --json node remove 'default/CLI-JSON')"
run_json "$result"

result="$(sh "$MODULE/netproxyctl" --json sub list)"
run_json "$result"

result="$(sh "$MODULE/netproxyctl" --json config list)"
run_json "$result"
printf '%s' "$result" | grep -q '"id":"singbox/confdir/03_dns.json"'
printf '%s' "$result" | grep -q '"id":"singbox/source/direct.json"'

result="$(sh "$MODULE/netproxyctl" --json config read singbox/confdir/03_dns.json)"
run_json "$result"
printf '%s' "$result" | grep -q '"target":"singbox/confdir/03_dns.json"'

mkdir -p "$MODULE/data/catalog/test-sub"
cp "$MODULE/data/catalog/default/meta.json" "$MODULE/data/catalog/test-sub/meta.json"
cp "$MODULE/data/catalog/default/provider.json" "$MODULE/data/catalog/test-sub/provider.json"
sed -i \
  -e 's/"id": "default"/"id": "test-sub"/' \
  -e 's/"name": "本地配置"/"name": "契约订阅"/' \
  -e 's/"type": "local"/"type": "subscription"/' \
  -e 's#"url": ""#"url": "https://example.test/sub?token=secret"#' \
  "$MODULE/data/catalog/test-sub/meta.json"
"$MODULE/bin/netproxy-native" provider append \
  --target "$MODULE/data/catalog/test-sub/provider.json" \
  --input 'socks://example.com:1080#SUB' > /dev/null

result="$(sh "$MODULE/netproxyctl" --json node edit 'test-sub/SUB' 'http://example.org:8080#SUB-EDITED')"
run_json "$result"
printf '%s' "$result" | grep -q '"code":"node.edited"'
result="$(sh "$MODULE/netproxyctl" --json node export 'test-sub/SUB-EDITED')"
run_json "$result"
printf '%s' "$result" | grep -q '"link":"http://example.org:8080#SUB-EDITED"'
result="$(sh "$MODULE/netproxyctl" --json node remove 'test-sub/SUB-EDITED')"
run_json "$result"
printf '%s' "$result" | grep -q '"code":"node.removed"'

result="$(sh "$MODULE/netproxyctl" --json sub show test-sub --private)"
run_json "$result"
printf '%s' "$result" | grep -q '"url":"https://example.test/sub?token=secret"'

printf '%s\n' \
  '{"schema":1,"group_id":"test-sub","stage":"done","message":"订阅更新完成"}' \
  > "$SUB_RUNTIME_DIR/test-sub.progress.json"
result="$(sh "$MODULE/netproxyctl" --json sub list)"
run_json "$result"
printf '%s' "$result" | grep -q '"id":"test-sub"'
printf '%s' "$result" | grep -q '"progress":null'

printf '%s\n' \
  '{"schema":1,"group_id":"test-sub","stage":"download","message":"正在下载订阅"}' \
  > "$SUB_RUNTIME_DIR/test-sub.progress.json"
result="$(sh "$MODULE/netproxyctl" --json sub list)"
run_json "$result"
printf '%s' "$result" | grep -q '"progress":{"schema":1,"group_id":"test-sub","stage":"download"'

result="$(sh "$MODULE/netproxyctl" --json service status)"
run_json "$result"
printf '%s' "$result" | grep -q '"state":"stopped"'
printf '%s' "$result" | grep -q '"active_group_name":"本地配置"'
printf '%s' "$result" | grep -q '"active_group_node_count":0'
printf '%s' "$result" | grep -q '"process_cpu_ticks":0'
printf '%s' "$result" | grep -q '"system_cpu_ticks":'
printf '%s' "$result" | grep -q '"cpu_count":'

if result="$(sh "$MODULE/netproxyctl" --json unknown 2> /dev/null)"; then
  printf '%s\n' 'unknown command should fail' >&2
  exit 1
fi
printf '%s' "$result" | grep -q '"ok":false'

printf '%s\n' "netproxyctl contract test passed"
