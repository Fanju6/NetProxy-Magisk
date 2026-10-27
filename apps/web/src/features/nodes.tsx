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
  LoaderCircle,
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
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'
import { cn } from '@/lib/utils'
import { PolicyEditor } from './policy-editor'
import { HeaderActions } from '@/app/header-actions'

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
      <HeaderActions>
        {api.supports('config.read') && api.supports('config.write') && (
          <Button
            variant="ghost"
            size="icon"
            className="size-12"
            aria-label={text('新增策略组', 'Add policy group')}
            onClick={() => setEditor({ tag: null })}
          >
            <Plus />
          </Button>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="ghost" size="icon" className="size-12" />}
            aria-label={text('显示方式', 'View options')}
          >
            <MoreHorizontal />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-52">
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
            <DropdownMenuSeparator />
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
      </HeaderActions>
      <div className="relative">
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
      {connection !== 'live' || snapshot.isError || (!live && data.source !== 'configuration') ? (
        <Badge variant="outline">{t('staleSnapshot')}</Badge>
      ) : data.source === 'configuration' && service.data?.state === 'ready' ? (
        <p className="text-xs text-muted-foreground">
          {text('实时状态暂不可用', 'Live state unavailable')}
        </p>
      ) : null}
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
      {presentation === 'tabs' && (
        <div
          role="tablist"
          aria-label={text('策略组', 'Policy groups')}
          className="flex gap-1 overflow-x-auto border-b pb-2"
          onKeyDown={(event) => {
            if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
            event.preventDefault()
            const tabs = [
              ...event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="tab"]'),
            ]
            const current = tabs.findIndex((tab) => tab.getAttribute('aria-selected') === 'true')
            const next =
              event.key === 'Home'
                ? 0
                : event.key === 'End'
                  ? tabs.length - 1
                  : (current + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length
            tabs[next]?.click()
            tabs[next]?.focus()
          }}
        >
          {data.groups.map((group) => (
            <button
              key={group.tag}
              role="tab"
              aria-selected={selectedTab === group.tag}
              tabIndex={selectedTab === group.tag ? 0 : -1}
              title={group.tag}
              className={cn(
                'min-h-12 max-w-48 shrink-0 truncate rounded-md px-3 text-sm',
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
                  <span className="inline-flex h-5 min-w-6 shrink-0 items-center text-xs text-muted-foreground">
                    {selection.isPending &&
                    operation?.target.kind === 'policy' &&
                    operation.target.value.group_tag === group.tag ? (
                      <LoaderCircle
                        role="status"
                        aria-label={t('working')}
                        className="size-4 animate-spin motion-reduce:animate-none"
                      />
                    ) : (
                      group.items.length
                    )}
                  </span>
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
                    {delay.isPending && delay.target === group.tag ? (
                      <LoaderCircle className="animate-spin motion-reduce:animate-none" />
                    ) : (
                      <Gauge />
                    )}
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
              {delay.target === group.tag && (
                <>
                  {delay.isPending && (
                    <div
                      role="status"
                      className="flex items-center justify-between gap-2 py-2 text-sm"
                    >
                      <span>{t('testing')}</span>
                      <Button variant="ghost" size="sm" onClick={delay.cancel}>
                        {t('cancelDelay')}
                      </Button>
                    </div>
                  )}
                  {delay.error != null && <ErrorState compact error={delay.error} />}
                  {delay.outcome && (
                    <p role="status" className="py-2 text-xs text-muted-foreground">
                      {t(delay.outcome === 'cached' ? 'delayCached' : 'delayEmpty')}
                    </p>
                  )}
                </>
              )}
              <CollapsibleContent className="pt-1">
                {!live && group.unavailable_providers?.length ? (
                  <p className="px-3 py-2 text-xs text-muted-foreground">
                    {text('无法离线展开的 Provider', 'Providers unavailable offline')}:{' '}
                    {group.unavailable_providers.join(', ')}
                  </p>
                ) : null}
                {group.items.length === 0 ? (
                  <p className="p-3 text-sm text-muted-foreground">{t('emptyGroup')}</p>
                ) : (
                  <div
                    className={cn(
                      'grid gap-2',
                      view === 'grid' ? 'grid-cols-2 xl:grid-cols-3' : 'grid-cols-1',
                    )}
                    data-testid="node-grid"
                  >
                    {members.map((item) => {
                      const name = item.tag.replace(/^np\/[^/]+\//, '')
                      const provider = /^np\/[^/]+\//.exec(item.tag)?.[0].slice(0, -1)
                      const child = data.groups.find((entry) => entry.tag === item.tag)
                      const selected = live && group.selected === item.tag
                      return (
                        <div
                          key={item.tag}
                          data-slot="node-card"
                          data-selected={selected}
                          className={cn(
                            'flex min-w-0 items-center rounded-lg border bg-card',
                            view === 'grid' ? 'h-24' : 'h-20',
                            selected ? 'border-foreground/50 bg-accent' : 'border-border',
                          )}
                        >
                          <button
                            type="button"
                            className="flex h-full min-w-0 flex-1 flex-col justify-between gap-2 rounded-lg px-3 py-3 text-left enabled:hover:bg-muted/50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:opacity-60"
                            aria-label={t('selectNode', { name })}
                            aria-pressed={selected}
                            disabled={
                              blocked || !group.selectable || !api.supports('policy.select')
                            }
                            onClick={() =>
                              selection.mutate({ group_tag: group.tag, member_tag: item.tag })
                            }
                          >
                            <span className="flex w-full min-w-0 items-start justify-between gap-2 text-sm font-medium">
                              <span
                                title={item.tag}
                                data-slot="node-name"
                                className={cn(
                                  'min-w-0 flex-1',
                                  view === 'grid' ? 'line-clamp-2 break-all' : 'truncate',
                                )}
                              >
                                {name}
                              </span>
                              <span
                                data-slot="node-choice"
                                aria-hidden="true"
                                className="flex size-5 shrink-0 items-center justify-end"
                              >
                                {selected && <Check className="size-4" />}
                              </span>
                            </span>
                            <span className="flex w-full min-w-0 items-end justify-between gap-2 text-xs text-muted-foreground">
                              <span className="min-w-0 truncate">
                                {item.type}
                                {provider ? ` · ${provider}` : ''}
                                {!live &&
                                data.source === 'configuration' &&
                                group.configured_default === item.tag
                                  ? ` · ${text('配置默认', 'Configured default')}`
                                  : ''}
                                {child && live && child.selected
                                  ? ` · ${child.selected.replace(/^np\/[^/]+\//, '')}`
                                  : ''}
                              </span>
                              {live && (
                                <span
                                  className="shrink-0 text-xs font-medium tabular-nums"
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
                              )}
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
