import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, readFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { checkBuild } from '../scripts/check-build.mjs'
import { decodeCtlResult } from '../src/contract.ts'
import { createPoller } from '../src/polling.ts'
import { complete, replaceCompletion } from '../src/autocomplete.ts'
import { parseCommandLine } from '../src/command.ts'
import { formatCtlOutput } from '../src/format.ts'
import { mockCtl } from '../src/mock.ts'
import { COMMANDS } from '../src/commands.ts'

test('构建检查拒绝缺失或空的页面资源及运行时角色帧', t => {
  const root = mkdtempSync(join(tmpdir(), 'netproxy-webui-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  writeFileSync(join(root, 'index.html'), '<script src="./main.js"></script><link href="https://mui.kernelsu.org/internal/insets.css">')
  assert.throws(() => checkBuild(root), /ENOENT/)
  writeFileSync(join(root, 'main.js'), 'const frame="./mascots/dragon/sleep-a.svg"')
  assert.throws(() => checkBuild(root), /ENOENT/)
  mkdirSync(join(root, 'mascots/dragon'), { recursive: true })
  writeFileSync(join(root, 'mascots/dragon/sleep-a.svg'), '')
  assert.throws(() => checkBuild(root), /资源无效/)
  writeFileSync(join(root, 'mascots/dragon/sleep-a.svg'), '<svg/>')
  assert.doesNotThrow(() => checkBuild(root))
})

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

test('节点选择运行时失败保留持久化状态与原始错误', () => {
  const response = { schema: 1, ok: false, code: 'node.runtime_sync_failed', message: '节点选择已保存，但运行时切换失败', data: { persisted: true, runtime_synced: false, group_id: 'default', mode: 'manual', selected: '本地配置/NODE' } }
  const output = { out: JSON.stringify(response), err: '', code: 1 }
  assert.deepEqual(decodeCtlResult(output), response)
  assert.deepEqual(JSON.parse(formatCtlOutput(output.out)), response)
})

test('模式补全仅使用配置列表，保留自定义名称和空格', () => {
  const modes = ['Rule', 'Direct', 'Office Network']
  assert.deepEqual(complete('mode ', [], [], modes).candidates, modes)
  assert.deepEqual(complete('mode ').candidates, [])
  assert.equal(complete('mode Off', [], [], modes).completed, 'mode "Office Network" ')
  const run = (...args) => decodeCtlResult(mockCtl(args))
  assert.equal(run('mode', 'global').ok, false)
  assert.equal(run('mode', 'Global').data.mode, 'Global')
  assert.deepEqual(run('mode').data.available, run('service', 'status').data.available_outbound_modes)
  run('mode', 'Rule')
})

test('入站帮助与补全只使用公共配置目标，诊断仍保留 ebpf status', () => {
  const targets = ['inbound', 'inbound/backend', 'inbound/ebpf', 'inbound/tun']
  for (const action of ['read', 'apply', 'validate']) {
    assert.deepEqual(complete(`config ${action} in`).candidates, targets)
    assert.ok(!complete(`config ${action} `).candidates.includes('ebpf'))
    assert.deepEqual(complete(`config ${action} ebpf`).candidates, [])
  }
  assert.deepEqual(complete('config read runtime/in').candidates, ['runtime/inbound.json'])
  assert.deepEqual(complete('config apply runtime/').candidates, [])
  assert.deepEqual(complete('ebpf ').candidates, ['status'])
  assert.deepEqual(complete('tun ').candidates, [])
  assert.match(COMMANDS.config.help, /不能用 \{\} 删除/)
  assert.match(COMMANDS.service.help, /active_backend.*ready.*PID\/API/)
})

test('mock 同步单文件模板、分区与真实后端状态边界', () => {
  const run = (...args) => decodeCtlResult(mockCtl(args))
  const defaults = JSON.parse(readFileSync(new URL('../../module/config/inbound/inbound.json', import.meta.url)))
  run('service', 'stop')
  assert.equal(run('service', 'status').data.configured_backend, 'ebpf')
  assert.equal(run('service', 'status').data.active_backend, null)
  assert.deepEqual(JSON.parse(run('config', 'read', 'inbound').data.content), defaults)
  assert.deepEqual(run('config', 'list').data.filter(item => item.category === 'inbound').map(item => item.id), [
    'inbound', 'inbound/backend', 'inbound/ebpf', 'inbound/tun',
  ])
  for (const section of ['backend', 'ebpf', 'tun']) {
    const result = run('config', 'read', `inbound/${section}`)
    assert.deepEqual(JSON.parse(result.data.content), { [section]: defaults[section] })
    assert.ok(result.data.revision)
  }
  assert.deepEqual(run('app', 'list').data, { enabled: true, mode: 'blacklist', proxy_apps: '', bypass_apps: '' })
  assert.equal(run('config', 'read', 'ebpf').ok, false)
  assert.equal(run('config', 'read', 'runtime/ebpf.json').ok, false)
  assert.equal(run('tun', 'status').ok, false)
  assert.equal(run('ebpf', 'status').ok, true)
  assert.equal(run('config', 'read', 'runtime/inbound.json').ok, false)
  run('service', 'start')
  assert.equal(run('service', 'status').data.active_backend, 'ebpf')
  assert.equal(run('service', 'status').data.pid, 4242)
  const runtime = JSON.parse(run('config', 'read', 'runtime/inbound.json').data.content)
  assert.equal(runtime.inbounds.length, 1)
  assert.equal(runtime.inbounds[0].tag, 'netproxy-in')
  assert.deepEqual(runtime.inbounds[0].shared, { enabled: false })
  assert.deepEqual(run('config', 'list').data.filter(item => item.category === 'runtime').map(item => [item.id, item.editable]), [
    ['runtime/inbound.json', false], ['runtime/providers.json', false], ['runtime/outbounds.json', false],
  ])
  run('service', 'stop')
  assert.equal(run('service', 'status').data.active_backend, null)
  assert.equal(run('config', 'read', 'runtime/inbound.json').ok, true)
})

test('eBPF mock 区分可读预检与显式原始报告', () => {
  const readable = decodeCtlResult(mockCtl(['ebpf', 'status'])).data
  assert.equal(readable.raw, false)
  assert.match(readable.content, /能力预检通过/)
  assert.match(readable.content, /尚未验证实际挂载/)
  assert.equal(readable.report.result, 'preflight_passed')
  assert.equal(readable.report.preflight, true)
  assert.equal(readable.report.exact_object_load, true)
  assert.equal(Object.hasOwn(readable.report, 'active_programs'), false)
  for (const args of [['--raw'], ['shared', '--raw'], ['--raw', 'all']]) {
    const raw = decodeCtlResult(mockCtl(['ebpf', 'status', ...args])).data
    assert.equal(raw.raw, true)
    assert.deepEqual(JSON.parse(raw.content), raw.report)
    assert.ok(['configured', 'shared', 'all'].includes(raw.mode))
  }
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
