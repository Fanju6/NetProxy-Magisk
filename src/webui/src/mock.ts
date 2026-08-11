import { CONTRACT_SCHEMA, type CtlResult, type ExecResult } from './contract'

type MockGroup = {
  id: string
  name: string
  runtime_tag: string
  type: 'local' | 'subscription'
  active: boolean
  node_count: number
  revision: number
  auto_update: boolean
  update_interval: number
  update_via_proxy: string
  usage: null
  profile_title: string
  profile_web_page_url: string
  last_attempt_at: string
  last_success_at: string
  next_update_at: string
  last_error: string
  updated_at: string
  progress: null
}

const GROUPS: MockGroup[] = [
  {
    id: 'default',
    name: '本地配置',
    runtime_tag: '本地配置',
    type: 'local',
    active: true,
    node_count: 1,
    revision: 1,
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
  },
  {
    id: 'demo-sub',
    name: '示例订阅',
    runtime_tag: '示例订阅',
    type: 'subscription',
    active: false,
    node_count: 2,
    revision: 3,
    auto_update: true,
    update_interval: 86400,
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
  },
]

const NODE = { tag: 'demo-node', protocol: 'socks', server: 'example.test', port: 1080 }
let serviceState = 'stopped'
let outboundMode = 'rule'

function response<T>(code: string, message: string, data?: T): CtlResult<T> {
  return { schema: CONTRACT_SCHEMA, ok: true, code, message, ...(data === undefined ? {} : { data }) }
}

function failure(code: string, message: string): CtlResult<Record<string, never>> {
  return { schema: CONTRACT_SCHEMA, ok: false, code, message, data: {} }
}

function serviceStatus() {
  return {
    state: serviceState,
    pid: null,
    started_at: serviceState === 'ready' ? 1_700_000_000_000 : 0,
    ready_at: serviceState === 'ready' ? 1_700_000_005_000 : 0,
    uptime_seconds: serviceState === 'ready' ? 120 : 0,
    error: '',
    outbound_mode: outboundMode,
    configured_outbound_mode: outboundMode,
    selector_mode: 'urltest',
    active_group_id: 'default',
    active_group_name: '本地配置',
    active_group_node_count: 1,
    selected_node_ref: '',
    runtime_selected: 'Auto/本地配置',
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
    runtime_selected: 'Auto/本地配置',
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
    if (action === 'start' || action === 'restart') serviceState = 'ready'
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
    if (!action) return response('mode.current', '当前出站模式', { mode: outboundMode, available: ['rule', 'global', 'direct', 'AllowAds'] })
    outboundMode = action
    return response('mode.changed', '出站模式已切换', { mode: outboundMode })
  }

  if (command === 'app' && action === 'list') {
    return response('app.list', '分应用代理配置', { enabled: false, mode: 'blacklist', android_users: '', proxy_apps: '', bypass_apps: '' })
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
    return response('ebpf.status', 'eBPF 能力诊断', { mode: clean[2] || 'configured', summary: '开发预览 mock' })
  }

  if (command === 'config' && action === 'list') {
    return response('config.list', 'sing-box 配置列表', [])
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
