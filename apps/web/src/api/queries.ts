import { useCallback } from 'react'
import { CancelledError, queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { useApi } from '@/app/context'
import {
  configSchema,
  groupSchema,
  infoSchema,
  modeSchema,
  serviceSchema,
  snapshotSchema,
} from './contracts'
import type { ApiClient } from './client'
import { ApiError } from './contracts'
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
      const status = await api.request({ method: 'GET', path: '/service' }, serviceSchema, signal)
      if (signal.aborted || serviceEpoch(client) !== epoch)
        throw new CancelledError({ silent: true, revert: false })
      observeService(client, status)
      return status
    },
  }),
  nodes: queryOptions({
    queryKey: ['nodes'],
    enabled: api.supports('nodes.read'),
    queryFn: ({ signal }) =>
      api.request({ method: 'GET', path: '/nodes/snapshot' }, snapshotSchema, signal),
  }),
  subscriptions: queryOptions({
    queryKey: ['subscriptions'],
    enabled: api.supports('subscriptions.read'),
    queryFn: ({ signal }) =>
      api.request({ method: 'GET', path: '/subscriptions' }, z.array(groupSchema), signal),
  }),
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
      if (current && current.phase !== 'idle') return
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
export function useSelectNode() {
  const api = useApi()
  const cache = useQueryClient()
  return useMutation({
    mutationFn: (body: { group_id: string; node_tag: string }) =>
      api.request(
        { method: 'PUT', path: '/selection', body },
        z.object({ group_id: z.string(), mode: z.string(), selected: z.string() }),
      ),
    onSettled: async () => {
      await Promise.all(
        ['nodes', 'service', 'subscriptions'].map((key) =>
          cache.invalidateQueries({ queryKey: [key] }),
        ),
      )
    },
  })
}
export function useChangeMode() {
  const api = useApi()
  const cache = useQueryClient()
  return useMutation({
    mutationFn: (mode: string) =>
      api.request({ method: 'PUT', path: '/mode', body: { mode } }, modeSchema),
    onSettled: async () => {
      await cache.invalidateQueries({ queryKey: ['service'] })
    },
  })
}
