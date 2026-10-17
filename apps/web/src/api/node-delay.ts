import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useApi } from '@/app/context'
import {
  ApiError,
  delaySchema,
  type NodeSnapshot,
  type NodeTarget,
  type ServiceStatus,
} from './contracts'
import { serviceEpoch } from './service-control'
import { routingEpoch } from './routing-control'

type Result = { delay: number; time: number }
export function useNodeDelay() {
  const api = useApi()
  const cache = useQueryClient()
  const request = useRef<AbortController | null>(null)
  const [state, setState] = useState<{
    target: NodeTarget | null
    isPending: boolean
    error: unknown
    results: Record<string, Result>
    core: number
    outcome: 'cached' | 'empty' | null
  }>({ target: null, isPending: false, error: null, results: {}, core: 0, outcome: null })
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
    // 离页、后台及连接中断只终止本次等待，不能补发已经接受的测速。
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
    setState((current) => ({ ...current, core, results: {}, outcome: null, error: null }))
  }, [core, cancel])
  useEffect(() => {
    if (connection.data && connection.data !== 'live') cancel()
  }, [connection.data, cancel])
  return {
    ...state,
    results: state.core === core ? state.results : {},
    cancel,
    mutate: (target: NodeTarget) => {
      if (request.current || !api.supports('nodes.delay')) return
      const snapshot = cache.getQueryData<NodeSnapshot>(['nodes'])
      const group = snapshot?.groups.find((entry) => entry.group.id === target.group_id)
      if (
        !group ||
        group.nodes.length === 0 ||
        (target.node_tag !== '' && !group.nodes.some((node) => node.tag === target.node_tag))
      )
        return
      const controller = new AbortController()
      request.current = controller
      const epoch = routingEpoch(cache)
      const service = serviceEpoch(cache)
      const runtime = group.group.runtime_tag
      const expected = target.node_tag ? `${runtime}/${target.node_tag}` : `Auto/${runtime}`
      const allowed = new Set(group.nodes.map((node) => `${runtime}/${node.tag}`))
      setState((current) => ({
        ...current,
        core,
        target,
        isPending: true,
        error: null,
        outcome: null,
      }))
      void (async () => {
        try {
          const result = await api.request(
            { method: 'POST', path: '/nodes/delay', body: target },
            delaySchema,
            controller.signal,
          )
          if (
            controller.signal.aborted ||
            request.current !== controller ||
            epoch !== routingEpoch(cache) ||
            service !== serviceEpoch(cache) ||
            core !== (cache.getQueryData<ServiceStatus>(['service'])?.core_started_at_millis ?? 0)
          )
            return
          if (
            result.target !== expected ||
            result.groups.length !== 1 ||
            result.groups[0]!.tag !== `Auto/${runtime}` ||
            result.groups[0]!.items.length === 0 ||
            result.groups[0]!.items.some(
              (item) => !allowed.has(item.tag) || (target.node_tag !== '' && item.tag !== expected),
            )
          )
            throw new ApiError('client.invalid_response', 'Invalid delay target', 502)
          const results: Record<string, Result> = {}
          for (const item of result.groups[0]!.items) {
            if ((item.url_test_delay ?? 0) > 0 && (item.url_test_time ?? 0) > 0)
              results[`${target.group_id}/${item.tag.slice(runtime.length + 1)}`] = {
                delay: item.url_test_delay!,
                time: item.url_test_time!,
              }
          }
          setState((current) => ({
            ...current,
            results: { ...current.results, ...results },
            outcome: Object.keys(results).length ? 'cached' : 'empty',
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
