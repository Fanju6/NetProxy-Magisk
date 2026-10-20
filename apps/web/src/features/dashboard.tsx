import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowDown,
  ArrowUp,
  ChevronRight,
  Power,
  Activity,
  FlaskConical,
  Ellipsis,
  RefreshCw,
  ScrollText,
} from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { HeaderActions } from '@/app/header-actions'
import { moduleQueries, useChangeMode, useSelectPolicyGroup, useServiceAction } from '@/api/queries'
import type { Traffic } from '@/api/contracts'
import { useEventConnection } from '@/api/events'
import { useRoutingOperation } from '@/api/routing-control'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Choice } from '@/components/choice'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'
import { bytes } from '@/lib/format'
import type { MessageKey } from '@/app/i18n'

export function DashboardPage() {
  const api = useApi()
  const { t, preferences } = usePreferences()
  const connection = useEventConnection()
  const status = useQuery(moduleQueries(api).service)
  const modeState = useQuery(moduleQueries(api).mode)
  const policies = useQuery(moduleQueries(api).policyGroups)
  const live = useQuery<Traffic>({
    queryKey: ['traffic'],
    queryFn: () => ({ available: false }),
    enabled: false,
  })
  const samples = useQuery<number[]>({
    queryKey: ['traffic-history'],
    queryFn: () => [],
    enabled: false,
  })
  const action = useServiceAction()
  const mode = useChangeMode()
  const selection = useSelectPolicyGroup()
  const routingOperation = useRoutingOperation()
  const [confirmStop, setConfirmStop] = useState(false)
  const [plottedCore, setPlottedCore] = useState<number | null>(null)
  if (status.isPending) return <LoadingState />
  if (!status.data) return <ErrorState error={status.error} retry={() => void status.refetch()} />
  const service = status.data
  const running = service.state === 'ready'
  const defaultMode = modeState.data?.mode ?? service.configured_outbound_mode
  const runtimeMode = modeState.data?.runtime_mode || (running ? service.outbound_mode : '')
  const modeLabel = (value: string) =>
    ['Rule', 'Proxy', 'Direct', 'RuleAllowAds'].includes(value) ? t(value as MessageKey) : value
  const traffic =
    running &&
    service.core_started_at_millis > 0 &&
    connection === 'live' &&
    live.data?.available &&
    live.data.started_at_millis === service.core_started_at_millis
      ? live.data.status
      : undefined
  const rateUnavailable = connection !== 'live' || (running && !traffic)
  const current = policies.data?.default_outbound || service.default_outbound || t('runtimeUnknown')
  const defaultGroup = policies.data?.groups.find((group) => group.tag === current)
  const policyLive =
    running &&
    service.core_started_at_millis > 0 &&
    connection === 'live' &&
    policies.data?.live &&
    policies.data.core_started_at_millis === service.core_started_at_millis
  const selected =
    policyLive &&
    defaultGroup?.selected &&
    defaultGroup.items.filter((item) => item.tag === defaultGroup.selected).length === 1
      ? defaultGroup.selected
      : undefined
  const currentNode =
    policyLive && current === service.default_outbound ? service.runtime_selected : ''
  const history = samples.data ?? []
  // Keep this core's chart space during reconnects without retaining invalid SSE samples.
  if (traffic && history.length > 1 && plottedCore !== service.core_started_at_millis)
    setPlottedCore(service.core_started_at_millis)
  const showPlot = running && plottedCore === service.core_started_at_millis
  const maximum = Math.max(1, ...history)
  const blocked =
    action.isPending ||
    action.isUncertain ||
    mode.isPending ||
    mode.isUncertain ||
    selection.isPending ||
    selection.isUncertain ||
    status.isError ||
    connection !== 'live' ||
    !['ready', 'stopped', 'failed'].includes(service.state)
  return (
    <div className="max-w-5xl space-y-5">
      <HeaderActions>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button variant="ghost" size="icon" className="size-12" aria-label={t('dashboard')} />
            }
          >
            <Ellipsis />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-52">
            <DropdownMenuItem
              disabled={!api.supports('service.read') || status.isFetching}
              onClick={() => void status.refetch()}
            >
              <RefreshCw />
              {t('checkStatus')}
            </DropdownMenuItem>
            {api.supports('logs.read') && (
              <DropdownMenuItem render={<Link to="/settings/logs" />}>
                <ScrollText />
                {t('logs')}
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </HeaderActions>
      <section className="flex items-center justify-between gap-4 py-2">
        <div className="min-w-0 space-y-2">
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <Activity className="size-4" />
            {t('service')}
          </p>
          <h2 className="text-2xl font-medium">{t(service.state)}</h2>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">
              {service.configured_backend === 'ebpf' ? 'eBPF' : 'TUN'}
            </Badge>
            {connection !== 'live' && <Badge variant="outline">{t('staleSnapshot')}</Badge>}
          </div>
          {running && (
            <p className="text-xs text-muted-foreground tabular-nums">
              {t('uptime')} ·{' '}
              {t('duration', {
                hours: Math.floor(service.uptime_seconds / 3600),
                minutes: Math.floor(service.uptime_seconds / 60) % 60,
              })}
            </p>
          )}
        </div>
        <Button
          size="default"
          variant={running ? 'outline' : 'default'}
          className="shrink-0"
          disabled={!api.supports(running ? 'service.stop' : 'service.start') || blocked}
          onClick={() => (running ? setConfirmStop(true) : action.mutate('start'))}
        >
          <Power />
          {action.isPending ? t('working') : running ? t('stop') : t('start')}
        </Button>
      </section>
      {action.isUncertain && (
        <section role="status" className="space-y-2 rounded-md border p-3 text-sm">
          <p className="font-medium">{t('serviceUncertain')}</p>
          <p className="text-muted-foreground">
            {t(action.persisted ? 'servicePersisted' : 'serviceUncertainDescription')}
          </p>
          <Button variant="outline" onClick={() => void action.verify()}>
            {t('checkStatus')}
          </Button>
        </section>
      )}
      {action.error != null && <ErrorState compact error={action.error} />}
      {status.isError && (
        <ErrorState compact error={status.error} retry={() => void status.refetch()} />
      )}

      <section
        aria-label={preferences.locale === 'zh' ? '实时速率' : 'Transfer speeds'}
        className="grid grid-cols-2 gap-4 border-y py-4"
      >
        {[
          { title: t('download'), value: traffic?.downlink, icon: ArrowDown },
          { title: t('upload'), value: traffic?.uplink, icon: ArrowUp },
        ].map((item) => (
          <section key={item.title} className="min-w-0 space-y-2">
            <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <item.icon className="size-4" />
              {item.title}
            </p>
            <p className="break-words text-2xl font-medium tabular-nums [overflow-wrap:anywhere]">
              {rateUnavailable ? '—' : `${bytes(item.value ?? 0, preferences.locale)}/s`}
            </p>
          </section>
        ))}
      </section>

      <section
        aria-label={preferences.locale === 'zh' ? '服务统计' : 'Service statistics'}
        className="space-y-3"
      >
        {showPlot && (
          <>
            <h2 className="text-sm font-medium">{t('traffic')}</h2>
            {traffic && history.length > 1 ? (
              <div
                role="img"
                aria-label={t('traffic')}
                className="traffic-plot flex h-20 items-end gap-1 px-1 pt-3"
              >
                {Array.from({ length: 30 }, (_, index) => (
                  <div
                    key={index}
                    className="min-w-0 flex-1 rounded-t-sm bg-foreground/70"
                    style={{
                      height: `${((history[index - (30 - history.length)] ?? 0) / maximum) * 90}%`,
                    }}
                  />
                ))}
              </div>
            ) : (
              <p role="status" className="flex h-20 items-center text-sm text-muted-foreground">
                {t('waiting')}
              </p>
            )}
          </>
        )}
        <dl className="grid grid-cols-2 gap-x-6 gap-y-4 py-2 text-sm lg:grid-cols-4">
          {[
            {
              label: t('totalDownload'),
              value: bytes(traffic?.downlink_total ?? service.download_total, preferences.locale),
            },
            {
              label: t('totalUpload'),
              value: bytes(traffic?.uplink_total ?? service.upload_total, preferences.locale),
            },
            { label: t('memory'), value: bytes(service.memory_bytes, preferences.locale) },
            {
              label: t('connections'),
              value: String(service.connections_in + service.connections_out),
            },
          ].map((item) => (
            <div key={item.label} className="min-w-0 space-y-1">
              <dt className="text-xs text-muted-foreground">{item.label}</dt>
              <dd className="break-words font-medium tabular-nums">{item.value}</dd>
            </div>
          ))}
        </dl>
      </section>

      <div className="divide-y border-t">
        <section aria-label={t('routing')} className="space-y-2 py-4">
          <div className="flex items-center justify-between gap-3">
            <h2 className="text-sm text-muted-foreground">{t('routing')}</h2>
            <Choice
              label={t('routing')}
              value={defaultMode}
              items={(modeState.data?.available ?? []).map((value) => ({
                value,
                label: modeLabel(value),
              }))}
              onChange={(value) => mode.mutate(value)}
              disabled={
                !api.supports('mode.write') || !modeState.data || modeState.isError || blocked
              }
            />
          </div>
          {running && runtimeMode !== defaultMode && (
            <p className="text-xs text-muted-foreground">
              {t('runtimeMode')} ·{' '}
              {runtimeMode && runtimeMode !== 'unknown'
                ? modeLabel(runtimeMode)
                : t('runtimeUnknown')}
            </p>
          )}
          {running &&
            runtimeMode &&
            runtimeMode !== 'unknown' &&
            runtimeMode !== defaultMode &&
            !mode.persisted && (
              <p className="text-xs text-muted-foreground">{t('modePolicyDifference')}</p>
            )}
          {(mode.isUncertain || mode.isUnconfirmed) && (
            <section role="status" className="space-y-2 rounded-md border p-3 text-sm">
              <p className="font-medium">
                {t(
                  mode.isUnconfirmed
                    ? 'routingUnconfirmed'
                    : routingOperation?.target.kind === 'policy'
                      ? 'selectionUncertain'
                      : 'modeUncertain',
                )}
              </p>
              <p className="text-muted-foreground">
                {t(
                  mode.isUnconfirmed
                    ? 'routingUnconfirmedDescription'
                    : mode.persisted || selection.persisted
                      ? 'persisted'
                      : routingOperation?.target.kind === 'policy'
                        ? 'selectionUncertainDescription'
                        : 'modeUncertainDescription',
                )}
              </p>
              <Button
                variant="outline"
                onClick={() =>
                  void (routingOperation?.target.kind === 'policy'
                    ? selection.verify()
                    : mode.verify())
                }
              >
                {t('checkStatus')}
              </Button>
            </section>
          )}
          {mode.error != null && <ErrorState error={mode.error} compact />}
          {selection.error != null && <ErrorState error={selection.error} compact />}
          {modeState.isError && (
            <ErrorState compact error={modeState.error} retry={() => void modeState.refetch()} />
          )}
        </section>
        <section aria-label={t('currentNode')}>
          <Link to="/nodes" className="flex min-w-0 items-center justify-between gap-3 py-4">
            <dl className="min-w-0 space-y-3 [overflow-wrap:anywhere]">
              <div className="space-y-1">
                <dt className="text-sm text-muted-foreground">{t('currentNode')}</dt>
                <dd className="break-words font-medium">{currentNode || t('runtimeUnknown')}</dd>
              </div>
              <div className="space-y-1">
                <dt className="text-xs text-muted-foreground">{t('defaultOutbound')}</dt>
                <dd className="break-words text-sm">{current}</dd>
              </div>
              {defaultGroup && (
                <div className="space-y-1">
                  <dt className="text-xs text-muted-foreground">{t('selected')}</dt>
                  <dd className="break-words text-sm">{selected || t('runtimeUnknown')}</dd>
                </div>
              )}
            </dl>
            <ChevronRight className="size-5 shrink-0 text-muted-foreground" />
          </Link>
          {policies.isError && (
            <ErrorState compact error={policies.error} retry={() => void policies.refetch()} />
          )}
        </section>
      </div>
      {service.error && <EmptyState title={t('failed')} description={t('requestFailed')} />}
      {api.transport.kind === 'mock' && (
        <p role="note" className="flex items-center gap-2 pb-1 text-xs text-muted-foreground">
          <FlaskConical className="size-4 shrink-0" />
          <span>
            {t('demo')} · {t('demoNotice')}
          </span>
        </p>
      )}
      <Dialog open={confirmStop} onOpenChange={setConfirmStop}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>
              {t(api.transport.kind === 'mock' ? 'stopTitle' : 'stopRealTitle')}
            </DialogTitle>
            <DialogDescription>
              {t(api.transport.kind === 'mock' ? 'stopDescription' : 'stopRealDescription')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmStop(false)}>
              {t('cancel')}
            </Button>
            <Button
              disabled={!api.supports('service.stop') || blocked || !running}
              onClick={() => {
                setConfirmStop(false)
                action.mutate('stop')
              }}
            >
              {t('confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
