#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NETPROXYCTL_BIN="${1:-$ROOT/src/module/bin/netproxyctl}"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

MODDIR="$ROOT/src/module"
TEST_MODULE="$TMP_ROOT/module"
MODULE_CONF="$TEST_MODULE/config/module.conf"
CATALOG_DIR="$TEST_MODULE/data/catalog"
SINGBOX_DIR="$MODDIR/config/singbox"
MIXED_INBOUND_FILE="$SINGBOX_DIR/config.json"
SUBSCRIPTION_UPDATE_SOURCE="$ROOT/src/native/netproxy/internal/subscription/update.go"
CATALOG_LIST_OUTPUT="$TMP_ROOT/catalog-list.json"
CATALOG_SHOW_OUTPUT="$TMP_ROOT/catalog-show.json"

mkdir -p "$TEST_MODULE/config/inbound" "$CATALOG_DIR/default" "$CATALOG_DIR/secondary" "$CATALOG_DIR/staging"
cp "$MODDIR/config/module.conf" "$MODULE_CONF"
cp "$MODDIR/config/inbound/inbound.json" "$TEST_MODULE/config/inbound/inbound.json"
mkdir -p "$TEST_MODULE/config/singbox" "$TEST_MODULE/runtime"
cp "$SINGBOX_DIR/config.json" "$TEST_MODULE/config/singbox/config.json"
printf '%s\n' '{"inbounds":[]}' > "$TEST_MODULE/runtime/inbound.json"
printf '%s\n' '{"providers":[]}' > "$TEST_MODULE/runtime/providers.json"
printf '%s\n' '{"outbounds":[]}' > "$TEST_MODULE/runtime/outbounds.json"
cp "$MODDIR/data/catalog/default/meta.json" "$CATALOG_DIR/default/meta.json"
cp "$MODDIR/data/catalog/default/meta.json" "$CATALOG_DIR/secondary/meta.json"
sed -i 's/"node_count": 0/"node_count": 1/' "$CATALOG_DIR/default/meta.json" "$CATALOG_DIR/secondary/meta.json"
sed -i 's/"id": "default"/"id": "secondary"/; s/"name": "本地配置"/"name": "备用配置"/' "$CATALOG_DIR/secondary/meta.json"

printf '%s\n' '{"outbounds":[{"type":"socks","tag":"SOCKS","server":"example.com","server_port":1080}]}' \
  > "$CATALOG_DIR/default/provider.json"
printf '%s\n' '{"outbounds":[{"type":"http","tag":"HTTP","server":"example.net","server_port":8080}]}' \
  > "$CATALOG_DIR/secondary/provider.json"

NETPROXY_MODULE_DIR="$TEST_MODULE" SUB_RUNTIME_DIR="$TMP_ROOT/subscriptions" \
  "$NETPROXYCTL_BIN" --json catalog list > "$CATALOG_LIST_OUTPUT"
grep -q '"code":"catalog.groups"' "$CATALOG_LIST_OUTPUT"
grep -q '"id":"default"' "$CATALOG_LIST_OUTPUT"
grep -q '"runtime_tag":"本地配置"' "$CATALOG_LIST_OUTPUT"
grep -q '"id":"secondary"' "$CATALOG_LIST_OUTPUT"
grep -q '"runtime_tag":"备用配置"' "$CATALOG_LIST_OUTPUT"

NETPROXY_MODULE_DIR="$TEST_MODULE" SUB_RUNTIME_DIR="$TMP_ROOT/subscriptions" \
  "$NETPROXYCTL_BIN" --json catalog show default > "$CATALOG_SHOW_OUTPUT"
grep -q '"code":"catalog.show"' "$CATALOG_SHOW_OUTPUT"
grep -q '"tag":"SOCKS"' "$CATALOG_SHOW_OUTPUT"

# 在隔离模块目录验证客户端共同消费的目标、revision 和空实例状态，不启动核心。
node --input-type=module - "$NETPROXYCTL_BIN" "$TEST_MODULE" <<'NODE'
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const [binary, moduleDir] = process.argv.slice(2);
const env = { ...process.env, NETPROXY_MODULE_DIR: moduleDir };
const run = (...args) => {
  const result = spawnSync(binary, ['--json', ...args], { env, encoding: 'utf8', timeout: 15000 });
  assert.ifError(result.error);
  const response = JSON.parse(result.stdout);
  assert.equal(response.schema, 1);
  assert.equal(response.ok, result.status === 0);
  return response;
};
const success = (...args) => {
  const response = run(...args);
  assert.equal(response.ok, true, response.message);
  return response.data;
};
const documents = success('config', 'list');
assert.deepEqual(documents.filter(item => item.category === 'inbound').map(item => item.id),
  ['inbound', 'inbound/backend', 'inbound/app', 'inbound/ebpf', 'inbound/tun']);
assert.equal(new Set(documents.map(item => item.id)).size, documents.length);
const inboundPath = join(moduleDir, 'config', 'inbound', 'inbound.json');
const template = JSON.parse(readFileSync(inboundPath, 'utf8'));
for (const target of ['inbound', 'inbound/backend', 'inbound/app', 'inbound/ebpf', 'inbound/tun']) {
  const document = documents.find(item => item.id === target);
  assert.equal(document.editable, true);
  const read = success('config', 'read', target);
  assert.equal(read.target, target);
  assert.equal(read.revision, createHash('sha256').update(read.content).digest('hex'));
  const content = JSON.parse(read.content);
  if (target === 'inbound') {
    assert.deepEqual(content, template);
  } else {
    const section = target.split('/')[1];
    assert.deepEqual(Object.keys(content), [section]);
    assert.deepEqual(content[section], template[section]);
  }
}
assert.deepEqual(documents.filter(item => item.category === 'runtime').map(item => item.id),
  ['runtime/inbound.json', 'runtime/outbounds.json', 'runtime/providers.json']);
for (const target of ['runtime/inbound.json', 'runtime/outbounds.json', 'runtime/providers.json']) {
  assert.equal(documents.find(item => item.id === target).editable, false);
  success('config', 'read', target);
  assert.equal(run('config', 'apply', target, inboundPath).ok, false);
}
for (const target of ['ebpf', 'runtime/ebpf.json', 'inbound/unknown']) {
  assert.equal(run('config', 'read', target).ok, false);
  assert.equal(run('config', 'apply', target, inboundPath).ok, false);
}
assert.equal(run('tun', 'status').code, 'usage.invalid');
for (const backend of ['ebpf', 'tun']) {
  template.backend = backend;
  writeFileSync(inboundPath, JSON.stringify(template));
  const status = success('service', 'status');
  assert.equal(status.configured_backend, backend);
  assert.equal(status.active_backend, null);
  assert.equal(status.pid, null);
  assert.equal(['stopped', 'failed'].includes(status.state), true);
}
NODE

grep -q '"external_controller": "127.0.0.1:9999"' "$SINGBOX_DIR/config.json"
grep -q '"listen": "127.0.0.1"' "$SINGBOX_DIR/config.json"
grep -q '"secret": "singbox"' "$SINGBOX_DIR/config.json"

# mixed 7080 仅供本机订阅下载使用，不得暴露到通配 IPv4/IPv6 地址。
grep -q '"tag": "mixed-in"' "$MIXED_INBOUND_FILE"
grep -q '"listen": "127.0.0.1"' "$MIXED_INBOUND_FILE"
grep -q '"listen_port": 7080' "$MIXED_INBOUND_FILE"
! grep -Eq '"listen"[[:space:]]*:[[:space:]]*"(0\.0\.0\.0|::)"' "$MIXED_INBOUND_FILE"
grep -q 'options.ProxyURL = "http://127.0.0.1:7080"' "$SUBSCRIPTION_UPDATE_SOURCE"
! grep -Eq 'ProxyURL = "http://(0\.0\.0\.0|::):7080"' "$SUBSCRIPTION_UPDATE_SOURCE"

node --test "$ROOT/tests/default_config_test.mjs"

printf '%s\n' "runtime catalog test passed"
