import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowUpRight, Folder, Rss } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { useApi, usePreferences } from '@/app/context'
import { moduleQueries } from '@/api/queries'
import type { GroupSummary } from '@/api/contracts'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Details } from '@/components/details'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'

export function SubscriptionsPage() {
  const api = useApi()
  const { t, preferences } = usePreferences()
  const list = useQuery(moduleQueries(api).subscriptions)
  const [details, setDetails] = useState<GroupSummary | null>(null)
  if (list.isPending) return <LoadingState />
  if (list.isError) return <ErrorState error={list.error} retry={() => void list.refetch()} />
  return (
    <div className="space-y-6">
      {list.data.length === 0 ? (
        <EmptyState />
      ) : (
        <div className="divide-y rounded-lg border">
          {list.data.map((group) => (
            <article key={group.id} className="flex min-w-0 flex-wrap items-center gap-4 p-5">
              <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted">
                {group.type === 'local' ? (
                  <Folder className="size-5" />
                ) : (
                  <Rss className="size-5" />
                )}
              </div>
              <div className="min-w-0 flex-1 space-y-2">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="truncate font-medium">{group.name}</h2>
                  {group.active && <Badge variant="secondary">{t('selected')}</Badge>}
                </div>
                <p className="text-xs text-muted-foreground">
                  {t(group.type)} · {t('nodeCount', { count: group.node_count })}
                </p>
              </div>
              <Button
                variant="ghost"
                size="icon"
                aria-label={`${t('viewDetails')} ${group.name}`}
                onClick={() => setDetails(group)}
              >
                <ArrowUpRight />
              </Button>
            </article>
          ))}
        </div>
      )}
      <p className="text-xs text-muted-foreground">{t('advancedLater')}</p>
      <Details
        open={details !== null}
        onOpenChange={(open) => {
          if (!open) setDetails(null)
        }}
        title={details?.name ?? t('subscriptionDetails')}
        description={details ? t(details.type) : ''}
      >
        {details && (
          <div className="space-y-6">
            <dl className="space-y-4 text-sm">
              <div className="flex justify-between gap-3">
                <dt className="text-muted-foreground">{t('nodes')}</dt>
                <dd>{details.node_count}</dd>
              </div>
              <div className="flex justify-between gap-3">
                <dt className="text-muted-foreground">{t('lastUpdate')}</dt>
                <dd>
                  {details.last_success_at
                    ? new Intl.DateTimeFormat(preferences.locale, { dateStyle: 'medium' }).format(
                        new Date(details.last_success_at),
                      )
                    : t('never')}
                </dd>
              </div>
              <div className="flex justify-between gap-3">
                <dt className="text-muted-foreground">{t('autoUpdate')}</dt>
                <dd>{t(details.auto_update ? 'enabled' : 'disabled')}</dd>
              </div>
            </dl>
            <Button render={<Link to="/nodes" />} onClick={() => setDetails(null)}>
              {t('viewGroup')}
              <ArrowUpRight />
            </Button>
          </div>
        )}
      </Details>
    </div>
  )
}
