import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const root = new URL('../', import.meta.url)
const readJSON = path => JSON.parse(readFileSync(new URL(path, root), 'utf8'))
const config = readJSON('src/module/config/singbox/config.json')
const upstream = readJSON('tests/fixtures/singbox-upstream.json')
const resources = readJSON('.github/resources.json').raw
const list = value => value === undefined ? [] : Array.isArray(value) ? value : [value]

test('默认配置仅保留部署与运行时生成所需的上游差异', () => {
  const expected = structuredClone(upstream)
  expected.log.output = '/data/adb/modules/netproxy/logs/sing-box.log'
  expected.experimental.cache_file.path = '/data/adb/modules/netproxy/config/singbox/cache.db'
  expected.experimental.clash_api.external_controller = '127.0.0.1:9999'
  expected.experimental.clash_api.external_ui = '/data/adb/modules/netproxy/webroot/zashboard'
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
  assert.deepEqual(config, expected)
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
  const ebpf = readFileSync(new URL('src/module/config/ebpf/ebpf.conf', root), 'utf8')
  const bypass = ebpf.match(/^EBPF_BYPASS_RULE_SET="([^"]*)"/m)[1].split(',')
  assert.deepEqual(bypass, upstream.inbounds.find(inbound => inbound.type === 'ebpf').bypass_rule_set)
  const tags = config.route.rule_set.flatMap(rule => list(rule.tag))
  for (const tag of bypass) assert.ok(tags.includes(tag), `${tag} 未在静态配置声明`)
})
