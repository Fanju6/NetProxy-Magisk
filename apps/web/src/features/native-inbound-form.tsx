import { usePreferences } from '@/app/context'
import { ChevronDown } from 'lucide-react'
import { Choice } from '@/components/choice'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { EmptyState } from '@/components/states'

export function NativeInboundForm({
  backend,
  content,
  change,
}: {
  backend: 'ebpf' | 'tun'
  content: string
  change: (content: string) => void
}) {
  const { preferences } = usePreferences()
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  let document: Record<string, unknown>
  try {
    document = JSON.parse(content) as Record<string, unknown>
  } catch {
    return <EmptyState />
  }
  const value = document[backend]
  if (!value || typeof value !== 'object' || Array.isArray(value)) return <EmptyState />
  const native = value as Record<string, unknown>
  const patch = (values: Record<string, unknown>) =>
    change(JSON.stringify({ ...document, [backend]: { ...native, ...values } }, null, 2))
  const list = (label: string, key: string, parent = native, save = patch) => (
    <div className="setting-row">
      <label className="block min-w-0 w-full space-y-2 text-sm">
        <span>{label}</span>
        <textarea
          key={`${backend}-${key}`}
          rows={2}
          className="w-full resize-y rounded-md border bg-transparent p-3 text-base md:text-sm"
          spellCheck={false}
          autoCapitalize="none"
          autoComplete="off"
          defaultValue={
            Array.isArray(parent[key])
              ? (parent[key] as unknown[]).join('\n')
              : typeof parent[key] === 'string'
                ? parent[key]
                : ''
          }
          onBlur={(event) =>
            save({
              [key]: event.target.value
                .split(/\r?\n/)
                .map((value) => value.trim())
                .filter(Boolean),
            })
          }
        />
      </label>
    </div>
  )
  const toggle = (label: string, key: string, parent = native, save = patch) => (
    <label className="setting-row">
      <span className="min-w-0 break-words">{label}</span>
      <Switch
        checked={parent[key] === true}
        onCheckedChange={(checked) => save({ [key]: checked })}
      />
    </label>
  )
  const choice = (
    label: string,
    key: string,
    choices: readonly string[],
    parent = native,
    save = patch,
  ) => (
    <div className="setting-row">
      <span className="min-w-0 break-words">{label}</span>
      <Choice
        variant="ghost"
        label={label}
        value={typeof parent[key] === 'string' ? parent[key] : ''}
        items={[
          { value: '', label: text('内核默认', 'Core default') },
          ...choices.map((value) => ({ value, label: value })),
        ]}
        onChange={(value) => save({ [key]: value || undefined })}
      />
    </div>
  )
  return (
    <div className="space-y-5">
      {backend === 'tun' ? (
        <section>
          <h3 className="section-label">{text('网络接口', 'Network interface')}</h3>
          <div className="settings-group">
            <div className="setting-row">
              <label className="block min-w-0 w-full space-y-2 text-sm">
                <span>{text('接口名称', 'Interface name')}</span>
                <Input
                  className="h-11 text-base md:text-sm"
                  autoCapitalize="none"
                  autoComplete="off"
                  defaultValue={String(native.interface_name ?? '')}
                  onBlur={(event) => patch({ interface_name: event.target.value })}
                />
              </label>
            </div>
            {list(text('接口地址，每行一个', 'Interface addresses, one per line'), 'address')}
            {choice(text('DNS 接管', 'DNS capture'), 'dns_mode', ['hijack', 'disabled'])}
          </div>
          <details className="settings-group group mt-5">
            <summary className="setting-row cursor-pointer list-none font-medium [&::-webkit-details-marker]:hidden">
              {text('高级参数', 'Advanced parameters')}
              <ChevronDown className="size-4 shrink-0 text-muted-foreground group-open:rotate-180" />
            </summary>
            <div className="divide-y border-t">
              {toggle(text('严格路由', 'Strict routing'), 'strict_route')}
              {choice(text('协议栈', 'Stack'), 'stack', ['system', 'mixed', 'gvisor'])}
              <div className="setting-row">
                <label className="block min-w-0 w-full space-y-2 text-sm">
                  <span>MTU</span>
                  <Input
                    className="h-11 text-base md:text-sm"
                    type="number"
                    min={576}
                    placeholder={text('内核默认', 'Core default')}
                    defaultValue={typeof native.mtu === 'number' ? native.mtu : ''}
                    onBlur={(event) =>
                      patch({
                        mtu: event.target.value === '' ? undefined : Number(event.target.value),
                      })
                    }
                  />
                </label>
              </div>
              {list(
                text('接管接口，每行一个', 'Included interfaces, one per line'),
                'include_interface',
              )}
              {list(
                text('绕过接口，每行一个', 'Excluded interfaces, one per line'),
                'exclude_interface',
              )}
              {list(
                text('绕过网段，每行一个', 'Excluded CIDRs, one per line'),
                'route_exclude_address',
              )}
            </div>
          </details>
        </section>
      ) : (
        <>
          {(['local', 'shared'] as const).map((path) => {
            const raw = native[path]
            const part =
              raw && typeof raw === 'object' && !Array.isArray(raw)
                ? (raw as Record<string, unknown>)
                : {}
            const save = (values: Record<string, unknown>) =>
              patch({ [path]: { ...part, ...values } })
            return (
              <section key={path}>
                <h3 className="section-label">
                  {path === 'local'
                    ? text('本机网络', 'Local network')
                    : text('共享网络', 'Shared network')}
                </h3>
                <div className="settings-group">
                  {toggle(text('启用', 'Enabled'), 'enabled', part, save)}
                  {choice(
                    text('数据平面', 'Data plane'),
                    'data_plane',
                    path === 'local' ? ['cgroup', 'tc'] : ['packet_rewrite', 'socket_assign'],
                    part,
                    save,
                  )}
                  {choice(
                    text('DNS 接管', 'DNS capture'),
                    'dns_mode',
                    ['respect_policy', 'hijack', 'off'],
                    part,
                    save,
                  )}
                  {toggle('IPv6', 'ipv6', part, save)}
                  {toggle(
                    text('绕过私网地址', 'Bypass private addresses'),
                    'bypass_private_address',
                    part,
                    save,
                  )}
                  {path === 'shared' &&
                    list(
                      text('下游接口，每行一个', 'Downstream interfaces, one per line'),
                      'interface',
                      part,
                      save,
                    )}
                  {list(
                    text('绕过规则集，每行一个', 'Bypass rule sets, one per line'),
                    'bypass_rule_set',
                    part,
                    save,
                  )}
                </div>
              </section>
            )
          })}
        </>
      )}
    </div>
  )
}
