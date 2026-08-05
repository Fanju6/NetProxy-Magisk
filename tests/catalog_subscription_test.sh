#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REAL_NETPROXY_NATIVE="${1:-$ROOT/src/module/bin/netproxy-native}"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

MODDIR="$ROOT/src/module"
MODULE_CONF="$TMP_ROOT/module.conf"
CATALOG_DIR="$TMP_ROOT/catalog"
CATALOG_STAGING_DIR="$CATALOG_DIR/staging"
SUB_RUNTIME_DIR="$TMP_ROOT/runtime"
NETPROXY_NATIVE_BIN="$TMP_ROOT/netproxy-native-mock"
SERVICE_SCRIPT="$MODDIR/scripts/core/service.sh"
SWITCH_SCRIPT="$MODDIR/scripts/core/switch.sh"
SING_BOX_BIN="$TMP_ROOT/sing-box"
LOG_FILE="$TMP_ROOT/subscription.log"
LOG_STDERR=0
LOG_TAG="subscription-test"
SUBSCRIPTION_LIBRARY_ONLY=1

mkdir -p "$CATALOG_DIR/default" "$CATALOG_STAGING_DIR" "$SUB_RUNTIME_DIR"
cp "$MODDIR/config/module.conf" "$MODULE_CONF"
cp "$MODDIR/config/catalog/default/meta.json" "$CATALOG_DIR/default/meta.json"
cp "$MODDIR/config/catalog/default/provider.json" "$CATALOG_DIR/default/provider.json"

"$REAL_NETPROXY_NATIVE" convert link \
  --input 'socks://example.com:1080#SOCKS' \
  --output "$TMP_ROOT/subscription-provider.json" > /dev/null

cat > "$NETPROXY_NATIVE_BIN" << 'EOF'
#!/usr/bin/env sh
set -eu

if [ "${1:-}" = "convert" ] && [ "${2:-}" = "subscription" ]; then
  shift 2
  url=""
  output=""
  metadata=""
  diagnostics=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --url) url="$2"; shift 2 ;;
      --output) output="$2"; shift 2 ;;
      --metadata-output) metadata="$2"; shift 2 ;;
      --diagnostics-output) diagnostics="$2"; shift 2 ;;
      --headers-file | --timeout | --user-agent | --hwid | --etag | --last-modified | --include | --exclude | --proxy) shift 2 ;;
      --allow-insecure) shift ;;
      *) shift ;;
    esac
  done
  case "$url" in
    */304)
      printf '%s\n' '{"status_code":304,"not_modified":true,"etag":"etag-1"}' > "$metadata"
      printf '%s\n' '[]' > "$diagnostics"
      printf '%s\n' '{"schema":1,"ok":true,"code":"subscription.not_modified","message":"订阅未发生变化"}'
      ;;
    */fail)
      printf '%s\n' '{"status_code":502,"not_modified":false}' > "$metadata"
      printf '%s\n' '[]' > "$diagnostics"
      printf '%s\n' '{"schema":1,"ok":false,"code":"command.failed","message":"subscription request failed"}' >&2
      exit 1
      ;;
    *)
      cp "$MOCK_PROVIDER" "$output"
      printf '%s\n' '{"status_code":200,"not_modified":false,"etag":"etag-1","profile_title":"测试订阅","profile_web_page_url":"https://example.com","update_interval_seconds":1800,"usage":{"upload":10,"download":20,"total":100,"expire":4102444800}}' > "$metadata"
      printf '%s\n' '[]' > "$diagnostics"
      printf '%s\n' '{"schema":1,"ok":true,"code":"conversion.completed","message":"转换完成","data":{"node_count":1}}'
      ;;
  esac
  exit 0
fi

exec "$REAL_NETPROXY_NATIVE" "$@"
EOF
chmod +x "$NETPROXY_NATIVE_BIN"
export REAL_NETPROXY_NATIVE
export MOCK_PROVIDER="$TMP_ROOT/subscription-provider.json"

. "$MODDIR/scripts/utils/common.sh"
. "$MODDIR/scripts/utils/config.sh"
. "$MODDIR/scripts/utils/api.sh"
. "$MODDIR/scripts/utils/catalog.sh"
. "$MODDIR/scripts/utils/metadata.sh"
. "$MODDIR/scripts/core/subscription.sh"

group_id="$(add_subscription "测试订阅" "https://example.test/ok")"
group_dir="$CATALOG_DIR/$group_id"
[ -f "$group_dir/provider.json" ]
[ "$(meta_get_raw "$group_dir/meta.json" node_count 0)" -eq 1 ]
[ "$(meta_get_raw "$group_dir/meta.json" revision 0)" -eq 1 ]
[ "$(meta_get_raw "$group_dir/meta.json" update_interval 0)" -eq 1800 ]
[ "$(meta_get_string "$group_dir/meta.json" interval_source '')" = "profile" ]
[ "$(meta_get_raw "$group_dir/meta.json" usage null)" != "null" ]
[ "$(read_conf "$MODULE_CONF" ACTIVE_GROUP_ID '')" = "$group_id" ]

load_catalog_meta "$group_dir/meta.json"
SUB_URL="https://example.test/304"
write_catalog_meta "$group_dir/meta.json"
update_subscription "$group_id"
[ "$(meta_get_raw "$group_dir/meta.json" revision 0)" -eq 1 ]
[ "$(wc -l < "$group_dir/history.jsonl" | tr -d ' ')" -eq 2 ]

before="$(cksum "$group_dir/provider.json")"
load_catalog_meta "$group_dir/meta.json"
SUB_URL="https://example.test/fail"
write_catalog_meta "$group_dir/meta.json"
if update_subscription "$group_id"; then
  printf '%s\n' 'expected subscription failure' >&2
  exit 1
fi
after="$(cksum "$group_dir/provider.json")"
[ "$before" = "$after" ]
[ "$(meta_get_string "$group_dir/meta.json" last_error '')" = "订阅下载、转换或校验失败" ]

append_local_node default 'http://example.net:8080#LOCAL'
[ "$(meta_get_raw "$CATALOG_DIR/default/meta.json" node_count 0)" -eq 1 ]
copy_node_to_local "$group_id/SOCKS" default
[ "$(meta_get_raw "$CATALOG_DIR/default/meta.json" node_count 0)" -eq 2 ]
remove_local_node 'default/LOCAL'
[ "$(meta_get_raw "$CATALOG_DIR/default/meta.json" node_count 0)" -eq 1 ]

if remove_local_node "$group_id/SOCKS"; then
  printf '%s\n' 'subscription nodes must be read-only' >&2
  exit 1
fi

if command -v python3 > /dev/null 2>&1; then
  python3 -m json.tool "$group_dir/meta.json" > /dev/null
  python3 -m json.tool "$group_dir/provider.json" > /dev/null
  python3 -m json.tool "$CATALOG_DIR/default/meta.json" > /dev/null
fi

printf '%s\n' "catalog subscription test passed"
