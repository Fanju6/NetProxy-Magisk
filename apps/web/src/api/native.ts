import { addPluginListener, invoke, isTauri } from '@tauri-apps/api/core'
import { z } from 'zod'
import {
  ApiError,
  type ApiEvent,
  type ApiRequest,
  type ApiTransport,
  type EventTopic,
} from './contracts'
import { unavailableTransport } from './client'

export interface NativeIpc {
  invoke<T>(command: string, args: Record<string, unknown>): Promise<T>
  listen(listener: (event: unknown) => void): Promise<{ unregister(): Promise<void> }>
}
const officialIpc: NativeIpc = {
  invoke,
  listen: (listener) => addPluginListener('module-api', 'visibility', listener),
}
const responseSchema = z.object({ status: z.number().int().min(100).max(599), body: z.unknown() })
const eventSchema = z.object({
  name: z.enum(['status', 'groups', 'subscriptions', 'traffic']),
  body: z.unknown(),
})
const visibilitySchema = z.object({ visible: z.boolean() })
const failureSchema = z.object({
  code: z
    .string()
    .regex(/^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+$/)
    .max(96),
  status: z.number().int().min(0).max(599),
  message: z.string(),
})
const command = (name: string) => `plugin:module-api|${name}`
export function createNativeTransport(): ApiTransport {
  return isTauri() ? new NativeTransport() : unavailableTransport
}
const invalid = () => new ApiError('client.invalid_response', 'Invalid native response', 502)
function nativeError(error: unknown): ApiError {
  if (error instanceof ApiError) return error
  const parsed = failureSchema.safeParse(error)
  // 不转发异常原文：桥接层之外的错误可能包含 URL、凭据或系统路径。
  return parsed.success
    ? new ApiError(parsed.data.code, 'Native module request failed', parsed.data.status)
    : new ApiError('client.native_failed', 'Native module connection failed', 0)
}
function bounded<T>(schema: z.ZodType<T>, value: unknown): T {
  const parsed = schema.safeParse(value)
  if (!parsed.success) throw invalid()
  if (new TextEncoder().encode(JSON.stringify(value)).length > 4 * 1024 * 1024)
    throw new ApiError('api.body_too_large', 'Native response exceeds 4 MiB', 413)
  return parsed.data
}

export class NativeTransport implements ApiTransport {
  readonly kind = 'native' as const
  readonly readOnly = true
  private releaseEvents: (() => void) | undefined
  private eventGeneration = 0
  private releaseVisibility: (() => void) | undefined
  private visibilityGeneration = 0

  constructor(private readonly ipc: NativeIpc = officialIpc) {}

  private call<T>(name: string, args: Record<string, unknown>): Promise<T> {
    try {
      return this.ipc.invoke<T>(command(name), args)
    } catch (error) {
      return Promise.reject(error)
    }
  }

  private discard(name: 'cancel' | 'close_events', args: Record<string, unknown>) {
    void this.call(name, args).catch(() => {})
  }

  async request(request: ApiRequest, signal?: AbortSignal) {
    if (
      request.method !== 'GET' ||
      !['/info', '/service'].includes(request.path) ||
      request.body !== undefined ||
      request.query !== undefined
    )
      throw new ApiError('client.read_only', 'Native API resource is not allowed', 403)
    if (signal?.aborted) throw signal.reason
    const requestId = crypto.randomUUID()
    return new Promise<{ status: number; body: unknown }>((resolve, reject) => {
      let settled = false
      const finish = (complete: () => void) => {
        if (settled) return
        settled = true
        signal?.removeEventListener('abort', cancel)
        window.removeEventListener('pagehide', cancel)
        complete()
      }
      const cancel = () =>
        finish(() => {
          this.discard('cancel', { requestId })
          reject(signal?.reason ?? new DOMException('Native request cancelled', 'AbortError'))
        })
      signal?.addEventListener('abort', cancel, { once: true })
      window.addEventListener('pagehide', cancel, { once: true })
      void this.call('read', { requestId, resource: request.path.slice(1) })
        .then((value) => {
          if (!settled) {
            const response = bounded(responseSchema, value)
            finish(() => resolve(response))
          }
        })
        .catch((error: unknown) => finish(() => reject(nativeError(error))))
    })
  }

  subscribe(
    topics: readonly EventTopic[],
    consume: (event: ApiEvent) => void,
    onError: (error: Error) => void,
  ) {
    const generation = ++this.eventGeneration
    this.releaseEvents?.()
    if (generation !== this.eventGeneration) return () => {}
    const subscriptionId = crypto.randomUUID()
    const selected = [...new Set(topics)]
    let closed = false
    let requested = false
    const close = () => {
      if (requested) this.discard('close_events', { subscriptionId })
    }
    const release = () => {
      if (closed) return
      closed = true
      window.removeEventListener('pagehide', release)
      if (this.releaseEvents === release) this.releaseEvents = undefined
      close()
    }
    this.releaseEvents = release
    window.addEventListener('pagehide', release, { once: true })
    const run = async () => {
      try {
        if (
          selected.length === 0 ||
          selected.some((topic) => !['status', 'groups', 'subscriptions'].includes(topic))
        )
          throw new ApiError('client.invalid_request', 'Invalid event topics', 400)
        requested = true
        await this.call('open_events', { subscriptionId, topics: selected })
        // 关闭可能先于异步 open 完成；补偿清理迟到的原生句柄。
        if (closed) {
          close()
          return
        }
        while (!closed) {
          const value = await this.call('next_event', { subscriptionId })
          if (closed) return
          const event = bounded(eventSchema, value)
          if (
            !selected.includes(event.name as EventTopic) &&
            !(event.name === 'traffic' && selected.includes('status'))
          )
            throw invalid()
          consume(event)
        }
      } catch (error) {
        if (!closed) {
          release()
          if (generation === this.eventGeneration) onError(nativeError(error))
        }
      }
    }
    // 消费方回调异常不能让已释放的会话产生未处理的 Promise 拒绝。
    void run().catch(() => {})
    return release
  }

  observeVisibility(listener: (visible: boolean) => void) {
    const generation = ++this.visibilityGeneration
    this.releaseVisibility?.()
    if (generation !== this.visibilityGeneration) return () => {}
    let disposed = false
    let registration: { unregister(): Promise<void> } | undefined
    let revision = 0
    let previous: boolean | undefined
    const publish = (visible: boolean) => {
      if (!disposed && visible !== previous) {
        previous = visible
        try {
          listener(visible)
        } catch {
          // 消费方异常不能破坏原生事件分发或阻止后续清理。
        }
      }
    }
    const unregister = (handle: { unregister(): Promise<void> }) => {
      try {
        void handle.unregister().catch(() => {})
      } catch {
        // 文档可能已经释放原生回调注册表。
      }
    }
    const release = () => {
      if (disposed) return
      disposed = true
      window.removeEventListener('pagehide', release)
      if (this.releaseVisibility === release) this.releaseVisibility = undefined
      if (registration) unregister(registration)
      registration = undefined
    }
    this.releaseVisibility = release
    window.addEventListener('pagehide', release, { once: true })
    void Promise.resolve()
      .then(() => {
        if (disposed) return
        return this.ipc.listen((event) => {
          revision++
          const parsed = visibilitySchema.safeParse(event)
          publish(parsed.success && parsed.data.visible)
        })
      })
      .then(async (handle) => {
        if (!handle) return
        if (disposed) {
          unregister(handle)
          return
        }
        registration = handle
        // 注册完成前也可能收到事件，初始查询不能覆盖它。
        if (revision !== 0) return
        try {
          const initial = await this.call('visibility', {})
          if (revision === 0) publish(visibilitySchema.parse(initial).visible)
        } catch {
          if (revision === 0) publish(false)
        }
      })
      .catch(() => publish(false))
    return release
  }
}
