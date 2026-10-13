import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { Script } from 'node:vm'

const root = resolve(import.meta.dirname, '../../module/webroot/control')
const html = readFileSync(resolve(root, 'index.html'), 'utf8')
const css = readFileSync(resolve(root, 'style.css'), 'utf8')
const js = readFileSync(resolve(root, 'app.js'), 'utf8')

test('本地面板资源齐全，JavaScript 可以解析', () => {
  new Script(js, { filename: 'app.js' })
  assert.match(html, /src="\.\/app\.js"/)
  assert.match(html, /href="\.\/style\.css"/)
  assert.match(css, /@media\(max-width:720px\)/)
})

test('Bearer 凭据只通过 Authorization Header，页面不持久化密钥', () => {
  assert.match(js, /Authorization:'Bearer '\+state\.secret/)
  assert.doesNotMatch(js, /localStorage|sessionStorage|document\.cookie/)
  assert.doesNotMatch(js, /new EventSource\(/)
  assert.match(js, /referrerPolicy:'no-referrer'/)
})

test('配置修改必须携带读取的 revision 并处理冲突', () => {
  assert.match(js, /revision:state\.configRevision,content:raw/)
  assert.match(js, /configRevision=data\.revision/)
  assert.match(js, /err\.persisted/)
  assert.match(js, /conflict/)
})

test('SSE 使用流式 fetch 和取消控制，处理状态及订阅', () => {
  assert.match(js, /new AbortController\(\)/)
  assert.match(js, /res\.body\.getReader\(\)/)
  assert.match(js, /visibilitychange/)
  assert.match(js, /name==='traffic'/)
  assert.match(js, /name==='subscriptions'/)
})
