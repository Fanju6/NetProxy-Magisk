import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { useApi, usePreferences } from '@/app/context'
import {
  envelopeSchema,
  groupSchema,
  serviceSchema,
  policySnapshotSchema,
  trafficSchema,
  type ServiceStatus,
  type Traffic,
  ApiError,
} from './contracts'
import { Button } from '@/components/ui/button'
import { LoaderCircle, Pause, WifiOff } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  observeService,
  serviceEpoch,
  serviceOperationKey,
  useServiceOperation,
  type ServiceOperation,
} from './service-control'
import { moduleQueries } from './queries'
import {
  observeRouting,
  routingEpoch,
  routingOperationKey,
  useRoutingOperation,
  type RoutingOperation,
} from './routing-control'
import type { PolicySnapshot } from './contracts'
import { acceptResourceEvent, resetResourceSession } from './resources'
import { subscriptionEventEpoch, useSubscriptionOperation } from './subscription-control'
import { managementEpoch, managementKey } from './management'

export function acceptTraffic(service: ServiceStatus | undefined, traffic: Traffic): boolean {
  return (
    traffic.available &&
    service?.state === 'ready' &&
    traffic.started_at_millis === service.core_started_at_millis
  )
}

type EventConnection = 'connecting' | 'live' | 'reconnecting' | 'offline' | 'paused'
const connectionMessages = {
  connecting: 'eventConnecting',
  live: 'connected',
  reconnecting: 'eventReconnecting',
  offline: 'offline',
  paused: 'eventPaused',
} as const
export function useEventConnection() {
  return useQuery({
    queryKey: ['events-connection'],
    queryFn: (): EventConnection => 'connecting',
    initialData: 'connecting' as EventConnection,
    enabled: false,
  }).data
}

export function ModuleEvents() {
  const api = useApi()
  const cache = useQueryClient()
  const { t } = usePreferences()
  const connection = useEventConnection()
  const [attempt, setAttempt] = useState(0)
  const [showConnection, setShowConnection] = useState(false)
  const operationEpoch = useServiceOperation()?.epoch ?? 0
  const routeEpoch = useRoutingOperation()?.epoch ?? 0
  const subscriptionOperation = useSubscriptionOperation()
  const resourceEpoch =
    (subscriptionOperation?.epoch ?? 0) - (subscriptionOperation?.phase === 'pending' ? 1 : 0)
  const managementVersion =
    useQuery({
      queryKey: managementKey,
      queryFn: () => ({ epoch: 0, pending: false }),
      enabled: false,
    }).data?.epoch ?? 0
  useEffect(() => {
    let close: (() => void) | undefined
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let snapshotTimer: ReturnType<typeof setTimeout> | undefined
    let disposed = false
    let generation = 0
    let retries = 0
    let nativeVisible: boolean | undefined = api.transport.observeVisibility ? undefined : true
    let lastVisible: boolean | undefined
    const isVisible = () => document.visibilityState !== 'hidden' && nativeVisible === true
    const setConnection = (value: EventConnection) =>
      cache.setQueryData(['events-connection'], value)
    const clearTraffic = () => {
      cache.setQueryData<Traffic>(['traffic'], { available: false })
      cache.setQueryData<number[]>(['traffic-history'], [])
    }
    const publish = (key: string, value: unknown) => {
      void cache.cancelQueries({ queryKey: [key], exact: true }, { revert: false, silent: true })
      cache.setQueryData([key], value)
    }
    const stop = () => {
      const stopped = ++generation
      clearTimeout(retryTimer)
      clearTimeout(snapshotTimer)
      retryTimer = undefined
      snapshotTimer = undefined
      const release = close
      close = undefined
      clearTraffic()
      resetResourceSession(cache)
      // 先撤销会话身份，关闭回调即使同步重入也不能再次释放或发布数据。
      release?.()
      return stopped
    }
    const start = () => {
      const currentGeneration = stop()
      if (disposed || generation !== currentGeneration) return
      if (!isVisible()) {
        lastVisible = false
        setConnection('paused')
        return
      }
      setConnection(retries === 0 ? 'connecting' : 'reconnecting')
      const active = () =>
        !disposed &&
        generation === currentGeneration &&
        serviceEpoch(cache) === operationEpoch &&
        routingEpoch(cache) === routeEpoch &&
        subscriptionEventEpoch(cache) === resourceEpoch &&
        managementEpoch(cache) === managementVersion
      const pending = new Set(['status', 'groups', 'subscriptions'])
      let liveAt: number | undefined
      const fail = (retry = true) => {
        if (!active()) return
        // 短暂接通不能无限重置预算，完整快照稳定 30 秒后才开启下一轮恢复。
        if (liveAt !== undefined && performance.now() - liveAt >= 30_000) retries = 0
        const stopped = stop()
        if (disposed || generation !== stopped) return
        if (!isVisible()) {
          lastVisible = false
          setConnection('paused')
          return
        }
        if (!retry || retries === 3) {
          setConnection('offline')
          return
        }
        setConnection('reconnecting')
        retryTimer = setTimeout(start, 1000 * 2 ** retries++)
      }
      const onError = (error: unknown) =>
        fail(
          !(
            error instanceof ApiError &&
            error.status >= 400 &&
            error.status < 500 &&
            ![408, 429].includes(error.status)
          ),
        )
      // 单一会话只接受当前连接的完整快照，断流后不沿用旧核心流量。
      let status: ServiceStatus | undefined
      let pendingNodes: PolicySnapshot | undefined
      let initialResources = true
      const routing = cache.getQueryData<RoutingOperation | null>(routingOperationKey)
      const checkRouting =
        (routing != null && routing.phase !== 'pending') ||
        resourceEpoch > 0 ||
        managementVersion > 0
      let confirmed = !checkRouting
      let confirming = false
      let changedDuringCheck = false
      const publishNodes = (snapshot: PolicySnapshot) => {
        if (
          !status ||
          (snapshot.live &&
            (status.state !== 'ready' ||
              snapshot.core_started_at_millis !== status.core_started_at_millis))
        )
          return
        publish('policy-groups', snapshot)
        observeRouting(cache, snapshot)
      }
      const subscribe = () => {
        if (!active()) return
        snapshotTimer = setTimeout(fail, 30_000)
        try {
          const release = api.transport.subscribe(
            ['status', 'groups', 'subscriptions'],
            (event) => {
              if (!active()) return
              try {
                const envelope = envelopeSchema.parse(event.body)
                if (!envelope.ok) throw new Error(envelope.code)
                if (event.name === 'status') {
                  const next = serviceSchema.parse(envelope.data)
                  if (!confirmed) {
                    if (
                      confirming &&
                      status &&
                      (status.core_started_at_millis !== next.core_started_at_millis ||
                        status.state !== next.state)
                    )
                      changedDuringCheck = true
                    status = next
                  } else {
                    if (
                      next.state !== 'ready' ||
                      status?.core_started_at_millis !== next.core_started_at_millis
                    ) {
                      clearTraffic()
                      // 核心身份改变后不得在新分组快照前沿用旧实例的选择和延迟。
                      for (const key of ['policy-groups']) {
                        void cache.cancelQueries(
                          { queryKey: [key], exact: true },
                          { revert: false, silent: true },
                        )
                      }
                      cache.setQueryData<PolicySnapshot>(['policy-groups'], (snapshot) =>
                        snapshot
                          ? {
                              ...snapshot,
                              live: false,
                              core_started_at_millis: 0,
                              groups: snapshot.groups.map((group) => ({
                                ...group,
                                selected: '',
                                items: group.items.map((item) => ({
                                  tag: item.tag,
                                  type: item.type,
                                })),
                              })),
                            }
                          : undefined,
                      )
                    }
                    status = next
                    if (!active()) return
                    publish('service', next)
                    observeService(cache, next)
                    if (api.supports('mode.read')) {
                      const mode = {
                        mode: next.configured_outbound_mode,
                        available: next.available_outbound_modes,
                        ...(next.state === 'ready' ? { runtime_mode: next.outbound_mode } : {}),
                      }
                      publish('mode', mode)
                      observeRouting(cache, mode)
                    }
                    if (pendingNodes) {
                      publishNodes(pendingNodes)
                      pendingNodes = undefined
                    }
                  }
                } else if (event.name === 'groups') {
                  const nodes = policySnapshotSchema.parse(envelope.data)
                  if (confirmed) {
                    if (
                      !status ||
                      (nodes.live && nodes.core_started_at_millis !== status.core_started_at_millis)
                    )
                      pendingNodes = nodes
                    else publishNodes(nodes)
                  }
                } else if (event.name === 'subscriptions') {
                  const summaries = z.array(groupSchema).parse(envelope.data)
                  if (api.supports('catalog.read'))
                    acceptResourceEvent(cache, summaries, initialResources)
                  else publish('subscriptions', summaries)
                  initialResources = false
                } else {
                  const traffic = trafficSchema.parse(envelope.data)
                  if (!traffic.available) clearTraffic()
                  else if (confirmed && acceptTraffic(status, traffic)) {
                    cache.setQueryData(['traffic'], traffic)
                    if (!active()) return
                    cache.setQueryData<number[]>(['traffic-history'], (previous) =>
                      [...(previous ?? []), traffic.status.downlink].slice(-30),
                    )
                  }
                }
                if (!active()) return
                pending.delete(event.name)
                if (pending.size === 0 && liveAt === undefined) {
                  clearTimeout(snapshotTimer)
                  snapshotTimer = undefined
                  if (confirmed) {
                    liveAt = performance.now()
                    setConnection('live')
                  } else if (!confirming) {
                    confirming = true
                    // Hub 可能重放同核心的旧缓存。先消耗初始帧，再查询权威状态；
                    // 复核期间到达的帧不能覆盖 GET，也不能作为写入确认。
                    void (async () => {
                      const queries = moduleQueries(api)
                      await Promise.all(
                        ['service', 'policy-groups', 'mode'].map((key) =>
                          cache.cancelQueries(
                            { queryKey: [key], exact: true },
                            { revert: false, silent: true },
                          ),
                        ),
                      )
                      if (!active()) return
                      await cache.fetchQuery({ ...queries.service, staleTime: 0 })
                      if (!active()) return
                      const reads: Promise<unknown>[] = []
                      if (api.supports('policy.read'))
                        reads.push(cache.fetchQuery({ ...queries.policyGroups, staleTime: 0 }))
                      if (api.supports('mode.read'))
                        reads.push(cache.fetchQuery({ ...queries.mode, staleTime: 0 }))
                      await Promise.all(reads)
                      if (!active()) return
                      const authoritative = cache.getQueryData<ServiceStatus>(['service'])
                      if (
                        changedDuringCheck &&
                        (status?.core_started_at_millis !== authoritative?.core_started_at_millis ||
                          status?.state !== authoritative?.state)
                      ) {
                        fail()
                        return
                      }
                      status = authoritative
                      confirmed = true
                      liveAt = performance.now()
                      setConnection('live')
                    })().catch(onError)
                  }
                }
              } catch {
                fail(false)
              }
            },
            onError,
          )
          // subscribe 可同步报错、发布帧或触发后台释放；返回时只接管仍有效的会话。
          if (active()) close = release
          else release()
        } catch (error) {
          onError(error)
        }
      }
      const operation = cache.getQueryData<ServiceOperation | null>(serviceOperationKey)
      if (operation && operation.phase !== 'pending' && !checkRouting) {
        // 操作后的 GET 必须先于新流建立；旧连接迟到帧没有服务端序号可供可靠排序。
        const queries = moduleQueries(api)
        void cache
          .fetchQuery({ ...queries.service, staleTime: 0 })
          .catch(() => {})
          .finally(subscribe)
      } else subscribe()
    }
    const visibility = () => {
      const visible = isVisible()
      if (lastVisible === visible) return
      lastVisible = visible
      if (!visible) {
        const stopped = stop()
        if (generation === stopped && !disposed) setConnection('paused')
      } else {
        retries = 0
        start()
      }
    }
    document.addEventListener('visibilitychange', visibility)
    const releaseVisibility = api.transport.observeVisibility?.((visible) => {
      if (disposed) return
      nativeVisible = visible
      visibility()
    })
    visibility()
    return () => {
      disposed = true
      document.removeEventListener('visibilitychange', visibility)
      releaseVisibility?.()
      stop()
      setConnection('paused')
    }
  }, [api, cache, attempt, operationEpoch, routeEpoch, resourceEpoch, managementVersion])
  const Icon = connection === 'offline' ? WifiOff : connection === 'paused' ? Pause : LoaderCircle
  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        className={`size-12 ${connection === 'live' ? 'invisible' : ''}`}
        aria-hidden={connection === 'live'}
        tabIndex={connection === 'live' ? -1 : undefined}
        aria-label={`${t('connection')}: ${t(connectionMessages[connection])}`}
        title={t(connectionMessages[connection])}
        onClick={() => setShowConnection(true)}
      >
        <Icon
          className={
            connection === 'offline'
              ? 'text-destructive'
              : connection === 'paused'
                ? 'text-muted-foreground'
                : 'animate-spin text-muted-foreground motion-reduce:animate-none'
          }
        />
      </Button>
      {connection !== 'live' && (
        <span
          role={connection === 'offline' || connection === 'reconnecting' ? 'alert' : 'status'}
          className="sr-only"
        >
          {t(connectionMessages[connection])} · {t('offlineDescription')}
        </span>
      )}
      <Dialog open={showConnection} onOpenChange={setShowConnection}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t(connectionMessages[connection])}</DialogTitle>
            <DialogDescription>
              {t(connection === 'live' ? 'connected' : 'offlineDescription')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              onClick={() => {
                setShowConnection(false)
                setAttempt((value) => value + 1)
              }}
            >
              {t('reconnect')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
