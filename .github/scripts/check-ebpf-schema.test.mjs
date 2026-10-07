import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { checkEbpfSchema } from './check-ebpf-schema.mjs'

const source = JSON.parse(readFileSync(new URL('../../src/android/app/src/main/assets/sing-box.schema.json', import.meta.url)))
test('校验真正的 eBPF/TUN 分支，缺失、重复和旧结构均失败', () => {
  checkEbpfSchema(source)
  for (const mutate of [
    schema => { delete schema.$defs.Inbound },
    schema => { schema.$defs.Inbound.oneOf = [] },
    schema => { schema.$defs.Inbound.oneOf.push(schema.$defs.Inbound.oneOf.find(branch => branch.properties?.type?.const === 'ebpf')) },
    schema => { schema.$defs.Inbound.oneOf.find(branch => branch.properties?.type?.const === 'ebpf').properties.mode = {} },
    schema => { delete schema.$defs.EBPFLocalOptions.properties.data_plane },
    schema => { schema.$defs.Inbound.oneOf.find(branch => branch.properties?.type?.const === 'ebpf').properties.shared.$ref = '#/$defs/Missing' },
    schema => { schema.$defs.EBPFLocalOptions.properties.dns_mode.enum = ['hijack', 'off'] },
    schema => { schema.$defs.EBPFSharedOptions.properties.enabled.type = 'integer' },
    schema => { schema.$defs.Inbound.oneOf = schema.$defs.Inbound.oneOf.filter(branch => branch.properties?.type?.const !== 'tun') },
    schema => { schema.$defs.Inbound.oneOf.push(schema.$defs.Inbound.oneOf.find(branch => branch.properties?.type?.const === 'tun')) },
    schema => { schema.$defs.Inbound.oneOf.find(branch => branch.properties?.type?.const === 'tun').properties.dns_mode.enum = ['off', 'hijack'] },
    schema => { schema.$defs.Inbound.oneOf.find(branch => branch.properties?.type?.const === 'tun').properties.auto_redirect.type = 'string' },
    schema => { schema.$defs.Inbound.oneOf.find(branch => branch.properties?.type?.const === 'tun').properties.stack = {} },
  ]) {
    const schema = structuredClone(source)
    mutate(schema)
    assert.throws(() => checkEbpfSchema(schema))
  }
})

test('删除实际入站分支依赖的任一原生字段都会阻止资源更新', () => {
  for (const backend of ['ebpf', 'tun']) {
    const branch = source.$defs.Inbound.oneOf.find(branch => branch.properties?.type?.const === backend)
    for (const key of Object.keys(branch.properties).filter(key => !['type', 'udp_fragment', 'fakeip_icmp',
      'netns', 'auto_redirect_disable_mark_mode', 'auto_redirect_iproute2_fallback_rule_index',
      'exclude_mptcp', 'loopback_address', 'udp_mapping', 'udp_filtering', 'udp_nat_max', 'multi_queue', 'platform'].includes(key))) {
      const schema = structuredClone(source)
      delete schema.$defs.Inbound.oneOf.find(branch => branch.properties?.type?.const === backend).properties[key]
      assert.throws(() => checkEbpfSchema(schema), undefined, `${backend}.${key}`)
    }
  }
  for (const name of ['EBPFLocalOptions', 'EBPFSharedOptions']) {
    for (const key of Object.keys(source.$defs[name].properties)) {
      const schema = structuredClone(source)
      delete schema.$defs[name].properties[key]
      assert.throws(() => checkEbpfSchema(schema), undefined, `${name}.${key}`)
    }
  }
})
