import { createServer } from 'node:http'

// Anonymous loopback-only subscription source for authorized device tests, never a product endpoint.
export async function startSubscriptionSource(port = 0) {
  const feeds = new Map(['a', 'b'].map((id) => [id, { version: 1, fail: false, delay: 0, requests: 0, notModified: 0, active: 0 }]))
  const timers = new Set()
  const server = createServer((request, response) => {
    const feed = feeds.get(request.url?.slice(1))
    if (request.method !== 'GET' || !feed) { response.writeHead(404).end(); return }
    feed.requests++
    feed.active++
    let timer
    let ended = false
    const finish = () => {
      if (ended) return
      ended = true
      feed.active--
      if (timer) { clearTimeout(timer); timers.delete(timer) }
    }
    response.on('close', finish)
    const send = () => {
      if (ended) return
      if (feed.fail) { response.writeHead(503).end('Fixture unavailable'); return }
      const tag = `Fixture-${feed.version}`
      const etag = `"fixture-${feed.version}"`
      if (request.headers['if-none-match'] === etag) {
        feed.notModified++
        response.writeHead(304, { ETag: etag }).end()
        return
      }
      response.writeHead(200, { 'Content-Type': 'application/json', ETag: etag,
        'Subscription-Userinfo': 'upload=1024; download=2048; total=1073741824; expire=1893456000' })
      response.end(JSON.stringify({ outbounds: [{ type: 'socks', tag, server: '127.0.0.1', server_port: 9, version: '5' }] }))
    }
    if (feed.delay) { timer = setTimeout(send, feed.delay); timers.add(timer) } else send()
  })
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(port, '127.0.0.1', resolve) })
  return { feeds, port: server.address().port, close: async () => {
    for (const timer of timers) clearTimeout(timer)
    timers.clear()
    server.closeAllConnections()
    await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
  } }
}
