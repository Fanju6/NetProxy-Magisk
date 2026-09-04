import assert from 'node:assert/strict'
import test from 'node:test'
import { checksForEvent, classifyChanges, lastVerifiedCommit } from './ci-changes.mjs'

test('纯 Android 修改不重打模块包', () => {
  assert.deepEqual(classifyChanges(['src/android/app/src/main/MainActivity.kt']), { module: false, android: true })
})

test('Native 与默认配置变化仍验证 Android 调用方', () => {
  for (const path of ['src/native/netproxy/internal/module/app.go', 'src/module/config/ebpf/ebpf.conf']) {
    assert.deepEqual(classifyChanges([path]), { module: true, android: true })
  }
})

test('模块、WebUI 和测试变化无需安装 Android 工具链', () => {
  for (const path of ['src/module/customize.sh', 'src/module/NetProxy.apk', 'src/webui/src/exec.ts', 'tests/ci_verify.sh']) {
    assert.deepEqual(classifyChanges([path]), { module: true, android: false })
  }
})

test('工作流和公共构建输入变化执行完整验证', () => {
  for (const path of ['.github/workflows/ci.yml', '.github/actions/build-module/action.yml', '.gitattributes']) {
    assert.deepEqual(classifyChanges([path]), { module: true, android: true })
  }
})

test('文档和空变更不打包，跨目录移动覆盖两端', () => {
  assert.deepEqual(classifyChanges(['docs/index.md', 'README.md']), { module: false, android: false })
  assert.deepEqual(classifyChanges([]), { module: false, android: false })
  assert.deepEqual(classifyChanges(['src/android/old name.kt', 'src/webui/新文件.ts']), { module: true, android: true })
})

test('手动、首次推送与失效基线不能跳过检查', () => {
  for (const [event, before] of [
    ['workflow_dispatch', undefined], ['push', '0'.repeat(40)],
    ['push', undefined], ['push', 'invalid'], ['push', 'f'.repeat(40)],
  ]) {
    assert.deepEqual(checksForEvent(event, before, 'HEAD'), { module: true, android: true })
  }
})

test('查询同分支上次成功的 push，跳过失败或取消的运行', () => {
  const sha = 'a'.repeat(40)
  const base = lastVerifiedCommit({ GITHUB_REPOSITORY: 'owner/repo', GITHUB_REF_NAME: 'main' }, (command, args, options) => {
    assert.equal(command, 'gh')
    assert.ok(args.includes('repos/owner/repo/actions/workflows/ci.yml/runs'))
    for (const query of ['branch=main', 'event=push', 'status=success', 'per_page=1']) {
      assert.ok(args.includes(query))
    }
    assert.equal(options.timeout, 15_000)
    return sha + '\n'
  })
  assert.equal(base, sha)
  assert.deepEqual(checksForEvent('push', base, 'HEAD', (command, args) => {
    assert.equal(command, 'git')
    assert.deepEqual(args, ['diff', '--name-only', '--no-renames', '-z', sha, 'HEAD', '--'])
    return 'src/module/customize.sh\0src/android/新 文件.kt\0'
  }), { module: true, android: true })
})

test('查询超时或没有成功运行时保守执行全部检查', () => {
  for (const query of [() => '', () => { throw new Error('API unavailable') }]) {
    const base = lastVerifiedCommit({}, query)
    assert.deepEqual(checksForEvent('push', base, 'HEAD'), { module: true, android: true })
  }
})
