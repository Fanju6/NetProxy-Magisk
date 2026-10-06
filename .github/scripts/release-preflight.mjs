import { readFile } from "node:fs/promises"
import { pathToFileURL } from "node:url"

import { extractReleaseNotes, normalizeVersion } from "./extract-release-notes.mjs"

function readModuleVersion(content) {
  const match = content.match(/^version=(.+)$/mu)
  if (!match) throw new Error("module.prop 缺少 version=")
  return match[1].trim()
}

export function validateRelease({ tag, moduleProp, changelog }) {
  if (!/^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/u.test(tag ?? "")) {
    throw new Error(`发布 tag 必须为 v<版本号>：${tag || "(空)"}`)
  }

  const version = normalizeVersion(tag)
  const moduleVersion = readModuleVersion(moduleProp)
  if (moduleVersion !== `v${version}`) {
    throw new Error(`模块版本 ${moduleVersion} 与发布 tag ${tag} 不一致`)
  }

  const notes = extractReleaseNotes(changelog, version)
  return { version, notes }
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
  const result = validateRelease({ tag, moduleProp, changelog })
  console.log(`发布预检通过: v${result.version}`)
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : String(error))
    process.exitCode = 1
  })
}
