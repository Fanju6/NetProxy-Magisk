import { managementRequest, configTargetSchema } from '@/api/management'
import type { ApiRequest } from '@/api/contracts'

const envelope = (data: unknown, code = 'demo.management', status = 200) => ({
  status,
  body: { schema: 1, ok: status < 400, code, message: 'Demo', data },
})
const initialInbound = {
  backend: 'ebpf',
  root_policy: 'include',
  app: { enabled: false, mode: 'blacklist', proxy_apps: [], bypass_apps: [] },
  ebpf: {
    type: 'ebpf',
    tag: 'netproxy-in',
    local: { enabled: true, data_plane: 'tc', dns_mode: 'respect_policy' },
    shared: { enabled: false },
  },
  tun: {
    type: 'tun',
    tag: 'netproxy-in',
    interface_name: 'netproxy',
    address: ['172.19.0.1/30'],
    auto_route: true,
    auto_redirect: true,
  },
}
export class ManagementMock {
  private documents = new Map<string, string>([
    [
      'module',
      JSON.stringify({
        auto_start: false,
        selection: { group_id: 'default', node_tag: '' },
        wifi: {
          enabled: false,
          mode: 'blacklist',
          blacklist: [],
          whitelist: [],
          proxy_on_non_wifi: true,
        },
      }),
    ],
    ['inbound', JSON.stringify(initialInbound)],
    [
      'singbox/config.json',
      JSON.stringify({ log: { level: 'info' }, dns: { servers: [] }, route: { rules: [] } }),
    ],
    ...['proxy', 'direct', 'block'].map((name): [string, string] => [
      `singbox/rules/local/${name}.json`,
      '{"version":3,"rules":[]}',
    ]),
    ...['inbound', 'outbounds', 'providers'].map((name): [string, string] => [
      `runtime/${name}.json`,
      '{}',
    ]),
  ])
  private logs = {
    service: '[2026-10-17 12:00:00] [INFO] [service] [start] [success] [-] Service ready',
    core: 'INFO sing-box started',
  }
  private root(target: string) {
    return target.startsWith('module/')
      ? 'module'
      : target.startsWith('inbound/')
        ? 'inbound'
        : target.startsWith('singbox/') && !target.includes('/rules/')
          ? 'singbox/config.json'
          : target
  }
  private content(target: string) {
    const root = this.root(target)
    const content = this.documents.get(root) ?? '{}'
    return root === target
      ? content
      : JSON.stringify({
          [target.split('/')[1]!]: (JSON.parse(content) as Record<string, unknown>)[
            target.split('/')[1]!
          ],
        })
  }
  async request(request: ApiRequest) {
    const management = managementRequest(request)
    if (!management) return
    const input = management.input as Record<string, unknown> | undefined
    switch (management.operation) {
      case 'config_list': {
        const ids = [
          ...this.documents.keys(),
          'module/auto_start',
          'module/wifi',
          ...['backend', 'root_policy', 'app', 'ebpf', 'tun'].map((part) => `inbound/${part}`),
          ...[
            'log',
            'dns',
            'ntp',
            'certificate',
            'certificate_providers',
            'http_clients',
            'network_namespaces',
            'endpoints',
            'inbounds',
            'outbounds',
            'providers',
            'route',
            'services',
            'experimental',
          ].map((part) => `singbox/${part}`),
        ]
        return envelope(
          ids
            .filter((id) => configTargetSchema.safeParse(id).success)
            .map((id) => ({
              id,
              filename: id.split('/').at(-1)!,
              category: id.startsWith('runtime/')
                ? 'runtime'
                : id.includes('/rules/')
                  ? 'rules'
                  : id.startsWith('inbound')
                    ? 'inbound'
                    : id.startsWith('module')
                      ? 'module'
                      : 'config',
              editable: !id.startsWith('runtime/'),
            })),
        )
      }
      case 'config_read': {
        const target = input!.target as string
        const content = this.content(target)
        return envelope({ target, content, revision: await this.revision(content) })
      }
      case 'config_apply':
      case 'config_validate': {
        const { target, content, revision } = input as {
          target: string
          content: string
          revision: string
        }
        if (revision !== (await this.revision(this.content(target))))
          return envelope({}, 'config.conflict', 409)
        try {
          const value: unknown = JSON.parse(content)
          if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error()
          const root = this.root(target)
          const key = target.split('/')[1]!
          if (
            root !== target &&
            (Object.keys(value).some((field) => field !== key) ||
              ((target.startsWith('module/') || target.startsWith('inbound/')) &&
                !Object.hasOwn(value, key)))
          )
            throw new Error()
          if (
            target === 'module/auto_start' &&
            typeof (value as Record<string, unknown>).auto_start !== 'boolean'
          )
            throw new Error()
          if (management.operation === 'config_apply') {
            this.documents.set(
              root,
              root === target
                ? content
                : JSON.stringify({ ...JSON.parse(this.documents.get(root)!), ...value }),
            )
          }
          return envelope({
            target,
            revision: await this.revision(
              management.operation === 'config_apply' ? this.content(target) : content,
            ),
          })
        } catch {
          return envelope({}, 'config.invalid', 422)
        }
      }
      case 'wifi':
        return envelope({ ssids: ['Home Wi-Fi', 'Office', 'Guest, Network'] })
      case 'apps':
        return envelope({
          users: [
            {
              id: 0,
              name: 'Owner',
              revision: 'demo-apps-1',
              apps: [
                {
                  package_name: 'com.android.chrome',
                  label: 'Chrome',
                  uid: 10100,
                  system: false,
                  icon_key: '',
                  last_update_time: 0,
                  version_code: 130,
                },
                {
                  package_name: 'com.termux',
                  label: 'Termux',
                  uid: 10101,
                  system: false,
                  icon_key: '',
                  last_update_time: 0,
                  version_code: 1,
                },
              ],
            },
            {
              id: 10,
              name: 'Work',
              revision: 'demo-work-1',
              apps: [
                {
                  package_name: 'org.example.work',
                  label: 'Work app',
                  uid: 1010100,
                  system: false,
                  icon_key: '',
                  last_update_time: 0,
                  version_code: 1,
                },
              ],
            },
          ],
        })
      case 'icon':
        return envelope({
          content:
            'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==',
        })
      case 'logs':
        return envelope({
          kind: input!.kind,
          content: this.logs[input!.kind as 'service' | 'core'],
          entries: [],
        })
      case 'logs_clear':
        this.logs[input!.kind as 'service' | 'core'] = ''
        return envelope({ kind: input!.kind })
      case 'ebpf':
        return envelope({
          raw: false,
          content:
            'Demo: eBPF capability checks passed. This does not prove that traffic is intercepted.',
        })
      case 'diagnostics':
        return envelope({ prepared: true })
    }
  }
  private async revision(content: string) {
    return [
      ...new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(content))),
    ]
      .map((byte) => byte.toString(16).padStart(2, '0'))
      .join('')
  }
}
