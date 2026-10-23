import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { MoreHorizontal, Pencil, Plus, Share2, Trash2 } from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { catalogNodesKey, resourceQueries } from '@/api/resources'
import { useEventConnection } from '@/api/events'
import { Details } from '@/components/details'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'
import { NodeManager, type NodeManagerTarget } from './node-manager'

function useCatalogNodes() {
  const api = useApi()
  const cache = useQueryClient()
  const connection = useEventConnection()
  const query = useQuery({
    ...resourceQueries(api).nodes,
    enabled: connection === 'live' && api.supports('nodes.read'),
  })
  useEffect(() => {
    if (connection !== 'live')
      void cache.cancelQueries(
        { queryKey: catalogNodesKey, exact: true },
        { revert: false, silent: true },
      )
  }, [cache, connection])
  return query
}

export function ResourceNodes({
  id,
  blocked,
  manage,
}: {
  id: string
  blocked: boolean
  manage: (target: NodeManagerTarget) => void
}) {
  const api = useApi()
  const { t, preferences } = usePreferences()
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const connection = useEventConnection()
  const query = useCatalogNodes()
  const group = query.data?.groups.find((entry) => entry.group.id === id)
  const disabled = blocked || connection !== 'live' || query.isError || query.isFetching
  if (!api.supports('nodes.read')) return null
  return (
    <section
      className="min-w-0 space-y-3 border-t pt-5"
      aria-label={text('资源节点', 'Catalog nodes')}
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="text-sm font-medium">{text('资源节点', 'Catalog nodes')}</h3>
        {group?.group.type === 'local' && api.supports('nodes.add') && (
          <Button
            variant="outline"
            className="min-h-11"
            disabled={disabled}
            onClick={() => manage({ operation: 'add', groupId: id })}
          >
            <Plus />
            {text('手动添加节点', 'Add node manually')}
          </Button>
        )}
      </div>
      {query.isError && (
        <ErrorState error={query.error} compact retry={() => void query.refetch()} />
      )}
      {!query.data ? (
        connection === 'live' && !query.isError ? (
          <LoadingState />
        ) : null
      ) : !group ? (
        <EmptyState title={t('resourceUnavailable')} />
      ) : group.nodes.length === 0 ? (
        <EmptyState />
      ) : (
        <ul className="divide-y">
          {group.nodes.map((node) => (
            <li key={node.tag} className="flex min-h-16 min-w-0 items-center gap-3 py-3">
              <div className="min-w-0 flex-1 space-y-1">
                <p className="break-all text-sm font-medium">{node.tag}</p>
                <p className="break-all text-xs text-muted-foreground">{node.protocol}</p>
              </div>
              {(api.supports('nodes.export') ||
                (group.group.type === 'local' &&
                  (api.supports('nodes.edit') || api.supports('nodes.remove')))) && (
                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={<Button variant="ghost" size="icon" className="size-11 shrink-0" />}
                    aria-label={`${text('节点操作', 'Node actions')} ${node.tag}`}
                    disabled={disabled}
                  >
                    <MoreHorizontal />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    {group.group.type === 'local' && api.supports('nodes.edit') && (
                      <DropdownMenuItem
                        className="min-h-11"
                        onClick={() => manage({ operation: 'edit', groupId: id, tag: node.tag })}
                      >
                        <Pencil />
                        {text('编辑节点', 'Edit node')}
                      </DropdownMenuItem>
                    )}
                    {api.supports('nodes.export') && (
                      <DropdownMenuItem
                        className="min-h-11"
                        onClick={() => manage({ operation: 'export', groupId: id, tag: node.tag })}
                      >
                        <Share2 />
                        {text('分享链接', 'Share link')}
                      </DropdownMenuItem>
                    )}
                    {group.group.type === 'local' && api.supports('nodes.remove') && (
                      <>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          variant="destructive"
                          className="min-h-11"
                          onClick={() =>
                            manage({ operation: 'remove', groupId: id, tag: node.tag })
                          }
                        >
                          <Trash2 />
                          {text('删除节点', 'Delete node')}
                        </DropdownMenuItem>
                      </>
                    )}
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

export function ResourceNodeManager({
  target,
  onClose,
  initialContent,
}: {
  target: NodeManagerTarget
  onClose: () => void
  initialContent?: string
}) {
  const api = useApi()
  const { t } = usePreferences()
  const connection = useEventConnection()
  const query = useCatalogNodes()
  if (query.data)
    return (
      <NodeManager
        key={JSON.stringify(target)}
        snapshot={query.data}
        target={target}
        {...(initialContent !== undefined ? { initialContent } : {})}
        onClose={onClose}
        disabled={query.isFetching || query.isError}
        snapshotError={query.error}
      />
    )
  return (
    <Details open onOpenChange={(open) => !open && onClose()} title={t('nodes')}>
      {query.isError ? (
        <ErrorState error={query.error} retry={() => void query.refetch()} />
      ) : connection !== 'live' || !api.supports('nodes.read') ? (
        <EmptyState title={t('resourceUnavailable')} />
      ) : (
        <LoadingState />
      )}
    </Details>
  )
}
