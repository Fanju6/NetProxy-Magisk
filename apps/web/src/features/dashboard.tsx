import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, ArrowUpRight, Power, Activity, MemoryStick, Link2 } from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { moduleQueries, useChangeMode, useServiceAction } from '@/api/queries'
import type { Traffic } from '@/api/contracts'
import { useEventConnection } from '@/api/events'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
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
import { bytes, displayNode } from '@/lib/format'
import type { MessageKey } from '@/app/i18n'

export function DashboardPage() {
  const api = useApi()
  const { t, preferences } = usePreferences()
  const connection = useEventConnection()
  const status = useQuery(moduleQueries(api).service)
  const info = useQuery(moduleQueries(api).info)
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
  const [confirmStop, setConfirmStop] = useState(false)
  if (status.isPending) return <LoadingState />
  if (!status.data) return <ErrorState error={status.error} retry={() => void status.refetch()} />
  const service = status.data
  const running = service.state === 'ready'
  const traffic =
    connection === 'live' &&
    live.data?.available &&
    live.data.started_at_millis === service.core_started_at_millis
      ? live.data.status
      : undefined
  const rateUnavailable = connection !== 'live' || (running && !traffic)
  const current = displayNode(service.runtime_selected || service.selected_node_ref) || t('auto')
  const values = [
    {
      title: t('download'),
      value: rateUnavailable ? '—' : `${bytes(traffic?.downlink ?? 0, preferences.locale)}/s`,
      icon: ArrowDown,
    },
    {
      title: t('upload'),
      value: rateUnavailable ? '—' : `${bytes(traffic?.uplink ?? 0, preferences.locale)}/s`,
      icon: ArrowUp,
    },
    {
      title: t('memory'),
      value: bytes(service.memory_bytes, preferences.locale),
      icon: MemoryStick,
    },
    {
      title: t('connections'),
      value: String(service.connections_in + service.connections_out),
      icon: Link2,
    },
  ]
  const history = samples.data ?? []
  const maximum = Math.max(1, ...history)
  return (
    <div className="space-y-8">
      <section className="flex flex-wrap items-center justify-between gap-5 border-b pb-7">
        <div className="space-y-3">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Activity className="size-4" />
            {t('service')}
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <h2 className="text-3xl font-semibold">{t(service.state)}</h2>
            {connection !== 'live' && <Badge variant="outline">{t('staleSnapshot')}</Badge>}
            <Badge variant="outline" className="gap-1.5">
              <span
                className={
                  running && connection === 'live'
                    ? 'size-1.5 rounded-full bg-emerald-500'
                    : 'size-1.5 rounded-full bg-muted-foreground'
                }
              />
              {service.configured_backend === 'ebpf' ? 'eBPF' : 'TUN'}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            {t('uptime')} ·{' '}
            {t('duration', {
              hours: Math.floor(service.uptime_seconds / 3600),
              minutes: Math.floor(service.uptime_seconds / 60) % 60,
            })}
          </p>
        </div>
        <Button
          size="lg"
          variant={running ? 'outline' : 'default'}
          disabled={
            !api.supports(running ? 'service.stop' : 'service.start') ||
            action.isPending ||
            action.isUncertain ||
            status.isError ||
            connection !== 'live' ||
            !['ready', 'stopped', 'failed'].includes(service.state)
          }
          onClick={() => (running ? setConfirmStop(true) : action.mutate('start'))}
        >
          <Power />
          {action.isPending ? t('working') : running ? t('stop') : t('start')}
        </Button>
      </section>
      {action.isUncertain && (
        <section
          role="status"
          className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-4 text-sm"
        >
          <div className="space-y-1">
            <p className="font-medium">{t('serviceUncertain')}</p>
            <p className="text-muted-foreground">
              {t(action.persisted ? 'servicePersisted' : 'serviceUncertainDescription')}
            </p>
          </div>
          <Button variant="outline" onClick={() => void action.verify()}>
            {t('checkStatus')}
          </Button>
        </section>
      )}
      {action.error != null && <ErrorState compact error={action.error} />}
      {status.isError && (
        <ErrorState compact error={status.error} retry={() => void status.refetch()} />
      )}
      <div className="grid grid-cols-2 gap-x-6 gap-y-6 lg:grid-cols-4">
        {values.map((item) => (
          <section key={item.title} className="min-w-0 space-y-3 border-b pb-5">
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <item.icon className="size-4" />
              {item.title}
            </p>
            <p className="text-2xl font-medium tabular-nums sm:text-3xl">{item.value}</p>
          </section>
        ))}
      </div>
      <div className="grid gap-8 xl:grid-cols-[1.5fr_1fr]">
        <section className="min-w-0 space-y-5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="font-medium">{t('traffic')}</h2>
            <p className="text-xs text-muted-foreground">{t('trafficCaption')}</p>
          </div>
          <div
            role="img"
            aria-label={t('traffic')}
            className="traffic-plot flex h-48 items-end gap-1 border-b px-1 pt-4"
          >
            {connection !== 'live' || !running || history.length === 0 ? (
              <div className="flex h-full w-full items-center justify-center text-sm text-muted-foreground">
                {t('waiting')}
              </div>
            ) : (
              Array.from({ length: 30 }, (_, index) => {
                const value = history[index - (30 - history.length)] ?? 0
                return (
                  <div
                    key={index}
                    className="min-w-0 flex-1 rounded-t-sm bg-foreground/75"
                    style={{ height: `${(value / maximum) * 90}%` }}
                  />
                )
              })
            )}
          </div>
          <div className="flex flex-wrap justify-between gap-2 text-xs text-muted-foreground">
            <span>
              {t('totalDownload')} ·{' '}
              {bytes(traffic?.downlink_total ?? service.download_total, preferences.locale)}
            </span>
            <span>
              {t('totalUpload')} ·{' '}
              {bytes(traffic?.uplink_total ?? service.upload_total, preferences.locale)}
            </span>
          </div>
        </section>
        <section className="space-y-5">
          <h2 className="font-medium">{t('connection')}</h2>
          <Link to="/nodes" className="group flex items-center justify-between gap-3 border-y py-5">
            <div className="min-w-0 space-y-2">
              <p className="text-xs text-muted-foreground">{t('currentNode')}</p>
              <p className="truncate font-medium">{current}</p>
              <p className="truncate text-xs text-muted-foreground">
                {service.active_group_name} ·{' '}
                {service.selector_mode === 'urltest' ? t('automatic') : t('manual')}
              </p>
            </div>
            <ArrowUpRight className="size-4 shrink-0 text-muted-foreground" />
          </Link>
          <div className="flex items-center justify-between gap-3">
            <p className="text-sm text-muted-foreground">{t('routing')}</p>
            <Choice
              label={t('routing')}
              value={service.outbound_mode}
              items={service.available_outbound_modes.map((value) => ({
                value,
                label: ['Rule', 'Proxy', 'Direct', 'RuleAllowAds'].includes(value)
                  ? t(value as MessageKey)
                  : value,
              }))}
              onChange={(value) => mode.mutate(value)}
              disabled={!api.supports('mode.write') || mode.isPending}
            />
          </div>
          {mode.isError && <ErrorState error={mode.error} compact />}
          <dl className="space-y-4 text-sm">
            <div className="flex justify-between gap-4">
              <dt className="text-muted-foreground">{t('version')}</dt>
              <dd>{info.data?.version ?? '—'}</dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-muted-foreground">{t('activeGroup')}</dt>
              <dd className="truncate">{service.active_group_name}</dd>
            </div>
          </dl>
        </section>
      </div>
      {service.error && <EmptyState title={t('failed')} description={t('requestFailed')} />}
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
              disabled={
                !api.supports('service.stop') ||
                action.isPending ||
                action.isUncertain ||
                status.isError ||
                !running ||
                connection !== 'live'
              }
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
