import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

export function checkEbpfSchema(schema) {
  const definitions = schema.$defs
  const branches = definitions?.Inbound?.oneOf?.filter(branch => branch.properties?.type?.const === 'ebpf') ?? []
  assert.equal(branches.length, 1, 'Inbound 必须且只能包含一个 eBPF 分支')
  const inbound = branches[0].properties
  assert.equal(Object.hasOwn(inbound, 'mode'), false, 'eBPF 不再使用 mode')
  for (const key of ['tag', 'network', 'udp_timeout', 'tc_priority', 'local', 'shared']) {
    assert.ok(inbound[key], 'eBPF 缺少字段: ' + key)
  }
  for (const [field, name, required] of [
    ['local', 'EBPFLocalOptions', [
      'enabled', 'data_plane', 'cgroup_path', 'dns_mode', 'ipv6', 'bypass_private_address',
      'bypass_port', 'bypass_port_range', 'bypass_rule_set', 'bypass_exclude',
      'include_uid', 'include_uid_range', 'exclude_uid', 'exclude_uid_range',
      'include_android_user', 'include_package', 'exclude_package',
    ]],
    ['shared', 'EBPFSharedOptions', [
      'enabled', 'data_plane', 'interface', 'dns_mode', 'ipv6', 'bypass_private_address',
      'bypass_port', 'bypass_port_range', 'bypass_rule_set', 'bypass_exclude',
      'include_source_cidr', 'exclude_source_cidr', 'include_mac_address', 'exclude_mac_address',
    ]],
  ]) {
    assert.equal(inbound[field]?.$ref, '#/$defs/' + name, 'eBPF 分支引用错误: ' + field)
    const properties = definitions[name]?.properties
    for (const key of required) {
      assert.ok(properties?.[key], name + ' 缺少字段: ' + key)
    }
    assert.equal(properties.enabled.type, 'boolean', name + ' enabled 必须是布尔值')
    assert.deepEqual(properties.data_plane.enum.toSorted(),
      (field === 'local' ? ['cgroup', 'tc'] : ['packet_rewrite', 'socket_assign']), name + ' 数据平面不匹配')
    assert.deepEqual(properties.dns_mode.enum.toSorted(), ['hijack', 'off', 'respect_policy'], name + ' DNS 模式不匹配')
  }
  const tunBranches = definitions?.Inbound?.oneOf?.filter(branch => branch.properties?.type?.const === 'tun') ?? []
  assert.equal(tunBranches.length, 1, 'Inbound 必须且只能包含一个 TUN 分支')
  const tun = tunBranches[0].properties
  for (const key of [
    'tag', 'interface_name', 'address', 'dns_mode', 'dns_address', 'auto_route', 'auto_redirect',
    'mtu', 'strict_route', 'udp_timeout', 'route_address', 'route_address_set',
    'route_exclude_address', 'route_exclude_address_set', 'include_interface', 'exclude_interface',
    'include_uid', 'include_uid_range', 'exclude_uid', 'exclude_uid_range',
    'include_android_user', 'include_package', 'exclude_package', 'include_mac_address', 'exclude_mac_address',
    'auto_redirect_input_mark', 'auto_redirect_output_mark', 'auto_redirect_reset_mark',
    'auto_redirect_tproxy_mark', 'auto_redirect_nfqueue', 'iproute2_table_index', 'iproute2_rule_index',
  ]) {
    assert.ok(tun[key], 'TUN 缺少字段: ' + key)
  }
  assert.deepEqual(tun.dns_mode.enum.toSorted(), ['disabled', 'hijack', 'native'], 'TUN DNS 模式不匹配')
  assert.equal(tun.auto_route.type, 'boolean', 'TUN auto_route 必须是布尔值')
  assert.equal(tun.auto_redirect.type, 'boolean', 'TUN auto_redirect 必须是布尔值')
  assert.equal(Object.hasOwn(tun, 'stack'), false, '当前内核不支持 TUN stack')
}

if (import.meta.main) checkEbpfSchema(JSON.parse(readFileSync(process.argv[2], 'utf8')))
