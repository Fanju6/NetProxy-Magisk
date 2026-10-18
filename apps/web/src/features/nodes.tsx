import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Check,
  ChevronDown,
  Grid2X2,
  List,
  Search,
  MoreHorizontal,
  Plus,
  Gauge,
  LoaderCircle,
} from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { moduleQueries, useChangeMode, useNodeDelay, useSelectNode } from '@/api/queries'
import type { NodeSnapshot } from '@/api/contracts'
import { useServiceOperation } from '@/api/service-control'
import { useRoutingOperation } from '@/api/routing-control'
import { useEventConnection } from '@/api/events'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from '@/components/ui/dropdown-menu'
import { Details } from '@/components/details'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'
import { displayNode } from '@/lib/format'
import { cn } from '@/lib/utils'
import { NodeManager, type NodeManagerTarget } from './node-manager'

type Node = NodeSnapshot['groups'][number]['nodes'][number]
export function NodesPage() {
  const api = useApi()
  const { t, preferences } = usePreferences()
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const snapshot = useQuery(moduleQueries(api).nodes)
  const service = useQuery(moduleQueries(api).service)
  const selection = useSelectNode()
  const mode = useChangeMode()
  const delay = useNodeDelay()
  const serviceOperation = useServiceOperation()
  const routingOperation = useRoutingOperation()
  const connection = useEventConnection()
  const [search, setSearch] = useState('')
  const [view, setView] = useState('list')
  const [details, setDetails] = useState<{ node: Node; groupId: string } | null>(null)
  const [manager, setManager] = useState<NodeManagerTarget | null>(null)
  if (snapshot.isPending) return <LoadingState />
  if (!snapshot.data)
    return <ErrorState error={snapshot.error} retry={() => void snapshot.refetch()} />
  const data = snapshot.data
  const runtimeItems = new Map(
    data.runtime_groups?.flatMap((group) => group.items.map((item) => [item.tag, item] as const)),
  )
  const blocked =
    selection.isPending ||
    selection.isUncertain ||
    mode.isPending ||
    mode.isUncertain ||
    (serviceOperation != null && serviceOperation.phase !== 'idle') ||
    ['preparing', 'starting', 'stopping'].includes(service.data?.state ?? '') ||
    connection !== 'live' ||
    snapshot.isError
  return (
    <div className="space-y-6">
      <div className="flex min-w-0 items-center gap-2">
        <div className="relative min-w-0 flex-1">
          <Search
            aria-hidden
            className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label={t('search')}
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t('search')}
            className="h-11 pl-9"
          />
        </div>
        {(api.supports('nodes.add') || api.supports('nodes.import')) && (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="outline" size="icon" className="size-11" />}
              aria-label={text('添加节点', 'Add node')}
            >
              <Plus />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-48">
              {api.supports('nodes.add') && (
                <DropdownMenuItem onClick={() => setManager({ operation: 'add' })}>
                  {text('手动添加节点', 'Add node manually')}
                </DropdownMenuItem>
              )}
              {api.supports('nodes.import') && (
                <DropdownMenuItem onClick={() => setManager({ operation: 'import' })}>
                  {text('导入文本', 'Import text')}
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="ghost" size="icon" className="size-11" />}
            aria-label={text('显示方式', 'View options')}
          >
            <MoreHorizontal />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-40">
            <DropdownMenuRadioGroup value={view} onValueChange={setView}>
              <DropdownMenuRadioItem value="list" closeOnClick>
                <List />
                {t('list')}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="grid" closeOnClick>
                <Grid2X2 />
                {t('grid')}
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {(selection.isUncertain || selection.isUnconfirmed) && (
        <section
          role="status"
          className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-4 text-sm"
        >
          <div className="space-y-1">
            <p className="font-medium">
              {t(
                selection.isUnconfirmed
                  ? 'routingUnconfirmed'
                  : routingOperation?.target.kind === 'mode'
                    ? 'modeUncertain'
                    : 'selectionUncertain',
              )}
            </p>
            <p className="text-muted-foreground">
              {t(
                selection.isUnconfirmed
                  ? 'routingUnconfirmedDescription'
                  : selection.persisted || mode.persisted
                    ? 'persisted'
                    : routingOperation?.target.kind === 'mode'
                      ? 'modeUncertainDescription'
                      : 'selectionUncertainDescription',
              )}
            </p>
          </div>
          <Button
            variant="outline"
            className="min-h-12"
            onClick={() =>
              void (routingOperation?.target.kind === 'mode' ? mode.verify() : selection.verify())
            }
          >
            {t('checkStatus')}
          </Button>
        </section>
      )}
      {selection.error != null && <ErrorState error={selection.error} compact />}
      {mode.error != null && <ErrorState error={mode.error} compact />}
      {snapshot.isError && (
        <ErrorState compact error={snapshot.error} retry={() => void snapshot.refetch()} />
      )}
      {api.supports('nodes.delay') && (
        <div className="space-y-2 text-xs text-muted-foreground">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p>{t('delayCaption')}</p>
            {delay.isPending && (
              <Button size="sm" variant="outline" className="min-h-12" onClick={delay.cancel}>
                {t('cancelDelay')}
              </Button>
            )}
          </div>
          {delay.isPending && <p role="status">{t('testing')}</p>}
          {delay.outcome != null && (
            <p role="status">{t(delay.outcome === 'cached' ? 'delayCached' : 'delayEmpty')}</p>
          )}
        </div>
      )}
      {delay.error != null && <ErrorState compact error={delay.error} />}
      {data.groups.length === 0 && <EmptyState />}
      {data.groups.map(({ group, nodes }) => {
        const filtered = nodes.filter((node) =>
          node.tag.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
        )
        const active = data.selection.active_group_id === group.id
        const auto = active && data.selection.selector_mode === 'urltest'
        const rowClassName = cn(
          'relative min-w-0 rounded-md',
          view === 'grid' ? 'border' : 'flex items-center',
        )
        const selectionClassName = cn(
          'min-w-0 space-y-1 rounded-md px-3 text-left text-sm hover:bg-muted active:bg-muted focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-50',
          view === 'grid'
            ? 'flex h-28 w-full flex-col items-start justify-start py-3'
            : 'h-16 flex-1',
        )
        return (
          <Collapsible
            key={group.id}
            defaultOpen
            className="border-b pb-3"
            data-testid={`group-${group.id}`}
          >
            <div className="flex min-w-0 items-center gap-2">
              <CollapsibleTrigger className="group flex min-h-12 min-w-0 flex-1 items-center gap-3 py-2 text-left">
                <ChevronDown className="size-4 shrink-0 text-muted-foreground transition-transform group-data-closed:-rotate-90" />
                <span className="min-w-0 truncate font-medium">{group.name}</span>
                <span className="text-xs tabular-nums text-muted-foreground">{nodes.length}</span>
                {active && (
                  <Badge variant="secondary" className="ml-auto">
                    {t('selected')}
                  </Badge>
                )}
              </CollapsibleTrigger>
              {api.supports('nodes.delay') && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-12 shrink-0"
                  disabled={blocked || delay.isPending || nodes.length === 0}
                  aria-label={t('testGroup', { name: group.name })}
                  onClick={() => delay.mutate({ group_id: group.id, node_tag: '' })}
                >
                  {delay.isPending &&
                  delay.target?.group_id === group.id &&
                  delay.target.node_tag === '' ? (
                    <LoaderCircle className="motion-safe:animate-spin" />
                  ) : (
                    <Gauge />
                  )}
                </Button>
              )}
            </div>
            <CollapsibleContent className="pt-1">
              {nodes.length === 0 ? (
                <p className="px-3 py-2 text-sm text-muted-foreground">{t('emptyGroup')}</p>
              ) : filtered.length === 0 ? (
                <p className="px-3 py-2 text-sm text-muted-foreground">{t('noMatches')}</p>
              ) : (
                <div
                  className={cn(
                    'grid',
                    view === 'grid' ? 'grid-cols-2 gap-2 xl:grid-cols-3' : 'grid-cols-1 divide-y',
                  )}
                  data-testid="node-grid"
                >
                  {search === '' && (
                    <div className={cn(rowClassName, auto && 'node-selected')}>
                      <button
                        type="button"
                        className={selectionClassName}
                        aria-label={t('selectNode', { name: t('auto') })}
                        aria-pressed={auto}
                        disabled={!api.supports('selection.write') || blocked || delay.isPending}
                        onClick={() => selection.mutate({ group_id: group.id, node_tag: '' })}
                      >
                        <div className="flex min-w-0 items-center gap-2">
                          <span className="truncate font-medium">{t('auto')}</span>
                          <Check
                            className={cn(
                              'size-4 shrink-0',
                              view === 'grid' && 'absolute bottom-3 left-3',
                              !auto && 'invisible',
                            )}
                          />
                        </div>
                        <p className="truncate text-xs text-muted-foreground">
                          {auto && data.selection.runtime_selected
                            ? data.selection.runtime_selected.startsWith(`${group.runtime_tag}/`)
                              ? data.selection.runtime_selected.slice(group.runtime_tag.length + 1)
                              : displayNode(data.selection.runtime_selected)
                            : t('autoDescription')}
                        </p>
                      </button>
                      {view === 'list' && <div aria-hidden className="w-28 shrink-0" />}
                    </div>
                  )}
                  {filtered.map((node) => {
                    const selected =
                      active && data.selection.selected_node_ref === `${group.id}/${node.tag}`
                    const runtimeTag = `${group.runtime_tag}/${node.tag}`
                    const runtime = runtimeItems.get(runtimeTag)
                    const measured = delay.results[`${group.id}/${node.tag}`]
                    const runtimeResult = runtime
                      ? { delay: runtime.url_test_delay ?? 0, time: runtime.url_test_time ?? 0 }
                      : undefined
                    const result =
                      measured &&
                      (!runtimeResult ||
                        runtimeResult.delay <= 0 ||
                        measured.time >= runtimeResult.time)
                        ? measured
                        : runtimeResult
                    const testing =
                      delay.isPending &&
                      delay.target?.group_id === group.id &&
                      (delay.target.node_tag === '' || delay.target.node_tag === node.tag)
                    const runtimeSelected =
                      connection === 'live' &&
                      service.data?.state === 'ready' &&
                      active &&
                      data.selection.runtime_selected === runtimeTag
                    return (
                      <div key={node.tag} className={cn(rowClassName, selected && 'node-selected')}>
                        <button
                          type="button"
                          className={selectionClassName}
                          aria-pressed={selected}
                          disabled={!api.supports('selection.write') || blocked || delay.isPending}
                          aria-label={t('selectNode', { name: node.tag })}
                          onClick={() =>
                            selection.mutate({ group_id: group.id, node_tag: node.tag })
                          }
                        >
                          <div
                            className={cn(
                              'flex min-w-0 items-center gap-2',
                              view === 'grid' && 'pr-8',
                            )}
                          >
                            <span
                              className={cn(
                                'font-medium',
                                view === 'grid' ? 'line-clamp-2 leading-5' : 'truncate',
                              )}
                            >
                              {node.tag}
                            </span>
                            <Check
                              className={cn(
                                'size-4 shrink-0',
                                view === 'grid' && 'absolute bottom-3 left-3',
                                !selected && 'invisible',
                              )}
                            />
                          </div>
                          <div
                            className={cn(
                              'flex min-w-0 items-center gap-2 text-xs text-muted-foreground',
                              view === 'grid' && 'absolute bottom-9 left-3 right-3',
                            )}
                          >
                            <span className="truncate uppercase">{node.protocol}</span>
                            {runtimeSelected && (
                              <span className="truncate">{t('runtimeNode')}</span>
                            )}
                          </div>
                        </button>
                        <div
                          className={
                            view === 'list' ? 'flex w-28 shrink-0 items-center' : 'contents'
                          }
                        >
                          {api.supports('nodes.delay') && (
                            <Button
                              variant="ghost"
                              className={cn(
                                'size-12 w-16 min-w-0 px-1 text-xs font-normal tabular-nums text-muted-foreground',
                                view === 'grid' && 'absolute bottom-1 right-1 h-8',
                              )}
                              aria-label={t('testNode', { name: node.tag })}
                              title={
                                result && result.time > 0
                                  ? `${t('lastMeasured')}: ${new Date(result.time * 1000).toLocaleString()}`
                                  : t('testNode', { name: node.tag })
                              }
                              disabled={blocked || delay.isPending}
                              onClick={() =>
                                delay.mutate({ group_id: group.id, node_tag: node.tag })
                              }
                            >
                              <span className="truncate">
                                {testing
                                  ? t('testing')
                                  : result && result.delay > 0
                                    ? `${result.delay} ms`
                                    : result && result.time > 0
                                      ? t('delayFailed')
                                      : t('noLatency')}
                              </span>
                            </Button>
                          )}
                          {!api.supports('nodes.delay') && (
                            <span
                              className={cn(
                                'w-20 truncate px-1 text-center text-xs tabular-nums text-muted-foreground',
                                view === 'grid' && 'absolute bottom-3 right-1',
                              )}
                            >
                              {result && result.delay > 0 ? `${result.delay} ms` : t('noLatency')}
                            </span>
                          )}
                          <Button
                            variant="ghost"
                            size="icon"
                            className={cn(
                              'size-12',
                              view === 'grid' && 'absolute right-0 top-0 size-10',
                            )}
                            aria-label={`${t('viewDetails')} ${node.tag}`}
                            onClick={() => setDetails({ node, groupId: group.id })}
                          >
                            <MoreHorizontal />
                          </Button>
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </CollapsibleContent>
          </Collapsible>
        )
      })}
      <Details
        open={details !== null}
        onOpenChange={(open) => {
          if (!open) setDetails(null)
        }}
        title={t('nodeDetails')}
        description={details?.node.tag ?? ''}
        headerActions={
          details && (
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button variant="ghost" size="icon" />}
                aria-label={text('更多操作', 'More actions')}
              >
                <MoreHorizontal />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-40">
                {(['edit', 'export', 'remove'] as const).map((operation) => {
                  const local =
                    data.groups.find((item) => item.group.id === details.groupId)?.group.type ===
                    'local'
                  if (!api.supports(`nodes.${operation}`) || (operation !== 'export' && !local))
                    return null
                  return (
                    <DropdownMenuItem
                      key={operation}
                      variant={operation === 'remove' ? 'destructive' : 'default'}
                      onClick={() => {
                        setManager({ operation, groupId: details.groupId, tag: details.node.tag })
                        setDetails(null)
                      }}
                    >
                      {operation === 'edit'
                        ? text('编辑节点', 'Edit node')
                        : operation === 'export'
                          ? text('分享链接', 'Share link')
                          : text('删除节点', 'Delete node')}
                    </DropdownMenuItem>
                  )
                })}
              </DropdownMenuContent>
            </DropdownMenu>
          )
        }
      >
        <dl className="space-y-5 text-sm">
          <div className="flex justify-between gap-5">
            <dt className="text-muted-foreground">{t('protocol')}</dt>
            <dd className="uppercase">{details?.node.protocol}</dd>
          </div>
          <div className="space-y-2">
            <dt className="text-muted-foreground">{t('address')}</dt>
            <dd className="break-all font-mono text-xs">{details?.node.server ?? '—'}</dd>
          </div>
          <div className="flex justify-between">
            <dt className="text-muted-foreground">{t('port')}</dt>
            <dd>{details?.node.port ?? '—'}</dd>
          </div>
        </dl>
      </Details>
      {manager && <NodeManager snapshot={data} target={manager} onClose={() => setManager(null)} />}
    </div>
  )
}
