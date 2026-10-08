import { CONTRACT_SCHEMA, type CtlResult, type ExecResult, type InboundBackend, type ServiceBackendStatus } from './contract.ts'

const GROUP_DEFAULTS = {
  auto_update: false,
  update_interval: 0,
  update_via_proxy: 'auto',
  usage: null,
  profile_title: '',
  profile_web_page_url: '',
  last_attempt_at: '',
  last_success_at: '',
  next_update_at: '',
  last_error: '',
  updated_at: '',
  progress: null,
}

const GROUPS = [
  { ...GROUP_DEFAULTS, id: 'default', name: '本地配置', runtime_tag: '本地配置', type: 'local' as const, active: true, node_count: 1, revision: 1 },
  { ...GROUP_DEFAULTS, id: 'demo-sub', name: '示例订阅', runtime_tag: '示例订阅', type: 'subscription' as const, active: false, node_count: 2, revision: 3, auto_update: true, update_interval: 86400 },
]

const NODE = { tag: 'demo-node', protocol: 'socks', server: 'example.test', port: 1080 }
let serviceState = 'stopped'
const availableModes = ['AllowAds', 'Rule', 'Global', 'Direct']
let outboundMode = 'Rule'
let runtimePrepared = false

const INBOUND_CONFIG = {
  backend: 'ebpf' as InboundBackend,
  app: { enabled: true, mode: 'blacklist', proxy_apps: [] as string[], bypass_apps: [] as string[] },
  ebpf: {
    type: 'ebpf', tag: 'netproxy-in', network: ['tcp', 'udp'], udp_timeout: '5m', tc_priority: 1,
    local: {
      enabled: true, data_plane: 'cgroup', dns_mode: 'respect_policy', ipv6: true,
      bypass_private_address: true, bypass_rule_set: ['geoip/cn'],
    },
    shared: {
      enabled: false, data_plane: 'packet_rewrite', dns_mode: 'hijack', interface: ['wlan2'],
      ipv6: true, bypass_private_address: true, bypass_rule_set: ['geoip/cn'],
    },
  },
  tun: {
    type: 'tun', tag: 'netproxy-in', interface_name: 'netproxy',
    address: ['172.19.0.1/30', 'fdfe:dcba:9876::1/126'],
    auto_route: true, auto_redirect: true, dns_mode: 'hijack',
  },
}

const STATIC_CONFIG: Record<string, unknown> = {
  log: { level: 'info' },
  dns: { final: 'dns-proxy' },
  inbounds: [],
  route: { final: 'Proxy' },
  experimental: {},
  http_clients: [],
  services: [],
}

const CONFIG_DOCUMENTS = [
  { id: 'inbound', filename: 'inbound.json', category: 'inbound', editable: true },
  ...['backend', 'app', 'ebpf', 'tun'].map(section => ({
    id: `inbound/${section}`, filename: section, category: 'inbound', editable: true, section,
  })),
  { id: 'singbox/config.json', filename: 'config.json', category: 'config', editable: true },
  ...[...Object.keys(STATIC_CONFIG), 'outbounds'].map(section => ({
    id: `singbox/${section}`, filename: section, category: 'config', editable: true, section,
  })),
  ...['inbound.json', 'providers.json', 'outbounds.json'].map(filename => ({
    id: `runtime/${filename}`, filename, category: 'runtime', editable: false,
  })),
]

function response<T>(code: string, message: string, data?: T): CtlResult<T> {
  return { schema: CONTRACT_SCHEMA, ok: true, code, message, ...(data === undefined ? {} : { data }) }
}

function failure(code: string, message: string): CtlResult<Record<string, never>> {
  return { schema: CONTRACT_SCHEMA, ok: false, code, message, data: {} }
}

function serviceStatus() {
  const backendStatus: ServiceBackendStatus = {
    configured_backend: INBOUND_CONFIG.backend,
    active_backend: serviceState === 'ready' ? INBOUND_CONFIG.backend : null,
  }
  return {
    state: serviceState,
    pid: serviceState === 'ready' ? 4242 : null,
    started_at: serviceState === 'ready' ? 1_700_000_000 : 0,
    ready_at: serviceState === 'ready' ? 1_700_000_005 : 0,
    uptime_seconds: serviceState === 'ready' ? 120 : 0,
    error: '',
    outbound_mode: outboundMode,
    configured_outbound_mode: outboundMode,
    available_outbound_modes: availableModes,
    ...backendStatus,
    selector_mode: 'urltest',
    active_group_id: 'default',
    active_group_name: '本地配置',
    active_group_runtime_tag: '本地配置',
    active_group_node_count: 1,
    selected_node_ref: '',
    runtime_selected: serviceState === 'ready' ? '本地配置/demo-node' : '',
    memory_bytes: 0,
    process_cpu_ticks: 0,
    system_cpu_ticks: 0,
    cpu_count: 1,
    connections_in: 0,
    connections_out: 0,
    upload_total: 0,
    download_total: 0,
    worker_state: 'stopped',
    worker_pid: null,
  }
}

function snapshot() {
  return { group: GROUPS[0], nodes: [NODE] }
}

function selection() {
  return {
    active_group_id: 'default',
    active_group_name: '本地配置',
    active_group_runtime_tag: '本地配置',
    active_group_node_count: 1,
    selector_mode: 'urltest',
    selected_node_ref: '',
    selected: 'Auto/本地配置',
    runtime_selected: serviceState === 'ready' ? '本地配置/demo-node' : '',
  }
}

function normalizeArgs(args: string[]): string[] {
  const result: string[] = []
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index]
    if (argument === '--json') continue
    if (argument === '--timeout') {
      index += 1
      continue
    }
    if (argument.startsWith('--timeout=')) continue
    result.push(argument)
  }
  return result
}

function execute(args: string[]): CtlResult<unknown> {
  const clean = normalizeArgs(args)
  const command = clean[0]
  const action = clean[1]

  if (command === 'service') {
    if (action === 'start' || action === 'restart') {
      serviceState = 'ready'
      runtimePrepared = true
    }
    if (action === 'stop') serviceState = 'stopped'
    if (action === 'status') return response('service.status', '服务状态', serviceStatus())
    return response(`service.${action || 'status'}`, '服务操作完成', { action, status: serviceStatus() })
  }

  if (command === 'catalog') {
    if (action === 'list') return response('catalog.groups', 'Catalog 分组快照', GROUPS)
    if (action === 'show') return response('catalog.show', 'Catalog 分组快照', snapshot())
  }

  if (command === 'sub') {
    if (action === 'list') return response('subscription.list', '订阅列表', GROUPS.filter(group => group.type === 'subscription'))
    return response(`subscription.${action || 'list'}`, '订阅操作完成', {})
  }

  if (command === 'node') {
    if (action === 'list') return response('node.list', '节点列表', [snapshot()])
    if (action === 'show') return response('catalog.show', 'Catalog 分组快照', snapshot())
    if (action === 'snapshot') return response('node.snapshot', '节点快照', { groups: [snapshot()], selection: selection() })
    if (action === 'current') return response('node.current', '当前节点选择', selection())
    return response(`node.${action || 'list'}`, '节点操作完成', {})
  }

  if (command === 'mode') {
    if (!action) return response('mode.current', '当前出站模式', { mode: outboundMode, available: availableModes })
    if (!availableModes.includes(action)) return failure('mode.invalid', '配置中不存在出站模式: ' + action)
    outboundMode = action
    return response('mode.changed', '出站模式已切换', { mode: outboundMode })
  }

  if (command === 'app' && action === 'list') {
    return response('app.list', '分应用代理配置', {
      enabled: INBOUND_CONFIG.app.enabled, mode: INBOUND_CONFIG.app.mode,
      proxy_apps: INBOUND_CONFIG.app.proxy_apps.join(','), bypass_apps: INBOUND_CONFIG.app.bypass_apps.join(','),
    })
  }

  if (command === 'network' && action === 'evaluate') {
    return response('network.evaluated', 'Wi-Fi 自动切换未启用', {
      enabled: false,
      network_type: 'not_wifi',
      target: 'proxying',
      desired_mode: outboundMode,
      runtime_mode: outboundMode,
      changed: false,
      reason: 'Wi-Fi 自动切换未启用',
    })
  }

  if (command === 'ebpf' && action === 'status') {
    const requestedMode = clean.slice(2).find(arg => arg !== '--raw') || 'configured'
    const raw = clean.includes('--raw')
    const reportMode = requestedMode === 'configured' ? 'local' : requestedMode
    const localDataPlane = reportMode === 'local' || reportMode === 'all' ? 'cgroup' : undefined
    const sharedDataPlane = reportMode === 'shared' || reportMode === 'all' ? 'packet_rewrite' : undefined
    const report = {
      platform: 'android',
      kernel_release: 'mock',
      architecture: 'arm64',
      mode: reportMode,
      ...(localDataPlane ? { local_data_plane: localDataPlane } : {}),
      ...(sharedDataPlane ? { shared_data_plane: sharedDataPlane } : {}),
      network: ['tcp', 'udp'],
      ipv6: true,
      findings: [],
      preflight: true,
      exact_object_load: true,
      summary: { pass: 1, warn: 0, fail: 0, unknown: 0, required_failures: 0, required_unknowns: 0, required_issues: 0 },
      result: 'preflight_passed',
    }
    return response('ebpf.status', 'eBPF 能力检查完成', {
      mode: requestedMode,
      raw,
      content: raw ? JSON.stringify(report, null, 2) : '结论: eBPF 能力预检通过\n尚未验证实际挂载与网络接管。',
      report,
    })
  }

  if (command === 'config' && action === 'list') {
    const documents = runtimePrepared ? CONFIG_DOCUMENTS : CONFIG_DOCUMENTS.filter(item => item.category !== 'runtime')
    return response('config.list', '配置列表', documents)
  }
  if (command === 'config' && action === 'read') {
    const document = CONFIG_DOCUMENTS.find(item => item.id === clean[2])
    if (!document) return failure('command.failed', '不支持的配置目标')
    if (document.category === 'runtime' && !runtimePrepared) return failure('command.failed', '尚未生成运行时配置')
    let content: unknown = {}
    if (document.id === 'inbound') content = INBOUND_CONFIG
    else if (document.category === 'inbound') content = { [document.filename]: INBOUND_CONFIG[document.filename as keyof typeof INBOUND_CONFIG] }
    else if (document.id === 'runtime/inbound.json') content = { inbounds: [{ ...INBOUND_CONFIG.ebpf, shared: { enabled: false } }] }
    else if (document.id === 'runtime/providers.json') content = { providers: [] }
    else if (document.id === 'runtime/outbounds.json') content = { outbounds: [] }
    else if (document.id === 'singbox/config.json') content = STATIC_CONFIG
    else if (Object.prototype.hasOwnProperty.call(STATIC_CONFIG, document.filename)) content = { [document.filename]: STATIC_CONFIG[document.filename] }
    return response('config.read', '配置内容', {
      target: document.id, content: JSON.stringify(content, null, 2), revision: `mock-${document.filename}-1`,
    })
  }

  if (command === 'logs' && action === 'show') {
    return response('logs.show', '日志内容', { kind: clean[2] || 'service', content: '开发预览 mock 日志\n' })
  }

  return failure('usage.invalid', '开发预览不支持此命令')
}

export function mockCtl(args: string[]): ExecResult {
  const result = execute(args)
  return { out: `${JSON.stringify(result)}\n`, err: '', code: result.ok ? 0 : 2 }
}
