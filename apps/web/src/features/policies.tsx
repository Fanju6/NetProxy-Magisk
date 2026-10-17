import { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import {
  AppWindow,
  Check,
  ChevronRight,
  Wifi,
  Network,
  Settings2,
  Plus,
  Trash2,
} from 'lucide-react'
import { useApi } from '@/app/context'
import type { ApiClient } from '@/api/client'
import {
  androidAppsSchema,
  androidIconSchema,
  appDocumentSchema,
  backendDocumentSchema,
  changeListMode,
  listMode,
  parseDocument,
  rootDocumentSchema,
  safeManagementError,
  savedWifiSchema,
  ssidSchema,
  wifiDocumentSchema,
  type AndroidUsers,
  type ListMode,
} from '@/api/management-data'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'
import { ConfigurationEditor, ManagementSelect } from './configuration'
import { useManagementCopy } from './management-copy'
import { NativeInboundForm } from './native-inbound-form'

type Change = (content: string) => void
const serialize = (value: unknown) => JSON.stringify(value, null, 2)
let iconRequests = 0
const iconQueue: (() => void)[] = []
function readIcon(api: ApiClient, key: string, signal: AbortSignal): Promise<string> {
  return new Promise((resolve, reject) => {
    let started = false
    const abort = () => {
      if (started) return
      const index = iconQueue.indexOf(start)
      if (index >= 0) iconQueue.splice(index, 1)
      reject(new DOMException('Icon request cancelled', 'AbortError'))
    }
    const start = () => {
      if (signal.aborted) {
        abort()
        return
      }
      started = true
      iconRequests++
      signal.removeEventListener('abort', abort)
      void api
        .request({ method: 'GET', path: `/android/icons/${key}` }, androidIconSchema, signal)
        .then((result) => resolve(`data:image/png;base64,${result.content}`), reject)
        .finally(() => {
          iconRequests--
          iconQueue.shift()?.()
        })
    }
    signal.addEventListener('abort', abort, { once: true })
    if (signal.aborted) abort()
    else if (iconRequests < 4) start()
    else iconQueue.push(start)
  })
}

function AppIcon({ iconKey }: { iconKey: string }) {
  const api = useApi()
  const host = useRef<HTMLSpanElement>(null)
  const [visible, setVisible] = useState(typeof IntersectionObserver === 'undefined')
  const [source, setSource] = useState<string | null>(null)
  useEffect(() => {
    if (!host.current || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        setVisible(true)
        observer.disconnect()
      }
    })
    observer.observe(host.current)
    return () => observer.disconnect()
  }, [])
  useEffect(() => {
    if (!visible || !iconKey) return
    const controller = new AbortController()
    void readIcon(api, iconKey, controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) setSource(value)
      })
      .catch(() => {})
    return () => controller.abort()
  }, [api, visible, iconKey])
  return (
    <span
      ref={host}
      className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted"
    >
      {source ? (
        <img src={source} width={40} height={40} alt="" onError={() => setSource(null)} />
      ) : (
        <AppWindow className="size-5 text-muted-foreground" />
      )}
    </span>
  )
}

function ApplicationPolicyForm({ content, change }: { content: string; change: Change }) {
  const api = useApi()
  const copy = useManagementCopy()
  const parsed = parseDocument(content, appDocumentSchema)
  const policy = parsed?.app
  const [users, setUsers] = useState<AndroidUsers | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [generation, setGeneration] = useState(0)
  const [userId, setUserId] = useState('')
  const [search, setSearch] = useState('')
  const [limit, setLimit] = useState(50)
  const enabled = policy?.enabled ?? false
  useEffect(() => {
    if (!enabled) return
    const controller = new AbortController()
    void api
      .request({ method: 'GET', path: '/android/apps' }, androidAppsSchema, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setUsers(result.users)
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(safeManagementError(failure))
      })
    return () => controller.abort()
  }, [api, enabled, generation])
  if (!policy) return <EmptyState title={copy.invalid} />
  const mode = listMode(policy)
  const selectedUser = users?.find((user) => String(user.id) === userId) ?? users?.[0]
  const key = policy.mode === 'whitelist' ? 'proxy_apps' : 'bypass_apps'
  const selected = new Set(policy[key])
  const query = search.toLocaleLowerCase()
  const apps =
    selectedUser?.apps
      .filter((app) => `${app.label}\n${app.package_name}`.toLocaleLowerCase().includes(query))
      .sort(
        (first, second) =>
          Number(selected.has(`${selectedUser.id}:${second.package_name}`)) -
            Number(selected.has(`${selectedUser.id}:${first.package_name}`)) ||
          first.label.localeCompare(second.label),
      ) ?? []
  return (
    <div className="space-y-3">
      <ManagementSelect
        label={copy.appMode}
        value={mode}
        choices={(['off', 'blacklist', 'whitelist'] as const).map((value) => ({
          value,
          label: copy[value],
        }))}
        onChange={(value) => {
          if (['off', 'blacklist', 'whitelist'].includes(value))
            change(serialize({ app: changeListMode(policy, value as ListMode) }))
        }}
      />
      {mode !== 'off' && (
        <>
          <p className="text-sm text-muted-foreground">
            {policy.mode === 'blacklist' ? copy.blacklistHint : copy.whitelistHint}
          </p>
          <Input
            type="search"
            aria-label={copy.searchApps}
            placeholder={copy.searchApps}
            className="h-11 text-base md:text-sm"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value)
              setLimit(50)
            }}
          />
          {error && (
            <ErrorState
              error={error}
              compact
              retry={() => {
                setError(null)
                setGeneration((value) => value + 1)
              }}
            />
          )}
          {!users && !error && <LoadingState />}
          {users && (
            <>
              {users.length > 1 && (
                <ManagementSelect
                  label={copy.users}
                  value={selectedUser ? String(selectedUser.id) : ''}
                  choices={users.map((user) => ({ value: String(user.id), label: user.name }))}
                  onChange={(value) => {
                    setUserId(value)
                    setLimit(50)
                  }}
                />
              )}
              {!apps.length && <EmptyState title={copy.noApps} />}
              <div className="divide-y">
                {apps.slice(0, limit).map((app) => {
                  const reference = `${selectedUser!.id}:${app.package_name}`
                  const active = selected.has(reference)
                  return (
                    <Button
                      key={reference}
                      variant="ghost"
                      className="h-auto min-h-16 w-full justify-start gap-3 whitespace-normal rounded-none px-1 py-2 text-left"
                      aria-pressed={active}
                      onClick={() =>
                        change(
                          serialize({
                            app: {
                              ...policy,
                              [key]: active
                                ? policy[key].filter((item) => item !== reference)
                                : [...policy[key], reference],
                            },
                          }),
                        )
                      }
                    >
                      <AppIcon key={app.icon_key} iconKey={app.icon_key} />
                      <span className="min-w-0 flex-1">
                        <span className="block break-words font-medium">{app.label}</span>
                        <span className="block break-all text-xs text-muted-foreground">
                          <span>{app.package_name}</span>
                          <span>
                            {' '}
                            · UID {app.uid}
                            {app.system ? ` · ${copy.systemApp}` : ''}
                          </span>
                        </span>
                      </span>
                      <Check
                        aria-hidden
                        className={`size-5 shrink-0 ${active ? '' : 'opacity-0'}`}
                      />
                    </Button>
                  )
                })}
              </div>
              {apps.length > limit && (
                <Button
                  variant="outline"
                  className="min-h-11 w-full"
                  onClick={() => setLimit((value) => value + 50)}
                >
                  {copy.more}
                </Button>
              )}
            </>
          )}
        </>
      )}
    </div>
  )
}

function WiFiPolicyForm({ content, change }: { content: string; change: Change }) {
  const api = useApi()
  const copy = useManagementCopy()
  const policy = parseDocument(content, wifiDocumentSchema)?.wifi
  const [name, setName] = useState('')
  const [invalid, setInvalid] = useState(false)
  const [saved, setSaved] = useState<string[] | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [loading, setLoading] = useState(false)
  const request = useRef<AbortController | null>(null)
  useEffect(() => () => request.current?.abort(), [])
  async function loadSaved() {
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    setLoading(true)
    setError(null)
    try {
      const result = await api.request(
        { method: 'GET', path: '/network/wifi-list' },
        savedWifiSchema,
        controller.signal,
      )
      if (!controller.signal.aborted) setSaved(result.ssids)
    } catch (failure) {
      if (!controller.signal.aborted) setError(safeManagementError(failure))
    } finally {
      if (!controller.signal.aborted) setLoading(false)
    }
  }
  if (!policy) return <EmptyState title={copy.invalid} />
  const mode = listMode(policy)
  const entries = policy[policy.mode]
  function add(value: string) {
    if (!policy || !ssidSchema.safeParse(value).success) {
      setInvalid(true)
      return
    }
    if (!policy[policy.mode].includes(value))
      change(serialize({ wifi: { ...policy, [policy.mode]: [...policy[policy.mode], value] } }))
    setName('')
    setInvalid(false)
  }
  return (
    <div className="space-y-4">
      <ManagementSelect
        label={copy.wifi}
        value={mode}
        choices={(['off', 'blacklist', 'whitelist'] as const).map((value) => ({
          value,
          label: copy[value],
        }))}
        onChange={(value) => {
          if (['off', 'blacklist', 'whitelist'].includes(value))
            change(serialize({ wifi: changeListMode(policy, value as ListMode) }))
        }}
      />
      <label
        className="flex min-h-11 items-center justify-between gap-4 text-sm"
        htmlFor="network-non-wifi"
      >
        <span>{copy.nonWifi}</span>
        <Switch
          id="network-non-wifi"
          checked={policy.proxy_on_non_wifi}
          onCheckedChange={(checked) =>
            change(serialize({ wifi: { ...policy, proxy_on_non_wifi: checked } }))
          }
        />
      </label>
      {mode !== 'off' && (
        <>
          <p className="text-sm text-muted-foreground">
            {mode === 'blacklist' ? copy.wifiBlacklistHint : copy.wifiWhitelistHint}
          </p>
          <div className="flex gap-2">
            <Input
              aria-label={copy.ssid}
              placeholder={copy.ssid}
              className="h-11 min-w-0 text-base md:text-sm"
              autoComplete="off"
              autoCapitalize="none"
              value={name}
              onChange={(event) => setName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  add(name)
                }
              }}
            />
            <Button
              variant="outline"
              size="icon"
              className="size-11 shrink-0"
              aria-label={copy.addSsid}
              onClick={() => add(name)}
            >
              <Plus />
            </Button>
          </div>
          {invalid && (
            <p role="alert" className="text-sm text-destructive">
              {copy.ssidInvalid}
            </p>
          )}
          <Button
            variant="outline"
            className="min-h-11"
            disabled={loading}
            onClick={() => void loadSaved()}
          >
            <Wifi />
            {loading ? copy.loading : copy.savedWifi}
          </Button>
          {error && <ErrorState error={error} compact />}
          {saved && (
            <ul className="divide-y" aria-label={copy.savedWifi}>
              {saved
                .filter((ssid) => !entries.includes(ssid))
                .map((ssid) => (
                  <li key={ssid}>
                    <Button
                      variant="ghost"
                      className="h-auto min-h-11 w-full justify-start gap-3 rounded-none px-1 py-2 text-left"
                      onClick={() => add(ssid)}
                    >
                      <Wifi className="size-4 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 whitespace-pre-wrap break-all">{ssid}</span>
                      <Plus className="size-4 shrink-0" />
                    </Button>
                  </li>
                ))}
            </ul>
          )}
          <h3 className="text-sm font-medium">{copy[policy.mode]}</h3>
          {!entries.length && <EmptyState title={copy.noWifi} />}
          <ul className="divide-y" aria-label={copy[policy.mode]}>
            {entries.map((ssid) => (
              <li key={ssid} className="flex items-center justify-between gap-3 py-2">
                <Wifi className="size-4 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 whitespace-pre-wrap break-all text-sm">{ssid}</span>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-11 shrink-0"
                  aria-label={`${copy.remove} ${ssid}`}
                  onClick={() =>
                    change(
                      serialize({
                        wifi: {
                          ...policy,
                          [policy.mode]: entries.filter((value) => value !== ssid),
                        },
                      }),
                    )
                  }
                >
                  <Trash2 className="size-4" />
                </Button>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  )
}

function InboundChoice({
  target,
  content,
  change,
}: {
  target: 'backend' | 'root_policy'
  content: string
  change: Change
}) {
  const copy = useManagementCopy()
  if (target === 'backend') {
    const parsed = parseDocument(content, backendDocumentSchema)
    return parsed ? (
      <ManagementSelect
        label={copy.backend}
        value={parsed.backend}
        choices={(['ebpf', 'tun'] as const).map((value) => ({ value, label: copy[value] }))}
        onChange={(backend) => {
          if (backend === 'ebpf' || backend === 'tun') change(serialize({ backend }))
        }}
      />
    ) : (
      <EmptyState title={copy.invalid} />
    )
  }
  const parsed = parseDocument(content, rootDocumentSchema)
  return parsed ? (
    <ManagementSelect
      label={copy.root}
      value={parsed.root_policy}
      choices={(['default', 'include', 'exclude'] as const).map((value) => ({
        value,
        label: copy[value],
      }))}
      onChange={(root_policy) => {
        if (['default', 'include', 'exclude'].includes(root_policy))
          change(serialize({ root_policy }))
      }}
    />
  ) : (
    <EmptyState title={copy.invalid} />
  )
}

export function PoliciesPage() {
  const { section } = useSearch({ from: '/settings/policies' })
  return <PolicyPanel key={section ?? 'all'} section={section} />
}

type PolicySection = 'inbound' | 'root' | 'apps' | 'wifi'

function PolicyPanel({ section }: { section?: PolicySection }) {
  const copy = useManagementCopy()
  const navigate = useNavigate()
  const initial =
    section === 'root'
      ? 'inbound/root_policy'
      : section === 'apps'
        ? 'inbound/app'
        : section === 'wifi'
          ? 'module/wifi'
          : null
  const [target, setTarget] = useState<string | null>(initial)
  const entries = [
    { target: 'inbound/backend', title: copy.backend, icon: Network },
    { target: 'inbound/root_policy', title: copy.root, icon: Settings2 },
    { target: 'inbound/ebpf', title: copy.ebpf, icon: Network },
    { target: 'inbound/tun', title: copy.tun, icon: Network },
    { target: 'inbound/app', title: copy.apps, icon: AppWindow },
    { target: 'module/wifi', title: copy.wifi, icon: Wifi },
  ]
  const selected = entries.find((entry) => entry.target === target)
  const groups = [
    { section: 'inbound', title: copy.inbound, entries: entries.slice(0, 4) },
    { section: 'apps', title: copy.apps, entries: entries.slice(4, 5) },
    { section: 'wifi', title: 'Wi-Fi', entries: entries.slice(5) },
  ]
  const close = () => {
    setTarget(null)
    if (section && section !== 'inbound') void navigate({ to: '/settings' })
  }
  return (
    <div className="max-w-3xl space-y-6">
      {(!section || section === 'inbound') &&
        groups
          .filter((group) => !section || group.section === section)
          .map((group) => (
            <section key={group.section} className="space-y-2">
              <h2 className="section-label">{group.title}</h2>
              <div className="divide-y">
                {group.entries
                  .filter((entry) => !section || entry.target !== 'inbound/root_policy')
                  .map((entry) => (
                    <Button
                      key={entry.target}
                      variant="ghost"
                      className="min-h-14 w-full justify-start gap-3 rounded-none"
                      onClick={() => setTarget(entry.target)}
                    >
                      <entry.icon className="size-4" />
                      <span className="min-w-0 flex-1 whitespace-normal break-words text-left">
                        {entry.title}
                      </span>
                      <ChevronRight className="size-4" />
                    </Button>
                  ))}
              </div>
            </section>
          ))}
      {selected && (
        <ConfigurationEditor
          key={selected.target}
          target={selected.target}
          title={selected.title}
          inline={section !== undefined && section !== 'inbound'}
          onClose={close}
          {...(target === 'inbound/app'
            ? {
                renderForm: (content: string, change: Change) => (
                  <ApplicationPolicyForm content={content} change={change} />
                ),
              }
            : target === 'module/wifi'
              ? {
                  renderForm: (content: string, change: Change) => (
                    <WiFiPolicyForm content={content} change={change} />
                  ),
                }
              : target === 'inbound/ebpf' || target === 'inbound/tun'
                ? {
                    renderForm: (content: string, change: Change) => (
                      <NativeInboundForm
                        backend={target === 'inbound/ebpf' ? 'ebpf' : 'tun'}
                        content={content}
                        change={change}
                      />
                    ),
                  }
                : target === 'inbound/backend' || target === 'inbound/root_policy'
                  ? {
                      renderForm: (content: string, change: Change) => (
                        <InboundChoice
                          target={target === 'inbound/backend' ? 'backend' : 'root_policy'}
                          content={content}
                          change={change}
                        />
                      ),
                    }
                  : {})}
        />
      )}
    </div>
  )
}
