import { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { AppWindow, Check, Wifi, Plus, Trash2 } from 'lucide-react'
import { useApi } from '@/app/context'
import type { ApiClient } from '@/api/client'
import {
  androidAppsSchema,
  androidIconSchema,
  appDocumentSchema,
  changeListMode,
  listMode,
  parseDocument,
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
import { Choice } from '@/components/choice'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'
import { ConfigurationEditor } from './configuration'
import { useManagementCopy } from './management-copy'
import { InboundSettingsForm } from './inbound-settings-form'

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
    <div className="space-y-5">
      <div className="settings-group">
        <div className="setting-row">
          <span className="min-w-0 break-words">{copy.appMode}</span>
          <Choice
            variant="ghost"
            label={copy.appMode}
            value={mode}
            items={(['off', 'blacklist', 'whitelist'] as const).map((value) => ({
              value,
              label: copy[value],
            }))}
            onChange={(value) => {
              if (['off', 'blacklist', 'whitelist'].includes(value))
                change(serialize({ app: changeListMode(policy, value as ListMode) }))
            }}
          />
        </div>
        {mode !== 'off' && users && users.length > 1 && (
          <div className="setting-row">
            <span className="min-w-0 break-words">{copy.users}</span>
            <Choice
              variant="ghost"
              label={copy.users}
              value={selectedUser ? String(selectedUser.id) : ''}
              items={users.map((user) => ({ value: String(user.id), label: user.name }))}
              onChange={(value) => {
                setUserId(value)
                setLimit(50)
              }}
            />
          </div>
        )}
      </div>
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
              {!apps.length && <EmptyState title={copy.noApps} />}
              <div className={apps.length ? 'settings-group' : undefined}>
                {apps.slice(0, limit).map((app) => {
                  const reference = `${selectedUser!.id}:${app.package_name}`
                  const active = selected.has(reference)
                  return (
                    <Button
                      key={reference}
                      variant="ghost"
                      className="setting-row h-auto min-h-16 w-full justify-start whitespace-normal rounded-none text-left font-normal"
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
    <div className="space-y-5">
      <div className="settings-group">
        <div className="setting-row">
          <span className="min-w-0 break-words">{copy.wifi}</span>
          <Choice
            variant="ghost"
            label={copy.wifi}
            value={mode}
            items={(['off', 'blacklist', 'whitelist'] as const).map((value) => ({
              value,
              label: copy[value],
            }))}
            onChange={(value) => {
              if (['off', 'blacklist', 'whitelist'].includes(value))
                change(serialize({ wifi: changeListMode(policy, value as ListMode) }))
            }}
          />
        </div>
        <label className="setting-row" htmlFor="network-non-wifi">
          <span className="min-w-0 break-words">{copy.nonWifi}</span>
          <Switch
            id="network-non-wifi"
            checked={policy.proxy_on_non_wifi}
            onCheckedChange={(checked) =>
              change(serialize({ wifi: { ...policy, proxy_on_non_wifi: checked } }))
            }
          />
        </label>
      </div>
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
            <ul className="settings-group empty:hidden" aria-label={copy.savedWifi}>
              {saved
                .filter((ssid) => !entries.includes(ssid))
                .map((ssid) => (
                  <li key={ssid}>
                    <Button
                      variant="ghost"
                      className="setting-row h-auto w-full justify-start rounded-none text-left font-normal"
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
          <section>
            <h3 className="section-label">{copy[policy.mode]}</h3>
            {!entries.length && <EmptyState title={copy.noWifi} />}
            <ul
              className={entries.length ? 'settings-group' : undefined}
              aria-label={copy[policy.mode]}
            >
              {entries.map((ssid) => (
                <li key={ssid} className="setting-row">
                  <Wifi className="size-4 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1 whitespace-pre-wrap break-all text-sm">
                    {ssid}
                  </span>
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
          </section>
        </>
      )}
    </div>
  )
}

export function PoliciesPage() {
  const { section } = useSearch({ from: '/settings/policies' })
  const copy = useManagementCopy()
  const navigate = useNavigate()
  const target = section === 'apps' ? 'inbound/app' : section === 'wifi' ? 'module/wifi' : 'inbound'
  return (
    <div className="max-w-3xl space-y-5">
      <ConfigurationEditor
        key={target}
        target={target}
        title={section === 'apps' ? copy.apps : section === 'wifi' ? copy.wifi : copy.inbound}
        inline
        onClose={() => void navigate({ to: '/settings' })}
        renderForm={(content, change) =>
          section === 'apps' ? (
            <ApplicationPolicyForm content={content} change={change} />
          ) : section === 'wifi' ? (
            <WiFiPolicyForm content={content} change={change} />
          ) : (
            <InboundSettingsForm content={content} change={change} />
          )
        }
      />
    </div>
  )
}
