import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Check,
  ChevronDown,
  ChevronRight,
  Gauge,
  Grid2X2,
  List,
  MoreHorizontal,
  Plus,
  Search,
} from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import {
  moduleQueries,
  useSelectPolicyGroup,
  usePolicyDelay,
  useChangeMode,
  useServiceAction,
} from '@/api/queries'
import { useEventConnection } from '@/api/events'
import { useRoutingOperation } from '@/api/routing-control'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'
import { cn } from '@/lib/utils'
import { PolicyEditor } from './policy-editor'

export function NodesPage() {
  const api = useApi()
  const { t, preferences } = usePreferences()
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const service = useQuery(moduleQueries(api).service)
  const snapshot = useQuery(moduleQueries(api).policyGroups)
  const connection = useEventConnection()
  const selection = useSelectPolicyGroup()
  const operation = useRoutingOperation()
  const mode = useChangeMode()
  const action = useServiceAction()
  const delay = usePolicyDelay()
  const [search, setSearch] = useState('')
  const [view, setView] = useState('list')
  const [presentation, setPresentation] = useState('expanded')
  const [activeTag, setActiveTag] = useState('')
  const [editor, setEditor] = useState<{ tag: string | null; remove?: boolean } | null>(null)
  if (!api.supports('policy.read') && !snapshot.data)
    return (
      <EmptyState
        title={t('proxies')}
        description={text(
          '此连接不支持策略组',
          'Policy groups are not available for this connection',
        )}
      />
    )
  if (snapshot.isPending) return <LoadingState />
  if (!snapshot.data)
    return <ErrorState error={snapshot.error} retry={() => void snapshot.refetch()} />
  const data = snapshot.data
  const live =
    connection === 'live' &&
    data.live &&
    service.data?.state === 'ready' &&
    data.core_started_at_millis === service.data.core_started_at_millis
  const blocked =
    !live ||
    snapshot.isError ||
    selection.isPending ||
    selection.isUncertain ||
    action.isPending ||
    action.isUncertain ||
    delay.isPending
  const selectedTab = data.groups.some((group) => group.tag === activeTag)
    ? activeTag
    : data.groups[0]?.tag
  const query = search.toLocaleLowerCase()
  const nested = (tag: string) => {
    setSearch('')
    setActiveTag(tag)
    setPresentation('tabs')
  }
  return (
    <div className="max-w-5xl space-y-4">
      <div className="flex items-center gap-2">
        <div className="relative min-w-0 flex-1">
          <Search className="pointer-events-none absolute left-3 top-3.5 size-4 text-muted-foreground" />
          <Input
            type="search"
            aria-label={text('搜索代理', 'Search proxies')}
            placeholder={text('搜索代理', 'Search proxies')}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className="h-11 pl-9"
          />
        </div>
        {api.supports('config.read') && api.supports('config.write') && (
          <Button
            variant="outline"
            size="icon"
            className="size-11"
            aria-label={text('新增策略组', 'Add policy group')}
            onClick={() => setEditor({ tag: null })}
          >
            <Plus />
          </Button>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="ghost" size="icon" className="size-11" />}
            aria-label={text('显示方式', 'View options')}
          >
            <MoreHorizontal />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
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
            <DropdownMenuRadioGroup value={presentation} onValueChange={setPresentation}>
              <DropdownMenuRadioItem value="expanded" closeOnClick>
                {text('展开列表', 'Expanded groups')}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="tabs" closeOnClick>
                {text('标签视图', 'Group tabs')}
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>
          {t('defaultOutbound')}:{' '}
          <span className="break-all">{data.default_outbound || t('runtimeUnknown')}</span>
        </span>
        {!live && <Badge variant="outline">{t('staleSnapshot')}</Badge>}
      </div>
      {(selection.isUncertain || selection.isUnconfirmed) && (
        <section role="status" className="space-y-2 border-y py-3 text-sm">
          <p>
            {t(
              selection.isUnconfirmed
                ? 'routingUnconfirmed'
                : operation?.target.kind === 'mode'
                  ? 'modeUncertain'
                  : 'selectionUncertain',
            )}
          </p>
          <p className="text-muted-foreground">
            {t(
              selection.persisted || mode.persisted ? 'persisted' : 'routingUnconfirmedDescription',
            )}
          </p>
          <Button
            variant="outline"
            onClick={() =>
              void (operation?.target.kind === 'mode' ? mode.verify() : selection.verify())
            }
          >
            {t('checkStatus')}
          </Button>
        </section>
      )}
      {selection.error != null && <ErrorState compact error={selection.error} />}
      {mode.error != null && <ErrorState compact error={mode.error} />}
      {snapshot.isError && (
        <ErrorState compact error={snapshot.error} retry={() => void snapshot.refetch()} />
      )}
      {delay.error != null && <ErrorState compact error={delay.error} />}
      {delay.isPending && (
        <div role="status" className="flex items-center justify-between text-sm">
          <span>{t('testing')}</span>
          <Button variant="outline" onClick={delay.cancel}>
            {t('cancelDelay')}
          </Button>
        </div>
      )}
      {delay.outcome && (
        <p role="status" className="text-xs text-muted-foreground">
          {t(delay.outcome === 'cached' ? 'delayCached' : 'delayEmpty')}
        </p>
      )}
      {presentation === 'tabs' && (
        <div
          role="tablist"
          aria-label={text('策略组', 'Policy groups')}
          className="flex flex-wrap gap-1 border-b pb-2"
        >
          {data.groups.map((group) => (
            <button
              key={group.tag}
              role="tab"
              aria-selected={selectedTab === group.tag}
              className={cn(
                'min-h-11 max-w-full break-all rounded-md px-3 text-sm',
                selectedTab === group.tag && 'bg-accent font-medium',
              )}
              onClick={() => setActiveTag(group.tag)}
            >
              {group.tag}
            </button>
          ))}
        </div>
      )}
      {data.groups.length === 0 && <EmptyState />}
      {data.groups
        .filter(
          (group) =>
            (presentation !== 'tabs' || group.tag === selectedTab) &&
            (group.tag.toLocaleLowerCase().includes(query) ||
              group.items.some((item) => item.tag.toLocaleLowerCase().includes(query))),
        )
        .map((group) => {
          const members = group.tag.toLocaleLowerCase().includes(query)
            ? group.items
            : group.items.filter((item) => item.tag.toLocaleLowerCase().includes(query))
          return (
            <Collapsible
              key={group.tag}
              defaultOpen
              className="border-b pb-3"
              data-testid={`policy-${group.tag}`}
            >
              <div className="flex min-w-0 items-center gap-1">
                <CollapsibleTrigger className="group flex min-h-12 min-w-0 flex-1 items-center gap-2 text-left">
                  <ChevronDown className="size-4 shrink-0 text-muted-foreground group-data-closed:-rotate-90" />
                  <span className="min-w-0 break-all font-medium">{group.tag}</span>
                  <Badge variant="outline">{group.type}</Badge>
                  <span className="text-xs text-muted-foreground">{group.items.length}</span>
                </CollapsibleTrigger>
                {api.supports('policy.delay') && (
                  <Button
                    size="icon"
                    variant="ghost"
                    className="size-11 shrink-0"
                    disabled={blocked || group.items.length === 0}
                    aria-label={t('testGroup', { name: group.tag })}
                    onClick={() => delay.mutate(group.tag)}
                  >
                    <Gauge />
                  </Button>
                )}
                {api.supports('config.read') && (
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={<Button variant="ghost" size="icon" className="size-11 shrink-0" />}
                      aria-label={text('编辑策略组', 'Edit policy group') + ' ' + group.tag}
                    >
                      <MoreHorizontal />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => setEditor({ tag: group.tag })}>
                        {text('原生配置', 'Native configuration')}
                      </DropdownMenuItem>
                      {api.supports('config.write') && (
                        <DropdownMenuItem
                          variant="destructive"
                          onClick={() => setEditor({ tag: group.tag, remove: true })}
                        >
                          {text('删除策略组', 'Delete policy group')}
                        </DropdownMenuItem>
                      )}
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </div>
              <CollapsibleContent className="pt-1">
                {group.items.length === 0 ? (
                  <p className="p-3 text-sm text-muted-foreground">{t('emptyGroup')}</p>
                ) : (
                  <div
                    className={cn(
                      'grid',
                      view === 'grid' ? 'grid-cols-2 gap-2 xl:grid-cols-3' : 'grid-cols-1 divide-y',
                    )}
                    data-testid="node-grid"
                  >
                    {members.map((item) => {
                      const child = data.groups.find((entry) => entry.tag === item.tag)
                      const selected = live && group.selected === item.tag
                      return (
                        <div
                          key={item.tag}
                          className={cn(
                            'relative flex min-w-0 items-center rounded-md',
                            view === 'grid' ? 'h-32 border' : 'h-16',
                            selected && 'node-selected',
                          )}
                        >
                          <button
                            type="button"
                            className="flex h-full min-w-0 flex-1 flex-col justify-center gap-1 rounded-md px-3 text-left hover:bg-muted disabled:opacity-60"
                            aria-label={t('selectNode', { name: item.tag })}
                            aria-pressed={selected}
                            disabled={
                              blocked || !group.selectable || !api.supports('policy.select')
                            }
                            onClick={() =>
                              selection.mutate({ group_tag: group.tag, member_tag: item.tag })
                            }
                          >
                            <span
                              className={cn(
                                'flex min-w-0 items-center gap-1 text-sm font-medium',
                                view === 'grid' ? 'pr-4' : '',
                              )}
                            >
                              <Check className={cn('size-3 shrink-0', !selected && 'invisible')} />
                              <span
                                className={view === 'grid' ? 'line-clamp-2 break-all' : 'truncate'}
                              >
                                {item.tag}
                              </span>
                            </span>
                            <span className="truncate text-xs text-muted-foreground">
                              {item.type}
                              {child && live && child.selected ? ` · ${child.selected}` : ''}
                            </span>
                            <span
                              className="text-xs tabular-nums text-muted-foreground"
                              title={
                                live && item.url_test_time
                                  ? new Date(item.url_test_time * 1000).toLocaleString()
                                  : ''
                              }
                            >
                              {live && (item.url_test_delay ?? 0) > 0
                                ? `${item.url_test_delay} ms`
                                : t('noLatency')}
                            </span>
                          </button>
                          {child && (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-11 shrink-0"
                              aria-label={text('打开策略组', 'Open group') + ' ' + child.tag}
                              onClick={() => nested(child.tag)}
                            >
                              <ChevronRight />
                            </Button>
                          )}
                        </div>
                      )
                    })}
                  </div>
                )}
              </CollapsibleContent>
            </Collapsible>
          )
        })}
      {editor && (
        <PolicyEditor
          key={editor.tag ?? 'new'}
          tag={editor.tag}
          remove={editor.remove ?? false}
          onClose={() => setEditor(null)}
        />
      )}
    </div>
  )
}
