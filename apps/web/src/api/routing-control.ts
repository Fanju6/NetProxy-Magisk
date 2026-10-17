import { useQuery, type QueryClient } from '@tanstack/react-query'
import { ApiError, type ModeState, type NodeTarget, type Selection } from './contracts'

// 两种持久写入共用一个短生命周期槽位，不把草稿或运行时数据另存一份。
export type RoutingOperation = {
  epoch: number
  target: { kind: 'selection'; value: NodeTarget } | { kind: 'mode'; value: string }
  phase: 'pending' | 'uncertain' | 'idle'
  deadline: number
  responseReceived: boolean
  error: unknown
}
export const routingOperationKey = ['routing-operation'] as const
export function useRoutingOperation() {
  return useQuery<RoutingOperation | null>({
    queryKey: routingOperationKey,
    queryFn: () => null,
    initialData: null,
    enabled: false,
    gcTime: Infinity,
  }).data
}
export function routingEpoch(cache: QueryClient) {
  return cache.getQueryData<RoutingOperation | null>(routingOperationKey)?.epoch ?? 0
}
export function observeRouting(cache: QueryClient, value: Selection | ModeState) {
  const operation = cache.getQueryData<RoutingOperation | null>(routingOperationKey)
  if (!operation || operation.phase !== 'uncertain') return
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
  if (matches || operation.responseReceived || Date.now() >= operation.deadline)
    cache.setQueryData<RoutingOperation>(routingOperationKey, {
      ...operation,
      phase: 'idle',
      error:
        matches &&
        !(
          operation.error instanceof ApiError &&
          (operation.error.persisted || operation.error.responseReceived)
        )
          ? null
          : operation.error,
    })
}
