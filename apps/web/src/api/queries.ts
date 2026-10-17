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
  selectionSchema,
  selectedSchema,
  serviceSchema,
  snapshotSchema,
  type NodeSnapshot,
  type ServiceStatus,
} from './contracts'
import type { ApiClient } from './client'
import { ApiError } from './contracts'
import { resourceQueries } from './resources'
import { subscriptionBusy } from './subscription-control'
import type { NodeTarget } from './contracts'
import {
  observeRouting,
  routingEpoch,
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
  nodes: queryOptions({
    queryKey: ['nodes'],
    networkMode: 'always',
    enabled: api.supports('nodes.read'),
    queryFn: async ({ signal, client }) => {
      const epoch = routingEpoch(client)
      const service = serviceEpoch(client)
      const core = coreIdentity(client)
      const result = await api.request(
        { method: 'GET', path: '/nodes/snapshot' },
        snapshotSchema,
        signal,
      )
      if (
        signal.aborted ||
        epoch !== routingEpoch(client) ||
        service !== serviceEpoch(client) ||
        core !== coreIdentity(client)
      )
        throw new CancelledError({ silent: true, revert: false })
      observeRouting(client, result.selection)
      return result
    },
  }),
  selection: queryOptions({
    queryKey: ['selection'],
    networkMode: 'always',
    enabled: api.supports('selection.read'),
    queryFn: async ({ signal, client }) => {
      const epoch = routingEpoch(client)
      const service = serviceEpoch(client)
      const core = coreIdentity(client)
      const result = await api.request(
        { method: 'GET', path: '/selection' },
        selectionSchema,
        signal,
      )
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
  return {
    mutate: (action: ServiceAction) => {
      const current = cache.getQueryData<ServiceOperation | null>(serviceOperationKey)
      if ((current && current.phase !== 'idle') || routingBusy(cache) || subscriptionBusy(cache))
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
function useRoutingAction(kind: 'selection' | 'mode') {
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
      kind === 'selection'
        ? [
            cache.fetchQuery({ ...queries.nodes, staleTime: 0 }),
            cache.fetchQuery({ ...queries.selection, staleTime: 0 }),
          ]
        : [cache.fetchQuery({ ...queries.mode, staleTime: 0 })]
    await Promise.allSettled(requests)
  }, [api, cache, kind])
  const cancelReads = async () => {
    await Promise.all(
      ['nodes', 'selection', 'mode', 'service'].map((key) =>
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
        if (target.kind === 'selection') {
          const runtime = cache
            .getQueryData<NodeSnapshot>(['nodes'])
            ?.groups.find((group) => group.group.id === target.value.group_id)?.group.runtime_tag
          const result = await api.request(
            { method: 'PUT', path: '/selection', body: target.value },
            selectedSchema,
          )
          if (
            result.group_id !== target.value.group_id ||
            result.mode !== (target.value.node_tag ? 'manual' : 'urltest') ||
            (runtime !== undefined &&
              result.selected !==
                (target.value.node_tag ? `${runtime}/${target.value.node_tag}` : `Auto/${runtime}`))
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
          // 先失效旧流和旧读取，再查询已提交状态；迟到响应不能覆盖新快照。
          cache.setQueryData<RoutingOperation>(routingOperationKey, (current) => ({
            ...current!,
            epoch: epoch + 1,
            phase: definiteServiceRejection(failure) ? 'idle' : 'uncertain',
            error: failure,
            responseReceived,
          }))
          await cancelReads()
          await verify()
        }
      }
    },
  })
  return {
    mutate: (target: RoutingOperation['target']) => {
      const service = cache.getQueryData<ServiceOperation>(serviceOperationKey)
      if (
        routingBusy(cache) ||
        subscriptionBusy(cache) ||
        (service && service.phase !== 'idle') ||
        !api.supports(`${target.kind}.write`)
      )
        return
      const epoch = routingEpoch(cache) + 1
      cache.setQueryData<RoutingOperation>(routingOperationKey, {
        epoch,
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
    },
  }
}
export function useSelectNode() {
  const action = useRoutingAction('selection')
  return { ...action, mutate: (value: NodeTarget) => action.mutate({ kind: 'selection', value }) }
}
export function useChangeMode() {
  const action = useRoutingAction('mode')
  return { ...action, mutate: (value: string) => action.mutate({ kind: 'mode', value }) }
}
export { useNodeDelay } from './node-delay'
