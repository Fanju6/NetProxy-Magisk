import { test } from 'node:test'
import assert from 'node:assert/strict'
import { decodeCtlResult } from '../src/contract.ts'
import { createPoller } from '../src/polling.ts'
import { complete, replaceCompletion } from '../src/autocomplete.ts'
import { parseCommandLine } from '../src/command.ts'
import { formatCtlOutput } from '../src/format.ts'

test('补全使用当前 Catalog，候选替换保留空格、引号与反斜杠', () => {
  const groups = ['default', 'Kitty Network', 'Kitty "Lab"\\Node']
  assert.deepEqual(complete('catalog show ', groups).candidates, groups)
  assert.deepEqual(complete('sub show ', groups, ['sub-id']).candidates, ['sub-id'])
  assert.deepEqual(complete('catalog show "Kitty ', groups).candidates, groups.slice(1))
  assert.deepEqual(complete('service\tst').candidates, ['status', 'start', 'stop'])
  assert.deepEqual(complete('! echo hi', groups).candidates, [])
  for (const source of ['catalog show ', 'catalog show "Ki', 'catalog show\t']) {
    for (const group of groups) {
      assert.deepEqual(parseCommandLine(replaceCompletion(source, group)), ['catalog', 'show', group])
    }
  }
  assert.equal(complete('node use auto def', groups).completed, 'node use auto default ')
})

test('格式化只改变 JSON 排版，文本、错误和危险字符保持原样', () => {
  const raw = '{"schema":1,"message":"<script>alert(1)</script>","data":{"name":"本地配置"}}'
  assert.deepEqual(JSON.parse(formatCtlOutput(raw)), JSON.parse(raw))
  assert.equal(formatCtlOutput('shell output\n'), 'shell output\n')
  assert.equal(formatCtlOutput('not { json'), 'not { json')
})

test('JSON 与进程退出状态必须同时成功，结构化失败保留', () => {
  const success = { schema: 1, ok: true, code: 'service.status', message: '服务状态', data: { state: 'ready' } }
  assert.deepEqual(decodeCtlResult({ out: JSON.stringify(success), err: '', code: 0 }), success)
  assert.equal(decodeCtlResult({ out: JSON.stringify(success), err: 'terminated', code: 1 }).ok, false)
  const failure = { ...success, ok: false, code: 'subscription.runtime_sync_failed', message: '运行时同步失败' }
  assert.deepEqual(decodeCtlResult({ out: JSON.stringify(failure), err: 'extra', code: 1 }), failure)
  assert.equal(decodeCtlResult({ out: '{"schema":2}', err: '', code: 0 }).code, 'transport.invalid_json')
  assert.equal(decodeCtlResult({ out: '', err: 'denied', code: 1 }).message, 'denied')
})

test('慢请求不重叠，隐藏时暂停，过期响应不可覆盖当前状态', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const pending = []
  const results = []
  const poller = createPoller(() => new Promise(resolve => pending.push(resolve)), value => results.push(value))
  const settle = async () => { for (let i = 0; i < 5; i++) await Promise.resolve() }
  poller.setActive(true)
  t.mock.timers.tick(30_000)
  assert.equal(pending.length, 1)
  poller.setActive(false)
  poller.setActive(true)
  assert.equal(pending.length, 1)
  pending.shift()('expired')
  await settle()
  assert.deepEqual(results, [])
  assert.equal(pending.length, 1)
  pending.shift()('ready')
  await settle()
  assert.deepEqual(results, ['ready'])
  t.mock.timers.tick(4999)
  assert.equal(pending.length, 0)
  t.mock.timers.tick(1)
  assert.equal(pending.length, 1)
  poller.refresh()
  poller.refresh()
  pending.shift()('before-command')
  await settle()
  assert.deepEqual(results, ['ready'])
  assert.equal(pending.length, 1)
  poller.setActive(false)
  pending.shift()('hidden')
  await settle()
  t.mock.timers.tick(30_000)
  assert.equal(pending.length, 0)
})
