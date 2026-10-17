import { expect, it } from 'vitest'
import { subscriptionCreateSchema, subscriptionInputSchema } from './subscription-input'

it('preserves omission and explicit clearing without inserting defaults', () => {
  expect(subscriptionInputSchema.parse({})).toEqual({})
  expect(subscriptionInputSchema.parse({ auto_update: false })).toEqual({ auto_update: false })
  const clear = {
    user_agent: '',
    hwid: '',
    include: '',
    exclude: '',
    custom_headers: {},
    allow_insecure: false,
  }
  expect(subscriptionInputSchema.parse(clear)).toEqual(clear)
  expect(subscriptionInputSchema.safeParse({ url: '' }).success).toBe(false)
  expect(subscriptionCreateSchema.safeParse({}).success).toBe(false)
  expect(
    subscriptionCreateSchema.parse({ url: 'https://example.invalid/feed?token=anonymous' }),
  ).toEqual({ url: 'https://example.invalid/feed?token=anonymous' })
})

it.each([
  { unknown: 'value' },
  { name: null },
  { url: null },
  { user_agent: null },
  { hwid: null },
  { custom_headers: null },
  { auto_update: null },
  { update_interval: null },
  { update_via_proxy: null },
  { include: null },
  { exclude: null },
  { allow_insecure: null },
  { download_timeout: null },
  { auto_update: 'true' },
  { update_interval: '900' },
  { update_interval: 899 },
  { update_interval: 900.5 },
  { update_via_proxy: 'direct' },
  { download_timeout: 0 },
  { download_timeout: -1 },
  { download_timeout: 1.5 },
  { allow_insecure: 1 },
  { custom_headers: [] },
  { custom_headers: { Authorization: 1 } },
  { custom_headers: { 'Invalid header': 'anonymous' } },
  { custom_headers: { 'X-Test': 'anonymous\r\nInjected: yes' } },
  { url: 'file:///private' },
  { url: 'http://' },
  { url: 'https://example.invalid/\tfeed' },
  { url: ' https://example.invalid/feed' },
  { name: '' },
  { name: '  ' },
])('rejects unknown fields, nulls, invalid types and invalid values (%#)', (input) => {
  expect(subscriptionInputSchema.safeParse(input).success).toBe(false)
})

it.each(['name', 'user_agent', 'hwid', 'include', 'exclude'] as const)(
  'rejects control characters in %s',
  (key) => {
    for (const control of ['\r', '\n', '\t', '\0'])
      expect(subscriptionInputSchema.safeParse({ [key]: `anonymous${control}value` }).success).toBe(
        false,
      )
  },
)

it('accepts exact field types, RFC token header names and all download modes', () => {
  for (const update_via_proxy of ['auto', 'always', 'never']) {
    expect(
      subscriptionInputSchema.safeParse({
        name: 'Anonymous subscription',
        url: 'http://example.invalid/feed',
        auto_update: true,
        update_interval: 900,
        update_via_proxy,
        download_timeout: 1,
        custom_headers: {
          "X-!#$%&'*+.^_`|~": 'anonymous\tvalue',
          Authorization: 'Bearer anonymous',
        },
      }).success,
    ).toBe(true)
  }
})
