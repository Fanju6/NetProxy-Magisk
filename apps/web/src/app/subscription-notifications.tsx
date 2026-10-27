import { useEffect, useRef } from 'react'
import { Toast } from '@base-ui/react/toast'
import { useSubscriptionOperation } from '@/api/subscription-control'
import { usePreferences } from './context'

/** One app-session consumer, independent of resource details mounting and navigation. */
export function SubscriptionNotifications() {
  const operation = useSubscriptionOperation()
  const { t } = usePreferences()
  const manager = Toast.useToastManager()
  const announced = useRef<number | null>(null)
  useEffect(() => {
    const result = operation?.result
    if (
      !operation ||
      operation.phase !== 'idle' ||
      operation.error ||
      !result ||
      result.runtime_sync_pending ||
      (result.failed?.length ?? 0) > 0 ||
      announced.current === operation.intent
    )
      return
    announced.current = operation.intent
    const key =
      operation.action === 'update_all'
        ? 'updateAllCompleted'
        : operation.action === 'remove'
          ? 'subscriptionDeleted'
          : operation.action === 'add' || operation.action === 'edit'
            ? 'subscriptionSaved'
            : result.not_modified
              ? 'subscriptionNotModified'
              : 'subscriptionUpdated'
    manager.add({ id: 'subscription-result', title: t(key) })
  }, [operation, manager, t])
  return null
}
