import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { appendFileSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

export const sourceRepository = 'Fanju6/NetProxy-Magisk'
export const distributionRepository = 'Fanju6/NetProxy-Releases'

export function releaseIdentity(tag, count) {
  if (!/^v\d+\.\d+\.\d+$/.test(tag ?? '')) throw new Error('KernelSU 分发仅接受正式版本 tag')
  if (!/^[1-9]\d*$/.test(String(count)) || Number(count) > 2147483647) {
    throw new Error('构建号必须是有效的正 int32')
  }
  return {
    tag, count: String(count),
    sourceName: `NetProxy_${tag}_${count}.zip`,
    distributionName: `netproxy-${count}-${tag}.zip`,
  }
}

function assetDigest(asset) {
  if (!/^sha256:[a-f0-9]{64}$/.test(asset?.digest ?? '')) {
    throw new Error('GitHub 资源缺少有效的 SHA-256，不能确认分发来源')
  }
  return asset.digest.slice(7)
}

export function sourceAsset(release, identity) {
  if (!release || release.draft || release.prerelease || !release.published_at || release.tag_name !== identity.tag) {
    throw new Error('主仓库必须已有对应 tag 的已发布正式 Release')
  }
  const zips = release.assets.filter((asset) => /\.zip$/i.test(asset.name))
  if (zips.length !== 1 || zips[0].name !== identity.sourceName) {
    throw new Error('主仓库的唯一模块包与 tag 或构建号不一致')
  }
  assetDigest(zips[0])
  return zips[0]
}

export function verifyExisting(release, identity, source) {
  if (!release || release.draft) return false
  if (!release.immutable || release.prerelease || !release.published_at || release.tag_name !== identity.tag) {
    throw new Error('分发仓库已有版本不是对应的不可变正式发布，禁止覆盖')
  }
  const assets = release.assets
  if (assets.length !== 1 || assets[0].name !== identity.distributionName ||
      assets[0].size !== source.size || assetDigest(assets[0]) !== assetDigest(source)) {
    throw new Error('已发布分发包与主仓库不同，禁止覆盖或删除')
  }
  return true
}

export function verifyPackage(content, identity) {
  for (const [key, value] of Object.entries({ id: 'netproxy', version: identity.tag, versionCode: identity.count })) {
    const fields = content.split(/\r?\n/).filter((line) => line.startsWith(`${key}=`))
    if (fields.length !== 1 || fields[0] !== `${key}=${value}`) {
      throw new Error(`模块包 ${key} 与发布身份不一致`)
    }
  }
}

function run(command, args) {
  return execFileSync(command, args, { encoding: 'utf8', timeout: 120_000, stdio: ['ignore', 'pipe', 'pipe'] }).trim()
}

async function release(repository, tag, token) {
  const response = await fetch(`https://api.github.com/repos/${repository}/releases/tags/${encodeURIComponent(tag)}`, {
    headers: {
      Accept: 'application/vnd.github+json',
      'X-GitHub-Api-Version': '2022-11-28',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    signal: AbortSignal.timeout(15_000),
  })
  if (response.status === 404) return undefined
  if (!response.ok) throw new Error(`读取 ${repository} 发布失败：HTTP ${response.status}`)
  return response.json()
}

export async function prepare(tag, directory, { readRelease = release, execute = run, env = process.env } = {}) {
  const identity = releaseIdentity(tag, execute('git', ['rev-list', '--count', 'HEAD']))
  const source = await readRelease(sourceRepository, tag, env.GH_TOKEN)
  const asset = sourceAsset(source, identity)
  const target = await readRelease(distributionRepository, tag, env.DIST_REPO_TOKEN)
  const alreadyPublished = verifyExisting(target, identity, asset)
  mkdirSync(directory, { recursive: true })
  const zip = resolve(directory, identity.sourceName)
  execute('gh', ['release', 'download', tag, '--repo', sourceRepository, '--pattern', asset.name, '--dir', resolve(directory)])
  if (createHash('sha256').update(readFileSync(zip)).digest('hex') !== assetDigest(asset)) {
    throw new Error('下载的模块包与 GitHub 源资源摘要不一致')
  }
  verifyPackage(execute('7z', ['x', '-so', zip, 'module.prop']), identity)
  const commit = execute('git', ['rev-parse', 'HEAD'])
  if (!/^[a-f0-9]{40}$/.test(commit)) throw new Error('无法确认源码提交')
  const notes = resolve(directory, 'release-notes.md')
  writeFileSync(notes, `${source.body || ''}\n\n[源码提交](https://github.com/${sourceRepository}/commit/${commit})\n\nSHA-256: \`${assetDigest(asset)}\`\n`)
  return { zip, notes, already_published: alreadyPublished }
}

async function main() {
  const [operation, tag, path, count] = process.argv.slice(2)
  if (operation === 'check') {
    verifyPackage(run('7z', ['x', '-so', path, 'module.prop']), releaseIdentity(tag, count))
    return
  }
  if (operation === 'prepare') {
    const output = await prepare(tag, path)
    appendFileSync(process.env.GITHUB_OUTPUT, Object.entries(output).map(([key, value]) => `${key}=${value}\n`).join(''))
    console.log(output.already_published ? '已发布版本摘要一致，无需重复发布' : '源模块包与版本身份验证通过')
    return
  }
  if (operation === 'verify') {
    const identity = releaseIdentity(tag, run('git', ['rev-list', '--count', 'HEAD']))
    const source = sourceAsset(await release(sourceRepository, tag, process.env.GH_TOKEN), identity)
    if (!verifyExisting(await release(distributionRepository, tag, process.env.DIST_REPO_TOKEN), identity, source)) {
      throw new Error('分发仓库未完成正式发布')
    }
    console.log('分发仓库发布不可变，ZIP 与主仓库完全一致')
    return
  }
  throw new Error('用法：prepare-kernelsu.mjs check|prepare|verify <tag> [路径] [构建号]')
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => {
    console.error(error.message)
    process.exitCode = 1
  })
}
