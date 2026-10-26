import type {
  ApiEvent,
  ApiRequest,
  ApiTransport,
  EventTopic,
  SubscriptionHistory,
} from '@/api/contracts'
import {
  apiOperations,
  groupIdSchema,
  policySelectSchema,
  policyDelaySchema,
} from '@/api/contracts'
import { createFixtures, demoTraffic, demoHistory, demoGroup } from './fixtures'
import {
  subscriptionCreateSchema,
  subscriptionInputSchema,
  type SubscriptionInput,
} from '@/api/subscription-input'
import { z } from 'zod'
import { ManagementMock } from './management'
import { managementRequest } from '@/api/management'

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
  private subscriptionUpdates = new Map<string, { cancelled: boolean }>()
  private histories = new Map<string, SubscriptionHistory>()
  private settings = new Map<string, SubscriptionInput>()
  private cachedSubscriptions = new Set<string>()
  private subscriptionCounter = 0
  private management = new ManagementMock()
  private nodeDocuments = new Map<string, unknown>()
  constructor(private readonly latency = 180) {
    this.refreshPolicies()
  }

  private refreshPolicies() {
    const { policy, service, snapshot } = this.state
    const objects = this.management.nativeOutbounds()
    policy.live = service.state === 'ready'
    policy.source = policy.live ? undefined : 'configuration'
    policy.core_started_at_millis = policy.live ? service.core_started_at_millis : 0
    policy.default_outbound = this.management.nativeDefaultOutbound()
    service.default_outbound = policy.default_outbound
    policy.groups = objects
      .filter((object) => ['selector', 'urltest', 'loadbalance'].includes(object.type))
      .map((object) => {
        const previous = policy.groups.find((group) => group.tag === object.tag)
        const tags = Array.isArray(object.outbounds) ? (object.outbounds as string[]) : []
        const items = tags.map((tag) => ({
          tag,
          type: objects.find((item) => item.tag === tag)?.type ?? '',
        }))
        {
          const providers = Array.isArray(object.providers) ? (object.providers as string[]) : []
          const resources =
            object.use_all_providers === true
              ? snapshot.groups
              : providers.flatMap((tag) =>
                  snapshot.groups.filter((resource) => resource.group.provider_tag === tag),
                )
          for (const resource of resources) {
            if (
              object.use_all_providers !== true &&
              !providers.includes(resource.group.provider_tag)
            )
              continue
            for (const node of resource.nodes) {
              const tag = `${resource.group.provider_tag}/${node.tag}`
              if (
                (typeof object.exclude === 'string' && new RegExp(object.exclude).test(tag)) ||
                (typeof object.include === 'string' && !new RegExp(object.include).test(tag))
              )
                continue
              if (!items.some((item) => item.tag === tag)) items.push({ tag, type: node.protocol })
            }
          }
        }
        return {
          tag: object.tag ?? '',
          type: object.type,
          selectable: object.type === 'selector',
          ...(!policy.live && typeof object.default === 'string'
            ? { configured_default: object.default }
            : {}),
          selected: !policy.live
            ? ''
            : previous?.selected && items.some((item) => item.tag === previous.selected)
              ? previous.selected
              : typeof object.default === 'string'
                ? object.default
                : (items[0]?.tag ?? ''),
          items: items.map((item) => ({
            ...(policy.live ? previous?.items.find((old) => old.tag === item.tag) : {}),
            ...item,
          })),
        }
      })
    this.resolveDefault()
  }
  private resolveDefault() {
    const { policy, service } = this.state
    let tag = policy.default_outbound
    const visited = new Set<string>()
    while (tag && !visited.has(tag)) {
      visited.add(tag)
      const group = policy.groups.find((item) => item.tag === tag)
      if (!group) {
        service.runtime_selected = policy.live ? tag : ''
        return
      }
      tag = group.selected ?? ''
    }
    service.runtime_selected = ''
  }

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
    const { service, snapshot, policy } = this.state
    const managed = await this.management.request(request)
    const operation = managementRequest(request)
    if (managed) {
      if (operation?.operation === 'config_apply' && managed.body.ok) {
        this.refreshPolicies()
        this.emit('groups')
      }
      return managed
    }
    if (
      (request.path === '/catalog' && request.method === 'POST') ||
      (request.path.startsWith('/catalog/') && ['PUT', 'DELETE'].includes(request.method))
    ) {
      if (!operation?.operation.startsWith('catalog_')) return failure(400, 'usage.invalid')
      const input = operation.input as { id: string; name?: string }
      const existing = snapshot.groups.find((entry) => entry.group.id === input.id)
      if (operation.operation === 'catalog_create') {
        if (existing) return failure(409, 'catalog.conflict')
        snapshot.groups.push({ group: demoGroup(input.id, input.name!, 'local', 0), nodes: [] })
      } else {
        if (!existing) return failure(404, 'catalog.not_found')
        if (existing.group.type !== 'local') return failure(422, 'catalog.read_only')
        if (operation.operation === 'catalog_remove') {
          if (input.id === 'default') return failure(422, 'catalog.protected')
          snapshot.groups = snapshot.groups.filter((entry) => entry !== existing)
        } else {
          existing.group.name = input.name!
          existing.group.revision++
        }
      }
      this.emit('subscriptions')
      return success('catalog.applied', { id: input.id })
    }
    if (operation?.operation.startsWith('node_')) {
      const input = operation.input as { group_id?: string; node_tag?: string; content?: string }
      const group = snapshot.groups.find((item) => item.group.id === (input.group_id ?? 'default'))
      if (!group) return failure(404, 'node.not_found')
      const node = group.nodes.find((item) => item.tag === input.node_tag)
      const documentKey = `${group.group.id}/${input.node_tag ?? ''}`
      if (operation.operation === 'node_read' || operation.operation === 'node_export') {
        if (!node) return failure(404, 'node.not_found')
        return operation.operation === 'node_read'
          ? success(
              'node.loaded',
              this.nodeDocuments.get(documentKey) ?? {
                outbounds: [
                  {
                    type: node.protocol,
                    tag: node.tag,
                    server: node.server ?? '127.0.0.1',
                    server_port: node.port ?? 1080,
                  },
                ],
              },
            )
          : success('node.exported', {
              tag: node.tag,
              protocol: node.protocol,
              link: `socks://127.0.0.1:1080#${encodeURIComponent(node.tag)}`,
            })
      }
      if (group.group.type !== 'local') return failure(422, 'node.read_only')
      if (operation.operation === 'node_remove') {
        if (!node) return failure(404, 'node.not_found')
        group.nodes = group.nodes.filter((item) => item !== node)
        this.nodeDocuments.delete(documentKey)
      } else {
        let outbounds: unknown[]
        try {
          const value: unknown = JSON.parse(input.content!)
          outbounds = Array.isArray(value)
            ? value
            : value &&
                typeof value === 'object' &&
                'outbounds' in value &&
                Array.isArray(value.outbounds)
              ? value.outbounds
              : [value]
        } catch {
          try {
            outbounds = input
              .content!.split(/\r?\n/)
              .filter(Boolean)
              .map((line) => {
                const url = new URL(line)
                return {
                  tag: decodeURIComponent(url.hash.slice(1)) || 'Imported node',
                  type: url.protocol.slice(0, -1),
                  server: url.hostname,
                  server_port: Number(url.port || 1080),
                }
              })
          } catch {
            return failure(422, 'node.invalid')
          }
        }
        const parsed = z
          .array(
            z
              .object({
                tag: z.string().min(1),
                type: z.string().min(1),
                server: z.string().optional(),
                server_port: z.number().int().optional(),
              })
              .passthrough(),
          )
          .min(1)
          .safeParse(outbounds)
        if (
          !parsed.success ||
          (operation.operation === 'node_edit' && (!node || parsed.data.length !== 1))
        )
          return failure(422, 'node.invalid')
        if (
          parsed.data.some((item) =>
            group.nodes.some((existing) => existing.tag === item.tag && existing !== node),
          )
        )
          return failure(409, 'node.duplicate')
        if (operation.operation === 'node_edit')
          group.nodes = group.nodes.filter((item) => item !== node)
        for (const item of parsed.data) {
          group.nodes.push({
            tag: item.tag,
            protocol: item.type,
            ...(item.server ? { server: item.server } : {}),
            ...(item.server_port ? { port: item.server_port } : {}),
          })
          this.nodeDocuments.set(`${group.group.id}/${item.tag}`, { outbounds: [item] })
        }
      }
      group.group.node_count = group.nodes.length
      group.group.revision++
      this.emit('subscriptions')
      this.emit('status')
      return success('node.applied', {
        group_id: group.group.id,
        revision: group.group.revision,
        node_count: group.group.node_count,
        persisted: true,
        runtime_synced: service.state === 'ready',
      })
    }
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
        if (request.body !== undefined || request.query !== undefined)
          return failure(400, 'usage.invalid')
        snapshot.groups = snapshot.groups.filter((entry) => entry !== item)
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
      case 'GET /policy-groups':
        this.refreshPolicies()
        return success('policy.groups', structuredClone(policy))
      case 'GET /nodes/snapshot': {
        const id = request.query?.group_id
        if (id !== undefined && !groupIdSchema.safeParse(id).success)
          return failure(400, 'usage.invalid')
        return success('node.snapshot', {
          groups: structuredClone(
            id ? snapshot.groups.filter((entry) => entry.group.id === id) : snapshot.groups,
          ),
        })
      }
      case 'POST /policy-groups/select': {
        const parsed = policySelectSchema.safeParse(request.body)
        if (!parsed.success) return failure(400, 'usage.invalid')
        if (!policy.live) return failure(409, 'policy.not_live')
        const group = policy.groups.find((group) => group.tag === parsed.data.group_tag)
        if (!group?.selectable || !group.items.some((item) => item.tag === parsed.data.member_tag))
          return failure(422, 'policy.invalid_member')
        group.selected = parsed.data.member_tag
        this.resolveDefault()
        this.emit('groups')
        return success('policy.selected', structuredClone(policy))
      }
      case 'POST /policy-groups/delay': {
        const parsed = policyDelaySchema.safeParse(request.body)
        if (!parsed.success) return failure(400, 'usage.invalid')
        if (!policy.live) return failure(409, 'policy.not_live')
        const group = policy.groups.find((group) => group.tag === parsed.data.group_tag)
        if (!group) return failure(404, 'policy.not_found')
        for (const [index, item] of group.items.entries()) {
          item.url_test_delay = 32 + index * 21
          item.url_test_time = Math.floor(Date.now() / 1000)
        }
        this.emit('groups')
        return success('policy.delay', structuredClone(policy))
      }
      case 'GET /catalog':
        return success('catalog.groups', structuredClone(snapshot))
      case 'GET /subscriptions':
        return success(
          'subscription.list',
          structuredClone(snapshot.groups.map((item) => item.group)),
        )
      case 'GET /mode':
        return success('mode.current', {
          mode: service.configured_outbound_mode,
          ...(service.state === 'ready' ? { runtime_mode: service.outbound_mode } : {}),
          available: service.available_outbound_modes,
        })
      case 'POST /service/start':
      case 'POST /service/restart':
      case 'POST /service/reload':
      case 'POST /service/stop': {
        const running = request.path !== '/service/stop'
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
        this.refreshPolicies()
        this.emit('status')
        this.emit('traffic')
        this.emit('groups')
        return success(running ? 'service.start' : 'service.stop', {
          action: request.path.slice('/service/'.length),
          status: structuredClone(service),
        })
      }
      case 'POST /service/check':
        return success('service.check', { action: 'check', status: structuredClone(service) })
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
    if (name === 'groups' || name === 'subscriptions') this.refreshPolicies()
    if (name === 'subscriptions')
      for (const listener of this.listeners) this.deliver('groups', listener)
    for (const listener of this.listeners) this.deliver(name, listener)
  }
  private deliver(
    name: ApiEvent['name'],
    listener: { topics: readonly EventTopic[]; consume: (event: ApiEvent) => void },
  ) {
    if (!listener.topics.includes(name === 'traffic' ? 'status' : name)) return
    const { service, snapshot, policy } = this.state
    const response =
      name === 'status'
        ? success('service.status', service)
        : name === 'groups'
          ? success('policy.groups', policy)
          : name === 'subscriptions'
            ? success(
                'subscription.list',
                snapshot.groups.map((item) => item.group),
              )
            : success('api.runtime_status', demoTraffic(service, this.tick))
    listener.consume({ name, body: structuredClone(response.body) })
  }
}
