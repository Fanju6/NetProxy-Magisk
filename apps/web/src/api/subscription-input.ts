import { z } from 'zod'

const text = z.string().refine((value) => !/\p{Cc}/u.test(value), 'Invalid text')
const url = text.refine((value) => {
  if (!/^https?:\/\//i.test(value) || /\s/.test(value)) return false
  try {
    return new URL(value).hostname !== ''
  } catch {
    return false
  }
}, 'Invalid subscription URL')
const headerName = z.string().regex(/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/)
const headerValue = z.string().refine((value) => !/\p{Cc}/u.test(value.replaceAll('\t', '')))

// Omission preserves a field; empty strings/objects explicitly clear supported fields.
export const subscriptionInputSchema = z.strictObject({
  name: text.refine((value) => value.trim() !== '', 'Invalid name').optional(),
  url: url.optional(),
  user_agent: text.optional(),
  hwid: text.optional(),
  custom_headers: z.record(headerName, headerValue).optional(),
  auto_update: z.boolean().optional(),
  update_interval: z.number().int().min(900).optional(),
  update_via_proxy: z.enum(['auto', 'always', 'never']).optional(),
  include: text.optional(),
  exclude: text.optional(),
  allow_insecure: z.boolean().optional(),
  download_timeout: z.number().int().positive().optional(),
})
export const subscriptionCreateSchema = subscriptionInputSchema.extend({ url })
export type SubscriptionInput = z.infer<typeof subscriptionInputSchema>
