import assert from 'node:assert/strict'
import test from 'node:test'

import { buildReport } from './render-update-report.mjs'

test('Android 自动更新进入标题、概览与详情', () => {
  const report = buildReport({
    android: {
      updated: [{
        name: 'JSON Schema Validator',
        from: '2.0.1',
        to: '2.0.4',
        latest: '3.0.6',
        files: ['src/android/gradle/libs.versions.toml'],
        sourceUrl: 'https://example.com/releases',
      }],
      manual: [{
        name: 'Gradle Wrapper',
        current: '9.5.0',
        latest: '9.6.1',
        reason: '工具链升级需人工验证',
      }],
      diagnostics: [],
    },
  })

  assert.match(report.title, /Android 依赖/)
  assert.match(report.body, /1 项自动更新，1 项待人工/)
  assert.match(report.body, /`2\.0\.1` → `2\.0\.4`/)
  assert.match(report.body, /Gradle Wrapper：`9\.5\.0` → `9\.6\.1`/)
})

test('只有人工升级时不会伪报自动更新', () => {
  const report = buildReport({
    android: {
      updated: [],
      manual: [{ name: 'Kotlin', current: '2.4.10', latest: '3.0.0', reason: '工具链升级需人工验证' }],
      diagnostics: [],
    },
  })

  assert.match(report.body, /本次检查未发现可提交的更新/)
  assert.match(report.body, /无自动更新，1 项待人工/)
  assert.match(report.body, /#### Android 依赖/)
})

test('Web 面板更新只渲染 sing-box Dashboard', () => {
  const report = buildReport({
    dashboard: {
      name: 'sing-box Dashboard',
      currentCommit: '1234567890abcdef',
      latestCommit: 'abcdef1234567890',
      path: 'src/module/webroot/sing-box-dashboard',
    },
  })

  assert.match(report.title, /sing-box Dashboard/)
  assert.match(report.body, /1 个面板/)
  assert.match(report.body, /src\/module\/webroot\/sing-box-dashboard/)
})

test('GitHub Actions 更新只作为人工处理候选项', () => {
  const report = buildReport({
    actions: [{
      name: 'actions/checkout',
      current: 'v6',
      latestMajor: 'v7',
      latestRelease: 'v7.1.0',
      repositoryUrl: 'https://github.com/actions/checkout',
      releaseUrl: 'https://github.com/actions/checkout/releases/tag/v7.1.0',
    }],
  })

  assert.equal(report.title, 'chore(维护): 更新资源、内核与依赖')
  assert.match(report.body, /1 项待人工升级/)
  assert.match(report.body, /GitHub Actions（待人工升级）/)
  assert.match(report.body, /仅生成报告，不自动升级 GitHub Actions/)
})
