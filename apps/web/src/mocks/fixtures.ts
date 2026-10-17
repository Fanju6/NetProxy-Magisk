import type { GroupSummary, NodeSnapshot, ServiceStatus, Traffic } from '@/api/contracts'

const timestamp = '2026-10-15T12:00:00Z'
export function demoGroup(
  id: string,
  name: string,
  type: 'local' | 'subscription',
  nodeCount: number,
): GroupSummary {
  return {
    id,
    name,
    runtime_tag: name,
    type,
    active: id === 'demo-global',
    node_count: nodeCount,
    revision: 1,
    auto_update: type === 'subscription',
    update_interval: 86400,
    update_via_proxy: 'auto',
    usage: null,
    profile_title: '',
    profile_web_page_url: '',
    last_attempt_at: type === 'subscription' ? timestamp : '',
    last_success_at: type === 'subscription' ? timestamp : '',
    next_update_at: type === 'subscription' ? '2026-10-18T12:00:00Z' : '',
    last_error: '',
    updated_at: timestamp,
    progress: null,
  }
}
export function createFixtures(): { service: ServiceStatus; snapshot: NodeSnapshot } {
  const groups: NodeSnapshot['groups'] = [
    {
      group: {
        ...demoGroup('demo-global', 'Global Connect', 'subscription', 6),
        usage: {
          upload: 1073741824,
          download: 10737418240,
          total: 107374182400,
          expire: 1794657600,
        },
      },
      nodes: [
        { tag: 'Hong Kong 01', protocol: 'vless', server: 'hk-01.example.invalid', port: 443 },
        { tag: 'Tokyo 01', protocol: 'trojan', server: 'jp-01.example.invalid', port: 443 },
        { tag: 'Singapore 01', protocol: 'hysteria2', server: 'sg-01.example.invalid', port: 443 },
        { tag: 'Los Angeles 01', protocol: 'vless', server: 'us-01.example.invalid', port: 443 },
        {
          tag: 'Frankfurt 01',
          protocol: 'shadowsocks',
          server: 'de-01.example.invalid',
          port: 8388,
        },
        {
          tag: '🇬🇧 London / 伦敦 · 一个很长的演示节点名称',
          protocol: 'vless',
          server: 'uk-01.example.invalid',
          port: 443,
        },
      ],
    },
    {
      group: demoGroup('default', '本地配置', 'local', 1),
      nodes: [
        { tag: 'Local relay', protocol: 'socks', server: 'relay.example.invalid', port: 1080 },
      ],
    },
    {
      group: {
        ...demoGroup('demo-empty', '备用线路', 'subscription', 0),
        last_error: 'Subscription download failed',
        progress: {
          schema: 1,
          group_id: 'demo-empty',
          stage: 'download',
          message: 'Downloading',
          updated_at: timestamp,
        },
      },
      nodes: [],
    },
    { group: demoGroup('demo-local', 'Travel backup', 'local', 0), nodes: [] },
  ]
  const selection = {
    active_group_id: 'demo-global',
    active_group_name: 'Global Connect',
    active_group_runtime_tag: 'Global Connect',
    active_group_node_count: 6,
    selector_mode: 'urltest' as const,
    selected_node_ref: '',
    selected: 'Auto/Global Connect',
    runtime_selected: 'Global Connect/Hong Kong 01',
  }
  const snapshot: NodeSnapshot = {
    groups,
    selection,
    runtime_groups: [
      {
        tag: 'Auto/Global Connect',
        type: 'urltest',
        selectable: false,
        selected: 'Global Connect/Hong Kong 01',
        expanded: true,
        items: groups[0]!.nodes.map((node, index) => ({
          tag: `Global Connect/${node.tag}`,
          type: node.protocol,
          url_test_time: 1792065600,
          url_test_delay: [32, 68, 54, 156, 192, 201][index]!,
        })),
      },
    ],
  }
  const service: ServiceStatus = {
    state: 'ready',
    configured_backend: 'ebpf',
    active_backend: 'ebpf',
    pid: 2450,
    started_at: 1792065600,
    core_started_at_millis: 1792065600000,
    ready_at: 1792065601,
    uptime_seconds: 8246,
    error: '',
    outbound_mode: 'Rule',
    configured_outbound_mode: 'Rule',
    available_outbound_modes: ['Rule', 'Proxy', 'Direct', 'RuleAllowAds'],
    selector_mode: selection.selector_mode,
    active_group_id: selection.active_group_id,
    active_group_name: selection.active_group_name,
    active_group_runtime_tag: selection.active_group_runtime_tag,
    active_group_node_count: 6,
    selected_node_ref: '',
    runtime_selected: selection.runtime_selected,
    memory_bytes: 46 * 1024 * 1024,
    process_cpu_ticks: 420,
    system_cpu_ticks: 42000,
    cpu_count: 8,
    connections_in: 18,
    connections_out: 12,
    upload_total: 189240320,
    download_total: 2791728742,
    worker_state: 'running',
    worker_pid: 2400,
  }
  return { service, snapshot }
}
export function demoHistory(id: string) {
  return id === 'demo-empty'
    ? [
        {
          at: timestamp,
          ok: false,
          code: 'subscription.convert_failed',
          message: 'Download failed',
          duration_seconds: 15,
        },
      ]
    : [
        {
          at: '2026-10-14T12:00:00Z',
          ok: false,
          code: 'subscription.convert_failed',
          message: 'Download failed',
          duration_seconds: 12,
        },
        {
          at: timestamp,
          ok: true,
          code: 'subscription.updated',
          node_count: 6,
          revision: 1,
          duration_seconds: 2,
        },
        {
          at: timestamp,
          ok: true,
          code: 'subscription.runtime_sync_applied',
          message: 'Runtime applied',
        },
      ]
}
export function demoTraffic(service: ServiceStatus, tick = 0): Traffic {
  if (service.state !== 'ready') return { available: false }
  return {
    available: true,
    started_at_millis: service.core_started_at_millis,
    process_cpu_ticks: service.process_cpu_ticks + tick,
    system_cpu_ticks: service.system_cpu_ticks + tick * 100,
    cpu_count: service.cpu_count,
    status: {
      memory: service.memory_bytes,
      goroutines: 42,
      connections_in: service.connections_in,
      connections_out: service.connections_out,
      traffic_available: true,
      uplink: 245760 + (tick % 7) * 16384,
      downlink: 2195456 + (tick % 9) * 131072,
      uplink_total: service.upload_total + tick * 245760,
      downlink_total: service.download_total + tick * 2195456,
    },
  }
}
