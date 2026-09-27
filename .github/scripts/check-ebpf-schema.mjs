import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

export function checkEbpfSchema(schema) {
  const definitions = schema.$defs
  const branches = definitions?.Inbound?.oneOf?.filter(branch => branch.properties?.type?.const === 'ebpf') ?? []
  assert.equal(branches.length, 1, 'Inbound 必须且只能包含一个 eBPF 分支')
  const inbound = branches[0].properties
  assert.equal(Object.hasOwn(inbound, 'mode'), false, 'eBPF 不再使用 mode')
  for (const [field, name, required] of [
    ['local', 'EBPFLocalOptions', ['enabled', 'data_plane', 'cgroup_path', 'bypass_port', 'bypass_port_range', 'bypass_rule_set']],
    ['shared', 'EBPFSharedOptions', ['enabled', 'data_plane', 'interface', 'bypass_port', 'bypass_port_range', 'bypass_rule_set']],
  ]) {
    assert.equal(inbound[field]?.$ref, '#/$defs/' + name, 'eBPF 分支引用错误: ' + field)
    const properties = definitions[name]?.properties
    for (const key of required) {
      assert.ok(properties?.[key], name + ' 缺少字段: ' + key)
    }
  }
}

if (import.meta.main) checkEbpfSchema(JSON.parse(readFileSync(process.argv[2], 'utf8')))
