import assert from 'node:assert/strict'
import { test } from 'node:test'
import { startSubscriptionSource } from './subscription-source.mjs'

test('controlled source serves valid anonymous data, conditional 304, changed content and failures', async () => {
  const fixture = await startSubscriptionSource()
  try {
    const url = `http://127.0.0.1:${fixture.port}/a`
    const first = await fetch(url)
    assert.equal(first.status, 200)
    assert.equal((await first.json()).outbounds[0].tag, 'Fixture-1')
    const cached = await fetch(url, { headers: { 'If-None-Match': first.headers.get('etag') } })
    assert.equal(cached.status, 304)
    fixture.feeds.get('a').version++
    assert.equal((await (await fetch(url)).json()).outbounds[0].tag, 'Fixture-2')
    fixture.feeds.get('a').fail = true
    assert.equal((await fetch(url)).status, 503)
    assert.equal(fixture.feeds.get('a').notModified, 1)
    assert.equal((await fetch(`http://127.0.0.1:${fixture.port}/missing`)).status, 404)
  } finally { await fixture.close() }
})

test('cancelled slow requests release the controlled source and remain independent per feed', async () => {
  const fixture = await startSubscriptionSource()
  try {
    fixture.feeds.get('a').delay = 10_000
    const controller = new AbortController()
    const slow = fetch(`http://127.0.0.1:${fixture.port}/a`, { signal: controller.signal })
    const rejected = assert.rejects(slow, { name: 'AbortError' })
    for (let i = 0; i < 100 && !fixture.feeds.get('a').active; i++) await new Promise((resolve) => setTimeout(resolve, 10))
    assert.equal(fixture.feeds.get('a').active, 1)
    assert.equal((await fetch(`http://127.0.0.1:${fixture.port}/b`)).status, 200)
    controller.abort()
    await rejected
    for (let i = 0; i < 50 && fixture.feeds.get('a').active; i++) await new Promise((resolve) => setTimeout(resolve, 10))
    assert.equal(fixture.feeds.get('a').active, 0)
  } finally { await fixture.close() }
})
