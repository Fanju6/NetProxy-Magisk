import { useEffect, useRef, useState } from 'react'
import {
  CancelledError,
  queryOptions,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { z } from 'zod'
import { useApi } from '@/app/context'
import type { ApiClient } from './client'
import {
  groupSchema,
  historySchema,
  subscriptionDetailSchema,
  subscriptionProgressSchema,
  type GroupSummary,
} from './contracts'

const sessionKey = ['resource-session'] as const
const epoch = (cache: QueryClient) => cache.getQueryData<number>(sessionKey) ?? 0
const resourceKey = (key: readonly unknown[]) => key[0] === 'resource' || key[0] === 'subscriptions'
export function resetResourceSession(cache: QueryClient) {
  cache.setQueryData(sessionKey, epoch(cache) + 1)
  void cache.cancelQueries(
    { predicate: (query) => resourceKey(query.queryKey) },
    { revert: false, silent: true },
  )
}
function version(group: GroupSummary) {
  return Date.parse(group.updated_at) || 0
}
export function mergeResourceSummaries(
  previous: GroupSummary[] | undefined,
  incoming: GroupSummary[],
) {
  return incoming.map((group) => {
    const old = previous?.find((item) => item.id === group.id)
    return old &&
      (old.revision > group.revision ||
        (old.revision === group.revision && version(old) > version(group)))
      ? old
      : group
  })
}
export function acceptResourceEvent(
  cache: QueryClient,
  incoming: GroupSummary[],
  initial: boolean,
) {
  const previous = cache.getQueryData<GroupSummary[]>(['subscriptions'])
  // Hub 首帧没有全局版本，不能据此覆盖更新的 GET，分组增删同样要重新读取。
  if (
    initial ||
    !previous ||
    previous.length !== incoming.length ||
    incoming.some((group) => !previous.some((old) => old.id === group.id))
  ) {
    void cache
      .cancelQueries({ queryKey: ['subscriptions'], exact: true }, { revert: false, silent: true })
      .then(() => cache.invalidateQueries({ queryKey: ['subscriptions'], exact: true }))
  } else {
    void cache.cancelQueries(
      { queryKey: ['subscriptions'], exact: true },
      { revert: false, silent: true },
    )
    cache.setQueryData(['subscriptions'], mergeResourceSummaries(previous, incoming))
  }
  for (const id of new Set([...incoming, ...(previous ?? [])].map((group) => group.id))) {
    const group = incoming.find((item) => item.id === id)
    const old = previous?.find((item) => item.id === id)
    if (
      !group ||
      !old ||
      old.revision !== group.revision ||
      old.updated_at !== group.updated_at ||
      JSON.stringify(old.progress) !== JSON.stringify(group.progress)
    )
      void cache
        .cancelQueries({ queryKey: ['resource', id] }, { revert: false, silent: true })
        .then(() => cache.invalidateQueries({ queryKey: ['resource', id] }))
  }
}
export const resourceQueries = (api: ApiClient) => {
  const list = (path: '/catalog' | '/subscriptions') =>
    queryOptions({
      queryKey: ['subscriptions'],
      networkMode: 'always',
      retry: false,
      enabled: api.supports(path === '/catalog' ? 'catalog.read' : 'subscriptions.read'),
      queryFn: async ({ signal, client }) => {
        const generation = epoch(client)
        const result = await api.request({ method: 'GET', path }, z.array(groupSchema), signal)
        if (signal.aborted || generation !== epoch(client))
          throw new CancelledError({ silent: true, revert: false })
        return mergeResourceSummaries(client.getQueryData(['subscriptions']), result)
      },
    })
  const read = <S extends z.ZodType>(
    id: string,
    kind: 'detail' | 'history' | 'progress',
    schema: S,
  ) =>
    queryOptions({
      queryKey: ['resource', id, kind],
      networkMode: 'always',
      retry: false,
      staleTime: 0,
      enabled: id !== '' && api.supports(`subscriptions.${kind}`),
      queryFn: async ({ signal, client }) => {
        const generation = epoch(client)
        const result = await api.request(
          { method: 'GET', path: `/subscriptions/${id}${kind === 'detail' ? '' : `/${kind}`}` },
          schema,
          signal,
        )
        if (signal.aborted || generation !== epoch(client))
          throw new CancelledError({ silent: true, revert: false })
        return result
      },
    })
  return {
    catalog: list('/catalog'),
    subscriptions: list('/subscriptions'),
    detail: (id: string) =>
      read(
        id,
        'detail',
        subscriptionDetailSchema.refine((value) => value.group.id === id),
      ),
    history: (id: string) => read(id, 'history', historySchema),
    progress: (id: string) =>
      read(
        id,
        'progress',
        subscriptionProgressSchema.refine(
          (value) => value.id === id && (!value.progress || value.progress.group_id === id),
        ),
      ),
  }
}

export function useResourceDetails(id: string, isURL: boolean, live: boolean) {
  const api = useApi()
  const cache = useQueryClient()
  const queries = resourceQueries(api)
  const [refreshEpoch, setRefreshEpoch] = useState(0)
  const [expired, setExpired] = useState<string | null>(null)
  const session = useQuery({
    queryKey: sessionKey,
    queryFn: () => 0,
    initialData: 0,
    enabled: false,
  }).data
  const detail = useQuery({
    ...queries.detail(id),
    enabled: !!id && live && api.supports('subscriptions.detail'),
  })
  const history = useQuery({
    ...queries.history(id),
    enabled: !!id && isURL && live && api.supports('subscriptions.history'),
  })
  const progress = useQuery({
    ...queries.progress(id),
    enabled: !!id && isURL && live && api.supports('subscriptions.progress'),
    refetchInterval: (query) =>
      live &&
      expired !== `${id}:${refreshEpoch}:${session}` &&
      !query.state.error &&
      query.state.data?.progress
        ? 5000
        : false,
    refetchIntervalInBackground: false,
  })
  const active = !!progress.data?.progress
  const previous = useRef({ id, active: false })
  useEffect(() => {
    const finished = previous.current.id === id && previous.current.active && !active
    previous.current = { id, active }
    if (finished && live) {
      // 进度消失不是成功结果；重新读取持久详情和历史。
      for (const kind of ['detail', 'history'])
        void cache.invalidateQueries({ queryKey: ['resource', id, kind], exact: true })
    }
  }, [active, cache, id, live])
  const window = `${id}:${refreshEpoch}:${session}`
  const observing = live && active && expired !== window
  useEffect(() => {
    if (!id || !observing) return
    const timer = setTimeout(() => setExpired(window), 60_000)
    return () => clearTimeout(timer)
  }, [id, observing, window])
  useEffect(() => {
    if (!live)
      void cache.cancelQueries({ queryKey: ['resource', id] }, { revert: false, silent: true })
  }, [cache, id, live])
  const refresh = async () => {
    if (!live || !id) return
    setRefreshEpoch((value) => value + 1)
    await Promise.allSettled([
      detail.refetch(),
      ...(isURL ? [history.refetch(), progress.refetch()] : []),
    ])
  }
  return {
    detail,
    history,
    progress,
    refresh,
    observing: live && active && observing,
    refreshLimitReached: live && active && !observing,
  }
}
