import { execFileSync } from 'node:child_process'
import { appendFileSync } from 'node:fs'
import { pathToFileURL } from 'node:url'

function allChecks() {
  return { module: true, core: true, webui: true, android: true, docs: true }
}

function isSharedBuildInput(path) {
  return path === '.gitattributes' ||
    path === '.github/resources.json' ||
    path.startsWith('.github/actions/') ||
    path.startsWith('.github/scripts/') ||
    path === '.github/workflows/ci.yml' ||
    path === '.github/workflows/build-module.yml' ||
    path === '.github/workflows/release.yml' ||
    path === '.github/workflows/update-resources.yml' ||
    path === '.github/workflows/verify.yml'
}

export function classifyChanges(paths) {
  const shared = paths.some(isSharedBuildInput)
  // WebUI 构建产物属于模块发布内容，但不需要触发 Native/Shell 核心验证。
  const moduleFiles = paths.some((path) =>
    path.startsWith('src/module/') && !path.startsWith('src/module/webroot/netproxy/'))
  const native = paths.some((path) => path.startsWith('src/native/netproxy/'))
  const tests = paths.some((path) => path.startsWith('tests/'))
  const webuiSource = paths.some((path) =>
    path.startsWith('src/webui/') || path.startsWith('src/module/webroot/netproxy/'))
  const androidSource = paths.some((path) => path.startsWith('src/android/'))
  const moduleConfig = paths.some((path) => path.startsWith('src/module/config/'))
  const docs = paths.some((path) =>
    path.startsWith('docs/') || path === '.github/workflows/docs.yml')
  const core = shared || moduleFiles || native || tests
  const android = shared || native || androidSource || moduleConfig
  const webui = shared || webuiSource

  return {
    module: core || webui || android,
    core,
    webui,
    android,
    docs,
  }
}

export function lastVerifiedCommit(env, run = execFileSync) {
  try {
    return run('gh', [
      'api', '--method', 'GET', `repos/${env.GITHUB_REPOSITORY}/actions/workflows/ci.yml/runs`,
      '-f', `branch=${env.GITHUB_REF_NAME}`, '-f', 'event=push', '-f', 'status=success',
      '-f', 'per_page=1', '--jq', '.workflow_runs[0].head_sha // empty',
    ], { encoding: 'utf8', timeout: 15_000, stdio: ['ignore', 'pipe', 'pipe'] }).trim()
  } catch {
    return undefined
  }
}

function isCommitSHA(value) {
  return /^[0-9a-f]{40}$/.test(value ?? '') && !/^0+$/.test(value)
}

export function checksForRange(before, after, run = execFileSync) {
  if (!isCommitSHA(before) || !isCommitSHA(after)) return allChecks()
  try {
    // 禁用重命名检测，移动前后的路径都必须参与验证范围判断。
    const changed = run('git', ['diff', '--name-only', '--no-renames', '-z', before, after, '--'], {
      encoding: 'utf8', maxBuffer: 16 * 1024 * 1024, stdio: ['ignore', 'pipe', 'pipe'],
    })
    return classifyChanges(changed.split('\0').filter(Boolean))
  } catch {
    // 强制推送或浅克隆造成范围不可得时，不能跳过验证。
    return allChecks()
  }
}

export function checksForEvent(event, before, after, run = execFileSync) {
  if (event !== 'push') return allChecks()
  return checksForRange(before, after, run)
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  let base
  let checks

  if (process.env.GITHUB_EVENT_NAME === 'push') {
    // 连续推送可能取消前一轮，必须覆盖上次成功验证以来的全部变更。
    base = lastVerifiedCommit(process.env)
    checks = checksForEvent('push', base, process.env.GITHUB_SHA)
  } else if (process.env.GITHUB_EVENT_NAME === 'pull_request') {
    base = process.env.NETPROXY_BASE_SHA
    checks = checksForRange(base, process.env.NETPROXY_HEAD_SHA)
  } else {
    checks = allChecks()
  }

  console.log(`验证基线: ${base || '无法确认，执行完整验证'}`)
  const output = Object.entries(checks).map(([key, value]) => `${key}=${value}`).join('\n') + '\n'
  appendFileSync(process.env.GITHUB_OUTPUT, output)
  console.log(output.trim())
}
