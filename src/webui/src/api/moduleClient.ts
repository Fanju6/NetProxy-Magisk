import { execAsync, isKsuEnv, writeFileContent } from '../utils/ksu';

const NETPROXYCTL = '/data/adb/modules/netproxy/netproxyctl';

export interface ModuleEnvelope<T> {
  schema: 1;
  ok: boolean;
  code: string;
  message: string;
  data: T;
}

export interface SubscriptionUsage {
  upload?: number;
  download?: number;
  total?: number;
  expire?: number;
}

export interface SubscriptionProgress {
  group_id: string;
  stage: string;
  message: string;
  updated_at: string;
}

export interface CatalogGroup {
  id: string;
  name: string;
  type: 'local' | 'subscription';
  active: boolean;
  node_count: number;
  revision: number;
  auto_update: boolean;
  update_interval: number;
  update_via_proxy: 'auto' | 'always' | 'never';
  usage: SubscriptionUsage | null;
  profile_title: string;
  profile_web_page_url: string;
  last_attempt_at: string;
  last_success_at: string;
  next_update_at: string;
  last_error: string;
  updated_at: string;
  progress: SubscriptionProgress | null;
}

export interface CatalogNode {
  tag: string;
  protocol: string;
  server: string;
  port: number;
}

export interface CatalogNodeGroup {
  group: CatalogGroup;
  nodes: CatalogNode[];
}

export interface NodeSelection {
  active_group_id: string;
  selector_mode: 'urltest' | 'manual';
  selected: string;
}

export interface ServiceStatus {
  state: 'stopped' | 'preparing' | 'starting' | 'ready' | 'stopping' | 'failed';
  pid: number | null;
  started_at: number;
  ready_at: number;
  uptime_seconds: number;
  error: string;
  outbound_mode: 'rule' | 'global' | 'direct';
  selector_mode: 'urltest' | 'manual';
  active_group_id: string;
  active_group_node_count: number;
  selected_node_ref: string;
  runtime_selected: string;
  subscription_worker: string;
  subscription_worker_pid: number | null;
}

export interface SubscriptionEditor {
  id: string;
  name: string;
  url: string;
  user_agent: string;
  hwid: string;
  custom_headers: Record<string, string>;
  auto_update: boolean;
  update_interval: number;
  interval_source: string;
  update_via_proxy: 'auto' | 'always' | 'never';
  include: string;
  exclude: string;
  allow_insecure: boolean;
  timeout: number;
}

export interface SubscriptionDraft {
  name: string;
  url: string;
  userAgent?: string;
  hwid?: string;
  customHeaders?: Record<string, string>;
  autoUpdate?: boolean;
  updateInterval?: number;
  updateViaProxy?: 'auto' | 'always' | 'never';
  include?: string;
  exclude?: string;
  allowInsecure?: boolean;
  timeout?: number;
}

export interface SubscriptionHistoryEntry {
  time: string;
  status: string;
  message: string;
  node_count?: number;
  revision?: number;
}

export interface AppProxyState {
  enabled: boolean;
  mode: 'blacklist' | 'whitelist';
  proxy_apps: string;
  bypass_apps: string;
}

export interface ApiBootstrap {
  service_api: { url: string; secret: string };
  clash_api: { url: string; secret: string };
}

export interface TrafficSnapshot {
  upload: number;
  download: number;
}

export type ConfigTarget = 'module' | 'ebpf' | `singbox/${string}`;
export type LogKind = 'service' | 'core' | 'sub';

export interface ConfigContent {
  target: ConfigTarget;
  content: string;
}

export interface LogContent {
  kind: LogKind;
  content: string;
}

export class ModuleClientError extends Error {
  readonly code: string;
  readonly data: unknown;

  constructor(
    code: string,
    message: string,
    data: unknown = {}
  ) {
    super(message);
    this.name = 'ModuleClientError';
    this.code = code;
    this.data = data;
  }
}

const shellQuote = (value: string): string => `'${value.replace(/'/g, `'"'"'`)}'`;

const parseEnvelope = <T>(stdout: string): ModuleEnvelope<T> => {
  const line = stdout
    .split(/\r?\n/)
    .reverse()
    .map(value => value.trim())
    .find(value => value.startsWith('{') && value.endsWith('}'));
  if (!line) throw new ModuleClientError('transport.invalid_output', '模块没有返回有效数据');

  let envelope: ModuleEnvelope<T>;
  try {
    envelope = JSON.parse(line) as ModuleEnvelope<T>;
  } catch {
    throw new ModuleClientError('transport.invalid_json', '模块返回的数据格式无效');
  }
  if (envelope.schema !== 1) {
    throw new ModuleClientError('transport.unsupported_schema', '模块管理接口版本不受支持');
  }
  if (!envelope.ok) {
    throw new ModuleClientError(envelope.code || 'command.failed', envelope.message || '模块操作失败', envelope.data);
  }
  return envelope;
};

const mockGroup = (): CatalogNodeGroup => ({
  group: {
    id: 'default', name: '本地配置', type: 'local', active: true, node_count: 2,
    revision: 2, auto_update: false, update_interval: 86400, update_via_proxy: 'auto',
    usage: null, profile_title: '', profile_web_page_url: '', last_attempt_at: '',
    last_success_at: '', next_update_at: '', last_error: '',
    updated_at: new Date().toISOString(), progress: null
  },
  nodes: [
    { tag: 'Tokyo', protocol: 'vless', server: 'example.com', port: 443 },
    { tag: 'Singapore', protocol: 'hysteria2', server: 'example.net', port: 443 }
  ]
});

const mockSubscriptionGroup = (): CatalogNodeGroup => ({
  group: {
    id: 'sub-demo', name: '演示订阅', type: 'subscription', active: false, node_count: 3,
    revision: 8, auto_update: true, update_interval: 86400, update_via_proxy: 'auto',
    usage: { upload: 1_073_741_824, download: 18_253_611_008, total: 107_374_182_400, expire: 1_799_000_000 },
    profile_title: '演示订阅', profile_web_page_url: '', last_attempt_at: new Date().toISOString(),
    last_success_at: new Date().toISOString(), next_update_at: new Date(Date.now() + 86400000).toISOString(),
    last_error: '', updated_at: new Date().toISOString(), progress: null
  },
  nodes: [
    { tag: 'Hong Kong', protocol: 'vless', server: 'hk.example.com', port: 443 },
    { tag: 'Japan', protocol: 'anytls', server: 'jp.example.com', port: 443 },
    { tag: 'United States', protocol: 'hysteria2', server: 'us.example.com', port: 8443 }
  ]
});

const mockConfigDefaults: Record<'module' | 'ebpf', string> = {
  module: [
    'AUTO_START=0',
    'GMS_FIX=0',
    'WIFI_AUTO_SWITCH=0',
    'WIFI_SSID_MODE="blacklist"',
    'WIFI_SSID_LIST=""',
    'PROXY_ON_CELLULAR=1',
    'WIFI_INTERFACE="wlan0"'
  ].join('\n'),
  ebpf: [
    'EBPF_NETWORK=""',
    'EBPF_UDP_TIMEOUT="5m"',
    'EBPF_DNS_MODE="hijack"',
    'EBPF_CGROUP_PATH=""',
    'EBPF_IPV6=1',
    'EBPF_BYPASS_RULE_SETS="direct ChinaIP"',
    'EBPF_SHARED_NETWORK=0',
    'EBPF_SHARED_INTERFACES="wlan2"',
    'EBPF_TCP_MAP_CAPACITY=65536',
    'EBPF_UDP_MAP_CAPACITY=65536',
    'EBPF_SOCKET_MAP_CAPACITY=65536',
    'EBPF_SHARED_MAP_CAPACITY=65536'
  ].join('\n')
};

const mockLogs: Record<LogKind, string> = {
  service: [
    '[2026-08-04 16:00:00] [INFO] NetProxy 服务启动',
    '[2026-08-04 16:00:01] [INFO] Catalog Provider 已加载',
    '[2026-08-04 16:00:02] [INFO] eBPF 入站已就绪'
  ].join('\n'),
  sub: '[2026-08-04 16:05:00] [INFO] 演示订阅更新完成，共 3 个节点',
  core: [
    '2026-08-04 16:00:01 INFO inbound/ebpf[ebpf-in]: started',
    '2026-08-04 16:00:02 INFO outbound/provider[default]: loaded 2 outbounds'
  ].join('\n')
};

const mockCall = <T>(args: string[]): T => {
  const command = args.join(' ');
  if (command === 'catalog list') return [mockGroup().group, mockSubscriptionGroup().group] as T;
  if (command.startsWith('catalog show')) {
    return (command.includes('sub-demo') ? mockSubscriptionGroup() : mockGroup()) as T;
  }
  if (command.startsWith('node list')) return [mockGroup(), mockSubscriptionGroup()] as T;
  if (command === 'node current') {
    return { active_group_id: 'default', selector_mode: 'urltest', selected: 'Auto/default' } as T;
  }
  if (command === 'sub list') return [mockSubscriptionGroup().group] as T;
  if (command.startsWith('sub history')) {
    return [{ time: new Date().toISOString(), status: 'success', message: '订阅更新完成', node_count: 3, revision: 8 }] as T;
  }
  if (command.startsWith('sub show') && command.includes('--private')) {
    return {
      id: 'sub-demo', name: '演示订阅', url: 'https://example.com/sub', user_agent: '', hwid: '',
      custom_headers: {}, auto_update: true, update_interval: 86400, interval_source: 'default',
      update_via_proxy: 'auto', include: '', exclude: '', allow_insecure: false, timeout: 60
    } as T;
  }
  if (command === 'service status') {
    return {
      state: 'ready', pid: 1234, started_at: 0, ready_at: Math.floor(Date.now() / 1000) - 120,
      uptime_seconds: 120, error: '', outbound_mode: 'rule', selector_mode: 'urltest',
      active_group_id: 'default', active_group_node_count: 2, selected_node_ref: '', runtime_selected: 'Tokyo',
      subscription_worker: 'running', subscription_worker_pid: 1235
    } as T;
  }
  if (command === 'app list') {
    return { enabled: true, mode: 'blacklist', proxy_apps: '', bypass_apps: '' } as T;
  }
  if (command === 'api bootstrap') {
    return {
      service_api: { url: 'http://127.0.0.1:9090', secret: 'mock' },
      clash_api: { url: 'http://127.0.0.1:9999', secret: 'mock' }
    } as T;
  }
  return {} as T;
};

class ModuleClient {
  private bootstrap: Promise<ApiBootstrap> | null = null;

  async call<T>(args: string[], options: { detach?: boolean } = {}): Promise<T> {
    if (!isKsuEnv()) return mockCall<T>(args);
    const raw = [NETPROXYCTL, '--json', ...args].map(shellQuote).join(' ');
    const command = options.detach ? `su -c ${shellQuote(raw)}` : raw;
    const result = await execAsync(command);
    let envelope: ModuleEnvelope<T>;
    try {
      envelope = parseEnvelope<T>(result.stdout);
    } catch (error) {
      if (result.errno !== 0 && result.stderr.trim()) {
        throw new ModuleClientError('command.failed', result.stderr.trim());
      }
      throw error;
    }
    if (result.errno !== 0) {
      throw new ModuleClientError(envelope.code || 'command.failed', envelope.message || result.stderr);
    }
    return envelope.data;
  }

  listCatalog = () => this.call<CatalogGroup[]>(['catalog', 'list']);
  showCatalog = (id: string) => this.call<CatalogNodeGroup>(['catalog', 'show', id]);
  listNodes = (groupId?: string) => this.call<CatalogNodeGroup[]>(
    ['node', 'list', ...(groupId ? [groupId] : [])]
  );
  currentNode = () => this.call<NodeSelection>(['node', 'current']);
  selectAuto = (groupId: string) => this.call(['node', 'use', 'auto', groupId]);
  selectNode = (groupId: string, tag: string) => this.call(['node', 'use', `${groupId}/${tag}`]);
  addNode = (link: string) => this.call(['node', 'add', link]);
  copyNode = (ref: string) => this.call(['node', 'copy', ref, 'default']);
  removeNode = (ref: string) => this.call(['node', 'remove', ref]);
  testDelay = (target = '') => this.call(['node', 'delay', ...(target ? [target] : [])]);

  listSubscriptions = () => this.call<CatalogGroup[]>(['sub', 'list']);
  showSubscription = (id: string) => this.call<CatalogGroup>(['sub', 'show', id]);
  editSubscriptionData = (id: string) => this.call<SubscriptionEditor>(['sub', 'show', id, '--private']);
  subscriptionHistory = (id: string) => this.call<SubscriptionHistoryEntry[]>(['sub', 'history', id]);
  updateSubscription = (id: string) => this.call(['sub', 'update', id]);
  updateAllSubscriptions = () => this.call(['sub', 'update-all']);
  activateSubscription = (id: string) => this.call(['sub', 'activate', id]);
  removeSubscription = (id: string, replacement = '') => this.call([
    'sub', 'remove', id, ...(replacement ? [replacement] : [])
  ]);
  cancelSubscription = (id: string) => this.call(['sub', 'cancel', id]);

  serviceStatus = () => this.call<ServiceStatus>(['service', 'status']);
  serviceAction = (action: 'start' | 'stop' | 'restart' | 'reload') =>
    this.call(['service', action], { detach: action === 'start' || action === 'restart' });
  setMode = (mode: 'rule' | 'global' | 'direct') => this.call(['mode', mode]);
  apiBootstrap = () => {
    if (!this.bootstrap) {
      this.bootstrap = this.call<ApiBootstrap>(['api', 'bootstrap'])
        .catch(error => {
          this.bootstrap = null;
          throw error;
        });
    }
    return this.bootstrap;
  };
  appState = () => this.call<AppProxyState>(['app', 'list']);
  setAppMode = (mode: 'blacklist' | 'whitelist') => this.call(['app', 'mode', mode]);
  setAppsEnabled = (enabled: boolean) => this.call(['app', enabled ? 'enable' : 'disable']);
  addApp = (id: string) => this.call(['app', 'add', id]);
  removeApp = (id: string) => this.call(['app', 'remove', id]);

  async readConfig(target: ConfigTarget): Promise<string> {
    if (!isKsuEnv()) {
      return localStorage.getItem(`mock_config_${target}`)
        ?? mockConfigDefaults[target as 'module' | 'ebpf']
        ?? '';
    }
    const data = await this.call<ConfigContent>(['config', 'read', target]);
    return data.content;
  }

  async applyConfig(target: ConfigTarget, content: string): Promise<void> {
    if (!isKsuEnv()) {
      localStorage.setItem(`mock_config_${target}`, content);
      return;
    }
    const path = `/data/local/tmp/netproxy-config-${Date.now()}-${Math.random().toString(16).slice(2)}.tmp`;
    await writeFileContent(path, content);
    try {
      await this.call(['config', 'apply', target, path]);
    } finally {
      await execAsync(`rm -f ${shellQuote(path)}`);
    }
  }

  async readLogs(kind: LogKind, lines = 200): Promise<string> {
    if (!isKsuEnv()) return mockLogs[kind];
    const data = await this.call<LogContent>(['logs', 'show', kind, String(lines)]);
    return data.content;
  }

  async clearLogs(kind: LogKind): Promise<void> {
    if (!isKsuEnv()) {
      mockLogs[kind] = '';
      return;
    }
    await this.call(['logs', 'clear', kind]);
  }

  async addSubscription(draft: SubscriptionDraft): Promise<void> {
    await this.withHeadersFile(draft.customHeaders, async headersFile => {
      const args = ['sub', 'add', draft.name, draft.url];
      this.appendSubscriptionOptions(args, draft, headersFile, true);
      await this.call(args);
    });
  }

  async editSubscription(
    id: string,
    draft: SubscriptionDraft,
    original?: SubscriptionEditor
  ): Promise<void> {
    await this.withHeadersFile(draft.customHeaders, async headersFile => {
      const args = ['sub', 'edit', id];
      const add = (changed: boolean, key: string, value: string) => {
        if (changed) args.push(key, value);
      };
      add(!original || original.name !== draft.name, '--name', draft.name);
      add(!original || original.url !== draft.url, '--url', draft.url);
      add(!original || original.user_agent !== (draft.userAgent ?? ''), '--user-agent', draft.userAgent ?? '');
      add(!original || original.hwid !== (draft.hwid ?? ''), '--hwid', draft.hwid ?? '');
      add(!original || original.update_interval !== (draft.updateInterval ?? 86400), '--interval', String(draft.updateInterval ?? 86400));
      add(!original || original.update_via_proxy !== (draft.updateViaProxy ?? 'auto'), '--via-proxy', draft.updateViaProxy ?? 'auto');
      add(!original || original.include !== (draft.include ?? ''), '--include', draft.include ?? '');
      add(!original || original.exclude !== (draft.exclude ?? ''), '--exclude', draft.exclude ?? '');
      add(!original || original.allow_insecure !== (draft.allowInsecure ?? false), '--allow-insecure', String(draft.allowInsecure ?? false));
      add(!original || original.timeout !== (draft.timeout ?? 60), '--timeout', String(draft.timeout ?? 60));
      add(!original || original.auto_update !== (draft.autoUpdate ?? true), '--auto-update', String(draft.autoUpdate ?? true));
      const originalHeaders = original?.custom_headers ?? {};
      if (JSON.stringify(originalHeaders) !== JSON.stringify(draft.customHeaders ?? {}) && headersFile) {
        args.push('--headers-file', headersFile);
      }
      if (args.length > 3) await this.call(args);
    }, true);
  }

  async trafficSnapshot(): Promise<TrafficSnapshot> {
    if (!isKsuEnv()) return { upload: 12_582_912, download: 98_713_600 };
    const bootstrap = await this.apiBootstrap();
    const response = await fetch(`${bootstrap.clash_api.url}/connections`, {
      headers: { Authorization: `Bearer ${bootstrap.clash_api.secret}` },
      cache: 'no-store'
    });
    if (!response.ok) {
      throw new ModuleClientError('clash.request_failed', `控制接口返回 ${response.status}`);
    }
    const payload = await response.json() as { uploadTotal?: number; downloadTotal?: number };
    return {
      upload: payload.uploadTotal ?? 0,
      download: payload.downloadTotal ?? 0
    };
  }

  async importContent(content: string, groupName = ''): Promise<void> {
    const path = `/data/local/tmp/netproxy-import-${Date.now()}.txt`;
    await writeFileContent(path, content);
    try {
      await this.call(['node', 'import', path, ...(groupName ? [groupName] : [])]);
    } finally {
      if (isKsuEnv()) await execAsync(`rm -f ${shellQuote(path)}`);
    }
  }

  private appendSubscriptionOptions(
    args: string[],
    draft: SubscriptionDraft,
    headersFile: string | null,
    addMode: boolean
  ) {
    if (draft.userAgent) args.push('--user-agent', draft.userAgent);
    if (draft.hwid) args.push('--hwid', draft.hwid);
    if (headersFile) args.push('--headers-file', headersFile);
    args.push('--interval', String(draft.updateInterval ?? 86400));
    args.push('--via-proxy', draft.updateViaProxy ?? 'auto');
    if (draft.include) args.push('--include', draft.include);
    if (draft.exclude) args.push('--exclude', draft.exclude);
    if (draft.allowInsecure) args.push('--allow-insecure');
    else if (!addMode) args.push('--allow-insecure', 'false');
    args.push('--timeout', String(draft.timeout ?? 60));
    args.push('--auto-update', String(draft.autoUpdate ?? true));
  }

  private async withHeadersFile<T>(
    headers: Record<string, string> | undefined,
    block: (path: string | null) => Promise<T>,
    always = false
  ): Promise<T> {
    if (!always && (!headers || Object.keys(headers).length === 0)) return block(null);
    if (!isKsuEnv()) return block('/tmp/mock-headers.json');
    const path = `/data/local/tmp/netproxy-headers-${Date.now()}.json`;
    await writeFileContent(path, JSON.stringify(headers ?? {}));
    try {
      return await block(path);
    } finally {
      await execAsync(`rm -f ${shellQuote(path)}`);
    }
  }
}

export const moduleClient = new ModuleClient();
