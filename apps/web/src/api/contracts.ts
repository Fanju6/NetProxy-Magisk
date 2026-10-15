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
export const groupSchema = z.looseObject({
  id: z.string(),
  name: z.string(),
  runtime_tag: z.string(),
  type: z.enum(['local', 'url']),
  active: z.boolean(),
  node_count: count,
  revision: count,
  auto_update: z.boolean(),
  update_interval: count,
  update_via_proxy: z.enum(['auto', 'always', 'never']),
  usage: z.unknown(),
  profile_title: z.string(),
  profile_web_page_url: z.string(),
  last_attempt_at: z.string(),
  last_success_at: z.string(),
  next_update_at: z.string(),
  last_error: z.string(),
  updated_at: z.string(),
  progress: z.unknown(),
})
export const nodeSchema = z.looseObject({
  tag: z.string(),
  protocol: z.string(),
  server: z.string().optional(),
  port: count.optional(),
})
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
  | '/nodes/snapshot'
  | '/selection'
  | '/mode'
  | '/subscriptions'
  | '/config'
export interface ApiRequest {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE'
  path: ApiPath
  query?: Readonly<Record<string, string>>
  body?: unknown
}
export interface ApiTransport {
  readonly kind: 'unavailable' | 'mock' | 'browser' | 'native'
  request(request: ApiRequest, signal?: AbortSignal): Promise<{ status: number; body: unknown }>
  // 允许同步回调；返回的函数释放本次订阅。若启动抛出异常，Transport 须先回收未交付资源。
  subscribe(
    topics: readonly EventTopic[],
    consume: (event: ApiEvent) => void,
    onError: (error: Error) => void,
  ): () => void
}

export class ApiError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly status: number,
    readonly data: unknown = null,
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
