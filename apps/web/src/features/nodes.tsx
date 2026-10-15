import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Check, ChevronDown, Grid2X2, List, Search, Zap, ArrowUpRight } from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { moduleQueries, useSelectNode } from '@/api/queries'
import type { NodeSnapshot } from '@/api/contracts'
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
  const selection = useSelectNode()
  const [search, setSearch] = useState('')
  const [view, setView] = useState('grid')
  const [details, setDetails] = useState<Node | null>(null)
  if (snapshot.isPending) return <LoadingState />
  if (snapshot.isError)
    return <ErrorState error={snapshot.error} retry={() => void snapshot.refetch()} />
  const data = snapshot.data
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
      {selection.isError && <ErrorState error={selection.error} compact />}
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
                    <button
                      type="button"
                      className={cn('node-tile', auto && 'node-selected')}
                      aria-label={t('selectNode', { name: t('auto') })}
                      aria-pressed={auto}
                      disabled={selection.isPending}
                      onClick={() => selection.mutate({ group_id: group.id, node_tag: '' })}
                    >
                      <div className="flex min-w-0 items-center gap-2">
                        <Zap className="size-4 shrink-0" />
                        <span className="truncate font-medium">{t('auto')}</span>
                        {auto && <Check className="ml-auto size-4 shrink-0" />}
                      </div>
                      <p className="truncate text-xs text-muted-foreground">
                        {auto && data.selection.runtime_selected
                          ? displayNode(data.selection.runtime_selected)
                          : t('autoDescription')}
                      </p>
                    </button>
                  )}
                  {filtered.map((node) => {
                    const selected =
                      active && data.selection.selected_node_ref === `${group.id}/${node.tag}`
                    const delay = data.runtime_groups
                      ?.flatMap((item) => item.items)
                      .find((item) => item.tag === `${group.id}/${node.tag}`)?.url_test_delay
                    return (
                      <div
                        key={node.tag}
                        className={cn('relative min-w-0', view === 'list' && 'w-full')}
                      >
                        <button
                          type="button"
                          className={cn('node-tile w-full pr-11', selected && 'node-selected')}
                          aria-pressed={selected}
                          disabled={selection.isPending}
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
                            <span className="tabular-nums">
                              {delay ? `${delay} ms` : t('noLatency')}
                            </span>
                          </div>
                        </button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="absolute right-1.5 top-3.5"
                          aria-label={`${t('viewDetails')} ${node.tag}`}
                          onClick={() => setDetails(node)}
                        >
                          <ArrowUpRight />
                        </Button>
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
