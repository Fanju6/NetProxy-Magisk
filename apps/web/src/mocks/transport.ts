import type { ApiEvent, ApiRequest, ApiTransport, EventTopic } from '@/api/contracts'
import { createFixtures, demoTraffic } from './fixtures'
import { z } from 'zod'

const success = (code: string, data: unknown) => ({
  status: 200,
  body: { schema: 1, ok: true, code, message: 'Demo', data },
})
const failure = (status: number, code: string, data: unknown = {}) => ({
  status,
  body: { schema: 1, ok: false, code, message: 'Demo request failed', data },
})

export class MockTransport implements ApiTransport {
  readonly kind = 'mock' as const
  readonly readOnly = false
  private state = createFixtures()
  private listeners = new Set<{
    topics: readonly EventTopic[]
    consume: (event: ApiEvent) => void
  }>()
  private timer: ReturnType<typeof setInterval> | undefined
  private tick = 0
  private config = { auto_start: false }
  private revision = 1
  constructor(private readonly latency = 180) {}

  async request(request: ApiRequest, signal?: AbortSignal) {
    // 读取可取消；写入一旦被接受即完成，模拟 Go 的事务语义。
    if (signal?.aborted) throw signal.reason
    await new Promise<void>((resolve, reject) => {
      const complete = () => {
        signal?.removeEventListener('abort', cancel)
        resolve()
      }
      const timer = setTimeout(complete, this.latency)
      const cancel = () => {
        clearTimeout(timer)
        signal?.removeEventListener('abort', cancel)
        reject(signal?.reason)
      }
      if (request.method === 'GET') signal?.addEventListener('abort', cancel, { once: true })
    })
    const { service, snapshot } = this.state
    const allowed =
      request.path === '/config'
        ? ['target']
        : request.path === '/nodes/snapshot'
          ? ['group_id']
          : []
    if (Object.keys(request.query ?? {}).some((key) => !allowed.includes(key)))
      return failure(400, 'usage.invalid')
    const key = `${request.method} ${request.path}`
    switch (key) {
      case 'GET /info':
        return success('api.info', {
          api_version: 1,
          version: 'v8.6.1',
          version_code: '1088',
          instance_id: 'demo-worker-1',
          pid: 2400,
          platform: 'android',
          capabilities: ['module', 'catalog', 'subscriptions', 'offline_delay', 'events'],
          memory: {
            heap_alloc_bytes: 4194304,
            heap_inuse_bytes: 6291456,
            heap_released_bytes: 1048576,
            stack_inuse_bytes: 524288,
          },
        })
      case 'GET /service':
        return success('service.status', structuredClone(service))
      case 'GET /nodes/snapshot': {
        const id = request.query?.group_id?.trim()
        if (id && (!/^[\w.-]+$/.test(id) || id.includes('..') || id === 'staging'))
          return failure(400, 'usage.invalid')
        const result = structuredClone(snapshot)
        if (id) {
          result.groups = result.groups.filter((item) => item.group.id === id)
          if (result.groups.length === 0) return failure(500, 'command.failed')
        }
        return success('node.snapshot', result)
      }
      case 'GET /subscriptions':
        return success(
          'subscription.list',
          structuredClone(snapshot.groups.map((item) => item.group)),
        )
      case 'GET /mode':
        return success('mode.current', {
          mode: service.configured_outbound_mode,
          runtime_mode: service.outbound_mode,
          available: service.available_outbound_modes,
        })
      case 'POST /service/start':
      case 'POST /service/stop': {
        const running = request.path === '/service/start'
        if (request.body !== undefined && JSON.stringify(request.body) !== '{}')
          return failure(400, 'usage.invalid')
        service.state = running ? 'ready' : 'stopped'
        service.pid = running ? 2451 : null
        service.active_backend = running ? service.configured_backend : null
        service.core_started_at_millis = running ? Date.now() : 0
        service.started_at = running ? Math.floor(service.core_started_at_millis / 1000) : 0
        service.ready_at = service.started_at
        service.uptime_seconds = 0
        service.memory_bytes = running ? 48234496 : 0
        service.connections_in = 0
        service.connections_out = 0
        if (!running) snapshot.runtime_groups = []
        snapshot.selection.runtime_selected = running
          ? snapshot.selection.selected_node_ref ||
            `${snapshot.selection.active_group_id}/${snapshot.groups.find((item) => item.group.active)!.nodes[0]!.tag}`
          : ''
        service.runtime_selected = snapshot.selection.runtime_selected
        this.emit('status')
        this.emit('traffic')
        this.emit('groups')
        return success(running ? 'service.start' : 'service.stop', {
          action: running ? 'start' : 'stop',
          status: structuredClone(service),
        })
      }
      case 'PUT /selection': {
        const parsed = z
          .strictObject({ group_id: z.string().min(1), node_tag: z.string() })
          .safeParse(request.body)
        if (!parsed.success) return failure(400, 'usage.invalid')
        const body = parsed.data
        const group = snapshot.groups.find((item) => item.group.id === body.group_id)
        if (
          !group ||
          group.nodes.length === 0 ||
          (body.node_tag !== '' && !group.nodes.some((item) => item.tag === body.node_tag))
        )
          return failure(500, 'command.failed')
        const manual = body.node_tag !== ''
        const reference = manual ? `${group.group.id}/${body.node_tag}` : ''
        snapshot.selection = {
          active_group_id: group.group.id,
          active_group_name: group.group.name,
          active_group_runtime_tag: group.group.runtime_tag,
          active_group_node_count: group.nodes.length,
          selector_mode: manual ? 'manual' : 'urltest',
          selected_node_ref: reference,
          selected: manual ? reference : `Auto/${group.group.runtime_tag}`,
          runtime_selected:
            service.state === 'ready'
              ? manual
                ? reference
                : `${group.group.id}/${group.nodes[0]!.tag}`
              : '',
        }
        for (const item of snapshot.groups) item.group.active = item.group.id === group.group.id
        Object.assign(service, {
          selector_mode: snapshot.selection.selector_mode,
          active_group_id: group.group.id,
          active_group_name: group.group.name,
          active_group_runtime_tag: group.group.runtime_tag,
          active_group_node_count: group.nodes.length,
          selected_node_ref: reference,
          runtime_selected: snapshot.selection.runtime_selected,
        })
        this.emit('groups')
        this.emit('status')
        this.emit('subscriptions')
        return success('node.selected', {
          group_id: group.group.id,
          mode: snapshot.selection.selector_mode,
          selected: snapshot.selection.selected,
        })
      }
      case 'PUT /mode': {
        const parsed = z.strictObject({ mode: z.string() }).safeParse(request.body)
        if (!parsed.success) return failure(400, 'usage.invalid')
        const body = parsed.data
        if (!service.available_outbound_modes.includes(body.mode))
          return failure(422, 'mode.invalid')
        service.configured_outbound_mode = body.mode
        service.outbound_mode = body.mode
        this.emit('status')
        return success('mode.changed', {
          mode: body.mode,
          runtime_mode: body.mode,
          available: service.available_outbound_modes,
        })
      }
      case 'GET /config':
        if (request.query?.target !== 'module/auto_start') return failure(400, 'usage.invalid')
        return success('config.read', {
          target: 'module/auto_start',
          revision: `demo-revision-${this.revision}`,
          content: JSON.stringify(this.config),
        })
      case 'PUT /config': {
        if (request.query?.target !== 'module/auto_start') return failure(400, 'usage.invalid')
        const parsed = z
          .strictObject({ revision: z.string().min(1), content: z.string() })
          .safeParse(request.body)
        if (!parsed.success) return failure(400, 'usage.invalid')
        const body = parsed.data
        if (body.revision !== `demo-revision-${this.revision}`)
          return failure(409, 'config.conflict')
        let value: unknown
        try {
          value = JSON.parse(body.content)
        } catch {
          return failure(500, 'command.failed')
        }
        if (
          typeof value !== 'object' ||
          value === null ||
          !('auto_start' in value) ||
          typeof value.auto_start !== 'boolean' ||
          Object.keys(value).length !== 1
        )
          return failure(500, 'command.failed')
        this.config = { auto_start: value.auto_start }
        this.revision++
        return success('config.apply', {
          target: 'module/auto_start',
          revision: `demo-revision-${this.revision}`,
        })
      }
      default:
        return failure(404, 'api.not_found')
    }
  }

  subscribe(topics: readonly EventTopic[], consume: (event: ApiEvent) => void) {
    const listener = { topics, consume }
    this.listeners.add(listener)
    queueMicrotask(() => {
      if (!this.listeners.has(listener)) return
      for (const topic of topics) this.deliver(topic, listener)
      if (topics.includes('status')) this.deliver('traffic', listener)
    })
    this.timer ??= setInterval(() => {
      this.tick++
      this.emit('traffic')
    }, 1000)
    return () => {
      this.listeners.delete(listener)
      if (this.listeners.size === 0) {
        clearInterval(this.timer)
        this.timer = undefined
      }
    }
  }
  private emit(name: ApiEvent['name']) {
    for (const listener of this.listeners) this.deliver(name, listener)
  }
  private deliver(
    name: ApiEvent['name'],
    listener: { topics: readonly EventTopic[]; consume: (event: ApiEvent) => void },
  ) {
    if (!listener.topics.includes(name === 'traffic' ? 'status' : name)) return
    const { service, snapshot } = this.state
    const response =
      name === 'status'
        ? success('service.status', service)
        : name === 'groups'
          ? success('node.snapshot', snapshot)
          : name === 'subscriptions'
            ? success(
                'subscription.list',
                snapshot.groups.map((item) => item.group),
              )
            : success('api.runtime_status', demoTraffic(service, this.tick))
    listener.consume({ name, body: structuredClone(response.body) })
  }
}
