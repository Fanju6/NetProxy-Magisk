import { readFile, writeFile } from "node:fs/promises"

const versionPattern = String.raw`\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?`
const releaseHeadingPatterns = [
  new RegExp(`^##\\s+版本\\s*v?(${versionPattern})(?=\\s|[（(]|$)`, "u"),
  new RegExp(`^##\\s+v(${versionPattern})(?=\\s|[（(]|$)`, "u"),
  new RegExp(`^##\\s+🎉\\s+v?(${versionPattern})(?=\\s|[（(]|$)`, "u"),
]

function normalizeVersion(value) {
  const match = value.trim().match(new RegExp(`^v?(${versionPattern})$`, "u"))
  if (!match) {
    throw new Error(`无效版本号: ${value}`)
  }
  return match[1]
}

function releaseVersion(line) {
  for (const pattern of releaseHeadingPatterns) {
    const match = line.match(pattern)
    if (match) {
      return match[1]
    }
  }
  return null
}

function extractReleaseNotes(markdown, rawVersion) {
  const version = normalizeVersion(rawVersion)
  const lines = markdown.replaceAll("\r\n", "\n").split("\n")
  const matchingHeadings = []

  for (let index = 0; index < lines.length; index += 1) {
    if (releaseVersion(lines[index]) === version) {
      matchingHeadings.push(index)
    }
  }

  if (matchingHeadings.length === 0) {
    throw new Error(`更新日志中找不到版本 ${version}`)
  }
  if (matchingHeadings.length > 1) {
    throw new Error(`更新日志中存在重复版本 ${version}`)
  }

  const start = matchingHeadings[0]
  let end = lines.length
  for (let index = start + 1; index < lines.length; index += 1) {
    if (releaseVersion(lines[index]) !== null) {
      end = index
      break
    }
  }

  const notes = lines.slice(start, end).join("\n").trim()
  if (notes.length === 0) {
    throw new Error(`版本 ${version} 的更新日志为空`)
  }
  return `${notes}\n`
}

async function main() {
  const [, , changelogPath, version, outputPath] = process.argv
  if (!changelogPath || !version || !outputPath) {
    throw new Error("用法: node extract-release-notes.mjs <changelog> <version> <output>")
  }

  const markdown = await readFile(changelogPath, "utf8")
  const notes = extractReleaseNotes(markdown, version)
  await writeFile(outputPath, notes, "utf8")
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error))
  process.exitCode = 1
})
