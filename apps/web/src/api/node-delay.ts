import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useApi } from '@/app/context'
import {
  ApiError,
  policySnapshotSchema,
  type PolicySnapshot,
  type ServiceStatus,
} from './contracts'
import { serviceEpoch, serviceOperationKey, type ServiceOperation } from './service-control'
import { routingBusy, routingEpoch } from './routing-control'
import { managementBusy } from './management'
import { subscriptionBusy } from './subscription-control'

export function usePolicyDelay() {
  const api = useApi()
  const cache = useQueryClient()
  const request = useRef<AbortController | null>(null)
  const [state, setState] = useState<{
    target: string | null
    isPending: boolean
    error: unknown
    outcome: 'cached' | 'empty' | null
  }>({ target: null, isPending: false, error: null, outcome: null })
  const service = useQuery<ServiceStatus | null>({
    queryKey: ['service'],
    queryFn: () => null,
    enabled: false,
  })
  const connection = useQuery<string>({
    queryKey: ['events-connection'],
    queryFn: () => 'connecting',
    enabled: false,
  })
  const core = service.data?.core_started_at_millis ?? 0
  const cancel = useCallback(() => {
    request.current?.abort()
    request.current = null
    setState((current) => ({ ...current, isPending: false }))
  }, [])
  useEffect(() => {
    const visibility = () => {
      if (document.visibilityState === 'hidden') cancel()
    }
    document.addEventListener('visibilitychange', visibility)
    return () => {
      document.removeEventListener('visibilitychange', visibility)
      request.current?.abort()
      request.current = null
    }
  }, [cancel])
  useEffect(() => {
    cancel()
    setState((current) => ({ ...current, outcome: null, error: null }))
  }, [core, cancel])
  useEffect(() => {
    if (!['live', 'refreshing'].includes(connection.data ?? '')) cancel()
  }, [connection.data, cancel])
  return {
    ...state,
    cancel,
    mutate: (groupTag: string) => {
      const snapshot = cache.getQueryData<PolicySnapshot>(['policy-groups'])
      const group = snapshot?.groups.find((group) => group.tag === groupTag)
      const action = cache.getQueryData<ServiceOperation>(serviceOperationKey)
      if (
        request.current ||
        !api.supports('policy.delay') ||
        !snapshot?.live ||
        !group ||
        routingBusy(cache) ||
        managementBusy(cache) ||
        subscriptionBusy(cache) ||
        (action && action.phase !== 'idle') ||
        service.data?.state !== 'ready' ||
        snapshot.core_started_at_millis !== core
      )
        return
      const controller = new AbortController()
      request.current = controller
      const epoch = routingEpoch(cache)
      const serviceVersion = serviceEpoch(cache)
      setState({ target: groupTag, isPending: true, error: null, outcome: null })
      void (async () => {
        try {
          const result = await api.request(
            { method: 'POST', path: '/policy-groups/delay', body: { group_tag: groupTag } },
            policySnapshotSchema,
            controller.signal,
          )
          if (
            controller.signal.aborted ||
            request.current !== controller ||
            epoch !== routingEpoch(cache) ||
            serviceVersion !== serviceEpoch(cache) ||
            core !== cache.getQueryData<ServiceStatus>(['service'])?.core_started_at_millis
          )
            return
          const measured = result.groups.find((item) => item.tag === groupTag)
          if (!result.live || result.core_started_at_millis !== core || !measured)
            throw new ApiError('client.invalid_response', 'Invalid policy delay snapshot', 502)
          // A delay response may race a selection event; read back instead of publishing it.
          await cache.invalidateQueries({ queryKey: ['policy-groups'], exact: true })
          if (controller.signal.aborted || request.current !== controller) return
          setState((current) => ({
            ...current,
            outcome: measured.items.some(
              (item) => (item.url_test_delay ?? 0) > 0 && (item.url_test_time ?? 0) > 0,
            )
              ? 'cached'
              : 'empty',
          }))
        } catch (error) {
          if (!controller.signal.aborted && request.current === controller)
            setState((current) => ({ ...current, error }))
        } finally {
          if (request.current === controller) {
            request.current = null
            setState((current) => ({ ...current, isPending: false }))
          }
        }
      })()
    },
  }
}
