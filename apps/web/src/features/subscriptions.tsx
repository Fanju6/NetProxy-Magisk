import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChevronRight,
  Folder,
  Rss,
  RefreshCw,
  LoaderCircle,
  MoreHorizontal,
  Pencil,
  FolderPlus,
  Server,
  FileUp,
  ClipboardPaste,
  RotateCw,
  Trash2,
} from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { useAndroidImport } from '@/app/android-import-context'
import { HeaderActions } from '@/app/header-actions'
import { catalogNodesKey, resourceQueries, useResourceDetails } from '@/api/resources'
import { useEventConnection } from '@/api/events'
import { useSubscriptionActions } from '@/api/subscription-control'
import type { GroupSummary } from '@/api/contracts'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Details } from '@/components/details'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'
import { bytes } from '@/lib/format'
import { SubscriptionEditor } from './subscription-editor'
import { ResourceNodes, ResourceNodeManager } from './resource-nodes'
import { ResourceProviderTag } from './resource-provider-tag'
import type { NodeManagerTarget } from './node-manager'
import { ResourceGroupManager, type ResourceGroupTarget } from './resource-group-manager'

type SubscriptionActions = ReturnType<typeof useSubscriptionActions>
const isWaiting = (operation: SubscriptionActions['operation']) =>
  operation?.phase === 'pending' || operation?.phase === 'uncertain'

export function SubscriptionFeedback({
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
  if (
    operation?.phase === 'idle' &&
    !operation.error &&
    !pendingRuntime &&
    (!batch || failed.length === 0) &&
    (!cancelState || cancelState === 'idle') &&
    !cancellation?.error
  )
    return null
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
              className="min-h-12"
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
      {pendingRuntime && (
        <p role="status" className="text-destructive">
          {t('subscriptionSavedRuntimePending')}
        </p>
      )}
      {batch &&
        (failed.length > 0 ||
          pendingRuntime ||
          operation?.error ||
          operation?.phase !== 'idle') && (
          <div className="space-y-3">
            <p
              role="status"
              className={failed.length ? 'text-destructive' : 'text-muted-foreground'}
            >
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
                      className={
                        ok ? 'shrink-0 text-muted-foreground' : 'shrink-0 text-destructive'
                      }
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
  const current = groups.find((group) => group.id === target.id)
  const allowed = current?.type === 'subscription'
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close()
      }}
    >
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto overscroll-contain rounded-lg">
        <DialogHeader className="pr-8">
          <DialogTitle>{t('deleteSubscription')}</DialogTitle>
          <DialogDescription>{t('deleteSubscriptionConfirm')}</DialogDescription>
        </DialogHeader>
        <div className="min-w-0 space-y-4">
          <p className="break-words font-medium">{target.name}</p>
          <SubscriptionFeedback actions={actions} groups={groups} />
          {!current && (
            <p role="status" className="text-sm text-destructive">
              {t('subscriptionTargetChanged')}
            </p>
          )}
        </div>
        <DialogFooter className="flex-row flex-wrap">
          <Button variant="outline" className="min-h-11" onClick={close}>
            {t('cancel')}
          </Button>
          <Button
            variant="destructive"
            className="min-h-11"
            disabled={!allowed || connection !== 'live' || isWaiting(actions.operation)}
            onClick={() => {
              if (allowed && actions.remove(target.id)) close()
            }}
          >
            {t('deleteSubscription')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
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
  manage,
  manageGroup,
}: {
  id: string
  summary: GroupSummary | undefined
  close: () => void
  actions: SubscriptionActions
  groups: GroupSummary[]
  edit: (group: GroupSummary) => void
  remove: (group: GroupSummary) => void
  manage: (target: NodeManagerTarget) => void
  manageGroup: (target: ResourceGroupTarget) => void
}) {
  const api = useApi()
  const { t, preferences } = usePreferences()
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const cache = useQueryClient()
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
  const refresh = () =>
    Promise.allSettled([
      data.refresh(),
      cache.invalidateQueries({ queryKey: catalogNodesKey, exact: true }),
    ])
  const displayGroup = group ?? summary
  const date = (value: string) =>
    value && Number.isFinite(Date.parse(value))
      ? new Intl.DateTimeFormat(preferences.locale, {
          dateStyle: 'medium',
          timeStyle: 'short',
        }).format(new Date(value))
      : t('notProvided')
  const stage = data.progress.data?.progress
  const rows = group
    ? [
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
    : []
  return (
    <Details
      open
      onOpenChange={(open) => {
        if (!open) close()
      }}
      title={group?.name ?? summary?.name ?? t('subscriptionDetails')}
      description={displayGroup ? t(displayGroup.type) : ''}
      headerActions={
        group && (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="ghost" size="icon" className="size-11 shrink-0" />}
              aria-label={preferences.locale === 'zh' ? '更多操作' : 'More actions'}
            >
              <MoreHorizontal />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-60 max-w-[calc(100vw-2rem)]">
              <DropdownMenuItem
                className="min-h-11"
                disabled={connection !== 'live' || data.detail.isFetching}
                onClick={() => void refresh()}
              >
                <RotateCw />
                {t('refreshResources')}
              </DropdownMenuItem>
              {group.type === 'subscription' && (
                <>
                  {api.supports('subscriptions.edit') && (
                    <DropdownMenuItem
                      className="min-h-11"
                      disabled={connection !== 'live' || blocked}
                      onClick={() => edit(group)}
                    >
                      <Pencil />
                      {t('editSubscription')}
                    </DropdownMenuItem>
                  )}
                  {api.supports('subscriptions.remove') && (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        className="min-h-11"
                        disabled={connection !== 'live' || blocked}
                        onClick={() => remove(group)}
                      >
                        <Trash2 />
                        {t('deleteSubscription')}
                      </DropdownMenuItem>
                    </>
                  )}
                </>
              )}
              {group.type === 'local' && api.supports('catalog.write') && (
                <>
                  <DropdownMenuItem
                    className="min-h-11"
                    disabled={connection !== 'live' || blocked}
                    onClick={() => manageGroup({ operation: 'rename', group })}
                  >
                    <Pencil />
                    {text('重命名分组', 'Rename group')}
                  </DropdownMenuItem>
                  {group.id !== 'default' && (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        className="min-h-11"
                        disabled={connection !== 'live' || blocked}
                        onClick={() => manageGroup({ operation: 'remove', group })}
                      >
                        <Trash2 />
                        {text('删除分组', 'Delete group')}
                      </DropdownMenuItem>
                    </>
                  )}
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        )
      }
      footer={
        group?.type === 'subscription' &&
        api.supports('subscriptions.update') && (
          <Button
            className="min-h-11 flex-1"
            disabled={connection !== 'live' || blocked}
            onClick={() => actions.update(id)}
          >
            {t('updateSubscription')}
          </Button>
        )
      }
    >
      {!data.detail.data && data.detail.isError ? (
        <>
          <SubscriptionFeedback actions={actions} groups={groups} targetId={id} />
          <ErrorState error={data.detail.error} retry={() => void refresh()} />
        </>
      ) : !group ? (
        connection === 'live' ? (
          <LoadingState />
        ) : (
          <EmptyState title={t('resourceUnavailable')} />
        )
      ) : (
        <div className="space-y-6">
          {connection !== 'live' && <Badge variant="outline">{t('staleSnapshot')}</Badge>}
          {data.detail.isError && <ErrorState error={data.detail.error} compact />}
          <SubscriptionFeedback actions={actions} groups={groups} targetId={id} />
          <dl className="settings-group divide-y text-sm">
            {rows.map(([label, value]) => (
              <div key={label} className="flex min-w-0 items-baseline justify-between gap-4 py-3">
                <dt className="min-w-0 text-muted-foreground">{label}</dt>
                <dd className="min-w-0 break-words text-right">{value}</dd>
              </div>
            ))}
          </dl>
          {group.last_error && (
            <p role="status" className="text-sm text-destructive">
              {t('updateError')}
            </p>
          )}
          <ResourceNodes id={id} blocked={blocked} manage={manage} />
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
                {(waiting || stage) && api.supports('subscriptions.cancel') && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="min-h-11"
                    disabled={
                      connection !== 'live' ||
                      cancelState === 'pending' ||
                      cancelState === 'requested'
                    }
                    onClick={() => void actions.cancel(id)}
                  >
                    {t('cancelUpdate')}
                  </Button>
                )}
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
          <details className="border-t pt-4">
            <summary className="min-h-11 cursor-pointer py-3 text-sm font-medium">
              {text('高级信息', 'Advanced information')}
            </summary>
            <ResourceProviderTag group={group} />
          </details>
        </div>
      )}
    </Details>
  )
}

export function SubscriptionsPage() {
  const api = useApi()
  const androidImport = useAndroidImport()
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
    | { kind: 'node'; target: NodeManagerTarget; returnId?: string }
    | { kind: 'group'; target: ResourceGroupTarget }
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
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  return (
    <div className="space-y-6">
      {!panel && <SubscriptionFeedback actions={actions} groups={list.data} />}
      {connection !== 'live' && <Badge variant="outline">{t('staleSnapshot')}</Badge>}
      <HeaderActions>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="ghost" size="icon" className="size-12" />}
            aria-label={text('资源操作', 'Resource actions')}
          >
            <MoreHorizontal />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-60 max-w-[calc(100vw-2rem)]">
            {androidImport.available &&
              api.supports('nodes.read') &&
              api.supports('nodes.import') && (
                <DropdownMenuItem
                  className="min-h-11"
                  disabled={androidImport.busy || panel !== null || isWaiting(actions.operation)}
                  onClick={() => androidImport.pick('nodes')}
                >
                  <FileUp />
                  {text('从文件导入', 'Import from file')}
                </DropdownMenuItem>
              )}
            {api.supports('subscriptions.add') && (
              <DropdownMenuItem
                className="min-h-11"
                disabled={connection !== 'live' || isWaiting(actions.operation)}
                onClick={() => setPanel({ kind: 'add' })}
              >
                <Rss />
                {t('addSubscription')}
              </DropdownMenuItem>
            )}
            {api.supports('catalog.write') && (
              <DropdownMenuItem
                className="min-h-11"
                disabled={connection !== 'live' || isWaiting(actions.operation)}
                onClick={() => setPanel({ kind: 'group', target: { operation: 'create' } })}
              >
                <FolderPlus />
                {text('新建本地分组', 'Create local group')}
              </DropdownMenuItem>
            )}
            {api.supports('nodes.read') && api.supports('nodes.add') && (
              <DropdownMenuItem
                className="min-h-11"
                disabled={
                  connection !== 'live' ||
                  isWaiting(actions.operation) ||
                  !list.data.some((group) => group.type === 'local')
                }
                onClick={() => setPanel({ kind: 'node', target: { operation: 'add' } })}
              >
                <Server />
                {text('手动添加节点', 'Add node manually')}
              </DropdownMenuItem>
            )}
            {api.supports('nodes.read') && api.supports('nodes.import') && (
              <DropdownMenuItem
                className="min-h-11"
                disabled={connection !== 'live' || isWaiting(actions.operation)}
                onClick={() => setPanel({ kind: 'node', target: { operation: 'import' } })}
              >
                <ClipboardPaste />
                {text('导入文本', 'Import text')}
              </DropdownMenuItem>
            )}
            {(api.supports('subscriptions.add') ||
              api.supports('catalog.write') ||
              (api.supports('nodes.read') &&
                (api.supports('nodes.add') || api.supports('nodes.import')))) && (
              <DropdownMenuSeparator />
            )}
            {api.supports('subscriptions.update_all') && (
              <DropdownMenuItem
                className="min-h-11"
                disabled={
                  connection !== 'live' ||
                  isWaiting(actions.operation) ||
                  !list.data.some((group) => group.type === 'subscription')
                }
                onClick={() => actions.updateAll()}
              >
                <RefreshCw />
                {t('updateAllSubscriptions')}
              </DropdownMenuItem>
            )}
            <DropdownMenuItem
              className="min-h-11"
              disabled={connection !== 'live' || list.isFetching}
              onClick={() => void list.refetch()}
            >
              <RotateCw />
              {t('refreshResources')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </HeaderActions>
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
            <div className="divide-y border-y">
              {groups.map((group) => (
                <button
                  type="button"
                  key={group.id}
                  className="flex min-h-18 w-full min-w-0 items-center gap-3 py-3 text-left outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring active:bg-muted sm:gap-4"
                  aria-label={`${t('viewDetails')} ${group.name}`}
                  disabled={!api.supports('subscriptions.detail')}
                  onClick={() => setPanel({ kind: 'detail', id: group.id })}
                >
                  <span
                    className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted"
                    aria-hidden
                  >
                    {type === 'local' ? <Folder className="size-5" /> : <Rss className="size-5" />}
                  </span>
                  <span className="min-w-0 flex-1 space-y-2">
                    <span className="flex flex-wrap items-center gap-2">
                      <span role="heading" aria-level={3} className="min-w-0 break-all font-medium">
                        {group.name}
                      </span>
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      {t('nodeCount', { count: group.node_count })}
                      {type === 'subscription' &&
                        ` · ${t('autoUpdate')} ${t(group.auto_update ? 'enabled' : 'disabled')}`}
                    </span>
                    {group.last_error && (
                      <span className="block text-xs text-destructive">{t('updateError')}</span>
                    )}
                    {group.progress && (
                      <span className="block text-xs text-muted-foreground">
                        {t(`${group.progress.stage}Stage`)}
                      </span>
                    )}
                  </span>
                  <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground" />
                </button>
              ))}
            </div>
          </section>
        )
      })}
      {panel?.kind === 'detail' && selected && (
        <ResourceDetails
          key={selected}
          id={selected}
          summary={summary}
          close={() => setPanel(null)}
          actions={actions}
          groups={list.data}
          edit={(group) => setPanel({ kind: 'edit', group })}
          remove={(group) => setPanel({ kind: 'remove', group })}
          manage={(target) => setPanel({ kind: 'node', target, returnId: selected })}
          manageGroup={(target) => setPanel({ kind: 'group', target })}
        />
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
      {panel?.kind === 'node' && (
        <ResourceNodeManager
          target={panel.target}
          onClose={() => setPanel(panel.returnId ? { kind: 'detail', id: panel.returnId } : null)}
        />
      )}
      {panel?.kind === 'group' && (
        <ResourceGroupManager
          target={panel.target}
          groups={list.data}
          onClose={(result) => {
            const id =
              result?.id ??
              (panel.target.operation === 'create' ? undefined : panel.target.group.id)
            setPanel(result?.removed || !id ? null : { kind: 'detail', id })
          }}
        />
      )}
    </div>
  )
}
