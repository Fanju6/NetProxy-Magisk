import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowUpRight, Folder, Rss, RefreshCw, LoaderCircle } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { useApi, usePreferences } from '@/app/context'
import { resourceQueries, useResourceDetails } from '@/api/resources'
import { useEventConnection } from '@/api/events'
import type { GroupSummary } from '@/api/contracts'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Details } from '@/components/details'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'
import { bytes } from '@/lib/format'

function ResourceDetails({
  id,
  summary,
  close,
}: {
  id: string
  summary: GroupSummary | undefined
  close: () => void
}) {
  const { t, preferences } = usePreferences()
  const connection = useEventConnection()
  const data = useResourceDetails(id, summary?.type === 'subscription', connection === 'live')
  const group = data.detail.data?.group
  const date = (value: string) =>
    value && Number.isFinite(Date.parse(value))
      ? new Intl.DateTimeFormat(preferences.locale, {
          dateStyle: 'medium',
          timeStyle: 'short',
        }).format(new Date(value))
      : t('notProvided')
  const stage = data.progress.data?.progress
  if (!data.detail.data && data.detail.isError)
    return <ErrorState error={data.detail.error} retry={() => void data.refresh()} />
  if (!group)
    return connection === 'live' ? (
      <LoadingState />
    ) : (
      <EmptyState title={t('resourceUnavailable')} />
    )
  const rows = [
    [t('nodes'), String(group.node_count)],
    [
      t(group.type === 'subscription' ? 'lastUpdate' : 'modifiedAt'),
      date(group.type === 'subscription' ? group.last_success_at : group.updated_at),
    ],
    ...(group.type === 'subscription'
      ? [
          [t('autoUpdate'), t(group.auto_update ? 'enabled' : 'disabled')],
          [t('interval'), t('intervalMinutes', { count: group.update_interval / 60 })],
          [t('nextUpdate'), group.auto_update ? date(group.next_update_at) : t('disabled')],
        ]
      : []),
  ]
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        {connection !== 'live' && <Badge variant="outline">{t('staleSnapshot')}</Badge>}
        <Button
          variant="outline"
          size="sm"
          disabled={connection !== 'live' || data.detail.isFetching}
          onClick={() => void data.refresh()}
        >
          <RefreshCw />
          {t('refreshResources')}
        </Button>
      </div>
      {data.detail.isError && <ErrorState error={data.detail.error} compact />}
      <dl className="space-y-4 text-sm">
        {rows.map(([label, value]) => (
          <div key={label} className="flex justify-between gap-4">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="text-right">{value}</dd>
          </div>
        ))}
      </dl>
      {group.last_error && (
        <p role="status" className="text-sm text-destructive">
          {t('updateError')}
        </p>
      )}
      {group.type === 'subscription' && (
        <>
          <section className="space-y-3 border-t pt-5">
            <h3 className="text-sm font-medium">{t('subscriptionUsage')}</h3>
            {group.usage === null ? (
              <p className="text-sm text-muted-foreground">{t('notProvided')}</p>
            ) : (
              <dl className="space-y-3 text-sm">
                <div className="flex justify-between gap-4">
                  <dt>{t('usedTraffic')}</dt>
                  <dd>
                    {group.usage.upload !== undefined || group.usage.download !== undefined
                      ? bytes(
                          (group.usage.upload ?? 0) + (group.usage.download ?? 0),
                          preferences.locale,
                        )
                      : t('notProvided')}
                  </dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt>{t('trafficLimit')}</dt>
                  <dd>
                    {group.usage.total === undefined
                      ? t('notProvided')
                      : bytes(group.usage.total, preferences.locale)}
                  </dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt>{t('expiresAt')}</dt>
                  <dd>
                    {group.usage.expire &&
                    Number.isFinite(new Date(group.usage.expire * 1000).getTime())
                      ? date(new Date(group.usage.expire * 1000).toISOString())
                      : t('notProvided')}
                  </dd>
                </div>
              </dl>
            )}
          </section>
          <section className="space-y-3 border-t pt-5">
            <h3 className="text-sm font-medium">{t('updateProgress')}</h3>
            {data.progress.isError ? (
              <ErrorState error={data.progress.error} compact />
            ) : data.progress.isPending ? (
              <LoadingState />
            ) : stage ? (
              <p role="status" className="flex items-center gap-2 text-sm">
                {data.observing && <LoaderCircle className="size-4 motion-safe:animate-spin" />}
                {t(`${stage.stage}Stage`)} · {date(stage.updated_at)}
              </p>
            ) : (
              <p className="text-sm text-muted-foreground">{t('noUpdateInProgress')}</p>
            )}
            <p className="text-xs text-muted-foreground">
              {t(data.refreshLimitReached ? 'progressLimit' : 'progressCaution')}
            </p>
          </section>
          <section className="space-y-3 border-t pt-5">
            <h3 className="text-sm font-medium">{t('updateHistory')}</h3>
            {data.history.isError ? (
              <ErrorState error={data.history.error} compact />
            ) : data.history.isPending ? (
              <LoadingState />
            ) : data.history.data?.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('noHistory')}</p>
            ) : (
              <ol className="divide-y">
                {data.history.data
                  ?.slice()
                  .reverse()
                  .map((entry, index) => (
                    <li
                      key={`${entry.at}-${entry.code}-${index}`}
                      className="space-y-2 py-3 text-sm"
                    >
                      <div className="flex flex-wrap justify-between gap-2">
                        <span className={entry.ok ? '' : 'text-destructive'}>
                          {t(
                            !entry.ok
                              ? 'historyFailure'
                              : entry.code === 'subscription.not_modified'
                                ? 'historyUnchanged'
                                : entry.code === 'subscription.runtime_sync_applied'
                                  ? 'historyRuntime'
                                  : 'historySuccess',
                          )}
                        </span>
                        <time className="text-xs text-muted-foreground">{date(entry.at)}</time>
                      </div>
                      <p className="break-words text-xs text-muted-foreground">{entry.code}</p>
                      {entry.node_count !== undefined && (
                        <p className="text-xs text-muted-foreground">
                          {t('nodeCount', { count: entry.node_count })}
                        </p>
                      )}
                    </li>
                  ))}
              </ol>
            )}
          </section>
        </>
      )}
      <Button nativeButton={false} render={<Link to="/nodes" />} onClick={close}>
        {t('viewGroup')}
        <ArrowUpRight />
      </Button>
    </div>
  )
}

export function SubscriptionsPage() {
  const api = useApi()
  const { t, preferences } = usePreferences()
  const connection = useEventConnection()
  const list = useQuery({
    ...resourceQueries(api).catalog,
    enabled: connection === 'live' && api.supports('catalog.read'),
    staleTime: 0,
  })
  const [selected, setSelected] = useState<string | null>(null)
  if (!list.data && list.isError)
    return <ErrorState error={list.error} retry={() => void list.refetch()} />
  if (!list.data)
    return connection === 'offline' || connection === 'paused' ? (
      <EmptyState title={t('resourceUnavailable')} />
    ) : (
      <LoadingState />
    )
  const summary = list.data.find((group) => group.id === selected)
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-end gap-3">
        {connection !== 'live' && <Badge variant="outline">{t('staleSnapshot')}</Badge>}
        <Button
          variant="ghost"
          size="icon"
          className="size-11"
          aria-label={t('refreshResources')}
          disabled={connection !== 'live' || list.isFetching}
          onClick={() => void list.refetch()}
        >
          <RefreshCw />
        </Button>
      </div>
      {list.isError && <ErrorState error={list.error} compact retry={() => void list.refetch()} />}
      {list.data.length === 0 && <EmptyState />}
      {(['local', 'subscription'] as const).map((type) => {
        const groups = list.data.filter((group) => group.type === type)
        if (!groups.length) return null
        return (
          <section key={type} className="space-y-3">
            <h2 className="text-sm font-medium text-muted-foreground">
              {t(type === 'local' ? 'localResources' : 'subscriptionResources')}
            </h2>
            <div className="divide-y rounded-lg border">
              {groups.map((group) => (
                <article
                  key={group.id}
                  className="flex min-w-0 items-start gap-3 p-4 sm:gap-4 sm:p-5"
                >
                  <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted">
                    {type === 'local' ? <Folder className="size-5" /> : <Rss className="size-5" />}
                  </div>
                  <div className="min-w-0 flex-1 space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <h3 className="break-words font-medium">{group.name}</h3>
                      {group.active && <Badge variant="secondary">{t('activeGroup')}</Badge>}
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {t('nodeCount', { count: group.node_count })}
                      {type === 'subscription' &&
                        ` · ${t('autoUpdate')} ${t(group.auto_update ? 'enabled' : 'disabled')}`}
                    </p>
                    {group.updated_at && Number.isFinite(Date.parse(group.updated_at)) && (
                      <p className="text-xs text-muted-foreground">
                        {new Intl.DateTimeFormat(preferences.locale, {
                          dateStyle: 'medium',
                          timeStyle: 'short',
                        }).format(new Date(group.updated_at))}
                      </p>
                    )}
                    {group.last_error && (
                      <p className="text-xs text-destructive">{t('updateError')}</p>
                    )}
                    {group.progress && (
                      <p className="text-xs text-muted-foreground">
                        {t(`${group.progress.stage}Stage`)}
                      </p>
                    )}
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-11 shrink-0"
                    aria-label={`${t('viewDetails')} ${group.name}`}
                    disabled={!api.supports('subscriptions.detail')}
                    onClick={() => setSelected(group.id)}
                  >
                    <ArrowUpRight />
                  </Button>
                </article>
              ))}
            </div>
          </section>
        )
      })}
      <Details
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) setSelected(null)
        }}
        title={summary?.name ?? t('subscriptionDetails')}
        description={summary ? t(summary.type) : ''}
      >
        {selected && (
          <ResourceDetails
            key={selected}
            id={selected}
            summary={summary}
            close={() => setSelected(null)}
          />
        )}
      </Details>
    </div>
  )
}
