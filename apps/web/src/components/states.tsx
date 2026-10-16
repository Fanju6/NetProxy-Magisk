import { AlertCircle, Inbox, LoaderCircle } from 'lucide-react'
import { ApiError } from '@/api/contracts'
import { usePreferences } from '@/app/context'
import { Button } from './ui/button'
import { Skeleton } from './ui/skeleton'

export function LoadingState() {
  const { t } = usePreferences()
  return (
    <div role="status" aria-label={t('loading')} className="space-y-5 py-8">
      <LoaderCircle aria-hidden className="size-5 animate-spin text-muted-foreground" />
      <Skeleton className="h-24 w-full" />
      <Skeleton className="h-40 w-full" />
    </div>
  )
}
export function EmptyState({ title, description }: { title?: string; description?: string }) {
  const { t } = usePreferences()
  return (
    <div className="flex min-h-44 flex-col items-center justify-center gap-3 px-5 py-10 text-center">
      <Inbox aria-hidden className="size-6 text-muted-foreground" />
      <p className="text-sm font-medium">{title ?? t('noData')}</p>
      {description && <p className="max-w-sm text-sm text-muted-foreground">{description}</p>}
    </div>
  )
}
export function ErrorState({
  error,
  retry,
  compact = false,
}: {
  error: unknown
  retry?: () => void
  compact?: boolean
}) {
  const { t } = usePreferences()
  const code = error instanceof ApiError ? error.code : ''
  const title =
    code === 'config.conflict'
      ? t('conflict')
      : error instanceof ApiError && error.persisted
        ? t('persisted')
        : code === 'client.transport_unavailable'
          ? t('unavailable')
          : code === 'api.connection_unavailable'
            ? t('connectionUnavailable')
            : code === 'api.unauthorized'
              ? t('unauthorized')
              : error instanceof ApiError && error.status === 403
                ? t('operationDenied')
                : error instanceof ApiError && [409, 429].includes(error.status)
                  ? t('operationBusy')
                  : t('requestFailed')
  return (
    <div
      role="alert"
      className={
        compact
          ? 'flex items-start gap-2 py-3 text-sm text-destructive'
          : 'flex min-h-56 flex-col items-center justify-center gap-3 px-5 py-10 text-center'
      }
    >
      <AlertCircle aria-hidden className="size-5 shrink-0 text-destructive" />
      <p className="max-w-md text-sm">{title}</p>
      {code === 'client.transport_unavailable' && (
        <p className="max-w-md text-sm text-muted-foreground">{t('unavailableDescription')}</p>
      )}
      {code === 'api.connection_unavailable' && (
        <p className="max-w-md text-sm text-muted-foreground">
          {t('connectionUnavailableDescription')}
        </p>
      )}
      {retry && (
        <Button variant="outline" onClick={retry}>
          {t('retry')}
        </Button>
      )}
    </div>
  )
}
