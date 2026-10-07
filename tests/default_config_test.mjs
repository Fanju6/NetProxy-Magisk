import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const root = new URL('../', import.meta.url)
const readJSON = path => JSON.parse(readFileSync(new URL(path, root), 'utf8'))
const config = readJSON('src/module/config/singbox/config.json')
const managed = readJSON('src/module/config/inbound/inbound.json')
const upstream = readJSON('tests/fixtures/singbox-upstream.json')
const resources = readJSON('.github/resources.json').raw
const list = value => value === undefined ? [] : Array.isArray(value) ? value : [value]

test('默认配置仅保留部署与运行时生成所需的上游差异', () => {
  const expected = structuredClone(upstream)
  expected.log.output = '/data/adb/modules/netproxy/logs/sing-box.log'
  expected.experimental.cache_file.path = '/data/adb/modules/netproxy/config/singbox/cache.db'
  expected.experimental.clash_api.external_controller = '127.0.0.1:9999'
  delete expected.experimental.clash_api.external_ui
  delete expected.experimental.clash_api.external_ui_download_url
  expected.services[0].listen = '127.0.0.1'
  expected.services[0].listen_port = 9090
  expected.services[0].dashboard.path = '/data/adb/modules/netproxy/webroot/sing-box-dashboard'
  expected.inbounds = expected.inbounds.filter(inbound => inbound.type !== 'ebpf')
  expected.inbounds[0].listen = '127.0.0.1'
  delete expected.outbounds
  delete expected.providers
  for (const rule of expected.route.rule_set) {
    rule.path = rule.path.replace('./source/rule_set/', './rules/remote/').replace('./source/', './rules/local/')
  }
  expected.route.rules.push({ clash_mode: 'Rule', action: 'route', outbound: expected.route.final })
  assert.deepEqual(config, expected)
})

test('默认规则显式提供 Rule，保存其他默认模式后仍可切回', () => {
  const rule = config.route.rules.at(-1)
  assert.deepEqual(rule, { clash_mode: 'Rule', action: 'route', outbound: config.route.final })
  assert.equal(config.experimental.clash_api.default_mode, 'Rule')
})

test('默认远程规则有对应的内置文件与更新来源', () => {
  for (const rule of config.route.rule_set.filter(rule => rule.type === 'remote')) {
    for (const tag of list(rule.tag)) {
      const path = `src/module/config/singbox/${rule.path.replace(/^\.\//, '').replaceAll('{tag}', tag)}`
      const resource = resources.find(resource => resource.path === path)
      assert.ok(resource, `${tag} 未登记资源更新来源`)
      assert.equal(decodeURI(resource.url), decodeURI(rule.url.replaceAll('{tag}', tag)))
      const content = readFileSync(new URL(path, root))
      assert.ok(content.length > 0, `${tag} 内置规则为空`)
      assert.equal(createHash('sha256').update(content).digest('hex'), resource.currentSha256, tag)
      if (rule.format === 'binary') {
        assert.equal(content.subarray(0, 3).toString(), 'SRS', tag)
      }
    }
  }
})

test('eBPF 默认绕过引用与上游和静态规则一致', () => {
  const localBypass = managed.ebpf.local.bypass_rule_set
  const sharedBypass = managed.ebpf.shared.bypass_rule_set
  const inbound = upstream.inbounds.find(inbound => inbound.type === 'ebpf')
  assert.deepEqual(localBypass, inbound.local.bypass_rule_set)
  assert.deepEqual(sharedBypass, inbound.shared.bypass_rule_set)
  assert.equal(Object.hasOwn(managed.ebpf, 'bypass_rule_set'), false)
  const tags = config.route.rule_set.flatMap(rule => list(rule.tag))
  for (const tag of [...localBypass, ...sharedBypass]) assert.ok(tags.includes(tag), `${tag} 未在静态配置声明`)
})

test('单一入站默认保留 eBPF 原生语义和禁用共享路径偏好', () => {
  assert.deepEqual(Object.keys(managed), ['backend', 'app', 'ebpf', 'tun'])
  assert.equal(managed.backend, 'ebpf')
  assert.deepEqual(managed.app, { enabled: true, mode: 'blacklist', proxy_apps: [], bypass_apps: [] })
  assert.deepEqual(managed.ebpf, {
    type: 'ebpf', tag: 'netproxy-in', network: ['tcp', 'udp'], udp_timeout: '5m', tc_priority: 1,
    local: {
      enabled: true, data_plane: 'cgroup', dns_mode: 'respect_policy', ipv6: true,
      bypass_private_address: true, bypass_rule_set: ['geoip/cn'],
    },
    shared: {
      enabled: false, data_plane: 'packet_rewrite', dns_mode: 'hijack', interface: ['wlan2'],
      ipv6: true, bypass_private_address: true, bypass_rule_set: ['geoip/cn'],
    },
  })
})

test('TUN 默认只固定必需接管参数，不覆盖上游可选默认', () => {
  assert.deepEqual(managed.tun, {
    type: 'tun', tag: 'netproxy-in', interface_name: 'netproxy',
    address: ['172.19.0.1/30', 'fdfe:dcba:9876::1/126'],
    auto_route: true, auto_redirect: true, dns_mode: 'hijack',
  })
  assert.equal(config.route.auto_detect_interface, true)
  assert.ok(config.inbounds.every(inbound => !['ebpf', 'tun'].includes(inbound.type) && inbound.tag !== 'netproxy-in'))
})
