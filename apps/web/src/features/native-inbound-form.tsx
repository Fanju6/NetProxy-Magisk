import { usePreferences } from '@/app/context'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { EmptyState } from '@/components/states'
import { ManagementSelect } from './configuration'

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
    <label className="block space-y-2 text-sm">
      <span>{label}</span>
      <textarea
        key={`${backend}-${key}`}
        rows={3}
        className="w-full rounded-md border bg-transparent p-3 text-base"
        spellCheck={false}
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
  )
  const toggle = (label: string, key: string, parent = native, save = patch) => (
    <label className="flex min-h-11 items-center justify-between gap-3 text-sm">
      <span>{label}</span>
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
    <ManagementSelect
      label={label}
      value={typeof parent[key] === 'string' ? parent[key] : ''}
      choices={[
        { value: '', label: text('内核默认', 'Core default') },
        ...choices.map((value) => ({ value, label: value })),
      ]}
      onChange={(value) => save({ [key]: value || undefined })}
    />
  )
  return (
    <div className="space-y-5">
      {backend === 'tun' ? (
        <>
          <label className="block space-y-2 text-sm">
            <span>{text('接口名称', 'Interface name')}</span>
            <Input
              defaultValue={String(native.interface_name ?? '')}
              onBlur={(event) => patch({ interface_name: event.target.value })}
            />
          </label>
          {list(text('接口地址，每行一个', 'Interface addresses, one per line'), 'address')}
          {choice(text('DNS 接管', 'DNS capture'), 'dns_mode', ['hijack', 'disabled'])}
          {toggle(text('严格路由', 'Strict routing'), 'strict_route')}
          {choice(text('协议栈', 'Stack'), 'stack', ['system', 'mixed', 'gvisor'])}
          <label className="block space-y-2 text-sm">
            <span>MTU</span>
            <Input
              type="number"
              min={576}
              placeholder={text('内核默认', 'Core default')}
              defaultValue={typeof native.mtu === 'number' ? native.mtu : ''}
              onBlur={(event) =>
                patch({ mtu: event.target.value === '' ? undefined : Number(event.target.value) })
              }
            />
          </label>
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
        </>
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
              <section key={path} className="space-y-3 border-b pb-4">
                <h3 className="text-sm font-medium">
                  {path === 'local'
                    ? text('本机网络', 'Local network')
                    : text('共享网络', 'Shared network')}
                </h3>
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
              </section>
            )
          })}
        </>
      )}
    </div>
  )
}
