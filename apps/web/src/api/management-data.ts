import { z } from 'zod'
import { ApiError } from './contracts'

const count = z.number().int().nonnegative()
const appRef = z.string().regex(/^(0|[1-9][0-9]*):[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)*$/)
export const appPolicySchema = z.strictObject({
  enabled: z.boolean(),
  mode: z.enum(['blacklist', 'whitelist']),
  proxy_apps: z.array(appRef),
  bypass_apps: z.array(appRef),
})
export const appDocumentSchema = z.strictObject({ app: appPolicySchema })
export const backendDocumentSchema = z.strictObject({ backend: z.enum(['ebpf', 'tun']) })
export const rootDocumentSchema = z.strictObject({
  root_policy: z.enum(['default', 'include', 'exclude']),
})
export const ssidSchema = z
  .string()
  .refine(
    (name) =>
      name.length > 0 &&
      new TextEncoder().encode(name).length <= 32 &&
      !/[\p{Cc}\uD800-\uDFFF]/u.test(name),
  )
const ssids = z.array(ssidSchema).refine((items) => new Set(items).size === items.length)
export const wifiPolicySchema = z.strictObject({
  enabled: z.boolean(),
  mode: z.enum(['blacklist', 'whitelist']),
  blacklist: ssids,
  whitelist: ssids,
  proxy_on_non_wifi: z.boolean(),
})
export const wifiDocumentSchema = z.strictObject({ wifi: wifiPolicySchema })
export const savedWifiSchema = z.object({ ssids: ssids })
export const androidAppSchema = z.object({
  package_name: z.string().min(1),
  uid: count,
  label: z.string(),
  system: z.boolean(),
  version_code: count,
  last_update_time: count,
  icon_key: z.string().regex(/^(?:[a-f0-9]{64})?$/),
})
export const androidAppsSchema = z.object({
  users: z.array(
    z.object({
      id: count.max(21474),
      name: z.string(),
      revision: z.string(),
      apps: z.array(androidAppSchema),
    }),
  ),
})
export const androidIconSchema = z.object({
  content: z
    .string()
    .max(350_000)
    .regex(/^iVBORw0KGgo[A-Za-z0-9+/]*={0,2}$/),
})
export type AppPolicy = z.infer<typeof appPolicySchema>
export type WiFiPolicy = z.infer<typeof wifiPolicySchema>
export type AndroidApp = z.infer<typeof androidAppSchema>
export type AndroidUsers = z.infer<typeof androidAppsSchema>['users']
export type ListMode = 'off' | 'blacklist' | 'whitelist'
export const listMode = (policy: {
  enabled: boolean
  mode: 'blacklist' | 'whitelist'
}): ListMode => (policy.enabled ? policy.mode : 'off')
export function changeListMode<T extends { enabled: boolean; mode: 'blacklist' | 'whitelist' }>(
  policy: T,
  mode: ListMode,
): T {
  return { ...policy, enabled: mode !== 'off', ...(mode === 'off' ? {} : { mode }) }
}
export function parseDocument<S extends z.ZodType>(
  content: string,
  schema: S,
): z.infer<S> | undefined {
  try {
    return schema.safeParse(JSON.parse(content)).data
  } catch {
    return undefined
  }
}
export function safeManagementError(error: unknown): ApiError {
  return error instanceof ApiError
    ? new ApiError(
        /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/.test(error.code)
          ? error.code
          : 'client.invalid_response',
        'Management operation failed',
        error.status,
        { persisted: error.persisted },
        error.responseReceived,
      )
    : new ApiError('client.connection_failed', 'Management result is not confirmed', 0)
}

export const ruleFields = [
  'domain_suffix',
  'domain',
  'domain_keyword',
  'domain_regex',
  'ip_cidr',
  'port',
  'port_range',
  'package_name',
] as const
export type RuleField = (typeof ruleFields)[number]
export const ruleDocumentSchema = z.looseObject({
  version: z.number().int().min(1).max(5),
  rules: z.array(z.unknown()),
})
export type RuleDocument = z.infer<typeof ruleDocumentSchema>
export function simpleRule(value: unknown): { field: RuleField; values: string[] } | undefined {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return
  const rule = value as Record<string, unknown>
  if (rule.type !== undefined && rule.type !== 'default') return
  const field = ruleFields.find((key) => key in rule)
  if (!field || Object.keys(rule).some((key) => key !== field && key !== 'type')) return
  const values = Array.isArray(rule[field]) ? rule[field] : [rule[field]]
  if (
    !values.length ||
    values.some((item) =>
      field === 'port'
        ? typeof item !== 'number' || !Number.isInteger(item) || item < 1 || item > 65535
        : typeof item !== 'string' || item.trim() === '' || /[\r\n]/.test(item),
    )
  )
    return
  return { field, values: values.map(String) }
}
export function makeSimpleRule(
  field: RuleField,
  text: string,
): Record<string, unknown> | undefined {
  const values = [
    ...new Set(
      text
        .split(/\r?\n/)
        .map((value) => value.trim())
        .filter(Boolean),
    ),
  ]
  if (!values.length) return
  if (
    field === 'port' &&
    values.some((value) => !/^\d+$/.test(value) || Number(value) < 1 || Number(value) > 65535)
  )
    return
  if (
    field === 'port_range' &&
    values.some((value) => {
      if (!/^\d+:\d+$/.test(value)) return true
      const [start, end] = value.split(':').map(Number)
      return start! < 1 || end! < start! || end! > 65535
    })
  )
    return
  if (
    (field === 'domain' || field === 'domain_suffix') &&
    values.some((value) => /[\s/:@]/.test(value))
  )
    return
  if (
    field === 'package_name' &&
    values.some((value) => !/^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)*$/.test(value))
  )
    return
  return { [field]: field === 'port' ? values.map(Number) : values }
}
export function replaceRule(
  document: RuleDocument,
  index: number | null,
  rule: unknown,
): RuleDocument {
  const rules = document.rules.slice()
  if (index === null) rules.push(rule)
  else {
    if (!Number.isInteger(index) || index < 0 || index >= rules.length)
      throw new Error('Invalid rule index')
    rules[index] = rule
  }
  return { ...document, rules }
}
