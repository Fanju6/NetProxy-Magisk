import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Menu } from '@base-ui/react/menu'
import {
  Check,
  ChevronDown,
  Grid2X2,
  List,
  Search,
  Zap,
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
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
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
      <div className="flex flex-wrap items-center gap-3">
        {(api.supports('nodes.add') || api.supports('nodes.import')) && (
          <Menu.Root>
            <Menu.Trigger render={<Button className="min-h-12" />}>
              <Plus />
              {text('添加节点', 'Add node')}
            </Menu.Trigger>
            <Menu.Portal>
              <Menu.Positioner sideOffset={4} className="z-50">
                <Menu.Popup className="min-w-48 max-w-[calc(100vw-2rem)] rounded-md border bg-popover p-1 text-popover-foreground shadow-md outline-none">
                  {api.supports('nodes.add') && (
                    <Menu.Item
                      className="flex min-h-12 cursor-pointer items-center rounded-sm px-3 text-sm outline-none data-highlighted:bg-muted"
                      onClick={() => setManager({ operation: 'add' })}
                    >
                      {text('手动添加节点', 'Add node manually')}
                    </Menu.Item>
                  )}
                  {api.supports('nodes.import') && (
                    <Menu.Item
                      className="flex min-h-12 cursor-pointer items-center rounded-sm px-3 text-sm outline-none data-highlighted:bg-muted"
                      onClick={() => setManager({ operation: 'import' })}
                    >
                      {text('导入文本', 'Import text')}
                    </Menu.Item>
                  )}
                </Menu.Popup>
              </Menu.Positioner>
            </Menu.Portal>
          </Menu.Root>
        )}
        <div className="relative min-w-0 flex-1 sm:max-w-sm">
          <Search
            aria-hidden
            className="pointer-events-none absolute left-3 top-4 size-4 text-muted-foreground"
          />
          <Input
            aria-label={t('search')}
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t('search')}
            className="h-12 pl-9"
          />
        </div>
        <ToggleGroup
          value={[view]}
          onValueChange={(values) => {
            if (values[0]) setView(values[0])
          }}
          variant="outline"
          aria-label={t('nodes')}
        >
          <ToggleGroupItem value="grid" aria-label={t('grid')} className="size-12">
            <Grid2X2 />
          </ToggleGroupItem>
          <ToggleGroupItem value="list" aria-label={t('list')} className="size-12">
            <List />
          </ToggleGroupItem>
        </ToggleGroup>
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
        return (
          <Collapsible
            key={group.id}
            defaultOpen
            className="border-b pb-6"
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
            <CollapsibleContent className="pt-4">
              {nodes.length === 0 ? (
                <EmptyState title={t('emptyGroup')} />
              ) : filtered.length === 0 ? (
                <EmptyState title={t('noMatches')} />
              ) : (
                <div
                  className={cn(
                    'grid',
                    view === 'grid' ? 'grid-cols-2 gap-3 xl:grid-cols-3' : 'grid-cols-1 divide-y',
                  )}
                  data-testid="node-grid"
                >
                  {search === '' && (
                    <div className="min-w-0 space-y-1">
                      <button
                        type="button"
                        className={cn(
                          view === 'grid'
                            ? 'node-tile w-full'
                            : 'min-h-16 w-full space-y-1 rounded-md px-3 py-3 text-left hover:bg-muted active:bg-muted',
                          auto && 'node-selected',
                        )}
                        aria-label={t('selectNode', { name: t('auto') })}
                        aria-pressed={auto}
                        disabled={!api.supports('selection.write') || blocked || delay.isPending}
                        onClick={() => selection.mutate({ group_id: group.id, node_tag: '' })}
                      >
                        <div className="flex min-w-0 items-center gap-2">
                          <Zap className="size-4 shrink-0" />
                          <span className="truncate font-medium">{t('auto')}</span>
                          {auto && <Check className="ml-auto size-4 shrink-0" />}
                        </div>
                        <p className="truncate text-xs text-muted-foreground">
                          {auto && data.selection.runtime_selected
                            ? data.selection.runtime_selected.startsWith(`${group.runtime_tag}/`)
                              ? data.selection.runtime_selected.slice(group.runtime_tag.length + 1)
                              : displayNode(data.selection.runtime_selected)
                            : t('autoDescription')}
                        </p>
                      </button>
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
                      <div
                        key={node.tag}
                        className={cn(
                          'min-w-0',
                          view === 'list' ? 'flex w-full items-center' : 'space-y-1',
                        )}
                      >
                        <button
                          type="button"
                          className={cn(
                            view === 'grid'
                              ? 'node-tile w-full'
                              : 'min-h-16 min-w-0 flex-1 space-y-1 rounded-md px-3 py-3 text-left hover:bg-muted active:bg-muted',
                            selected && 'node-selected',
                          )}
                          aria-pressed={selected}
                          disabled={!api.supports('selection.write') || blocked || delay.isPending}
                          aria-label={t('selectNode', { name: node.tag })}
                          onClick={() =>
                            selection.mutate({ group_id: group.id, node_tag: node.tag })
                          }
                        >
                          <div className="flex min-w-0 items-center gap-2">
                            <span className="truncate font-medium">{node.tag}</span>
                            {selected && <Check className="size-4 shrink-0" />}
                          </div>
                          <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                            <span className="uppercase">{node.protocol}</span>
                            <span
                              className="truncate tabular-nums"
                              title={
                                result && result.time > 0
                                  ? `${t('lastMeasured')}: ${new Date(result.time * 1000).toLocaleString()}`
                                  : undefined
                              }
                            >
                              {result && result.delay > 0
                                ? `${result.delay} ms`
                                : testing
                                  ? t('testing')
                                  : result && result.time > 0
                                    ? t('delayFailed')
                                    : t('noLatency')}
                            </span>
                          </div>
                          {runtimeSelected && (
                            <p className="truncate text-xs text-muted-foreground">
                              {t('runtimeNode')}
                            </p>
                          )}
                        </button>
                        <div className="flex min-h-12 shrink-0 items-center gap-1">
                          <div className="ml-auto flex shrink-0 items-center">
                            {api.supports('nodes.delay') && (
                              <Button
                                variant="ghost"
                                size="icon"
                                className="size-12"
                                aria-label={t('testNode', { name: node.tag })}
                                disabled={blocked || delay.isPending}
                                onClick={() =>
                                  delay.mutate({ group_id: group.id, node_tag: node.tag })
                                }
                              >
                                {testing ? (
                                  <LoaderCircle className="motion-safe:animate-spin" />
                                ) : (
                                  <Gauge />
                                )}
                              </Button>
                            )}
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-12"
                              aria-label={`${t('viewDetails')} ${node.tag}`}
                              onClick={() => setDetails({ node, groupId: group.id })}
                            >
                              <MoreHorizontal />
                            </Button>
                          </div>
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
        {details && (
          <div className="mt-6 divide-y border-t">
            {(['edit', 'export', 'remove'] as const).map((operation) => {
              const local =
                data.groups.find((item) => item.group.id === details.groupId)?.group.type ===
                'local'
              if (!api.supports(`nodes.${operation}`) || (operation !== 'export' && !local))
                return null
              return (
                <Button
                  key={operation}
                  variant="ghost"
                  className={cn(
                    'min-h-12 w-full justify-start whitespace-normal text-left',
                    operation === 'remove' && 'text-destructive',
                  )}
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
                </Button>
              )
            })}
          </div>
        )}
      </Details>
      {manager && <NodeManager snapshot={data} target={manager} onClose={() => setManager(null)} />}
    </div>
  )
}
