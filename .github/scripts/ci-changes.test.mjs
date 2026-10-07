import assert from 'node:assert/strict'
import test from 'node:test'
import { checksForEvent, checksForRange, classifyChanges, lastVerifiedCommit } from './ci-changes.mjs'

const allChecks = { module: true, core: true, webui: true, android: true, docs: true }
const noChecks = { module: false, core: false, webui: false, android: false, docs: false }

test('Android 修改会重建模块并执行 Android 验证', () => {
  assert.deepEqual(classifyChanges(['src/android/app/src/main/MainActivity.kt']), {
    module: true, core: false, webui: false, android: true, docs: false,
  })
})

test('Native 与默认配置变化仍验证 Android 调用方', () => {
  for (const path of ['src/native/netproxy/internal/module/app.go', 'src/module/config/ebpf/ebpf.conf']) {
    assert.deepEqual(classifyChanges([path]), {
      module: true, core: true, webui: false, android: true, docs: false,
    })
  }
})

test('模块、WebUI 和测试按各自范围验证', () => {
  assert.deepEqual(classifyChanges(['src/module/customize.sh']), {
    module: true, core: true, webui: false, android: false, docs: false,
  })
  assert.deepEqual(classifyChanges(['src/webui/src/exec.ts']), {
    module: true, core: false, webui: true, android: false, docs: false,
  })
  assert.deepEqual(classifyChanges(['src/module/webroot/netproxy/index.html']), {
    module: true, core: false, webui: true, android: false, docs: false,
  })
  assert.deepEqual(classifyChanges(['tests/ci_verify.sh']), {
    module: true, core: true, webui: false, android: false, docs: false,
  })
})

test('工作流和公共构建输入变化执行所有产品范围验证', () => {
  for (const path of ['.github/workflows/ci.yml', '.github/actions/build-module/action.yml']) {
    assert.deepEqual(classifyChanges([path]), {
      module: true, core: true, webui: true, android: true, docs: false,
    })
  }
})

test('统一验证入口与范围规则修改必须执行全部检查', () => {
  for (const path of ['tests/verify.sh', '.github/scripts/ci-changes.mjs', '.github/workflows/verify.yml', '.gitattributes']) {
    assert.deepEqual(classifyChanges([path]), allChecks)
  }
})

test('文档与 Issue 模板不触发模块打包', () => {
  assert.deepEqual(classifyChanges(['docs/index.md', 'README.md']), {
    module: false, core: false, webui: false, android: false, docs: true,
  })
  assert.deepEqual(classifyChanges(['.github/ISSUE_TEMPLATE/bug_report.yml']), noChecks)
  assert.deepEqual(classifyChanges([]), noChecks)
  assert.deepEqual(classifyChanges(['src/android/old name.kt', 'src/webui/新文件.ts']), {
    module: true, core: false, webui: true, android: true, docs: false,
  })
})

test('手动、首次推送与失效基线不能跳过检查', () => {
  for (const [event, before] of [
    ['workflow_dispatch', undefined], ['push', '0'.repeat(40)],
    ['push', undefined], ['push', 'invalid'], ['push', 'f'.repeat(40)],
  ]) {
    assert.deepEqual(checksForEvent(event, before, 'a'.repeat(40)), allChecks)
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
  assert.deepEqual(checksForEvent('push', base, 'b'.repeat(40), (command, args) => {
    assert.equal(command, 'git')
    assert.deepEqual(args, ['diff', '--name-only', '--no-renames', '-z', sha, 'b'.repeat(40), '--'])
    return 'src/module/customize.sh\0src/android/新 文件.kt\0'
  }), { module: true, core: true, webui: false, android: true, docs: false })
})

test('PR 以基准与头提交准确判断验证范围', () => {
  const base = 'a'.repeat(40)
  const head = 'b'.repeat(40)
  assert.deepEqual(checksForRange(base, head, (command, args) => {
    assert.equal(command, 'git')
    assert.deepEqual(args, ['diff', '--name-only', '--no-renames', '-z', base, head, '--'])
    return 'docs/guide/quick-start.md\0'
  }), { module: false, core: false, webui: false, android: false, docs: true })
})

test('查询失败或范围不可得时保守执行全部检查', () => {
  for (const query of [() => '', () => { throw new Error('API unavailable') }]) {
    const base = lastVerifiedCommit({}, query)
    assert.deepEqual(checksForEvent('push', base, 'a'.repeat(40)), allChecks)
  }
  assert.deepEqual(checksForRange('invalid', 'a'.repeat(40)), allChecks)
})
