import { useCallback } from 'react'
import {
  CancelledError,
  queryOptions,
  useMutation,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { z } from 'zod'
import { useApi } from '@/app/context'
import {
  configSchema,
  infoSchema,
  modeSchema,
  policySnapshotSchema,
  policySelectSchema,
  serviceSchema,
  type ServiceStatus,
  type PolicySnapshot,
} from './contracts'
import type { ApiClient } from './client'
import { ApiError } from './contracts'
import { resourceQueries } from './resources'
import { subscriptionBusy } from './subscription-control'
import { managementBusy } from './management'
import type { PolicyTarget } from './contracts'
import {
  observeRouting,
  routingEpoch,
  routingEventEpoch,
  routingBusy,
  routingOperationKey,
  useRoutingOperation,
  type RoutingOperation,
} from './routing-control'
import {
  definiteServiceRejection,
  observeService,
  serviceEpoch,
  serviceOperationKey,
  useServiceOperation,
  type ServiceAction,
  type ServiceOperation,
} from './service-control'

export const moduleQueries = (api: ApiClient) => ({
  info: queryOptions({
    queryKey: ['info'],
    networkMode: 'always',
    queryFn: ({ signal }) => api.request({ method: 'GET', path: '/info' }, infoSchema, signal),
  }),
  service: queryOptions({
    queryKey: ['service'],
    networkMode: 'always',
    queryFn: async ({ signal, client }) => {
      const epoch = serviceEpoch(client)
      const routing = routingEpoch(client)
      const status = await api.request({ method: 'GET', path: '/service' }, serviceSchema, signal)
      if (signal.aborted || serviceEpoch(client) !== epoch || routingEpoch(client) !== routing)
        throw new CancelledError({ silent: true, revert: false })
      observeService(client, status)
      return status
    },
  }),
  policyGroups: queryOptions({
    queryKey: ['policy-groups'],
    retry: false,
    networkMode: 'always',
    enabled: api.supports('policy.read'),
    queryFn: async ({ signal, client }) => {
      const epoch = routingEpoch(client)
      const service = serviceEpoch(client)
      const core = coreIdentity(client)
      const result = await api.request(
        { method: 'GET', path: '/policy-groups' },
        policySnapshotSchema,
        signal,
      )
      if (
        signal.aborted ||
        epoch !== routingEpoch(client) ||
        service !== serviceEpoch(client) ||
        core !== coreIdentity(client) ||
        (result.live && result.core_started_at_millis !== core)
      )
        throw new CancelledError({ silent: true, revert: false })
      observeRouting(client, result)
      return result
    },
  }),
  mode: queryOptions({
    queryKey: ['mode'],
    networkMode: 'always',
    enabled: api.supports('mode.read'),
    queryFn: async ({ signal, client }) => {
      const epoch = routingEpoch(client)
      const service = serviceEpoch(client)
      const core = coreIdentity(client)
      const result = await api.request({ method: 'GET', path: '/mode' }, modeSchema, signal)
      if (
        signal.aborted ||
        epoch !== routingEpoch(client) ||
        service !== serviceEpoch(client) ||
        core !== coreIdentity(client)
      )
        throw new CancelledError({ silent: true, revert: false })
      observeRouting(client, result)
      return result
    },
  }),
  subscriptions: resourceQueries(api).subscriptions,
  autoStart: queryOptions({
    queryKey: ['config', 'module/auto_start'],
    enabled: api.supports('config.read'),
    queryFn: ({ signal }) =>
      api.request(
        { method: 'GET', path: '/config', query: { target: 'module/auto_start' } },
        configSchema,
        signal,
      ),
  }),
})
export function useServiceAction() {
  const api = useApi()
  const cache = useQueryClient()
  const operation = useServiceOperation()
  const verify = useCallback(async () => {
    try {
      await cache.fetchQuery({ ...moduleQueries(api).service, staleTime: 0 })
    } catch {
      // 新会话快照可取消本次 GET；保持待确认，交由当前快照或显式复核处理。
    }
  }, [api, cache])
  const mutation = useMutation({
    retry: false,
    networkMode: 'always',
    mutationFn: async ({ action, epoch }: { action: ServiceAction; epoch: number }) => {
      let failure: unknown = null
      let responseReceived = false
      try {
        const result = await api.request(
          { method: 'POST', path: `/service/${action}` },
          z.object({ action: z.literal(action), status: serviceSchema }),
        )
        responseReceived = true
        return result
      } catch (error) {
        failure = error
        responseReceived = error instanceof ApiError && error.responseReceived
        throw error
      } finally {
        if (serviceEpoch(cache) === epoch) {
          await cache.cancelQueries({ queryKey: ['service'], exact: true })
          cache.setQueryData<ServiceOperation>(serviceOperationKey, (current) => ({
            ...current!,
            epoch: epoch + 1,
            phase: definiteServiceRejection(failure) ? 'idle' : 'uncertain',
            error: failure,
            responseReceived,
          }))
          await verify()
        }
      }
    },
  })
  // A successful POST still awaits its authoritative GET; this is normal progress,
  // not an unknown outcome. Keep the shared operation slot locked throughout.
  const confirming =
    operation?.phase === 'uncertain' &&
    operation.responseReceived &&
    operation.error === null &&
    mutation.isPending
  return {
    mutate: (action: ServiceAction) => {
      const current = cache.getQueryData<ServiceOperation | null>(serviceOperationKey)
      if (
        (current && current.phase !== 'idle') ||
        routingBusy(cache) ||
        subscriptionBusy(cache) ||
        managementBusy(cache)
      )
        return
      if (!api.supports(`service.${action}`)) return
      const epoch = serviceEpoch(cache) + 1
      cache.setQueryData<ServiceOperation>(serviceOperationKey, {
        epoch,
        action,
        phase: 'pending',
        deadline: Date.now() + 150_000,
        responseReceived: false,
        error: null,
      })
      void cache.cancelQueries({ queryKey: ['service'], exact: true })
      mutation.mutate({ action, epoch })
    },
    isPending: operation?.phase === 'pending',
    isUncertain: operation?.phase === 'uncertain',
    isConfirming: confirming,
    target: operation?.action,
    error: operation?.phase === 'idle' ? operation.error : null,
    persisted: operation?.error instanceof ApiError && operation.error.persisted,
    verify: async () => {
      const current = cache.getQueryData<ServiceOperation | null>(serviceOperationKey)
      if (current?.phase === 'pending') return
      if (current) cache.setQueryData(serviceOperationKey, { ...current, epoch: current.epoch + 1 })
      await cache.cancelQueries({ queryKey: ['service'], exact: true })
      await verify()
    },
  }
}
function coreIdentity(cache: QueryClient) {
  return cache.getQueryData<ServiceStatus>(['service'])?.core_started_at_millis ?? 0
}

// 节点与模式只共享写入准入和复核边界，各自仍读取自己的权威资源。
function useRoutingAction(kind: 'policy' | 'mode') {
  const api = useApi()
  const cache = useQueryClient()
  const operation = useRoutingOperation()
  const verify = useCallback(async () => {
    const queries = moduleQueries(api)
    // 模式应用可能重建核心，先确认新身份，避免随后正确的资源读取被旧身份保护丢弃。
    try {
      await cache.fetchQuery({ ...queries.service, staleTime: 0 })
    } catch {
      return
    }
    const requests =
      kind === 'policy'
        ? [cache.fetchQuery({ ...queries.policyGroups, staleTime: 0 })]
        : [cache.fetchQuery({ ...queries.mode, staleTime: 0 })]
    await Promise.allSettled(requests)
  }, [api, cache, kind])
  const cancelReads = async () => {
    await Promise.all(
      ['policy-groups', 'mode', 'service'].map((key) =>
        cache.cancelQueries({ queryKey: [key], exact: true }, { revert: false, silent: true }),
      ),
    )
  }
  const mutation = useMutation({
    retry: false,
    networkMode: 'always',
    mutationFn: async ({ target, epoch }: Pick<RoutingOperation, 'target' | 'epoch'>) => {
      let failure: unknown = null
      let responseReceived = false
      try {
        if (target.kind === 'policy') {
          const result = await api.request(
            { method: 'POST', path: '/policy-groups/select', body: target.value },
            policySnapshotSchema,
          )
          if (
            !result.live ||
            result.core_started_at_millis !== coreIdentity(cache) ||
            !result.groups.some(
              (group) =>
                group.tag === target.value.group_tag &&
                group.selectable &&
                group.selected === target.value.member_tag &&
                group.items.some((item) => item.tag === target.value.member_tag),
            )
          )
            throw new ApiError('client.invalid_response', 'Invalid selection result', 502)
        } else {
          const result = await api.request(
            { method: 'PUT', path: '/mode', body: { mode: target.value } },
            modeSchema,
          )
          if (result.mode !== target.value)
            throw new ApiError('client.invalid_response', 'Invalid mode result', 502)
        }
        responseReceived = true
      } catch (error) {
        failure = error
        responseReceived = error instanceof ApiError && error.responseReceived
        throw error
      } finally {
        if (routingEpoch(cache) === epoch) {
          // Fence old routing frames and reads; hand over the stream after authoritative reads.
          cache.setQueryData<RoutingOperation>(routingOperationKey, (current) => ({
            ...current!,
            epoch: epoch + 1,
            phase: definiteServiceRejection(failure) ? 'idle' : 'uncertain',
            error: failure,
            responseReceived,
          }))
          await cancelReads()
          await verify()
          cache.setQueryData<RoutingOperation>(routingOperationKey, (current) =>
            current?.epoch === epoch + 1 ? { ...current, eventEpoch: current.epoch } : current,
          )
        }
      }
    },
  })
  return {
    mutate: (target: RoutingOperation['target']) => {
      if (target.kind === 'policy') {
        const snapshot = cache.getQueryData<PolicySnapshot>(['policy-groups'])
        const status = cache.getQueryData<ServiceStatus>(['service'])
        const group = snapshot?.groups.find((group) => group.tag === target.value.group_tag)
        if (
          !policySelectSchema.safeParse(target.value).success ||
          !snapshot?.live ||
          status?.state !== 'ready' ||
          snapshot.core_started_at_millis !== status.core_started_at_millis ||
          cache.getQueryData(['events-connection']) !== 'live' ||
          !group?.selectable ||
          !group.items.some((item) => item.tag === target.value.member_tag)
        )
          return
      }
      const service = cache.getQueryData<ServiceOperation>(serviceOperationKey)
      if (
        routingBusy(cache) ||
        subscriptionBusy(cache) ||
        managementBusy(cache) ||
        (service && service.phase !== 'idle') ||
        !api.supports(target.kind === 'policy' ? 'policy.select' : 'mode.write')
      )
        return
      const epoch = routingEpoch(cache) + 1
      const eventEpoch = routingEventEpoch(cache)
      cache.setQueryData<RoutingOperation>(routingOperationKey, {
        epoch,
        eventEpoch,
        target,
        phase: 'pending',
        deadline: Date.now() + 150_000,
        responseReceived: false,
        error: null,
      })
      void cancelReads()
      mutation.mutate({ target, epoch })
    },
    isPending: operation?.phase === 'pending',
    isUncertain: operation?.phase === 'uncertain',
    isConfirming:
      operation?.phase === 'uncertain' &&
      operation.responseReceived &&
      operation.error === null &&
      mutation.isPending,
    isUnconfirmed: operation?.phase === 'unconfirmed',
    error: operation?.target.kind === kind ? operation.error : null,
    persisted:
      operation?.target.kind === kind &&
      operation.error instanceof ApiError &&
      operation.error.persisted,
    verify: async () => {
      const current = cache.getQueryData<RoutingOperation>(routingOperationKey)
      if (current?.phase === 'pending') return
      if (current) cache.setQueryData(routingOperationKey, { ...current, epoch: current.epoch + 1 })
      await cancelReads()
      await verify()
      cache.setQueryData<RoutingOperation>(routingOperationKey, (operation) =>
        operation ? { ...operation, eventEpoch: operation.epoch } : operation,
      )
    },
  }
}
export function useSelectPolicyGroup() {
  const action = useRoutingAction('policy')
  return { ...action, mutate: (value: PolicyTarget) => action.mutate({ kind: 'policy', value }) }
}
export function useChangeMode() {
  const action = useRoutingAction('mode')
  return { ...action, mutate: (value: string) => action.mutate({ kind: 'mode', value }) }
}
export { usePolicyDelay } from './node-delay'
