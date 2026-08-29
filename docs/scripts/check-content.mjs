import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'

const docsRoot = resolve(import.meta.dirname, '..')
const historyAllowlist = new Set([
  'changelog.md',
  'guide/compare-v7-v8.md',
  'guide/upgrade-v7.md'
])
const bannedCurrentPhrases = [
  /Android 8\.0/,
  /NetProxy 8\.0/,
  /当前 8\.0/,
  /测试阶段/,
  /测试版文档/,
  /以 reF1nd[^。\n]*为核心/
]

function walk(directory) {
  return readdirSync(directory).flatMap((name) => {
    const path = join(directory, name)
    if (name === 'node_modules' || name === '.vitepress') return []
    return statSync(path).isDirectory() ? walk(path) : [path]
  })
}

function routeExists(target, sourceFile) {
  const clean = decodeURIComponent(target.split(/[?#]/, 1)[0])
  if (!clean) return true

  if (clean.startsWith('/')) {
    const path = clean.slice(1)
    if (!path) return existsSync(join(docsRoot, 'index.md'))
    if (extname(path)) return existsSync(join(docsRoot, path)) || existsSync(join(docsRoot, 'public', path))
    return existsSync(join(docsRoot, `${path}.md`)) || existsSync(join(docsRoot, path, 'index.md'))
  }

  const resolved = resolve(dirname(sourceFile), clean)
  if (extname(resolved)) return existsSync(resolved)
  return existsSync(`${resolved}.md`) || existsSync(join(resolved, 'index.md'))
}

const markdownFiles = walk(docsRoot).filter((file) => file.endsWith('.md'))
const failures = []

for (const file of markdownFiles) {
  const name = relative(docsRoot, file).replaceAll('\\', '/')
  const content = readFileSync(file, 'utf8')

  if (!historyAllowlist.has(name)) {
    for (const pattern of bannedCurrentPhrases) {
      if (pattern.test(content)) failures.push(`${name}: 当前文档包含版本绑定文案 ${pattern}`)
    }
  }

  for (const match of content.matchAll(/\[[^\]]*\]\(([^)]+)\)/g)) {
    const rawTarget = match[1].trim().replace(/^<|>$/g, '')
    if (/^(https?:|mailto:|tel:)/i.test(rawTarget) || rawTarget.startsWith('#')) continue
    const target = rawTarget.split(/\s+["']/)[0]
    if (!routeExists(target, file)) failures.push(`${name}: 本地链接不存在 ${target}`)
  }
}

if (failures.length > 0) {
  console.error(failures.join('\n'))
  process.exit(1)
}

console.log(`文档内容检查通过：${markdownFiles.length} 个 Markdown 文件。`)
