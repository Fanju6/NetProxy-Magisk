import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { distributionRepository, prepare, releaseIdentity, sourceAsset, sourceRepository, verifyExisting, verifyPackage } from './prepare-kernelsu.mjs'

const identity = releaseIdentity('v8.6.0', '1200')
const bytes = Buffer.from('anonymous release fixture')
const digest = `sha256:${createHash('sha256').update(bytes).digest('hex')}`
const source = { name: identity.sourceName, size: bytes.length, digest }
const published = { tag_name: identity.tag, draft: false, prerelease: false, published_at: '2026-10-11T00:00:00Z', assets: [source] }
const target = { ...published, immutable: true, assets: [{ ...source, name: identity.distributionName }] }
const prop = `id=netproxy\nversion=${identity.tag}\nversionCode=${identity.count}\n`

test('正式 tag 与正 int32 构建号决定两个仓库的命名', () => {
  assert.equal(identity.sourceName, 'NetProxy_v8.6.0_1200.zip')
  assert.equal(identity.distributionName, 'netproxy-1200-v8.6.0.zip')
  for (const tag of ['nightly', 'v8.6.0-beta.1', '--help', '', undefined]) {
    assert.throws(() => releaseIdentity(tag, 1200))
  }
  for (const count of [0, -1, 1.5, '01', 'abc', 2147483648]) {
    assert.throws(() => releaseIdentity(identity.tag, count))
  }
})

test('源 Release 必须已正式发布，包名和摘要都有效', () => {
  assert.deepEqual(sourceAsset(published, identity), source)
  for (const release of [undefined, { ...published, draft: true }, { ...published, prerelease: true },
    { ...published, published_at: null }, { ...published, tag_name: 'v8.5.1' },
    { ...published, assets: [] }, { ...published, assets: [source, { ...source, name: 'debug.zip' }] },
    { ...published, assets: [{ ...source, name: 'NetProxy_v8.6.0_1199.zip' }] },
    { ...published, assets: [{ ...source, digest: null }] },
  ]) assert.throws(() => sourceAsset(release, identity))
})

test('重试只接受相同的不可变版本，草稿交由官方 Action 恢复', () => {
  assert.equal(verifyExisting(undefined, identity, source), false)
  assert.equal(verifyExisting({ draft: true }, identity, source), false)
  assert.equal(verifyExisting(target, identity, source), true)
  for (const release of [{ ...target, immutable: false }, { ...target, prerelease: true },
    { ...target, published_at: null }, { ...target, tag_name: 'v8.5.1' },
    { ...target, assets: [] }, { ...target, assets: [...target.assets, source] },
    { ...target, assets: [{ ...target.assets[0], size: source.size + 1 }] },
    { ...target, assets: [{ ...target.assets[0], digest: `sha256:${'0'.repeat(64)}` }] },
    { ...target, assets: [{ ...target.assets[0], name: source.name }] },
  ]) assert.throws(() => verifyExisting(release, identity, source))
})

test('ZIP 的 ID、版本、构建号不得缺失、重复或与 tag 不同', () => {
  verifyPackage(prop, identity)
  verifyPackage(prop.replaceAll('\n', '\r\n'), identity)
  for (const content of [prop.replace('netproxy', 'other'), prop.replace('v8.6.0', 'v8.5.1'),
    prop.replace('1200', '1199'), prop + 'versionCode=1200\n', prop.replace('id=netproxy\n', ''),
  ]) assert.throws(() => verifyPackage(content, identity))
})

test('独立重试下载既有源包、不构建、不改已发布目标', async (t) => {
  const directory = mkdtempSync(join(tmpdir(), 'netproxy-distribution-'))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const reads = []
  const commands = []
  const result = await prepare(identity.tag, directory, {
    env: { GH_TOKEN: 'source-fixture', DIST_REPO_TOKEN: 'distribution-fixture' },
    readRelease: async (repository, tag, token) => {
      reads.push([repository, tag, token])
      return repository === sourceRepository ? published : target
    },
    execute: (command, args) => {
      commands.push([command, args])
      if (command === 'git') return args[0] === 'rev-list' ? identity.count : 'a'.repeat(40)
      if (command === 'gh') {
        assert.deepEqual(args, ['release', 'download', identity.tag, '--repo', sourceRepository,
          '--pattern', source.name, '--dir', directory])
        writeFileSync(join(directory, source.name), bytes)
        return ''
      }
      assert.equal(command, '7z')
      return prop
    },
  })
  assert.equal(result.already_published, true)
  assert.deepEqual(reads, [[sourceRepository, identity.tag, 'source-fixture'],
    [distributionRepository, identity.tag, 'distribution-fixture']])
  assert.match(readFileSync(result.notes, 'utf8'), /源码提交/)
  assert.equal(commands.length, 4)
})

test('下载内容被替换时停止，不能继续分发', async (t) => {
  const directory = mkdtempSync(join(tmpdir(), 'netproxy-distribution-corrupt-'))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  await assert.rejects(prepare(identity.tag, directory, {
    env: {},
    readRelease: async (repository) => repository === sourceRepository ? published : undefined,
    execute: (command) => {
      if (command === 'git') return identity.count
      writeFileSync(join(directory, source.name), 'different')
      return ''
    },
  }), /摘要不一致/)
})

test('正式发布先预检再发布，独立重试不构建，nightly 不分发', () => {
  const main = readFileSync(new URL('../workflows/release.yml', import.meta.url), 'utf8')
  const retry = readFileSync(new URL('../workflows/publish-kernelsu.yml', import.meta.url), 'utf8')
  const nightly = readFileSync(new URL('../workflows/ci.yml', import.meta.url), 'utf8')
  assert.ok(main.indexOf('name: KernelSU 分发预检') < main.indexOf('name: 发布 GitHub Release'))
  assert.match(main, /needs: release\s+uses: \.\/\.github\/workflows\/publish-kernelsu.yml/)
  assert.match(main, /主仓库.*已发布，KernelSU 分发失败/)
  assert.match(retry, /default: true/)
  assert.match(retry, /ref: \$\{\{ inputs.tag }}/)
  assert.doesNotMatch(retry, /build-module|softprops|telegram|git push/)
  assert.doesNotMatch(nightly, /uses:.*publish-kernelsu|module-release@/)
  const pin = 'ca1187c92135b3c31e48d37e008d61153120a08b'
  assert.ok(main.includes(`module-release@${pin}`) && retry.includes(`module-release@${pin}`))
})
