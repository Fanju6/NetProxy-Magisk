import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { useApi } from '@/app/context'
import { ApiError, groupIdSchema, type ApiRequest } from './contracts'
import type { ApiClient } from './client'
import { serviceOperationKey, type ServiceOperation } from './service-control'
import { routingBusy } from './routing-control'
import { subscriptionBusy } from './subscription-control'

const sections = [
  'log',
  'dns',
  'ntp',
  'certificate',
  'certificate_providers',
  'http_clients',
  'network_namespaces',
  'endpoints',
  'inbounds',
  'outbounds',
  'providers',
  'route',
  'services',
  'experimental',
]
export const configTargetSchema = z
  .string()
  .refine(
    (target) =>
      [
        'module',
        'module/auto_start',
        'module/wifi',
        'inbound',
        'inbound/backend',
        'inbound/root_policy',
        'inbound/app',
        'inbound/ebpf',
        'inbound/tun',
        'singbox/config.json',
        'runtime/inbound.json',
        'runtime/outbounds.json',
        'runtime/providers.json',
        ...sections.map((section) => `singbox/${section}`),
      ].includes(target) ||
      (/^singbox\/rules\/local\/[A-Za-z0-9_-][A-Za-z0-9_.-]*\.json$/.test(target) &&
        !target.includes('..')),
  )
export const configDocumentSchema = z.object({
  target: configTargetSchema,
  content: z.string(),
  revision: z.string().min(1),
})
export const configTargetsSchema = z.array(
  z.object({
    id: configTargetSchema,
    filename: z.string(),
    category: z.string(),
    editable: z.boolean(),
    section: z.string().optional(),
  }),
)
const configInput = z.strictObject({ revision: z.string().min(1), content: z.string() })
const nodeInput = z.strictObject({
  group_id: groupIdSchema,
  content: z.string().min(1),
  allow_insecure: z.boolean(),
})
const nodeTarget = z.strictObject({ group_id: groupIdSchema, node_tag: z.string().min(1) })
const kindSchema = z.enum(['service', 'core'])

/** Converts only named domain requests, never a caller-controlled native HTTP request. */
export function managementRequest(
  request: ApiRequest,
): { operation: string; input?: unknown } | undefined {
  const { path, method, query, body } = request
  const noQuery = query === undefined
  if (method === 'GET' && body === undefined && noQuery) {
    const reads: Record<string, string> = {
      '/configs': 'config_list',
      '/android/apps': 'apps',
      '/network/wifi-list': 'wifi',
      '/diagnostics/export': 'diagnostics',
    }
    const read = Object.hasOwn(reads, path) ? reads[path] : undefined
    if (read) return { operation: read }
    const icon = /^\/android\/icons\/([a-f0-9]{64})$/.exec(path)
    if (icon) return { operation: 'icon', input: { icon_key: icon[1] } }
  }
  if (
    (path === '/config' || path === '/config/validate') &&
    Object.keys(query ?? {}).length === 1 &&
    configTargetSchema.safeParse(query?.target).success
  ) {
    const target = query!.target!
    if (path === '/config' && method === 'GET' && body === undefined)
      return { operation: 'config_read', input: { target } }
    if (
      !target.startsWith('runtime/') &&
      ((path === '/config' && method === 'PUT') ||
        (path === '/config/validate' && method === 'POST'))
    ) {
      const parsed = configInput.safeParse(body)
      if (parsed.success)
        return {
          operation: method === 'PUT' ? 'config_apply' : 'config_validate',
          input: { target, ...parsed.data },
        }
    }
  }
  if (
    path === '/logs' &&
    body === undefined &&
    Object.keys(query ?? {}).every((key) =>
      ['kind', ...(method === 'GET' ? ['lines'] : [])].includes(key),
    )
  ) {
    const kind = kindSchema.safeParse(query?.kind ?? 'service')
    const lines = Number(query?.lines ?? 200)
    if (kind.success && method === 'GET' && Number.isInteger(lines) && lines >= 1 && lines <= 2000)
      return { operation: 'logs', input: { kind: kind.data, lines } }
    if (kind.success && method === 'DELETE')
      return { operation: 'logs_clear', input: { kind: kind.data } }
  }
  if (path === '/diagnostics/ebpf' && method === 'POST' && noQuery) {
    const parsed = z
      .strictObject({ mode: z.enum(['configured', 'all', 'local', 'shared']), raw: z.boolean() })
      .safeParse(body)
    if (parsed.success) return { operation: 'ebpf', input: parsed.data }
  }
  if (
    ['/nodes/get', '/nodes/export', '/nodes'].includes(path) &&
    body === undefined &&
    (method === 'GET' || method === 'DELETE')
  ) {
    const parsed = nodeTarget.safeParse(query)
    if (parsed.success && (method === 'DELETE' ? path === '/nodes' : path !== '/nodes'))
      return {
        operation:
          method === 'DELETE' ? 'node_remove' : path === '/nodes/get' ? 'node_read' : 'node_export',
        input: parsed.data,
      }
  }
  if (path === '/nodes' && noQuery && (method === 'POST' || method === 'PUT')) {
    const parsed = (
      method === 'POST' ? nodeInput : nodeInput.extend({ node_tag: z.string().min(1) })
    ).safeParse(body)
    if (parsed.success)
      return { operation: method === 'POST' ? 'node_add' : 'node_edit', input: parsed.data }
  }
  if (path === '/nodes/import' && method === 'POST' && noQuery) {
    const parsed = nodeInput.omit({ group_id: true }).safeParse(body)
    if (parsed.success) return { operation: 'node_import', input: parsed.data }
  }
}

export function readConfig(api: ApiClient, target: string, signal?: AbortSignal) {
  return api.request(
    { method: 'GET', path: '/config', query: { target } },
    configDocumentSchema,
    signal,
  )
}
export const managementKey = ['management-operation'] as const
type Operation = { epoch: number; pending: boolean }
export const managementBusy = (cache: QueryClient) =>
  cache.getQueryData<Operation>(managementKey)?.pending === true
export const managementEpoch = (cache: QueryClient) =>
  cache.getQueryData<Operation>(managementKey)?.epoch ?? 0

// Only operation metadata is shared; configuration, node credentials and form bodies stay local.
export function useManagementAction() {
  const api = useApi()
  const cache = useQueryClient()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const calls = useRef(new Set<AbortController>())
  useEffect(() => {
    const release = () => {
      for (const call of calls.current) call.abort()
    }
    const visibility = () => {
      if (document.visibilityState === 'hidden') release()
    }
    document.addEventListener('visibilitychange', visibility)
    const stop = api.transport.observeVisibility?.((visible) => {
      if (!visible) release()
    })
    return () => {
      release()
      stop?.()
      document.removeEventListener('visibilitychange', visibility)
    }
  }, [api])
  const run = useCallback(
    async <S extends z.ZodType>(
      request: ApiRequest,
      schema: S,
      signal?: AbortSignal,
    ): Promise<z.infer<S>> => {
      const writing = request.method !== 'GET'
      const service = cache.getQueryData<ServiceOperation>(serviceOperationKey)
      if (
        writing &&
        (managementBusy(cache) ||
          routingBusy(cache) ||
          subscriptionBusy(cache) ||
          (service && service.phase !== 'idle'))
      )
        throw new ApiError('api.busy', 'Another operation is still pending', 409)
      const controller = new AbortController()
      const abort = () => controller.abort()
      signal?.addEventListener('abort', abort, { once: true })
      if (signal?.aborted) controller.abort()
      calls.current.add(controller)
      if (writing)
        cache.setQueryData(managementKey, { epoch: managementEpoch(cache), pending: true })
      setPending(true)
      setError(null)
      try {
        return await api.request(request, schema, controller.signal)
      } catch (failure) {
        // Native errors are sanitized; never render an unknown exception carrying form content.
        const safe =
          failure instanceof ApiError
            ? failure
            : new ApiError(
                'client.connection_failed',
                'Result is not confirmed; reload before trying again',
                0,
              )
        setError(safe)
        throw safe
      } finally {
        signal?.removeEventListener('abort', abort)
        calls.current.delete(controller)
        setPending(calls.current.size > 0)
        if (writing) {
          cache.setQueryData(managementKey, { epoch: managementEpoch(cache) + 1, pending: false })
          for (const key of [
            'service',
            'nodes',
            'selection',
            'mode',
            'subscriptions',
            'resource',
            'config',
          ])
            void cache.invalidateQueries({ queryKey: [key] })
        }
      }
    },
    [api, cache],
  )
  return { run, pending, error }
}
