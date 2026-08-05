import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const REPORT_FILES = {
  rules: 'rules.json',
  zashboard: 'zashboard.json',
  dashboard: 'sing-box-dashboard.json',
  singBox: 'sing-box.json',
  npm: 'npm.json',
  android: 'android.json',
  actions: 'actions.json',
  verification: 'verification.json',
}

function readJson(reportDir, name, fallback) {
  const file = path.join(reportDir, REPORT_FILES[name])
  if (!fs.existsSync(file)) return fallback
  return JSON.parse(fs.readFileSync(file, 'utf8'))
}

function shortSha(value) {
  return value ? value.slice(0, 7) : '未知'
}

function firstLine(value) {
  return String(value || '').split(/\r?\n/, 1)[0].trim()
}

function formatDate(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.toISOString().slice(0, 10)} ${date.toISOString().slice(11, 16)} UTC`
}

function formatBytes(value) {
  const size = Number(value)
  if (!Number.isFinite(size) || size < 0) return '未知'
  if (size < 1024) return `${size} B`
  if (size < 1024 ** 2) return `${(size / 1024).toFixed(1)} KiB`
  return `${(size / 1024 ** 2).toFixed(2)} MiB`
}

function markdownLink(label, url) {
  return url ? `[${label}](${url})` : label
}

function joinChinese(items) {
  if (items.length === 0) return ''
  if (items.length === 1) return items[0]
  const last = items.at(-1)
  const separator = /^[A-Za-z0-9]/.test(last) ? '与 ' : '与'
  if (items.length === 2) return `${items[0]}${separator}${last}`
  return `${items.slice(0, -1).join('、')}${separator}${last}`
}

function appendReleaseNotes(lines, notes, url) {
  const normalized = String(notes || '').replace(/\r\n/g, '\n').trim()
  if (!normalized) return

  const limit = 1800
  const clipped = normalized.length > limit
    ? `${normalized.slice(0, limit).replace(/\s+\S*$/, '')}\n\n……`
    : normalized

  lines.push('')
  lines.push('<details>')
  lines.push('<summary>上游发布说明</summary>')
  lines.push('')
  lines.push(clipped)
  if (normalized.length > limit && url) {
    lines.push('')
    lines.push(`[查看完整发布说明](${url})`)
  }
  lines.push('')
  lines.push('</details>')
}

function renderRules(lines, rules) {
  if (rules.length === 0) return
  lines.push('#### 规则资源', '')

  for (const item of rules) {
    lines.push(`- **${item.name || path.basename(item.path, '.srs')}**`)
    lines.push(`  - 模块文件：\`${item.path}\``)

    const oldCommit = shortSha(item.currentCommit)
    const newCommit = shortSha(item.latestCommit)
    const commitLabel = item.currentCommit && item.currentCommit !== item.latestCommit
      ? `${oldCommit} → ${newCommit}`
      : newCommit
    lines.push(`  - 上游版本：${markdownLink(commitLabel, item.compareUrl || item.commitUrl)}`)

    const message = firstLine(item.commitMessage)
    const date = formatDate(item.commitDate)
    if (message || date) {
      lines.push(`  - 最新提交：${[message, date].filter(Boolean).join('，')}`)
    }

    if (Number.isFinite(Number(item.oldRuleCount)) && Number.isFinite(Number(item.newRuleCount))) {
      lines.push(`  - 规则条目：${item.oldRuleCount} → ${item.newRuleCount}`)
    }

    lines.push(`  - 文件大小：${formatBytes(item.oldSize)} → ${formatBytes(item.newSize)}`)
    if (item.oldSha256 && item.newSha256) {
      lines.push(`  - SHA-256：\`${shortSha(item.oldSha256)}\` → \`${shortSha(item.newSha256)}\``)
    }
  }

  lines.push('')
}

function renderZashboard(lines, item) {
  if (!item) return
  lines.push('##### zashboard', '')
  lines.push(`- 版本：${markdownLink(`${item.currentVersion || '初始'} → ${item.latestVersion}`, item.releaseUrl)}`)
  if (item.releaseName && item.releaseName !== item.latestVersion) {
    lines.push(`- 发布名称：${item.releaseName}`)
  }
  if (item.publishedAt) lines.push(`- 发布时间：${formatDate(item.publishedAt)}`)
  lines.push(`- 模块目录：\`${item.path}\``)
  if (item.assetSize) lines.push(`- 发布包大小：${formatBytes(item.assetSize)}`)
  if (item.assetSha256) lines.push(`- 发布包 SHA-256：\`${item.assetSha256}\``)
  appendReleaseNotes(lines, item.releaseNotes, item.releaseUrl)
  lines.push('')
}

function renderDashboard(lines, item) {
  if (!item) return
  lines.push('##### sing-box Dashboard', '')
  lines.push(`- 部署版本：${markdownLink(`${shortSha(item.currentCommit)} → ${shortSha(item.latestCommit)}`, item.deployCommitUrl)}`)

  if (item.currentSourceCommit || item.latestSourceCommit) {
    lines.push(`- 源码版本：${markdownLink(`${shortSha(item.currentSourceCommit)} → ${shortSha(item.latestSourceCommit)}`, item.compareUrl)}`)
  }

  if (item.fileCount !== undefined) {
    lines.push(`- 代码变化：${item.fileCount} 个文件，+${item.additions || 0} / -${item.deletions || 0}`)
  }

  if (Array.isArray(item.commits) && item.commits.length > 0) {
    lines.push('- 上游提交：')
    for (const commit of item.commits.slice(0, 8)) {
      lines.push(`  - ${markdownLink(shortSha(commit.sha), commit.url)} ${firstLine(commit.message)}`)
    }
    if (item.commits.length > 8 && item.compareUrl) {
      lines.push(`  - 其余 ${item.commits.length - 8} 条提交请查看[完整比较](${item.compareUrl})`)
    }
  }

  lines.push(`- 模块目录：\`${item.path}\``, '')
}

function renderSingBox(lines, item) {
  if (!item) return
  lines.push('#### sing-box 内核', '')
  lines.push(`- 版本：${markdownLink(`${item.currentVersion} → ${item.latestVersion}`, item.releaseUrl || item.compareUrl)}`)
  if (item.publishedAt) lines.push(`- 发布时间：${formatDate(item.publishedAt)}`)
  lines.push(`- 二进制大小：${formatBytes(item.oldSize)} → ${formatBytes(item.newSize)}`)
  if (item.sha256) lines.push(`- SHA-256：\`${item.sha256}\``)
  if (Array.isArray(item.buildTags) && item.buildTags.length > 0) {
    lines.push(`- 构建标签：${item.buildTags.map((tag) => `\`${tag}\``).join('、')}`)
  }
  if (Array.isArray(item.checks) && item.checks.length > 0) {
    lines.push(`- 构建检查：${item.checks.join('、')}`)
  }
  appendReleaseNotes(lines, item.releaseNotes, item.releaseUrl)
  lines.push('')
}

function renderNpm(lines, projects) {
  if (projects.length === 0) return
  lines.push('#### npm 依赖', '')

  for (const project of projects) {
    lines.push(`##### ${project.label}`, '')
    for (const dependency of project.changes || []) {
      const type = dependency.type === 'devDependencies' ? '开发依赖' : '运行依赖'
      const name = markdownLink(dependency.name, `https://www.npmjs.com/package/${dependency.name}`)
      lines.push(`- ${name}：\`${dependency.from}\` → \`${dependency.to}\`（${type}）`)
    }
    if (project.lockfileChangeCount > 0) {
      lines.push(`- 锁文件：${project.lockfileChangeCount} 项解析版本变化（包含直接与传递依赖）`)
    }
    lines.push('')
  }
}

function renderAndroid(lines, report) {
  if (!report) return
  const updated = Array.isArray(report.updated) ? report.updated : []
  const manual = Array.isArray(report.manual) ? report.manual : []
  const diagnostics = Array.isArray(report.diagnostics) ? report.diagnostics : []
  if (updated.length === 0 && manual.length === 0 && diagnostics.length === 0) return

  lines.push('#### Android 依赖', '')
  if (updated.length > 0) {
    lines.push('##### 自动更新', '')
    for (const dependency of updated) {
      const name = markdownLink(dependency.name, dependency.sourceUrl)
      lines.push(`- ${name}：\`${dependency.from}\` → \`${dependency.to}\``)
      if (dependency.latest && dependency.latest !== dependency.to) {
        lines.push(`  - 最新稳定版：\`${dependency.latest}\`，超出自动更新范围`)
      }
      if (Array.isArray(dependency.files) && dependency.files.length > 0) {
        lines.push(`  - 版本目录：${[...new Set(dependency.files)].map((file) => `\`${file}\``).join('、')}`)
      }
    }
    lines.push('')
  }

  if (manual.length > 0) {
    lines.push('##### 待人工升级', '')
    for (const dependency of manual) {
      const name = markdownLink(dependency.name, dependency.sourceUrl)
      lines.push(`- ${name}：\`${dependency.current}\` → \`${dependency.latest}\`（${dependency.reason}）`)
    }
    lines.push('')
  }

  if (diagnostics.length > 0) {
    lines.push('##### 检查警告', '')
    for (const diagnostic of diagnostics) {
      lines.push(`- ${diagnostic.name}：${diagnostic.message}`)
    }
    lines.push('')
  }
}

function renderActions(lines, actions) {
  if (actions.length === 0) return
  lines.push('#### GitHub Actions', '')
  for (const action of actions) {
    const name = markdownLink(action.name, action.repositoryUrl)
    const release = markdownLink(action.latestRelease || action.latestMajor, action.releaseUrl)
    const publishedAt = formatDate(action.publishedAt)
    lines.push(`- ${name}：\`${action.current}\` → \`${action.latestMajor}\``)
    lines.push(`  - 最新发布：${release}${publishedAt ? `，${publishedAt}` : ''}`)
    if (action.releaseName && action.releaseName !== action.latestRelease) {
      lines.push(`  - 发布名称：${action.releaseName}`)
    }
  }
  lines.push('')
}

export function buildReport(data, context = {}) {
  const rules = Array.isArray(data.rules) ? data.rules : []
  const dashboards = [data.zashboard, data.dashboard].filter(Boolean)
  const npmProjects = Array.isArray(data.npm) ? data.npm : []
  const npmDirectChanges = npmProjects.reduce((total, project) => total + (project.changes?.length || 0), 0)
  const npmLockChanges = npmProjects.reduce((total, project) => total + (project.lockfileChangeCount || 0), 0)
  const actions = Array.isArray(data.actions) ? data.actions : []
  const android = data.android || {}
  const androidUpdated = Array.isArray(android.updated) ? android.updated : []
  const androidManual = Array.isArray(android.manual) ? android.manual : []
  const androidDiagnostics = Array.isArray(android.diagnostics) ? android.diagnostics : []

  const categories = [
    { id: 'rules', label: '规则资源', changed: rules.length > 0, result: `${rules.length} 个文件` },
    { id: 'web', label: 'Web 面板', changed: dashboards.length > 0, result: `${dashboards.length} 个面板` },
    { id: 'core', label: 'sing-box 内核', changed: Boolean(data.singBox), result: '1 个版本' },
    {
      id: 'npm',
      label: 'npm 依赖',
      changed: npmProjects.length > 0,
      result: `${npmProjects.length} 个项目，${npmDirectChanges} 项直接依赖、${npmLockChanges} 项锁文件变化`,
    },
    {
      id: 'android',
      label: 'Android 依赖',
      changed: androidUpdated.length > 0,
      result: androidUpdated.length > 0
        ? `${androidUpdated.length} 项自动更新${androidManual.length > 0 ? `，${androidManual.length} 项待人工` : ''}`
        : androidManual.length > 0
          ? `无自动更新，${androidManual.length} 项待人工`
          : androidDiagnostics.length > 0
            ? `${androidDiagnostics.length} 项检查警告`
            : '无更新',
      showResultWhenUnchanged: androidManual.length > 0 || androidDiagnostics.length > 0,
    },
    { id: 'actions', label: 'GitHub Actions', changed: actions.length > 0, result: `${actions.length} 个 Action` },
  ]

  const changedCategories = categories.filter((category) => category.changed)
  const npmItemCount = npmDirectChanges > 0 ? npmDirectChanges : npmProjects.length
  const itemCount = rules.length + dashboards.length + (data.singBox ? 1 : 0) + npmItemCount + androidUpdated.length + actions.length
  const titleParts = []

  if (rules.length === 1) titleParts.push(`${rules[0].name || path.basename(rules[0].path, '.srs')} 规则`)
  else if (rules.length > 1) titleParts.push(`${rules.length} 项规则资源`)
  if (dashboards.length === 1) titleParts.push(dashboards[0].name)
  else if (dashboards.length > 1) titleParts.push('Web 面板')
  if (data.singBox) titleParts.push('sing-box 内核')
  if (npmProjects.length > 0) titleParts.push('npm 依赖')
  if (androidUpdated.length > 0) titleParts.push('Android 依赖')
  if (actions.length > 0) titleParts.push('GitHub Actions')

  let title = `chore(维护): 更新 ${joinChinese(titleParts)}`
  if (titleParts.length === 0 || title.length > 72) {
    title = 'chore(维护): 更新资源、内核与依赖'
  }

  const serverUrl = context.serverUrl || 'https://github.com'
  const repository = context.repository || 'Fanju6/NetProxy-Magisk'
  const workflowUrl = `${serverUrl}/${repository}/actions/workflows/update-resources.yml`
  const lines = [
    '## 每周自动更新',
    '',
    `本 PR 由[每周更新资源、内核与依赖](${workflowUrl})工作流自动生成。`,
    '',
    changedCategories.length > 0
      ? `本次更新 **${changedCategories.length} 个分类、${itemCount} 项内容**。`
      : '本次检查未发现可提交的更新。',
    '',
    '### 更新概览',
    '',
    '| 分类 | 更新结果 |',
    '| --- | --- |',
    ...categories.map((category) => `| ${category.label} | ${category.changed || category.showResultWhenUnchanged ? category.result : '无更新'} |`),
    '',
  ]

  if (changedCategories.length > 0) {
    lines.push('### 更新详情', '')
    renderRules(lines, rules)
    if (dashboards.length > 0) {
      lines.push('#### Web 面板', '')
      renderZashboard(lines, data.zashboard)
      renderDashboard(lines, data.dashboard)
    }
    renderSingBox(lines, data.singBox)
    renderNpm(lines, npmProjects)
    renderAndroid(lines, android)
    renderActions(lines, actions)
  } else if (androidManual.length > 0 || androidDiagnostics.length > 0) {
    lines.push('### 检查详情', '')
    renderAndroid(lines, android)
  }

  if (Array.isArray(data.verification) && data.verification.length > 0) {
    lines.push('### 验证结果', '')
    for (const item of data.verification) lines.push(`- ${item}`)
    lines.push('')
  }

  lines.push('> 合并后将触发 CI 重新构建打包；请先核对 diff 是否符合预期。', '')
  return { title, body: lines.join('\n') }
}

export function loadReport(reportDir) {
  return {
    rules: readJson(reportDir, 'rules', []),
    zashboard: readJson(reportDir, 'zashboard', null),
    dashboard: readJson(reportDir, 'dashboard', null),
    singBox: readJson(reportDir, 'singBox', null),
    npm: readJson(reportDir, 'npm', []),
    android: readJson(reportDir, 'android', null),
    actions: readJson(reportDir, 'actions', []),
    verification: readJson(reportDir, 'verification', []),
  }
}

function parseArgs(argv) {
  const args = {}
  for (let index = 0; index < argv.length; index += 2) {
    const key = argv[index]
    const value = argv[index + 1]
    if (!key?.startsWith('--') || value === undefined) throw new Error(`无效参数: ${key || ''}`)
    args[key.slice(2)] = value
  }
  return args
}

function run() {
  const args = parseArgs(process.argv.slice(2))
  if (!args['report-dir'] || !args.body) {
    throw new Error('用法: render-update-report.mjs --report-dir <目录> --body <文件> [--output <GITHUB_OUTPUT>]')
  }

  const report = loadReport(args['report-dir'])
  const result = buildReport(report, {
    serverUrl: process.env.GITHUB_SERVER_URL,
    repository: process.env.GITHUB_REPOSITORY,
  })
  const bodyPath = path.resolve(args.body)
  fs.writeFileSync(bodyPath, result.body, 'utf8')

  if (args.output) {
    fs.appendFileSync(args.output, `title=${result.title}\nbody_path=${bodyPath}\n`, 'utf8')
  }

  process.stdout.write(`${result.title}\n\n${result.body}`)
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  run()
}
