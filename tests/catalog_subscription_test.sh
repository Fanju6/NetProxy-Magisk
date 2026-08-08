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
  proxy=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --url) url="$2"; shift 2 ;;
      --output) output="$2"; shift 2 ;;
      --metadata-output) metadata="$2"; shift 2 ;;
      --diagnostics-output) diagnostics="$2"; shift 2 ;;
      --proxy) proxy="$2"; shift 2 ;;
      --headers-file | --timeout | --user-agent | --hwid | --etag | --last-modified | --include | --exclude) shift 2 ;;
      --allow-insecure) shift ;;
      *) shift ;;
    esac
  done
  if [ -n "$proxy" ] && [ "${MOCK_FAIL_PROXY:-0}" = "1" ]; then
    printf '%s\n' '{"status_code":502,"not_modified":false}' > "$metadata"
    printf '%s\n' '[]' > "$diagnostics"
    printf '%s\n' '{"schema":1,"ok":false,"code":"command.failed","message":"proxy request failed"}' >&2
    exit 1
  fi
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

if [ "$1" = "subscription" ] && [ "$2" = "update" ]; then
  shift 2
  root=""
  group=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --root) root="$2"; shift 2 ;;
      --group) group="$2"; shift 2 ;;
      --proxy) shift 2 ;;
      --progress-dir | --now) shift 2 ;;
      --fallback-direct) shift ;;
      *) shift ;;
    esac
  done
  group_dir="$root/$group"
  meta="$group_dir/meta.json"
  provider="$group_dir/provider.json"
  url="$(sed -n 's/.*"url":[[:space:]]*"\([^"]*\)".*/\1/p' "$meta")"
  if printf "%s" "$url" | grep -q '/fail$'; then
    sed -i 's/"last_error":[[:space:]]*"[^"]*"/"last_error": "订阅下载、转换或校验失败"/' "$meta"
    exit 1
  fi
  if printf "%s" "$url" | grep -q '/304$'; then
    printf '%s\n' '{"at":"fixture","ok":true,"code":"subscription.not_modified"}' >> "$group_dir/history.jsonl"
    printf '%s\n' '{"schema":1,"ok":true,"code":"subscription.not_modified","message":"订阅未发生变化","data":{"not_modified":true,"node_count":1,"revision":2}}'
    exit 0
  fi
  cp "$MOCK_PROVIDER" "$provider"
  revision="$(sed -n 's/.*"revision":[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$meta")"
  [ -n "$revision" ] || revision=0
  revision=$((revision + 1))
  sed -i \
    -e "s/\"node_count\":[[:space:]]*[0-9][0-9]*/\"node_count\": 1/" \
    -e "s/\"revision\":[[:space:]]*[0-9][0-9]*/\"revision\": $revision/" \
    -e 's/"update_interval":[[:space:]]*[0-9][0-9]*/"update_interval": 1800/' \
    -e 's/"interval_source":[[:space:]]*"[^"]*"/"interval_source": "profile"/' \
    -e 's/"usage":[[:space:]]*null/"usage": {"upload": 10, "download": 20, "total": 100, "expire": 4102444800}/' \
    -e 's/"last_error":[[:space:]]*"[^"]*"/"last_error": ""/' \
    "$meta"
  printf '%s\n' "{\"at\":\"fixture\",\"ok\":true,\"code\":\"subscription.updated\",\"node_count\":1,\"revision\":$revision}" >> "$group_dir/history.jsonl"
  structure_changed=false
  [ "$revision" -eq 1 ] && structure_changed=true
  printf '%s\n' "{\"schema\":1,\"ok\":true,\"code\":\"subscription.updated\",\"message\":\"订阅更新完成\",\"data\":{\"group_id\":\"$group\",\"node_count\":1,\"revision\":$revision,\"structure_changed\":$structure_changed}}"
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
. "$MODDIR/scripts/core/subscription.sh"

meta_get_raw() {
  "$NETPROXY_NATIVE_BIN" catalog meta-get --input "$1" --field "$2" --format raw
}

meta_get_string() {
  "$NETPROXY_NATIVE_BIN" catalog meta-get --input "$1" --field "$2" --format string
}

set_test_url() {
  sed -i "s#\"url\"[[:space:]]*:[[:space:]]*\"[^\"]*\"#\"url\": \"$1\"#" "$group_dir/meta.json"
}

group_id="$(add_subscription "测试订阅" "https://example.test/ok")"
group_dir="$CATALOG_DIR/$group_id"
[ -f "$group_dir/provider.json" ]
[ "$(meta_get_raw "$group_dir/meta.json" node_count 0)" -eq 1 ]
[ "$(meta_get_raw "$group_dir/meta.json" revision 0)" -eq 1 ]
[ "$(meta_get_raw "$group_dir/meta.json" update_interval 0)" -eq 1800 ]
[ "$(meta_get_string "$group_dir/meta.json" interval_source '')" = "profile" ]
[ "$(meta_get_raw "$group_dir/meta.json" usage null)" != "null" ]
[ "$(read_conf "$MODULE_CONF" ACTIVE_GROUP_ID '')" = "$group_id" ]

export MOCK_USE_PROXY=1 MOCK_FAIL_PROXY=1
update_subscription "$group_id"
unset MOCK_USE_PROXY MOCK_FAIL_PROXY
[ "$(meta_get_string "$group_dir/meta.json" last_error '')" = "" ]
[ "$(meta_get_raw "$group_dir/meta.json" revision 0)" -eq 2 ]

set_test_url "https://example.test/304"
update_subscription "$group_id"
[ "$(meta_get_raw "$group_dir/meta.json" revision 0)" -eq 2 ]
[ "$(wc -l < "$group_dir/history.jsonl" | tr -d ' ')" -eq 3 ]

before="$(cksum "$group_dir/provider.json")"
set_test_url "https://example.test/fail"
if update_subscription "$group_id"; then
  printf '%s\n' 'expected subscription failure' >&2
  exit 1
fi
after="$(cksum "$group_dir/provider.json")"
[ "$before" = "$after" ]
[ "$(meta_get_string "$group_dir/meta.json" last_error '')" = "订阅下载、转换或校验失败" ]

append_local_node default 'http://example.net:8080#LOCAL'
[ "$(meta_get_raw "$CATALOG_DIR/default/meta.json" node_count 0)" -eq 1 ]
remove_catalog_node 'default/LOCAL'
[ "$(meta_get_raw "$CATALOG_DIR/default/meta.json" node_count 0)" -eq 0 ]

edit_catalog_node "$group_id/SOCKS" 'http://example.org:8080#EDITED'
catalog_group_contains_tag "$group_id" EDITED
[ "$(meta_get_raw "$group_dir/meta.json" node_count 0)" -eq 1 ]
remove_catalog_node "$group_id/EDITED"
[ "$(meta_get_raw "$group_dir/meta.json" node_count 0)" -eq 0 ]

set_test_url "https://example.test/ok"
update_subscription "$group_id"
catalog_group_contains_tag "$group_id" SOCKS

if command -v python3 > /dev/null 2>&1; then
  python3 -m json.tool "$group_dir/meta.json" > /dev/null
  python3 -m json.tool "$group_dir/provider.json" > /dev/null
  python3 -m json.tool "$CATALOG_DIR/default/meta.json" > /dev/null
fi

printf '%s\n' "catalog subscription test passed"
