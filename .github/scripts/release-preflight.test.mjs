import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

import { readReleaseHistory, validateRelease } from "./release-preflight.mjs"

const changelog = `# 更新日志

## 版本 8.2.3

本次更新。

## 版本 8.2.2

上一版更新。
`

test("发布预检接受匹配的 tag、模块版本和更新日志", () => {
  const result = validateRelease({
    tag: "v8.2.3",
    moduleProp: "id=netproxy\nversion=v8.2.3\n",
    changelog,
    versionCode: 101,
    previousVersionCode: 100,
  })

  assert.equal(result.version, "8.2.3")
  assert.match(result.notes, /本次更新/)
  assert.doesNotMatch(result.notes, /上一版更新/)
})

test("发布预检使用打包提交数而不是源码占位 versionCode", () => {
  const result = validateRelease({
    tag: "v8.2.3", moduleProp: "version=v8.2.3\nversionCode=8\n", changelog,
    versionCode: 1115, previousVersionCode: 1111, previousTag: "v8.2.2",
  })
  assert.equal(result.versionCode, 1115)
})

test("发布预检拒绝等于或小于上一 tag 的实际构建号并给出修复建议", () => {
  for (const versionCode of [1111, 1110, 8]) {
    assert.throws(() => validateRelease({
      tag: "v8.2.3", moduleProp: "version=v8.2.3\nversionCode=8\n", changelog,
      versionCode, previousVersionCode: 1111, previousTag: "v8.2.2",
    }), /versionCode.*v8\.2\.2.*1111.*新增提交/u)
  }
})

test("发布预检 checkout 获取完整历史和标签", () => {
  const workflow = readFileSync(new URL("../workflows/release.yml", import.meta.url), "utf8")
  const preflight = workflow.split("  build:")[0]
  assert.match(preflight, /fetch-depth: 0/u)
})

test("发布历史忽略当前 tag，并比较所有已合并发布的最大构建号", () => {
  const replies = new Map([
    ["rev-parse --is-shallow-repository", "false"],
    ["rev-list --count HEAD", "1201"],
    ["tag --merged HEAD --list v[0-9]*", "v8.2.3\nv8.2.2\nv8.2.1\nv8.2.3-beta.1\nv-invalid"],
    ["rev-list --count v8.2.2", "1100"],
    ["rev-list --count v8.2.1", "1099"],
    ["rev-list --count v8.2.3-beta.1", "1200"],
  ])
  const history = readReleaseHistory("v8.2.3", (args) => {
    const key = args.join(" ")
    assert.ok(replies.has(key), `Unexpected Git command: ${key}`)
    return replies.get(key)
  })
  assert.deepEqual(history, { versionCode: 1201, previousVersionCode: 1200, previousTag: "v8.2.3-beta.1" })
})

test("首次发布没有上一 tag；浅克隆明确拒绝", () => {
  const history = readReleaseHistory("v8.2.3", (args) => {
    if (args[0] === "rev-parse") return "false"
    if (args[0] === "rev-list") return "1"
    assert.equal(args[0], "tag")
    return ""
  })
  assert.equal(history.previousVersionCode, 0)
  assert.equal(history.versionCode, 1)
  assert.throws(() => readReleaseHistory("v8.2.3", () => "true"), /fetch-depth: 0/u)
})

test("发布预检拒绝不带 v 前缀的 tag", () => {
  assert.throws(() => validateRelease({
    tag: "8.2.3",
    moduleProp: "version=v8.2.3\n",
    changelog,
  }), /发布 tag 必须为/)
})

test("发布预检拒绝模块版本和 tag 不一致", () => {
  assert.throws(() => validateRelease({
    tag: "v8.2.3",
    moduleProp: "version=v8.2.2\n",
    changelog,
  }), /不一致/)
})

test("发布预检拒绝缺少对应更新日志的 tag", () => {
  assert.throws(() => validateRelease({
    tag: "v8.2.4",
    moduleProp: "version=v8.2.4\n",
    changelog,
  }), /找不到版本/)
})

test("发布预检拒绝只有标题或空白正文的更新日志", () => {
  for (const body of ["", "\n \t\n"]) {
    assert.throws(() => validateRelease({
      tag: "v8.2.3",
      moduleProp: "version=v8.2.3\n",
      changelog: `## 版本 8.2.3\n${body}\n## 版本 8.2.2\n旧版说明`,
    }), /更新日志为空/)
  }
})
