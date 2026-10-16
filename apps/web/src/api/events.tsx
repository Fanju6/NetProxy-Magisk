import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { useApi, usePreferences } from '@/app/context'
import {
  envelopeSchema,
  groupSchema,
  serviceSchema,
  snapshotSchema,
  trafficSchema,
  type ServiceStatus,
  type Traffic,
  ApiError,
} from './contracts'
import { Button } from '@/components/ui/button'
import {
  observeService,
  serviceEpoch,
  serviceOperationKey,
  useServiceOperation,
  type ServiceOperation,
} from './service-control'
import { moduleQueries } from './queries'

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
  const operationEpoch = useServiceOperation()?.epoch ?? 0
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
        !disposed && generation === currentGeneration && serviceEpoch(cache) === operationEpoch
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
                  if (
                    next.state !== 'ready' ||
                    status?.core_started_at_millis !== next.core_started_at_millis
                  )
                    clearTraffic()
                  status = next
                  if (!active()) return
                  publish('service', next)
                  observeService(cache, next)
                } else if (event.name === 'groups')
                  publish('nodes', snapshotSchema.parse(envelope.data))
                else if (event.name === 'subscriptions')
                  publish('subscriptions', z.array(groupSchema).parse(envelope.data))
                else {
                  const traffic = trafficSchema.parse(envelope.data)
                  if (!traffic.available) clearTraffic()
                  else if (acceptTraffic(status, traffic)) {
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
                  liveAt = performance.now()
                  setConnection('live')
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
      if (operation && operation.phase !== 'pending') {
        // 操作后的 GET 必须先于新流建立；旧连接迟到帧没有服务端序号可供可靠排序。
        void cache
          .fetchQuery({ ...moduleQueries(api).service, staleTime: 0 })
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
  }, [api, cache, attempt, operationEpoch])
  return connection !== 'live' ? (
    <div
      role={connection === 'offline' || connection === 'reconnecting' ? 'alert' : 'status'}
      className="flex flex-wrap items-center justify-between gap-3 border-b bg-muted/50 px-5 py-3 text-sm"
    >
      <div className="space-y-1">
        <p>{t(connectionMessages[connection])}</p>
        <p className="text-xs text-muted-foreground">{t('offlineDescription')}</p>
      </div>
      <Button
        variant="link"
        onClick={() => {
          setAttempt((value) => value + 1)
        }}
      >
        {t('reconnect')}
      </Button>
    </div>
  ) : null
}
