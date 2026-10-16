import { useQuery, type QueryClient } from '@tanstack/react-query'
import { ApiError, type ServiceStatus } from './contracts'

export type ServiceAction = 'start' | 'stop'
export type ServiceOperation = {
  epoch: number
  action: ServiceAction
  phase: 'pending' | 'uncertain' | 'idle'
  deadline: number
  responseReceived: boolean
  error: unknown
}
export const serviceOperationKey = ['service-operation'] as const

export function useServiceOperation() {
  return useQuery<ServiceOperation | null>({
    queryKey: serviceOperationKey,
    queryFn: () => null,
    initialData: null,
    enabled: false,
    gcTime: Infinity,
  }).data
}

export function serviceEpoch(cache: QueryClient) {
  return cache.getQueryData<ServiceOperation | null>(serviceOperationKey)?.epoch ?? 0
}

export function definiteServiceRejection(error: unknown) {
  return (
    error instanceof ApiError &&
    !error.persisted &&
    error.code !== 'client.invalid_response' &&
    error.status >= 400 &&
    error.status < 500 &&
    error.status !== 408
  )
}

export function observeService(cache: QueryClient, status: ServiceStatus) {
  const operation = cache.getQueryData<ServiceOperation | null>(serviceOperationKey)
  if (!operation || operation.phase !== 'uncertain') return
  const target = operation.action === 'start' ? 'ready' : 'stopped'
  // 通信失败不能证明未执行；只有目标状态、明确失败或事务时限后的终态才能解除等待。
  if (
    status.state === target ||
    ((operation.responseReceived || Date.now() >= operation.deadline) &&
      ['ready', 'stopped', 'failed'].includes(status.state))
  )
    cache.setQueryData(serviceOperationKey, {
      ...operation,
      phase: 'idle',
      error:
        status.state === target &&
        !(
          operation.error instanceof ApiError &&
          (operation.error.persisted || operation.error.responseReceived)
        )
          ? null
          : operation.error,
    })
}
