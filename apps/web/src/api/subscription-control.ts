import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { useApi } from '@/app/context'
import { ApiError, groupIdSchema, type GroupSummary } from './contracts'
import type { ApiClient } from './client'
import { resourceQueries, resetResourceSession } from './resources'
import { routingBusy } from './routing-control'
import { serviceOperationKey, type ServiceOperation } from './service-control'
import {
  subscriptionCreateSchema,
  subscriptionInputSchema,
  type SubscriptionInput,
} from './subscription-input'
import type { ApiRequest } from './contracts'

export const subscriptionResultSchema = z.object({
  group_id: groupIdSchema,
  node_count: z.number().int().nonnegative(),
  revision: z.number().int().nonnegative(),
  not_modified: z.boolean(),
  persisted: z.boolean(),
  runtime_synced: z.boolean(),
  runtime_sync_state: z.enum(['applied', 'not_running', 'failed']),
  runtime_sync_pending: z.boolean(),
})
const safeFacts = subscriptionResultSchema
  .partial()
  .extend({ name_changed: z.boolean().optional(), requires_update: z.boolean().optional() })
export const subscriptionOutcomeSchema = safeFacts.extend({
  id: groupIdSchema.optional(),
  updated: z.array(groupIdSchema).optional(),
  failed: z.array(groupIdSchema).optional(),
  first_error: safeFacts.optional(),
})
type SubscriptionOutcome = z.infer<typeof subscriptionOutcomeSchema>
export type SubscriptionCommand =
  | { action: 'update'; groupId: string }
  | { action: 'add'; input: SubscriptionInput }
  | { action: 'edit'; groupId: string; input: SubscriptionInput }
  | { action: 'remove'; groupId: string; replacementId?: string }
  | { action: 'update_all' }
export type SubscriptionOperation = {
  epoch: number
  intent: number
  action: SubscriptionCommand['action']
  groupId?: string
  phase: 'pending' | 'uncertain' | 'unconfirmed' | 'idle'
  deadline: number
  error: ApiError | null
  result?: SubscriptionOutcome
  cancel: 'idle' | 'pending' | 'requested' | 'unknown'
  cancelError?: ApiError
}
export const subscriptionOperationKey = ['subscription-operation'] as const
export const subscriptionEpoch = (cache: QueryClient) =>
  cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)?.epoch ?? 0
export function subscriptionEventEpoch(cache: QueryClient) {
  const operation = cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)
  return (operation?.epoch ?? 0) - (operation?.phase === 'pending' ? 1 : 0)
}

function safeError(error: unknown) {
  return error instanceof ApiError
    ? new ApiError(
        /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/.test(error.code)
          ? error.code
          : 'client.invalid_response',
        'Subscription operation failed',
        error.status,
        subscriptionOutcomeSchema.safeParse(error.data).data ?? {},
        error.responseReceived,
      )
    : new ApiError('client.connection_failed', 'Subscription result is not confirmed', 0)
}
export function subscriptionBusy(cache: QueryClient) {
  const current = cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)
  if (current?.phase === 'uncertain' && Date.now() >= current.deadline) {
    cache.setQueryData(subscriptionOperationKey, { ...current, phase: 'unconfirmed' })
    return false
  }
  return current?.phase === 'pending' || current?.phase === 'uncertain'
}
export function useSubscriptionOperation() {
  const cache = useQueryClient()
  const operation = useQuery<SubscriptionOperation | null>({
    queryKey: subscriptionOperationKey,
    queryFn: () => null,
    enabled: false,
    initialData: null,
    gcTime: Infinity,
  }).data
  useEffect(() => {
    if (operation?.phase !== 'uncertain') return
    const timer = setTimeout(
      () => subscriptionBusy(cache),
      Math.max(0, operation.deadline - Date.now()),
    )
    return () => clearTimeout(timer)
  }, [cache, operation])
  return operation
}

export async function verifySubscription(api: ApiClient, cache: QueryClient) {
  const current = cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)
  if (!current || current.phase === 'pending') return
  const queries = resourceQueries(api)
  await cache.cancelQueries(
    { queryKey: ['subscriptions'], exact: true },
    { revert: false, silent: true },
  )
  try {
    const groups = await cache.fetchQuery({ ...queries.catalog, staleTime: 0 })
    const group = groups.find((item) => item.id === current.groupId)
    const detailReads =
      group?.type === 'subscription'
        ? await Promise.allSettled([
            cache.fetchQuery(queries.detail(group.id)),
            cache.fetchQuery(queries.history(group.id)),
            cache.fetchQuery(queries.progress(group.id)),
          ])
        : []
    if (subscriptionEpoch(cache) !== current.epoch) return
    const result = current.result
    // 丢失响应时没有任务身份，历史变化只能证明当前状态，不能归因本次操作。
    const confirmed =
      result &&
      (current.action === 'remove'
        ? !group && result.id === current.groupId
        : current.action === 'update_all'
          ? Array.isArray(result.updated) && Array.isArray(result.failed)
          : group &&
            result.group_id === group.id &&
            group.revision >= (result.revision ?? Infinity) &&
            group.node_count === result.node_count)
    const progress = detailReads[2]
    const completedError =
      current.error?.responseReceived &&
      current.error.code !== 'client.invalid_response' &&
      (!group ||
        (progress?.status === 'fulfilled' && !(progress.value as { progress?: unknown }).progress))
    if (confirmed || completedError)
      cache.setQueryData(subscriptionOperationKey, { ...current, phase: 'idle' })
    else subscriptionBusy(cache)
  } catch {
    // 读取失败保留原始写入错误；等待既有连接恢复或显式只读复核。
  }
}

export function beginSubscriptionUpdate(
  api: ApiClient,
  cache: QueryClient,
  groupId: string,
  signal: AbortSignal,
) {
  return beginSubscriptionWrite(api, cache, { action: 'update', groupId }, signal)
}

export function beginSubscriptionWrite(
  api: ApiClient,
  cache: QueryClient,
  command: SubscriptionCommand,
  signal: AbortSignal,
) {
  const groupId = 'groupId' in command ? command.groupId : undefined
  const service = cache.getQueryData<ServiceOperation>(serviceOperationKey)
  const group = cache
    .getQueryData<GroupSummary[]>(['subscriptions'])
    ?.find((item) => item.id === groupId)
  if (
    (groupId !== undefined &&
      (!groupIdSchema.safeParse(groupId).success || group?.type !== 'subscription')) ||
    !api.supports(`subscriptions.${command.action}`) ||
    subscriptionBusy(cache) ||
    routingBusy(cache) ||
    (service && service.phase !== 'idle')
  )
    return null
  if (
    'input' in command &&
    !(command.action === 'add' ? subscriptionCreateSchema : subscriptionInputSchema).safeParse(
      command.input,
    ).success
  )
    return null
  if (
    command.action === 'remove' &&
    command.replacementId !== undefined &&
    (!groupIdSchema.safeParse(command.replacementId).success || command.replacementId === groupId)
  )
    return null
  const epoch = subscriptionEpoch(cache) + 1
  cache.setQueryData<SubscriptionOperation>(subscriptionOperationKey, {
    epoch,
    intent: epoch,
    ...(groupId ? { groupId } : {}),
    action: command.action,
    phase: 'pending',
    deadline: Date.now() + 620_000,
    error: null,
    cancel: 'idle',
  })
  resetResourceSession(cache)
  return (async () => {
    let result: SubscriptionOutcome | undefined
    let error: ApiError | null = null
    try {
      let request: ApiRequest
      let schema: z.ZodType<SubscriptionOutcome> = subscriptionResultSchema
      if (command.action === 'remove') {
        await cache.cancelQueries(
          { queryKey: ['subscriptions'], exact: true },
          { revert: false, silent: true },
        )
        const fresh = await cache.fetchQuery({ ...resourceQueries(api).catalog, staleTime: 0 })
        const target = fresh.find((item) => item.id === groupId)
        const replacement = fresh.find((item) => item.id === command.replacementId)
        if (
          target?.type !== 'subscription' ||
          (target.active && (!replacement || replacement.node_count === 0)) ||
          (command.replacementId !== undefined && (!replacement || replacement.node_count === 0))
        )
          throw new ApiError(
            'subscription.conflict',
            'The deletion target or replacement changed',
            409,
            null,
            true,
          )
        request = {
          method: 'DELETE',
          path: `/subscriptions/${groupId}`,
          ...(command.replacementId
            ? { query: { replacement_group_id: command.replacementId } }
            : {}),
        }
        schema = z.object({ id: z.literal(groupId!) })
      } else if (command.action === 'add')
        request = { method: 'POST', path: '/subscriptions', body: command.input }
      else if (command.action === 'edit') {
        request = { method: 'PUT', path: `/subscriptions/${groupId}`, body: command.input }
        schema = subscriptionResultSchema.extend({
          name_changed: z.boolean(),
          requires_update: z.boolean(),
        })
      } else if (command.action === 'update_all') {
        request = { method: 'POST', path: '/subscriptions/update-all' }
        schema = z
          .object({ updated: z.array(groupIdSchema), failed: z.array(groupIdSchema) })
          .refine(
            (value) =>
              new Set([...value.updated, ...value.failed]).size ===
              value.updated.length + value.failed.length,
          )
      } else request = { method: 'POST', path: `/subscriptions/${groupId}/update` }
      result = await api.request(request, schema, signal)
      if (groupId && command.action !== 'remove' && result.group_id !== groupId)
        throw new ApiError('client.invalid_response', 'Invalid subscription result', 502)
    } catch (failure) {
      result = undefined
      error = safeError(failure)
    }
    if (subscriptionEpoch(cache) !== epoch) return
    const latest = cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)!
    if (command.action === 'update_all' && error) {
      const partial = subscriptionOutcomeSchema.safeParse(error.data)
      if (partial.success && partial.data.updated && partial.data.failed) result = partial.data
    }
    const rejected =
      error &&
      !error.persisted &&
      ((error.responseReceived &&
        [400, 401, 403, 404, 409, 413, 422, 429].includes(error.status)) ||
        [
          'client.operation_denied',
          'api.invalid_request',
          'api.body_too_large',
          'api.read_only',
          'api.busy',
          'api.request_exists',
        ].includes(error.code))
    cache.setQueryData<SubscriptionOperation>(subscriptionOperationKey, {
      ...latest,
      ...(!groupId && result?.group_id ? { groupId: result.group_id } : {}),
      epoch: epoch + 1,
      phase: rejected ? 'idle' : 'uncertain',
      error,
      ...(result ? { result } : {}),
    })
    for (const key of ['subscriptions', 'resource', 'nodes', 'selection', 'service', 'mode'])
      void cache.invalidateQueries({ queryKey: [key] })
    if (!signal.aborted) await verifySubscription(api, cache)
  })()
}

export function useSubscriptionActions() {
  const api = useApi()
  const cache = useQueryClient()
  const operation = useSubscriptionOperation()
  const active = useRef<AbortController | null>(null)
  const cancelCall = useRef<AbortController | null>(null)
  const [cancellation, setCancellation] = useState<{
    groupId: string
    state: 'pending' | 'requested' | 'unknown'
    error?: ApiError
  } | null>(null)
  const checked = useRef<number | null>(null)
  const connection = useQuery<string>({
    queryKey: ['events-connection'],
    queryFn: () => 'connecting',
    enabled: false,
  }).data
  const release = useCallback(() => {
    active.current?.abort()
    cancelCall.current?.abort()
  }, [])
  useEffect(() => {
    const visibility = () => {
      if (document.visibilityState === 'hidden') release()
    }
    document.addEventListener('visibilitychange', visibility)
    return () => {
      document.removeEventListener('visibilitychange', visibility)
      release()
    }
  }, [release])
  useEffect(() => {
    if (connection && connection !== 'live') release()
  }, [connection, release])
  useEffect(() => {
    if (connection !== 'live') {
      checked.current = null
      return
    }
    if (operation && operation.phase !== 'pending' && checked.current !== operation.epoch) {
      checked.current = operation.epoch
      void verifySubscription(api, cache)
    }
  }, [api, cache, connection, operation])
  const start = (command: SubscriptionCommand) => {
    const controller = new AbortController()
    const task = beginSubscriptionWrite(api, cache, command, controller.signal)
    if (!task) return false
    setCancellation(null)
    active.current = controller
    void task.finally(() => {
      if (active.current === controller) active.current = null
    })
    return true
  }
  return {
    operation,
    cancellation,
    update: (groupId: string) => start({ action: 'update', groupId }),
    add: (input: SubscriptionInput) => start({ action: 'add', input }),
    edit: (groupId: string, input: SubscriptionInput) => start({ action: 'edit', groupId, input }),
    remove: (groupId: string, replacementId?: string) =>
      start({ action: 'remove', groupId, ...(replacementId ? { replacementId } : {}) }),
    updateAll: () => start({ action: 'update_all' }),
    verify: () => verifySubscription(api, cache),
    cancel: async (groupId: string) => {
      if (
        cancelCall.current ||
        !api.supports('subscriptions.cancel') ||
        !groupIdSchema.safeParse(groupId).success ||
        cache.getQueryData<GroupSummary[]>(['subscriptions'])?.find((item) => item.id === groupId)
          ?.type !== 'subscription'
      )
        return
      const controller = new AbortController()
      cancelCall.current = controller
      setCancellation({ groupId, state: 'pending' })
      const current = cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)
      if (current?.groupId === groupId)
        cache.setQueryData(subscriptionOperationKey, { ...current, cancel: 'pending' })
      let error: ApiError | undefined
      try {
        await api.request(
          { method: 'POST', path: `/subscriptions/${groupId}/cancel` },
          z.object({ id: z.literal(groupId) }),
          controller.signal,
        )
      } catch (failure) {
        error = safeError(failure)
      }
      const latest = cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)
      if (latest?.groupId === groupId && latest.intent === current?.intent)
        cache.setQueryData(subscriptionOperationKey, {
          ...latest,
          cancel: error ? 'unknown' : 'requested',
          ...(error ? { cancelError: error } : {}),
        })
      if (cancelCall.current === controller) cancelCall.current = null
      if (!controller.signal.aborted)
        setCancellation({
          groupId,
          state: error ? 'unknown' : 'requested',
          ...(error ? { error } : {}),
        })
      if (!controller.signal.aborted) await verifySubscription(api, cache)
    },
  }
}
