import type {
  ApiEvent,
  ApiRequest,
  ApiTransport,
  EventTopic,
  SubscriptionHistory,
} from '@/api/contracts'
import { apiOperations, groupIdSchema } from '@/api/contracts'
import { createFixtures, demoTraffic, demoHistory, demoGroup } from './fixtures'
import {
  subscriptionCreateSchema,
  subscriptionInputSchema,
  type SubscriptionInput,
} from '@/api/subscription-input'
import { z } from 'zod'

const success = <T>(code: string, data: T) => ({
  status: 200,
  body: { schema: 1, ok: true, code, message: 'Demo', data },
})
const failure = (status: number, code: string, data: Record<string, unknown> = {}) => ({
  status,
  body: { schema: 1, ok: false, code, message: 'Demo request failed', data },
})

export class MockTransport implements ApiTransport {
  readonly kind = 'mock' as const
  readonly capabilities = apiOperations
  private state = createFixtures()
  private listeners = new Set<{
    topics: readonly EventTopic[]
    consume: (event: ApiEvent) => void
  }>()
  private timer: ReturnType<typeof setInterval> | undefined
  private tick = 0
  private config = { auto_start: false }
  private revision = 1
  private subscriptionUpdates = new Map<string, { cancelled: boolean }>()
  private histories = new Map<string, SubscriptionHistory>()
  private settings = new Map<string, SubscriptionInput>()
  private cachedSubscriptions = new Set<string>()
  private subscriptionCounter = 0
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
          : request.method === 'DELETE' && request.path.startsWith('/subscriptions/')
            ? ['replacement_group_id']
            : []
    if (Object.keys(request.query ?? {}).some((key) => !allowed.includes(key)))
      return failure(400, 'usage.invalid')
    if (request.method === 'GET' && request.path.startsWith('/subscriptions/')) {
      const match = /^\/subscriptions\/([^/]+)(?:\/(history|progress))?$/.exec(request.path)
      if (!match || !groupIdSchema.safeParse(match[1]).success || request.body !== undefined)
        return failure(400, 'usage.invalid')
      const item = snapshot.groups.find(({ group }) => group.id === match[1])
      if (!item) return failure(404, 'subscription.not_found')
      if (!match[2]) return success('subscription.show', structuredClone(item))
      if (item.group.type !== 'subscription') return failure(422, 'subscription.read_only')
      return match[2] === 'history'
        ? success(
            'subscription.history',
            this.histories.get(item.group.id) ?? demoHistory(item.group.id),
          )
        : success('subscription.progress', {
            id: item.group.id,
            progress: structuredClone(item.group.progress),
          })
    }
    if (request.method === 'POST' && request.path === '/subscriptions') {
      const parsed = subscriptionCreateSchema.safeParse(request.body)
      if (!parsed.success) return failure(422, 'subscription.edit_invalid')
      const id = `demo-subscription-${++this.subscriptionCounter}`
      const group = demoGroup(id, parsed.data.name ?? 'Subscription', 'subscription', 0)
      group.active = false
      group.auto_update = parsed.data.auto_update ?? true
      group.update_interval = parsed.data.update_interval ?? 86400
      group.update_via_proxy = parsed.data.update_via_proxy ?? 'auto'
      group.last_success_at = ''
      snapshot.groups.push({ group, nodes: [] })
      this.settings.set(id, parsed.data)
      const response = await this.updateSubscription(id)
      return response.body.ok
        ? { ...response, body: { ...response.body, code: 'subscription.added' } }
        : {
            ...response,
            body: { ...response.body, data: { ...response.body.data, persisted: true } },
          }
    }
    if (request.method === 'POST' && request.path === '/subscriptions/update-all') {
      if (request.body !== undefined) return failure(400, 'usage.invalid')
      const updated: string[] = [],
        failed: string[] = []
      let first: ReturnType<typeof failure> | undefined
      for (const item of snapshot.groups.filter((item) => item.group.type === 'subscription')) {
        const result = await this.updateSubscription(item.group.id)
        if (result.body.ok) updated.push(item.group.id)
        else {
          failed.push(item.group.id)
          first ??= result
        }
      }
      return first
        ? failure(first.status, first.body.code, { updated, failed, first_error: first.body.data })
        : success('subscription.updated_all', { updated, failed })
    }
    if (
      (request.method === 'PUT' || request.method === 'DELETE') &&
      request.path.startsWith('/subscriptions/')
    ) {
      const match = /^\/subscriptions\/([^/]+)$/.exec(request.path)
      if (!match || !groupIdSchema.safeParse(match[1]).success) return failure(400, 'usage.invalid')
      const item = snapshot.groups.find((item) => item.group.id === match[1])
      if (!item) return failure(404, 'subscription.not_found')
      if (item.group.type !== 'subscription') return failure(422, 'subscription.read_only')
      if (this.subscriptionUpdates.has(item.group.id)) return failure(409, 'subscription.busy')
      if (request.method === 'DELETE') {
        const replacement = snapshot.groups.find(
          (entry) => entry.group.id === request.query?.replacement_group_id,
        )
        if (
          request.body !== undefined ||
          (request.query?.replacement_group_id &&
            (!replacement || replacement === item || replacement.nodes.length === 0)) ||
          (item.group.active && !replacement)
        )
          return failure(409, 'subscription.conflict')
        snapshot.groups = snapshot.groups.filter((entry) => entry !== item)
        if (item.group.active && replacement) {
          replacement.group.active = true
          snapshot.selection = {
            active_group_id: replacement.group.id,
            active_group_name: replacement.group.name,
            active_group_runtime_tag: replacement.group.runtime_tag,
            active_group_node_count: replacement.nodes.length,
            selector_mode: 'urltest',
            selected_node_ref: '',
            selected: `Auto/${replacement.group.runtime_tag}`,
            runtime_selected:
              service.state === 'ready'
                ? `${replacement.group.runtime_tag}/${replacement.nodes[0]!.tag}`
                : '',
          }
          service.active_group_id = replacement.group.id
          service.active_group_name = replacement.group.name
          service.runtime_selected = snapshot.selection.runtime_selected
        }
        this.settings.delete(item.group.id)
        this.histories.delete(item.group.id)
        this.emit('subscriptions')
        this.emit('groups')
        this.emit('status')
        return success('subscription.removed', { id: item.group.id })
      }
      const parsed = subscriptionInputSchema.safeParse(request.body)
      if (!parsed.success) return failure(422, 'subscription.edit_invalid')
      const input = parsed.data
      const nameChanged = input.name !== undefined && input.name !== item.group.name
      const requiresUpdate = Object.keys(input).some(
        (key) => !['name', 'auto_update', 'update_interval'].includes(key),
      )
      this.settings.set(item.group.id, { ...this.settings.get(item.group.id), ...input })
      if (input.name !== undefined) item.group.name = input.name
      if (input.auto_update !== undefined) item.group.auto_update = input.auto_update
      if (input.update_interval !== undefined) item.group.update_interval = input.update_interval
      if (input.update_via_proxy !== undefined) item.group.update_via_proxy = input.update_via_proxy
      item.group.updated_at = new Date().toISOString()
      if (requiresUpdate) this.cachedSubscriptions.delete(item.group.id)
      const result = requiresUpdate
        ? await this.updateSubscription(item.group.id)
        : success('subscription.edited', this.subscriptionResult(item.group.id, false))
      this.emit('subscriptions')
      return result.body.ok
        ? {
            ...result,
            body: {
              ...result.body,
              code: 'subscription.edited',
              data: {
                ...result.body.data,
                name_changed: nameChanged,
                requires_update: requiresUpdate,
              },
            },
          }
        : { ...result, body: { ...result.body, data: { ...result.body.data, persisted: true } } }
    }
    if (request.method === 'POST' && request.path.startsWith('/subscriptions/')) {
      const match = /^\/subscriptions\/([^/]+)\/(update|cancel)$/.exec(request.path)
      if (!match || !groupIdSchema.safeParse(match[1]).success || request.body !== undefined)
        return failure(400, 'usage.invalid')
      const item = snapshot.groups.find(({ group }) => group.id === match[1])
      if (!item) return failure(404, 'subscription.not_found')
      if (item.group.type !== 'subscription') return failure(422, 'subscription.read_only')
      if (match[2] === 'cancel') {
        const pending = this.subscriptionUpdates.get(item.group.id)
        if (pending) pending.cancelled = true
        return success('subscription.cancelled', { id: item.group.id })
      }
      return this.updateSubscription(item.group.id)
    }
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
      case 'GET /selection':
        return success('node.current', structuredClone(snapshot.selection))
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
      case 'GET /catalog':
      case 'GET /subscriptions':
        return success(
          request.path === '/catalog' ? 'catalog.groups' : 'subscription.list',
          structuredClone(snapshot.groups.map((item) => item.group)),
        )
      case 'GET /mode':
        return success('mode.current', {
          mode: service.configured_outbound_mode,
          ...(service.state === 'ready' ? { runtime_mode: service.outbound_mode } : {}),
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
        const active = snapshot.groups.find(
          (item) => item.group.id === snapshot.selection.active_group_id,
        )!
        const selectedTag = snapshot.selection.selected_node_ref.slice(active.group.id.length + 1)
        snapshot.selection.runtime_selected = running
          ? `${active.group.runtime_tag}/${selectedTag || active.nodes[0]!.tag}`
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
                ? `${group.group.runtime_tag}/${body.node_tag}`
                : `${group.group.runtime_tag}/${group.nodes[0]!.tag}`
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
          selected: manual
            ? `${group.group.runtime_tag}/${body.node_tag}`
            : `Auto/${group.group.runtime_tag}`,
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
          ...(service.state === 'ready' ? { runtime_mode: body.mode } : {}),
          available: service.available_outbound_modes,
        })
      }
      case 'POST /nodes/delay': {
        const parsed = z
          .strictObject({ group_id: z.string().min(1), node_tag: z.string() })
          .safeParse(request.body)
        if (!parsed.success) return failure(400, 'usage.invalid')
        const { group_id, node_tag } = parsed.data
        const group = snapshot.groups.find((entry) => entry.group.id === group_id)
        if (
          !group ||
          group.nodes.length === 0 ||
          (node_tag !== '' && !group.nodes.some((node) => node.tag === node_tag))
        )
          return failure(500, 'command.failed')
        const items = group.nodes
          .filter((node) => node_tag === '' || node.tag === node_tag)
          .map((node, index) => ({
            tag: `${group.group.runtime_tag}/${node.tag}`,
            type: node.protocol,
            url_test_delay: 32 + index * 21,
            url_test_time: Math.floor(Date.now() / 1000),
          }))
        return success('node.delay', {
          target: node_tag
            ? `${group.group.runtime_tag}/${node_tag}`
            : `Auto/${group.group.runtime_tag}`,
          groups: [
            {
              tag: `Auto/${group.group.runtime_tag}`,
              type: node_tag ? '' : 'urltest',
              selectable: false,
              expanded: false,
              items,
            },
          ],
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

  private subscriptionResult(id: string, notModified: boolean) {
    const group = this.state.snapshot.groups.find((item) => item.group.id === id)!.group
    return {
      group_id: id,
      node_count: group.node_count,
      revision: group.revision,
      not_modified: notModified,
      persisted: true,
      runtime_synced: this.state.service.state === 'ready',
      runtime_sync_state: this.state.service.state === 'ready' ? 'applied' : 'not_running',
      runtime_sync_pending: false,
    }
  }
  private async updateSubscription(id: string) {
    const item = this.state.snapshot.groups.find((entry) => entry.group.id === id)!
    if (this.subscriptionUpdates.has(id)) return failure(409, 'subscription.busy')
    const pending = { cancelled: false }
    this.subscriptionUpdates.set(id, pending)
    item.group.progress = {
      schema: 1,
      group_id: id,
      stage: 'download',
      message: 'Demo',
      updated_at: new Date().toISOString(),
    }
    this.emit('subscriptions')
    await new Promise((resolve) => setTimeout(resolve, Math.max(250, this.latency * 3)))
    item.group.progress = null
    this.subscriptionUpdates.delete(id)
    const at = new Date().toISOString()
    item.group.last_attempt_at = at
    item.group.updated_at = at
    const history: SubscriptionHistory = this.histories.get(id) ?? demoHistory(id)
    const failed =
      pending.cancelled || id === 'demo-empty' || this.settings.get(id)?.url?.includes('/fail')
    if (failed) {
      const code = pending.cancelled ? 'subscription.cancelled' : 'subscription.convert_failed'
      item.group.last_error = pending.cancelled ? '' : 'Demo subscription unavailable'
      history.push({ at, ok: false, code })
      this.histories.set(id, history)
      this.emit('subscriptions')
      return failure(pending.cancelled ? 408 : 500, code)
    }
    const notModified = this.cachedSubscriptions.has(id)
    if (!notModified) {
      item.group.revision++
      if (!item.nodes.length)
        item.nodes.push({
          tag: 'Demo relay',
          protocol: 'socks',
          server: 'relay.example.invalid',
          port: 1080,
        })
      item.group.node_count = item.nodes.length
    }
    this.cachedSubscriptions.add(id)
    item.group.last_success_at = at
    item.group.last_error = ''
    history.push({
      at,
      ok: true,
      code: notModified ? 'subscription.not_modified' : 'subscription.updated',
      node_count: item.group.node_count,
      revision: item.group.revision,
      duration_seconds: 0,
    })
    this.histories.set(id, history)
    this.emit('subscriptions')
    this.emit('groups')
    return success('subscription.updated', this.subscriptionResult(id, notModified))
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
