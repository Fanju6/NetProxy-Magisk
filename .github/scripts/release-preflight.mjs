import { execFileSync } from "node:child_process"
import { readFile } from "node:fs/promises"
import { pathToFileURL } from "node:url"

import { extractReleaseNotes, normalizeVersion } from "./extract-release-notes.mjs"

function readModuleVersion(content) {
  const match = content.match(/^version=(.+)$/mu)
  if (!match) throw new Error("module.prop 缺少 version=")
  return match[1].trim()
}

export function readReleaseHistory(tag, git = (args) => execFileSync("git", args, { encoding: "utf8" }).trim()) {
  if (git(["rev-parse", "--is-shallow-repository"]) !== "false") {
    throw new Error("[release/preflight] 需要完整 Git 历史与标签；checkout 请设置 fetch-depth: 0")
  }
  const versionCode = Number(git(["rev-list", "--count", "HEAD"]))
  let previousVersionCode = 0
  let previousTag = "(首个发布)"
  const tags = git(["tag", "--merged", "HEAD", "--list", "v[0-9]*"]).split(/\r?\n/u)
  // Use the greatest shipped code in reachable tag history, including merged branches.
  for (const candidate of tags.filter((value) => value !== tag && /^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/u.test(value))) {
    const count = Number(git(["rev-list", "--count", candidate]))
    if (count > previousVersionCode) {
      previousVersionCode = count
      previousTag = candidate
    }
  }
  return { versionCode, previousVersionCode, previousTag }
}

export function validateRelease({ tag, moduleProp, changelog, versionCode, previousVersionCode, previousTag }) {
  if (!/^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/u.test(tag ?? "")) {
    throw new Error(`发布 tag 必须为 v<版本号>：${tag || "(空)"}`)
  }

  const version = normalizeVersion(tag)
  const moduleVersion = readModuleVersion(moduleProp)
  if (moduleVersion !== `v${version}`) {
    throw new Error(`模块版本 ${moduleVersion} 与发布 tag ${tag} 不一致`)
  }

  const notes = extractReleaseNotes(changelog, version)
  if (!(versionCode > previousVersionCode)) {
    throw new Error(`[release/preflight] versionCode ${versionCode} 必须大于上一 tag ${previousTag} 的 ${previousVersionCode}；请在完整发布历史上新增提交，不要复用旧提交或重写历史`)
  }
  return { version, notes, versionCode }
}

async function main() {
  const [, , modulePropPath, changelogPath, tag] = process.argv
  if (!modulePropPath || !changelogPath || !tag) {
    throw new Error("用法: node release-preflight.mjs <module.prop> <changelog> <tag>")
  }

  const [moduleProp, changelog] = await Promise.all([
    readFile(modulePropPath, "utf8"),
    readFile(changelogPath, "utf8"),
  ])
  const result = validateRelease({ tag, moduleProp, changelog, ...readReleaseHistory(tag) })
  console.log(`发布预检通过: v${result.version} (versionCode=${result.versionCode})`)
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => {
    console.error(error)
    process.exitCode = 1
  })
}
