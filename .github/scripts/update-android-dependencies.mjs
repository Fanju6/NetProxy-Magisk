import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const MAIN_CATALOG = 'src/android/gradle/libs.versions.toml'
const SCRIPTA_CATALOG = 'src/android/third_party/scripta/gradle/libs.versions.toml'
const GRADLE_WRAPPER = 'src/android/gradle/wrapper/gradle-wrapper.properties'

const REPOSITORIES = {
  google: 'https://dl.google.com/dl/android/maven2',
  central: 'https://repo.maven.apache.org/maven2',
  jitpack: 'https://jitpack.io',
}

const ref = (file, key) => ({ file, key })
const moduleSource = (group, artifact, repositories, url) => ({
  type: 'maven',
  group,
  artifact,
  repositories,
  url,
})

export const ANDROID_DEPENDENCIES = [
  {
    id: 'agp',
    name: 'Android Gradle Plugin',
    policy: 'manual',
    refs: [ref(MAIN_CATALOG, 'agp'), ref(SCRIPTA_CATALOG, 'agp')],
    source: moduleSource('com.android.tools.build', 'gradle', ['google'], 'https://developer.android.com/build/releases/gradle-plugin'),
  },
  {
    id: 'kotlin',
    name: 'Kotlin',
    policy: 'manual',
    refs: [ref(MAIN_CATALOG, 'kotlin'), ref(SCRIPTA_CATALOG, 'kotlin')],
    source: moduleSource('org.jetbrains.kotlin', 'kotlin-gradle-plugin', ['central'], 'https://github.com/JetBrains/kotlin/releases'),
  },
  {
    id: 'kotlinx-serialization',
    name: 'Kotlinx Serialization',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'kotlinxSerialization')],
    source: moduleSource('org.jetbrains.kotlinx', 'kotlinx-serialization-json', ['central'], 'https://github.com/Kotlin/kotlinx.serialization/releases'),
  },
  {
    id: 'activity-compose',
    name: 'AndroidX Activity Compose',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'activityCompose'), ref(SCRIPTA_CATALOG, 'androidx-activity')],
    source: moduleSource('androidx.activity', 'activity-compose', ['google'], 'https://developer.android.com/jetpack/androidx/releases/activity'),
  },
  {
    id: 'core-ktx',
    name: 'AndroidX Core KTX',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'coreKtx')],
    source: moduleSource('androidx.core', 'core-ktx', ['google'], 'https://developer.android.com/jetpack/androidx/releases/core'),
  },
  {
    id: 'lifecycle',
    name: 'AndroidX Lifecycle',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'lifecycleRuntimeKtx')],
    source: moduleSource('androidx.lifecycle', 'lifecycle-runtime-ktx', ['google'], 'https://developer.android.com/jetpack/androidx/releases/lifecycle'),
  },
  {
    id: 'navigation3',
    name: 'AndroidX Navigation3',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'navigation3')],
    source: moduleSource('androidx.navigation3', 'navigation3-runtime', ['google'], 'https://developer.android.com/jetpack/androidx/releases/navigation3'),
  },
  {
    id: 'navigation-event',
    name: 'AndroidX Navigation Event',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'navigationevent')],
    source: moduleSource('androidx.navigationevent', 'navigationevent-compose', ['google'], 'https://developer.android.com/jetpack/androidx/releases/navigationevent'),
  },
  {
    id: 'compose-bom',
    name: 'AndroidX Compose BOM',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'composeBom')],
    source: moduleSource('androidx.compose', 'compose-bom', ['google'], 'https://developer.android.com/develop/ui/compose/bom/bom-mapping'),
  },
  {
    id: 'libsu',
    name: 'libsu',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'libsu')],
    source: moduleSource('com.github.topjohnwu.libsu', 'core', ['central', 'jitpack'], 'https://github.com/topjohnwu/libsu/releases'),
  },
  {
    id: 'miuix',
    name: 'Miuix',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'miuix')],
    source: moduleSource('top.yukonga.miuix.kmp', 'miuix-ui-android', ['central'], 'https://github.com/compose-miuix-ui/miuix/releases'),
  },
  {
    id: 'hidden-api-bypass',
    name: 'HiddenApiBypass',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'hiddenapibypass')],
    source: moduleSource('org.lsposed.hiddenapibypass', 'hiddenapibypass', ['central'], 'https://github.com/LSPosed/AndroidHiddenApiBypass/releases'),
  },
  {
    id: 'json-schema-validator',
    name: 'JSON Schema Validator',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'jsonSchemaValidator')],
    source: moduleSource('com.networknt', 'json-schema-validator', ['central'], 'https://github.com/networknt/json-schema-validator/releases'),
  },
  {
    id: 'junit',
    name: 'JUnit 4',
    policy: 'auto-minor',
    refs: [ref(MAIN_CATALOG, 'junit')],
    source: moduleSource('junit', 'junit', ['central'], 'https://github.com/junit-team/junit4/releases'),
  },
  {
    id: 'compose-multiplatform',
    name: 'Compose Multiplatform',
    policy: 'manual',
    refs: [ref(SCRIPTA_CATALOG, 'compose-multiplatform')],
    source: moduleSource('org.jetbrains.compose', 'compose-gradle-plugin', ['central'], 'https://github.com/JetBrains/compose-multiplatform/releases'),
  },
  {
    id: 'gradle',
    name: 'Gradle Wrapper',
    policy: 'manual',
    refs: [],
    source: { type: 'gradle', url: 'https://gradle.org/releases/' },
  },
]

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export function parseVersion(value) {
  const match = String(value).trim().match(/^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:\.(\d+))?$/)
  if (!match) return null
  return match.slice(1).map((part) => Number(part || 0))
}

export function compareVersions(left, right) {
  const a = parseVersion(left)
  const b = parseVersion(right)
  if (!a || !b) throw new Error(`无法比较版本: ${left} / ${right}`)
  for (let index = 0; index < Math.max(a.length, b.length); index += 1) {
    const difference = (a[index] || 0) - (b[index] || 0)
    if (difference !== 0) return Math.sign(difference)
  }
  return 0
}

export function isStableVersion(value) {
  return parseVersion(value) !== null
}

export function selectAutomaticTarget(current, versions, policy) {
  if (policy !== 'auto-minor') return current
  const currentParts = parseVersion(current)
  if (!currentParts) return current

  const candidates = versions
    .filter(isStableVersion)
    .filter((version) => compareVersions(version, current) > 0)
    .filter((version) => {
      const parts = parseVersion(version)
      if (parts[0] !== currentParts[0]) return false
      // 0.x 尚未提供稳定兼容边界，只自动接受补丁更新。
      return currentParts[0] !== 0 || parts[1] === currentParts[1]
    })
    .sort(compareVersions)

  return candidates.at(-1) || current
}

export function parseCatalogVersions(content) {
  const versions = new Map()
  let inVersions = false
  for (const line of content.split(/\r?\n/)) {
    const section = line.match(/^\s*\[([^\]]+)]\s*$/)
    if (section) {
      inVersions = section[1] === 'versions'
      continue
    }
    if (!inVersions) continue
    const match = line.match(/^\s*([A-Za-z0-9_-]+)\s*=\s*"([^"]+)"/)
    if (match) versions.set(match[1], match[2])
  }
  return versions
}

export function updateCatalogVersion(content, key, version) {
  const lines = content.split(/\r?\n/)
  let inVersions = false
  let changed = false
  const keyPattern = new RegExp(`^(\\s*${escapeRegExp(key)}\\s*=\\s*)"[^"]+"(.*)$`)

  const updated = lines.map((line) => {
    const section = line.match(/^\s*\[([^\]]+)]\s*$/)
    if (section) {
      inVersions = section[1] === 'versions'
      return line
    }
    if (!inVersions) return line
    const match = line.match(keyPattern)
    if (!match) return line
    changed = true
    return `${match[1]}"${version}"${match[2]}`
  })

  if (!changed) throw new Error(`版本目录缺少键: ${key}`)
  return updated.join('\n')
}

export function parseMavenMetadata(xml) {
  return [...String(xml).matchAll(/<version>([^<]+)<\/version>/g)]
    .map((match) => match[1].trim())
    .filter(Boolean)
}

function latestStable(versions) {
  return versions.filter(isStableVersion).sort(compareVersions).at(-1) || ''
}

async function fetchWithRetry(url, fetchImpl) {
  let lastError
  for (let attempt = 1; attempt <= 2; attempt += 1) {
    try {
      const response = await fetchImpl(url, {
        headers: { 'user-agent': 'NetProxy dependency updater' },
        signal: AbortSignal.timeout(20_000),
      })
      if (response.ok) return response
      lastError = new Error(`${response.status} ${response.statusText}`)
      if (response.status === 404) break
    } catch (error) {
      lastError = error
    }
  }
  throw new Error(`${url}: ${lastError?.message || '请求失败'}`)
}

async function fetchSourceVersions(source, fetchImpl) {
  if (source.type === 'gradle') {
    const response = await fetchWithRetry('https://services.gradle.org/versions/all', fetchImpl)
    const releases = await response.json()
    return releases
      .filter((release) => !release.snapshot && !release.nightly && !release.releaseNightly && !release.broken)
      .map((release) => release.version)
  }

  const relative = `${source.group.replaceAll('.', '/')}/${source.artifact}/maven-metadata.xml`
  const versions = new Set()
  const errors = []
  for (const repository of source.repositories) {
    const url = `${REPOSITORIES[repository]}/${relative}`
    try {
      const response = await fetchWithRetry(url, fetchImpl)
      for (const version of parseMavenMetadata(await response.text())) versions.add(version)
    } catch (error) {
      errors.push(error.message)
    }
  }
  if (versions.size === 0) throw new Error(errors.join('；') || 'Maven 元数据为空')
  return [...versions]
}

function readGradleWrapperVersion(rootDir) {
  const content = fs.readFileSync(path.join(rootDir, GRADLE_WRAPPER), 'utf8')
  const match = content.match(/gradle-([0-9.]+)-(?:bin|all)\.zip/)
  if (!match) throw new Error('无法读取 Gradle Wrapper 版本')
  return match[1]
}

function atomicWrite(file, content) {
  const temporary = `${file}.tmp-${process.pid}`
  fs.writeFileSync(temporary, content, 'utf8')
  fs.renameSync(temporary, file)
}

export async function updateAndroidDependencies({
  rootDir = process.cwd(),
  fetchImpl = globalThis.fetch,
  write = true,
} = {}) {
  const catalogContents = new Map([
    [MAIN_CATALOG, fs.readFileSync(path.join(rootDir, MAIN_CATALOG), 'utf8').replace(/\r\n/g, '\n')],
    [SCRIPTA_CATALOG, fs.readFileSync(path.join(rootDir, SCRIPTA_CATALOG), 'utf8').replace(/\r\n/g, '\n')],
  ])
  const catalogVersions = new Map(
    [...catalogContents].map(([file, content]) => [file, parseCatalogVersions(content)]),
  )
  const report = { checked: ANDROID_DEPENDENCIES.length, updated: [], manual: [], diagnostics: [] }

  await Promise.all(ANDROID_DEPENDENCIES.map(async (dependency) => {
    let current
    try {
      if (dependency.source.type === 'gradle') {
        current = readGradleWrapperVersion(rootDir)
      } else {
        const values = dependency.refs.map(({ file, key }) => {
          const value = catalogVersions.get(file)?.get(key)
          if (!value) throw new Error(`${file} 缺少版本键 ${key}`)
          return value
        })
        if (new Set(values).size !== 1) {
          throw new Error(`重复版本未同步: ${values.join(' / ')}`)
        }
        current = values[0]
      }

      const versions = await fetchSourceVersions(dependency.source, fetchImpl)
      const latest = latestStable(versions)
      if (!latest || compareVersions(latest, current) <= 0) return

      const target = selectAutomaticTarget(current, versions, dependency.policy)
      if (compareVersions(target, current) > 0) {
        for (const { file, key } of dependency.refs) {
          const content = updateCatalogVersion(catalogContents.get(file), key, target)
          catalogContents.set(file, content)
          catalogVersions.get(file).set(key, target)
        }
        report.updated.push({
          id: dependency.id,
          name: dependency.name,
          from: current,
          to: target,
          latest,
          files: dependency.refs.map(({ file }) => file),
          sourceUrl: dependency.source.url,
        })
      }

      if (compareVersions(latest, target) > 0) {
        report.manual.push({
          id: dependency.id,
          name: dependency.name,
          current: target,
          latest,
          reason: dependency.policy === 'manual' ? '工具链升级需人工验证' : '超出自动更新兼容范围',
          sourceUrl: dependency.source.url,
        })
      }
    } catch (error) {
      report.diagnostics.push({ id: dependency.id, name: dependency.name, message: error.message })
    }
  }))

  report.updated.sort((a, b) => a.name.localeCompare(b.name, 'en'))
  report.manual.sort((a, b) => a.name.localeCompare(b.name, 'en'))
  report.diagnostics.sort((a, b) => a.name.localeCompare(b.name, 'en'))

  if (write && report.updated.length > 0) {
    for (const [file, content] of catalogContents) {
      atomicWrite(path.join(rootDir, file), `${content.replace(/\n*$/, '')}\n`)
    }
  }
  return report
}

function parseArgs(argv) {
  const args = { write: true }
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index]
    if (item === '--dry-run') {
      args.write = false
      continue
    }
    if (!['--root', '--report', '--output'].includes(item) || !argv[index + 1]) {
      throw new Error(`无效参数: ${item}`)
    }
    args[item.slice(2)] = argv[index + 1]
    index += 1
  }
  return args
}

async function run() {
  const args = parseArgs(process.argv.slice(2))
  const rootDir = path.resolve(args.root || process.cwd())
  const report = await updateAndroidDependencies({ rootDir, write: args.write })
  const serialized = `${JSON.stringify(report, null, 2)}\n`
  if (args.report) fs.writeFileSync(path.resolve(args.report), serialized, 'utf8')
  if (args.output) {
    fs.appendFileSync(path.resolve(args.output), `changed=${report.updated.length > 0}\n`, 'utf8')
  }

  for (const diagnostic of report.diagnostics) {
    console.warn(`::warning title=Android 依赖检查失败::${diagnostic.name}: ${diagnostic.message}`)
  }
  console.log(`Android 依赖: 检查 ${report.checked} 项，更新 ${report.updated.length} 项，待人工 ${report.manual.length} 项`)
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  run().catch((error) => {
    console.error(error)
    process.exitCode = 1
  })
}
