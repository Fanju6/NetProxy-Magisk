import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowUpRight, Folder, Rss, RefreshCw, LoaderCircle, Plus } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { useApi, usePreferences } from '@/app/context'
import { resourceQueries, useResourceDetails } from '@/api/resources'
import { useEventConnection } from '@/api/events'
import { useSubscriptionActions } from '@/api/subscription-control'
import type { GroupSummary } from '@/api/contracts'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Details } from '@/components/details'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'
import { bytes } from '@/lib/format'
import { SubscriptionEditor } from './subscription-editor'

type SubscriptionActions = ReturnType<typeof useSubscriptionActions>
const isWaiting = (operation: SubscriptionActions['operation']) =>
  operation?.phase === 'pending' || operation?.phase === 'uncertain'

function SubscriptionFeedback({
  actions,
  groups,
  targetId,
}: {
  actions: SubscriptionActions
  groups: GroupSummary[]
  targetId?: string
}) {
  const { t } = usePreferences()
  const connection = useEventConnection()
  const operation =
    !targetId ||
    actions.operation?.groupId === targetId ||
    actions.operation?.action === 'update_all'
      ? actions.operation
      : null
  const cancellation =
    actions.cancellation && (!targetId || actions.cancellation.groupId === targetId)
      ? actions.cancellation
      : null
  const cancelState =
    cancellation?.state ??
    (!targetId || operation?.groupId === targetId ? operation?.cancel : undefined)
  if (!operation && !cancellation) return null
  const result = operation?.result
  const batch = result && 'updated' in result && 'failed' in result ? result : null
  const updated = batch?.updated ?? []
  const failed = batch?.failed ?? []
  const pendingRuntime = result && 'runtime_sync_pending' in result && result.runtime_sync_pending
  return (
    <section aria-label={t('subscriptionOperation')} className="space-y-3 text-sm">
      {operation && operation.phase !== 'idle' && (
        <div className="flex flex-wrap items-center gap-3">
          <p role="status">
            {t(
              operation.phase === 'pending'
                ? 'subscriptionPending'
                : operation.phase === 'uncertain'
                  ? 'subscriptionUncertain'
                  : 'subscriptionUnconfirmed',
            )}
          </p>
          {operation.phase !== 'pending' && (
            <Button
              variant="outline"
              size="sm"
              className="min-h-11"
              disabled={connection !== 'live'}
              onClick={() => void actions.verify()}
            >
              {t('reviewCurrentState')}
            </Button>
          )}
        </div>
      )}
      {operation?.error && <ErrorState error={operation.error} compact />}
      {cancelState === 'pending' && (
        <p role="status" className="text-muted-foreground">
          {t('cancelPending')}
        </p>
      )}
      {cancelState === 'requested' && (
        <p role="status" className="text-muted-foreground">
          {t('cancelRequested')}
        </p>
      )}
      {cancelState === 'unknown' && (
        <p role="status" className="text-muted-foreground">
          {t('cancelUnknown')}
        </p>
      )}
      {cancellation?.error && cancellation.error !== operation?.error && (
        <ErrorState error={cancellation.error} compact />
      )}
      {operation && result && !batch && (
        <p role="status" className={pendingRuntime ? 'text-destructive' : 'text-muted-foreground'}>
          {t(
            pendingRuntime
              ? 'subscriptionSavedRuntimePending'
              : operation.action === 'remove'
                ? 'subscriptionDeleted'
                : operation.action === 'add' || operation.action === 'edit'
                  ? 'subscriptionSaved'
                  : 'not_modified' in result && result.not_modified
                    ? 'subscriptionNotModified'
                    : 'subscriptionUpdated',
          )}
        </p>
      )}
      {batch && (
        <div className="space-y-3">
          <p role="status" className={failed.length ? 'text-destructive' : 'text-muted-foreground'}>
            {t(failed.length ? 'updateAllPartial' : 'updateAllCompleted')}
          </p>
          <ul className="divide-y">
            {[
              ...updated.map((id) => ({ id, ok: true })),
              ...failed.map((id) => ({ id, ok: false })),
            ]
              .filter(({ id }) => !targetId || id === targetId)
              .map(({ id, ok }) => (
                <li key={id} className="flex justify-between gap-4 py-2">
                  <span className="min-w-0 break-words">
                    {groups.find((group) => group.id === id)?.name ?? t('subscription')}
                  </span>
                  <span
                    className={ok ? 'shrink-0 text-muted-foreground' : 'shrink-0 text-destructive'}
                  >
                    {t(ok ? 'historySuccess' : 'historyFailure')}
                  </span>
                </li>
              ))}
          </ul>
        </div>
      )}
    </section>
  )
}

function DeleteSubscription({
  target,
  groups,
  actions,
  close,
}: {
  target: GroupSummary
  groups: GroupSummary[]
  actions: SubscriptionActions
  close: () => void
}) {
  const { t } = usePreferences()
  const connection = useEventConnection()
  const [replacement, setReplacement] = useState('')
  const current = groups.find((group) => group.id === target.id)
  const candidates = groups.filter((group) => group.id !== target.id && group.node_count > 0)
  const replacementValid = candidates.some((group) => group.id === replacement)
  const allowed = current?.type === 'subscription' && (!current.active || replacementValid)
  return (
    <Details
      open
      onOpenChange={(open) => {
        if (!open) close()
      }}
      title={t('deleteSubscription')}
      description={t('deleteSubscriptionConfirm')}
    >
      <div className="space-y-5">
        <p className="break-words font-medium">{target.name}</p>
        <SubscriptionFeedback actions={actions} groups={groups} />
        {current?.active && (
          <div className="space-y-2">
            <label htmlFor="subscription-replacement" className="text-sm font-medium">
              {t('replacementGroup')}
            </label>
            <Select value={replacement} onValueChange={(value) => setReplacement(value ?? '')}>
              <SelectTrigger
                id="subscription-replacement"
                className="min-h-11 w-full text-base md:text-sm"
              >
                <SelectValue placeholder={t('chooseReplacement')} />
              </SelectTrigger>
              <SelectContent>
                {candidates.map((group) => (
                  <SelectItem key={group.id} value={group.id}>
                    {group.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {!candidates.length && (
              <p role="status" className="text-sm text-muted-foreground">
                {t('noReplacementGroup')}
              </p>
            )}
          </div>
        )}
        {!current && (
          <p role="status" className="text-sm text-destructive">
            {t('subscriptionTargetChanged')}
          </p>
        )}
        <Button
          variant="destructive"
          className="min-h-11 w-full"
          disabled={!allowed || connection !== 'live' || isWaiting(actions.operation)}
          onClick={() => {
            if (allowed && actions.remove(target.id, current?.active ? replacement : undefined))
              close()
          }}
        >
          {t('deleteSubscription')}
        </Button>
      </div>
    </Details>
  )
}

function ResourceDetails({
  id,
  summary,
  close,
  actions,
  groups,
  edit,
  remove,
}: {
  id: string
  summary: GroupSummary | undefined
  close: () => void
  actions: SubscriptionActions
  groups: GroupSummary[]
  edit: (group: GroupSummary) => void
  remove: (group: GroupSummary) => void
}) {
  const api = useApi()
  const { t, preferences } = usePreferences()
  const connection = useEventConnection()
  const operation = actions.operation?.groupId === id ? actions.operation : null
  const observation = actions.operation?.action === 'update_all' ? actions.operation : operation
  const cancellation = actions.cancellation?.groupId === id ? actions.cancellation : null
  const cancelState = cancellation?.state ?? operation?.cancel
  const waiting = operation?.phase === 'pending' || operation?.phase === 'uncertain'
  const blocked = isWaiting(actions.operation)
  const data = useResourceDetails(
    id,
    summary?.type === 'subscription',
    connection === 'live',
    observation && isWaiting(observation) ? observation.deadline : undefined,
  )
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
    return (
      <>
        <SubscriptionFeedback actions={actions} groups={groups} targetId={id} />
        <ErrorState error={data.detail.error} retry={() => void data.refresh()} />
      </>
    )
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
      {group.type === 'subscription' && (
        <div className="flex flex-wrap gap-2">
          {api.supports('subscriptions.update') && (
            <Button
              className="min-h-11"
              disabled={connection !== 'live' || blocked}
              onClick={() => actions.update(id)}
            >
              {t('updateSubscription')}
            </Button>
          )}
          {(waiting || stage) && api.supports('subscriptions.cancel') && (
            <Button
              variant="outline"
              className="min-h-11"
              disabled={
                connection !== 'live' || cancelState === 'pending' || cancelState === 'requested'
              }
              onClick={() => void actions.cancel(id)}
            >
              {t('cancelUpdate')}
            </Button>
          )}
          {api.supports('subscriptions.edit') && (
            <Button
              variant="outline"
              className="min-h-11"
              disabled={connection !== 'live' || blocked}
              onClick={() => edit(group)}
            >
              {t('editSubscription')}
            </Button>
          )}
          {api.supports('subscriptions.remove') && (
            <Button
              variant="outline"
              className="min-h-11 text-destructive"
              disabled={connection !== 'live' || blocked}
              onClick={() => remove(group)}
            >
              {t('deleteSubscription')}
            </Button>
          )}
        </div>
      )}
      <SubscriptionFeedback actions={actions} groups={groups} targetId={id} />
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
  const actions = useSubscriptionActions()
  const list = useQuery({
    ...resourceQueries(api).catalog,
    enabled: connection === 'live' && api.supports('catalog.read'),
    staleTime: 0,
  })
  const [panel, setPanel] = useState<
    | { kind: 'detail'; id: string }
    | { kind: 'add' }
    | { kind: 'edit' | 'remove'; group: GroupSummary }
    | null
  >(null)
  const selected = panel?.kind === 'detail' ? panel.id : null
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
      {!panel && <SubscriptionFeedback actions={actions} groups={list.data} />}
      <div className="flex flex-wrap items-center justify-end gap-3">
        {connection !== 'live' && <Badge variant="outline">{t('staleSnapshot')}</Badge>}
        {api.supports('subscriptions.add') && (
          <Button
            className="min-h-11"
            disabled={connection !== 'live' || isWaiting(actions.operation)}
            onClick={() => setPanel({ kind: 'add' })}
          >
            <Plus />
            {t('addSubscription')}
          </Button>
        )}
        {api.supports('subscriptions.update_all') && (
          <Button
            variant="outline"
            className="min-h-11"
            disabled={
              connection !== 'live' ||
              isWaiting(actions.operation) ||
              !list.data.some((group) => group.type === 'subscription')
            }
            onClick={() => actions.updateAll()}
          >
            {t('updateAllSubscriptions')}
          </Button>
        )}
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
      {!panel && list.isError && (
        <ErrorState error={list.error} compact retry={() => void list.refetch()} />
      )}
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
                    onClick={() => setPanel({ kind: 'detail', id: group.id })}
                  >
                    <ArrowUpRight />
                  </Button>
                </article>
              ))}
            </div>
          </section>
        )
      })}
      {panel?.kind === 'detail' && (
        <Details
          open
          onOpenChange={(open) => {
            if (!open) setPanel(null)
          }}
          title={summary?.name ?? t('subscriptionDetails')}
          description={summary ? t(summary.type) : ''}
        >
          {selected && (
            <ResourceDetails
              key={selected}
              id={selected}
              summary={summary}
              close={() => setPanel(null)}
              actions={actions}
              groups={list.data}
              edit={(group) => setPanel({ kind: 'edit', group })}
              remove={(group) => setPanel({ kind: 'remove', group })}
            />
          )}
        </Details>
      )}
      {(panel?.kind === 'add' || panel?.kind === 'edit') && (
        <SubscriptionEditor
          key={panel.kind === 'edit' ? panel.group.id : 'new'}
          {...(panel.kind === 'edit' ? { group: panel.group } : {})}
          busy={isWaiting(actions.operation)}
          disabled={connection !== 'live'}
          feedback={<SubscriptionFeedback actions={actions} groups={list.data} />}
          onSubmit={(input) =>
            panel.kind === 'edit' ? actions.edit(panel.group.id, input) : actions.add(input)
          }
          onClose={() => setPanel(null)}
        />
      )}
      {panel?.kind === 'remove' && (
        <DeleteSubscription
          key={panel.group.id}
          target={panel.group}
          groups={list.data}
          actions={actions}
          close={() => setPanel(null)}
        />
      )}
    </div>
  )
}
