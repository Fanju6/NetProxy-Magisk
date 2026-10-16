import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
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

export const moduleQueries = (api: ApiClient) => ({
  info: queryOptions({
    queryKey: ['info'],
    queryFn: ({ signal }) => api.request({ method: 'GET', path: '/info' }, infoSchema, signal),
  }),
  service: queryOptions({
    queryKey: ['service'],
    queryFn: ({ signal }) =>
      api.request({ method: 'GET', path: '/service' }, serviceSchema, signal),
  }),
  nodes: queryOptions({
    queryKey: ['nodes'],
    enabled: !api.transport.readOnly,
    queryFn: ({ signal }) =>
      api.request({ method: 'GET', path: '/nodes/snapshot' }, snapshotSchema, signal),
  }),
  subscriptions: queryOptions({
    queryKey: ['subscriptions'],
    enabled: !api.transport.readOnly,
    queryFn: ({ signal }) =>
      api.request({ method: 'GET', path: '/subscriptions' }, z.array(groupSchema), signal),
  }),
  autoStart: queryOptions({
    queryKey: ['config', 'module/auto_start'],
    enabled: !api.transport.readOnly,
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
  return useMutation({
    mutationFn: (action: 'start' | 'stop') =>
      api.request(
        { method: 'POST', path: `/service/${action}` },
        z.object({ action: z.string(), status: serviceSchema }),
      ),
    onSettled: async () => {
      await Promise.all([
        cache.invalidateQueries({ queryKey: ['service'] }),
        cache.invalidateQueries({ queryKey: ['nodes'] }),
      ])
    },
  })
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
