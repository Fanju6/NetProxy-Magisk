import { useEffect } from 'react'
import { useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { ApiError, type ModeState, type NodeTarget, type Selection } from './contracts'

// 两种持久写入共用一个短生命周期槽位，不把草稿或运行时数据另存一份。
export type RoutingOperation = {
  epoch: number
  target: { kind: 'selection'; value: NodeTarget } | { kind: 'mode'; value: string }
  phase: 'pending' | 'uncertain' | 'unconfirmed' | 'idle'
  deadline: number
  responseReceived: boolean
  error: unknown
}
export const routingOperationKey = ['routing-operation'] as const
function endObservation(cache: QueryClient, epoch: number) {
  const operation = cache.getQueryData<RoutingOperation>(routingOperationKey)
  if (operation?.epoch === epoch && operation.phase === 'uncertain')
    cache.setQueryData(routingOperationKey, { ...operation, phase: 'unconfirmed' })
}
export function routingBusy(cache: QueryClient) {
  const operation = cache.getQueryData<RoutingOperation>(routingOperationKey)
  if (operation?.phase === 'uncertain' && Date.now() >= operation.deadline) {
    endObservation(cache, operation.epoch)
    return false
  }
  return operation?.phase === 'pending' || operation?.phase === 'uncertain'
}
export function useRoutingOperation() {
  const cache = useQueryClient()
  const { data } = useQuery<RoutingOperation | null>({
    queryKey: routingOperationKey,
    queryFn: () => null,
    initialData: null,
    enabled: false,
    gcTime: Infinity,
  })
  useEffect(() => {
    if (data?.phase !== 'uncertain') return
    // 到期只结束排他观察，不声明成功或失败，也不发起读取或重放写入。
    const timer = setTimeout(
      () => endObservation(cache, data.epoch),
      Math.max(0, data.deadline - Date.now()),
    )
    return () => clearTimeout(timer)
  }, [cache, data])
  return data
}
export function routingEpoch(cache: QueryClient) {
  return cache.getQueryData<RoutingOperation | null>(routingOperationKey)?.epoch ?? 0
}
export function observeRouting(cache: QueryClient, value: Selection | ModeState) {
  const operation = cache.getQueryData<RoutingOperation | null>(routingOperationKey)
  if (!operation || !['uncertain', 'unconfirmed'].includes(operation.phase)) return
  const target = operation.target
  let matches: boolean
  if (target.kind === 'selection' && 'active_group_id' in value) {
    const selection = value as Selection
    matches =
      selection.active_group_id === target.value.group_id &&
      selection.selected_node_ref ===
        (target.value.node_tag ? `${target.value.group_id}/${target.value.node_tag}` : '') &&
      selection.selector_mode === (target.value.node_tag ? 'manual' : 'urltest')
  } else if (target.kind === 'mode' && 'mode' in value) {
    matches = value.mode === target.value
  } else return
  if (matches)
    cache.setQueryData<RoutingOperation>(routingOperationKey, {
      ...operation,
      phase: 'idle',
      error:
        operation.error instanceof ApiError &&
        (operation.error.persisted || operation.error.responseReceived)
          ? operation.error
          : null,
    })
  else if (Date.now() >= operation.deadline) endObservation(cache, operation.epoch)
}
