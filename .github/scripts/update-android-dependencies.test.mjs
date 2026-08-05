import assert from 'node:assert/strict'
import test from 'node:test'

import {
  compareVersions,
  isStableVersion,
  parseCatalogVersions,
  parseMavenMetadata,
  selectAutomaticTarget,
  updateCatalogVersion,
} from './update-android-dependencies.mjs'

test('版本比较支持语义版本与日期版本', () => {
  assert.equal(compareVersions('1.14.0', '1.13.9'), 1)
  assert.equal(compareVersions('2026.07.00', '2026.06.01'), 1)
  assert.equal(compareVersions('6.1', '6.1.0'), 0)
  assert.equal(isStableVersion('2.4.20-RC'), false)
  assert.equal(isStableVersion('2.4.20'), true)
})

test('自动更新不跨主版本', () => {
  const versions = ['1.13.0', '1.14.2', '2.0.0', '1.15.0-beta01']
  assert.equal(selectAutomaticTarget('1.13.0', versions, 'auto-minor'), '1.14.2')
  assert.equal(selectAutomaticTarget('1.13.0', versions, 'manual'), '1.13.0')
})

test('0.x 依赖只自动接受补丁版本', () => {
  const versions = ['0.9.3', '0.9.5', '0.10.0', '1.0.0']
  assert.equal(selectAutomaticTarget('0.9.3', versions, 'auto-minor'), '0.9.5')
})

test('版本目录只修改 versions 区段中的目标键', () => {
  const input = `[versions]\ncompose-multiplatform = "1.10.0"\n\n[libraries]\nexample = { version = "1.10.0" }\n`
  const output = updateCatalogVersion(input, 'compose-multiplatform', '1.11.0')
  assert.match(output, /compose-multiplatform = "1\.11\.0"/)
  assert.match(output, /example = \{ version = "1\.10\.0" \}/)
  assert.equal(parseCatalogVersions(output).get('compose-multiplatform'), '1.11.0')
})

test('Maven metadata 只提取版本列表', () => {
  const versions = parseMavenMetadata('<metadata><versioning><versions><version>1.0.0</version><version>1.1.0</version></versions></versioning></metadata>')
  assert.deepEqual(versions, ['1.0.0', '1.1.0'])
})
