import { z } from 'zod'

const count = z.number().int().nonnegative()
export const envelopeSchema = z.looseObject({
  schema: z.literal(1),
  ok: z.boolean(),
  code: z.string().min(1),
  message: z.string(),
  data: z.unknown().optional(),
})
export const serviceSchema = z.looseObject({
  state: z.enum(['stopped', 'preparing', 'starting', 'ready', 'stopping', 'failed']),
  configured_backend: z.enum(['ebpf', 'tun']),
  active_backend: z.enum(['ebpf', 'tun']).nullable(),
  pid: count.nullable(),
  started_at: count,
  core_started_at_millis: count,
  ready_at: count,
  uptime_seconds: count,
  error: z.string(),
  outbound_mode: z.string(),
  configured_outbound_mode: z.string(),
  available_outbound_modes: z.array(z.string()),
  selector_mode: z.enum(['manual', 'urltest']),
  active_group_id: z.string(),
  active_group_name: z.string(),
  active_group_runtime_tag: z.string(),
  active_group_node_count: count,
  selected_node_ref: z.string(),
  runtime_selected: z.string(),
  memory_bytes: count,
  process_cpu_ticks: count,
  system_cpu_ticks: count,
  cpu_count: count,
  connections_in: count,
  connections_out: count,
  upload_total: count,
  download_total: count,
  worker_state: z.string(),
  worker_pid: count.nullable(),
})
export const groupIdSchema = z
  .string()
  .regex(/^[A-Za-z0-9][A-Za-z0-9._-]*$/)
  .refine((id) => id !== 'staging' && !id.includes('..'))
export const usageSchema = z.object({
  upload: count.optional(),
  download: count.optional(),
  total: count.optional(),
  expire: count.optional(),
})
export const updateProgressSchema = z.object({
  schema: z.literal(1),
  group_id: groupIdSchema,
  stage: z.enum(['download', 'convert', 'validate', 'apply']),
  message: z.string(),
  updated_at: z.iso.datetime({ offset: true }),
})
export const groupSchema = z
  .object({
    id: groupIdSchema,
    name: z.string(),
    runtime_tag: z.string(),
    type: z.enum(['local', 'subscription']),
    active: z.boolean(),
    node_count: count,
    revision: count,
    auto_update: z.boolean(),
    update_interval: count,
    update_via_proxy: z.enum(['auto', 'always', 'never']),
    usage: usageSchema.nullable(),
    profile_title: z.string(),
    profile_web_page_url: z.string().default(''),
    last_attempt_at: z.string(),
    last_success_at: z.string(),
    next_update_at: z.string(),
    last_error: z.string(),
    updated_at: z.string(),
    progress: updateProgressSchema.nullable(),
  })
  .refine((group) => group.progress === null || group.progress.group_id === group.id)
export const nodeSchema = z.object({
  tag: z.string(),
  protocol: z.string(),
  server: z.string().optional(),
  port: count.optional(),
})
export const subscriptionDetailSchema = z.object({ group: groupSchema, nodes: z.array(nodeSchema) })
export const historySchema = z.array(
  z.object({
    at: z.iso.datetime({ offset: true }),
    ok: z.boolean(),
    code: z.string().min(1),
    message: z.string().optional(),
    node_count: count.optional(),
    revision: count.optional(),
    duration_seconds: count.optional(),
  }),
)
export const subscriptionProgressSchema = z.object({
  id: groupIdSchema,
  progress: updateProgressSchema.nullable(),
})
export type SubscriptionDetail = z.infer<typeof subscriptionDetailSchema>
export type SubscriptionHistory = z.infer<typeof historySchema>
export type SubscriptionProgress = z.infer<typeof subscriptionProgressSchema>
export type UpdateProgress = z.infer<typeof updateProgressSchema>
export const selectionSchema = z.looseObject({
  active_group_id: z.string(),
  active_group_name: z.string(),
  active_group_runtime_tag: z.string(),
  active_group_node_count: count,
  selector_mode: z.enum(['manual', 'urltest']),
  selected_node_ref: z.string(),
  selected: z.string(),
  runtime_selected: z.string(),
})
export const runtimeGroupSchema = z.looseObject({
  tag: z.string(),
  type: z.string(),
  selectable: z.boolean(),
  selected: z.string().optional(),
  expanded: z.boolean(),
  items: z.array(
    z.looseObject({
      tag: z.string(),
      type: z.string(),
      url_test_time: count.optional(),
      url_test_delay: count.optional(),
    }),
  ),
})
export const snapshotSchema = z.looseObject({
  groups: z.array(z.object({ group: groupSchema, nodes: z.array(nodeSchema) })),
  selection: selectionSchema,
  runtime_groups: z.array(runtimeGroupSchema).optional(),
})
export const modeSchema = z.looseObject({
  mode: z.string(),
  runtime_mode: z.string().optional(),
  available: z.array(z.string()),
})
export const nodeTargetSchema = z.strictObject({
  group_id: z.string().min(1),
  node_tag: z.string(),
})
export const selectedSchema = z.object({
  group_id: z.string(),
  mode: z.enum(['manual', 'urltest']),
  selected: z.string(),
})
export const delaySchema = z.object({
  target: z.string(),
  groups: z.array(runtimeGroupSchema),
})
export type NodeTarget = z.infer<typeof nodeTargetSchema>
export type Selection = z.infer<typeof selectionSchema>
export type ModeState = z.infer<typeof modeSchema>
export const configSchema = z.looseObject({
  target: z.string(),
  revision: z.string(),
  content: z.string(),
})
export const infoSchema = z.looseObject({
  api_version: z.literal(1),
  version: z.string(),
  version_code: z.string(),
  instance_id: z.string(),
  pid: count,
  platform: z.string(),
  capabilities: z.array(z.string()),
  memory: z.object({
    heap_alloc_bytes: count,
    heap_inuse_bytes: count,
    heap_released_bytes: count,
    stack_inuse_bytes: count,
  }),
})
export const trafficSchema = z.discriminatedUnion('available', [
  z.object({ available: z.literal(false) }),
  z.looseObject({
    available: z.literal(true),
    started_at_millis: count,
    process_cpu_ticks: count,
    system_cpu_ticks: count,
    cpu_count: count,
    status: z.looseObject({
      memory: count,
      goroutines: count,
      connections_in: count,
      connections_out: count,
      traffic_available: z.boolean(),
      uplink: count,
      downlink: count,
      uplink_total: count,
      downlink_total: count,
    }),
  }),
])
export type ServiceStatus = z.infer<typeof serviceSchema>
export type GroupSummary = z.infer<typeof groupSchema>
export type NodeSnapshot = z.infer<typeof snapshotSchema>
export type Traffic = z.infer<typeof trafficSchema>
export type EventTopic = 'status' | 'groups' | 'subscriptions'
export type ApiEvent = { name: EventTopic | 'traffic'; body: unknown }
export type ApiPath =
  | '/info'
  | '/service'
  | '/service/start'
  | '/service/stop'
  | '/service/restart'
  | '/service/reload'
  | '/service/check'
  | '/configs'
  | '/config/validate'
  | '/android/apps'
  | `/android/icons/${string}`
  | '/network/wifi-list'
  | '/logs'
  | '/diagnostics/ebpf'
  | '/diagnostics/export'
  | '/nodes'
  | '/nodes/get'
  | '/nodes/export'
  | '/nodes/import'
  | '/nodes/snapshot'
  | '/nodes/delay'
  | '/selection'
  | '/mode'
  | '/subscriptions'
  | '/catalog'
  | `/subscriptions/${string}`
  | '/config'
export interface ApiRequest {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE'
  path: ApiPath
  query?: Readonly<Record<string, string>>
  body?: unknown
}
export interface ApiTransport {
  readonly kind: 'unavailable' | 'mock' | 'browser' | 'native'
  readonly capabilities: readonly ApiOperation[]
  observeVisibility?(listener: (visible: boolean) => void): () => void
  request(request: ApiRequest, signal?: AbortSignal): Promise<{ status: number; body: unknown }>
  // 允许同步回调；返回的函数释放本次订阅。若启动抛出异常，Transport 须先回收未交付资源。
  subscribe(
    topics: readonly EventTopic[],
    consume: (event: ApiEvent) => void,
    onError: (error: Error) => void,
  ): () => void
}

export const apiOperations = [
  'info.read',
  'service.read',
  'service.start',
  'service.stop',
  'nodes.read',
  'nodes.delay',
  'selection.read',
  'selection.write',
  'mode.read',
  'mode.write',
  'subscriptions.read',
  'catalog.read',
  'subscriptions.detail',
  'subscriptions.history',
  'subscriptions.progress',
  'subscriptions.update',
  'subscriptions.cancel',
  'subscriptions.add',
  'subscriptions.edit',
  'subscriptions.remove',
  'subscriptions.update_all',
  'config.read',
  'config.write',
  'config.list',
  'config.validate',
  'android.apps',
  'android.icon',
  'network.wifi',
  'logs.read',
  'logs.clear',
  'diagnostics.ebpf',
  'diagnostics.export',
  'nodes.get',
  'nodes.export',
  'nodes.add',
  'nodes.edit',
  'nodes.remove',
  'nodes.import',
  'service.restart',
  'service.reload',
  'service.check',
] as const
export type ApiOperation = (typeof apiOperations)[number]

const operations: Partial<Record<ApiRequest['method'], Partial<Record<ApiPath, ApiOperation>>>> = {
  GET: {
    '/info': 'info.read',
    '/service': 'service.read',
    '/nodes/snapshot': 'nodes.read',
    '/selection': 'selection.read',
    '/mode': 'mode.read',
    '/subscriptions': 'subscriptions.read',
    '/catalog': 'catalog.read',
    '/config': 'config.read',
    '/configs': 'config.list',
    '/android/apps': 'android.apps',
    '/network/wifi-list': 'network.wifi',
    '/logs': 'logs.read',
    '/diagnostics/export': 'diagnostics.export',
    '/nodes/get': 'nodes.get',
    '/nodes/export': 'nodes.export',
  },
  POST: {
    '/service/start': 'service.start',
    '/service/stop': 'service.stop',
    '/nodes/delay': 'nodes.delay',
    '/nodes': 'nodes.add',
    '/nodes/import': 'nodes.import',
    '/config/validate': 'config.validate',
    '/diagnostics/ebpf': 'diagnostics.ebpf',
    '/service/restart': 'service.restart',
    '/service/reload': 'service.reload',
    '/service/check': 'service.check',
  },
  PUT: {
    '/selection': 'selection.write',
    '/mode': 'mode.write',
    '/config': 'config.write',
    '/nodes': 'nodes.edit',
  },
  DELETE: { '/logs': 'logs.clear', '/nodes': 'nodes.remove' },
}
export function requestOperation(request: ApiRequest): ApiOperation | undefined {
  if (request.method === 'GET' && /^\/android\/icons\/[a-f0-9]{64}$/.test(request.path))
    return 'android.icon'
  if (request.query === undefined && request.method === 'POST') {
    if (request.path === '/subscriptions') return 'subscriptions.add'
    if (request.path === '/subscriptions/update-all' && request.body === undefined)
      return 'subscriptions.update_all'
  }
  if (
    (request.method === 'PUT' || request.method === 'DELETE') &&
    request.path.startsWith('/subscriptions/')
  ) {
    const match = /^\/subscriptions\/([^/]+)$/.exec(request.path)
    if (!match || !groupIdSchema.safeParse(match[1]).success) return
    if (request.method === 'PUT' && request.query === undefined) return 'subscriptions.edit'
    if (
      request.method === 'DELETE' &&
      request.body === undefined &&
      Object.keys(request.query ?? {}).every((key) => key === 'replacement_group_id') &&
      (request.query?.replacement_group_id === undefined ||
        groupIdSchema.safeParse(request.query.replacement_group_id).success)
    )
      return 'subscriptions.remove'
    return
  }
  if (request.method === 'POST' && request.path.startsWith('/subscriptions/')) {
    const match = /^\/subscriptions\/([^/]+)\/(update|cancel)$/.exec(request.path)
    if (
      match &&
      groupIdSchema.safeParse(match[1]).success &&
      request.query === undefined &&
      request.body === undefined
    )
      return match[2] === 'update' ? 'subscriptions.update' : 'subscriptions.cancel'
    return
  }
  if (request.method === 'GET' && request.path.startsWith('/subscriptions/')) {
    const match = /^\/subscriptions\/([^/]+)(?:\/(history|progress))?$/.exec(request.path)
    if (
      !match ||
      !groupIdSchema.safeParse(match[1]).success ||
      request.query !== undefined ||
      request.body !== undefined
    )
      return
    return match[2] === 'history'
      ? 'subscriptions.history'
      : match[2] === 'progress'
        ? 'subscriptions.progress'
        : 'subscriptions.detail'
  }
  return operations[request.method]?.[request.path]
}

export class ApiError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly status: number,
    readonly data: unknown = null,
    readonly responseReceived = false,
  ) {
    super(message)
    this.name = 'ApiError'
  }
  get persisted(): boolean {
    return (
      typeof this.data === 'object' &&
      this.data !== null &&
      'persisted' in this.data &&
      this.data.persisted === true
    )
  }
}
