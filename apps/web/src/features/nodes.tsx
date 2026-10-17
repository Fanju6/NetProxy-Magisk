import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Check,
  ChevronDown,
  Grid2X2,
  List,
  Search,
  Zap,
  ArrowUpRight,
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

type Node = NodeSnapshot['groups'][number]['nodes'][number]
export function NodesPage() {
  const api = useApi()
  const { t } = usePreferences()
  const snapshot = useQuery(moduleQueries(api).nodes)
  const service = useQuery(moduleQueries(api).service)
  const selection = useSelectNode()
  const mode = useChangeMode()
  const delay = useNodeDelay()
  const serviceOperation = useServiceOperation()
  const routingOperation = useRoutingOperation()
  const connection = useEventConnection()
  const [search, setSearch] = useState('')
  const [view, setView] = useState('grid')
  const [details, setDetails] = useState<Node | null>(null)
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
      <div className="flex items-center gap-3">
        <div className="relative min-w-0 flex-1 sm:max-w-sm">
          <Search
            aria-hidden
            className="pointer-events-none absolute left-3 top-3 size-4 text-muted-foreground"
          />
          <Input
            aria-label={t('search')}
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t('search')}
            className="h-10 pl-9"
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
          <ToggleGroupItem value="grid" aria-label={t('grid')}>
            <Grid2X2 />
          </ToggleGroupItem>
          <ToggleGroupItem value="list" aria-label={t('list')}>
            <List />
          </ToggleGroupItem>
        </ToggleGroup>
      </div>
      {selection.isUncertain && (
        <section
          role="status"
          className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-4 text-sm"
        >
          <div className="space-y-1">
            <p className="font-medium">
              {t(routingOperation?.target.kind === 'mode' ? 'modeUncertain' : 'selectionUncertain')}
            </p>
            <p className="text-muted-foreground">
              {t(
                selection.persisted || mode.persisted
                  ? 'persisted'
                  : routingOperation?.target.kind === 'mode'
                    ? 'modeUncertainDescription'
                    : 'selectionUncertainDescription',
              )}
            </p>
          </div>
          <Button
            variant="outline"
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
              <Button size="sm" variant="outline" onClick={delay.cancel}>
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
            <CollapsibleTrigger className="group flex min-h-12 w-full items-center gap-3 py-2 text-left">
              <ChevronDown className="size-4 shrink-0 text-muted-foreground transition-transform group-data-closed:-rotate-90" />
              <span className="min-w-0 truncate font-medium">{group.name}</span>
              <span className="text-xs tabular-nums text-muted-foreground">{nodes.length}</span>
              {active && (
                <Badge variant="secondary" className="ml-auto">
                  {t('selected')}
                </Badge>
              )}
            </CollapsibleTrigger>
            <CollapsibleContent className="pt-4">
              {nodes.length === 0 ? (
                <EmptyState title={t('emptyGroup')} />
              ) : filtered.length === 0 ? (
                <EmptyState title={t('noMatches')} />
              ) : (
                <div
                  className={cn(
                    'grid gap-3',
                    view === 'grid' ? 'grid-cols-2 xl:grid-cols-3' : 'grid-cols-1',
                  )}
                  data-testid="node-grid"
                >
                  {search === '' && (
                    <div className="min-w-0 space-y-1">
                      <button
                        type="button"
                        className={cn('node-tile w-full', auto && 'node-selected')}
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
                      {api.supports('nodes.delay') && (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="min-h-10 w-full"
                          disabled={blocked || delay.isPending}
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
                          {t('testGroupShort')}
                        </Button>
                      )}
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
                        className={cn('min-w-0 space-y-1', view === 'list' && 'w-full')}
                      >
                        <button
                          type="button"
                          className={cn('node-tile w-full', selected && 'node-selected')}
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
                        </button>
                        <div className="flex min-h-10 items-center gap-1">
                          {runtimeSelected && (
                            <span className="min-w-0 truncate pl-2 text-xs text-muted-foreground">
                              {t('runtimeNode')}
                            </span>
                          )}
                          <div className="ml-auto flex shrink-0 items-center">
                            {api.supports('nodes.delay') && (
                              <Button
                                variant="ghost"
                                size="icon"
                                className="size-10"
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
                              className="size-10"
                              aria-label={`${t('viewDetails')} ${node.tag}`}
                              onClick={() => setDetails(node)}
                            >
                              <ArrowUpRight />
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
        description={details?.tag ?? ''}
      >
        <dl className="space-y-5 text-sm">
          <div className="flex justify-between gap-5">
            <dt className="text-muted-foreground">{t('protocol')}</dt>
            <dd className="uppercase">{details?.protocol}</dd>
          </div>
          <div className="space-y-2">
            <dt className="text-muted-foreground">{t('address')}</dt>
            <dd className="break-all font-mono text-xs">{details?.server ?? '—'}</dd>
          </div>
          <div className="flex justify-between">
            <dt className="text-muted-foreground">{t('port')}</dt>
            <dd>{details?.port ?? '—'}</dd>
          </div>
        </dl>
      </Details>
    </div>
  )
}
