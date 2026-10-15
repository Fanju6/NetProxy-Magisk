import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
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
} from './contracts'
import { Button } from '@/components/ui/button'

export function acceptTraffic(service: ServiceStatus | undefined, traffic: Traffic): boolean {
  return (
    traffic.available &&
    service?.state === 'ready' &&
    traffic.started_at_millis === service.core_started_at_millis
  )
}

export function ModuleEvents() {
  const api = useApi()
  const cache = useQueryClient()
  const { t } = usePreferences()
  const [error, setError] = useState(false)
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let close: (() => void) | undefined
    let disposed = false
    let generation = 0
    const clearTraffic = () => {
      cache.setQueryData<Traffic>(['traffic'], { available: false })
      cache.setQueryData<number[]>(['traffic-history'], [])
    }
    const publish = (key: string, value: unknown) => {
      void cache.cancelQueries({ queryKey: [key], exact: true }, { revert: false, silent: true })
      cache.setQueryData([key], value)
    }
    const stop = () => {
      generation++
      close?.()
      close = undefined
      clearTraffic()
    }
    const start = () => {
      stop()
      if (document.visibilityState === 'hidden' || disposed) return
      const currentGeneration = generation
      // 单一会话只接受当前连接的完整快照，断流后不沿用旧核心流量。
      let status: ServiceStatus | undefined
      close = api.transport.subscribe(
        ['status', 'groups', 'subscriptions'],
        (event) => {
          if (disposed || generation !== currentGeneration) return
          try {
            const envelope = envelopeSchema.parse(event.body)
            if (!envelope.ok) throw new Error(envelope.code)
            if (event.name === 'status') {
              status = serviceSchema.parse(envelope.data)
              publish('service', status)
              if (status.state !== 'ready') clearTraffic()
            } else if (event.name === 'groups')
              publish('nodes', snapshotSchema.parse(envelope.data))
            else if (event.name === 'subscriptions')
              publish('subscriptions', z.array(groupSchema).parse(envelope.data))
            else {
              const traffic = trafficSchema.parse(envelope.data)
              if (!traffic.available) clearTraffic()
              else if (acceptTraffic(status, traffic)) {
                cache.setQueryData(['traffic'], traffic)
                cache.setQueryData<number[]>(['traffic-history'], (previous) =>
                  [...(previous ?? []), traffic.status.downlink].slice(-30),
                )
              }
            }
            setError(false)
          } catch {
            stop()
            setError(true)
          }
        },
        () => {
          if (!disposed && generation === currentGeneration) {
            stop()
            setError(true)
          }
        },
      )
    }
    start()
    const visibility = () => (document.visibilityState === 'hidden' ? stop() : start())
    document.addEventListener('visibilitychange', visibility)
    return () => {
      disposed = true
      document.removeEventListener('visibilitychange', visibility)
      stop()
    }
  }, [api, cache, attempt])
  return error ? (
    <div role="alert" className="border-b bg-destructive/5 px-5 py-3 text-sm">
      <span>{t('offline')}</span>
      <Button
        variant="link"
        onClick={() => {
          setError(false)
          setAttempt((value) => value + 1)
        }}
      >
        {t('reconnect')}
      </Button>
    </div>
  ) : null
}
