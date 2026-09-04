import { execFileSync } from 'node:child_process'
import { appendFileSync } from 'node:fs'
import { pathToFileURL } from 'node:url'

export function classifyChanges(paths) {
  const shared = paths.some((path) =>
    path.startsWith('.github/') || path === '.gitattributes')
  const native = paths.some((path) => path.startsWith('src/native/netproxy/'))
  return {
    module: shared || native || paths.some((path) =>
      path.startsWith('src/module/') || path.startsWith('src/webui/') || path.startsWith('tests/')),
    android: shared || native || paths.some((path) =>
      path.startsWith('src/android/') || path.startsWith('src/module/config/')),
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

export function checksForEvent(event, before, after, run = execFileSync) {
  if (event !== 'push' || !/^[0-9a-f]{40}$/.test(before ?? '') || /^0+$/.test(before)) {
    return { module: true, android: true }
  }
  try {
    // 不依赖推送文件清单的数量上限；禁用重命名检测，旧路径和新路径均参与判断。
    const changed = run('git', ['diff', '--name-only', '--no-renames', '-z', before, after, '--'], {
      encoding: 'utf8', maxBuffer: 16 * 1024 * 1024, stdio: ['ignore', 'pipe', 'pipe'],
    })
    return classifyChanges(changed.split('\0').filter(Boolean))
  } catch {
    // 强制推送后旧提交可能已不可达，无法可靠比较时不能跳过验证。
    return { module: true, android: true }
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  // 连续推送可能取消前一轮，必须覆盖上次成功验证以来的全部变更。
  const base = process.env.GITHUB_EVENT_NAME === 'push' ? lastVerifiedCommit(process.env) : undefined
  const checks = checksForEvent(process.env.GITHUB_EVENT_NAME, base, process.env.GITHUB_SHA)
  console.log(`验证基线: ${base || '无法确认，执行完整验证'}`)
  const output = Object.entries(checks).map(([key, value]) => `${key}=${value}`).join('\n') + '\n'
  appendFileSync(process.env.GITHUB_OUTPUT, output)
  console.log(output.trim())
}
