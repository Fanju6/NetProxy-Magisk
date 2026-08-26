#!/usr/bin/env sh
# 文件: tests/release_notes_test.sh
# 功能: 检查 GitHub Release 仅使用目标版本的更新日志。
# 用法: sh tests/release_notes_test.sh
# 依赖: POSIX sh、Node.js、grep

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
SCRIPT="$ROOT/.github/scripts/extract-release-notes.mjs"
CHANGELOG="$ROOT/docs/changelog.md"
TMP_DIR="$ROOT/.tmp/release-notes-test.$$"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT HUP INT TERM
mkdir -p "$TMP_DIR"

node "$SCRIPT" "$CHANGELOG" "v8.0.0" "$TMP_DIR/8.0.0.md"
grep -Fq '## 版本 8.0.0' "$TMP_DIR/8.0.0.md"
if grep -Fq '## 版本 7.2.0' "$TMP_DIR/8.0.0.md"; then
  printf '%s\n' '8.0.0 Release 日志混入了旧版本' >&2
  exit 1
fi

node "$SCRIPT" "$CHANGELOG" "4.0.1" "$TMP_DIR/4.0.1.md"
grep -Fq '## ⚠️ 重要提示（4.0.1）' "$TMP_DIR/4.0.1.md"
if grep -Fq '## 版本 4.0.0' "$TMP_DIR/4.0.1.md"; then
  printf '%s\n' '4.0.1 Release 日志越过了下一个正式版本标题' >&2
  exit 1
fi

node "$SCRIPT" "$CHANGELOG" "4.0.2" "$TMP_DIR/4.0.2.md"
grep -Fq '4.0.2 元旦特别版' "$TMP_DIR/4.0.2.md"
if grep -Fq '## 版本 4.0.1' "$TMP_DIR/4.0.2.md"; then
  printf '%s\n' '特殊版本标题未被正确截断' >&2
  exit 1
fi

if node "$SCRIPT" "$CHANGELOG" "99.0.0" "$TMP_DIR/missing.md" >/dev/null 2>&1; then
  printf '%s\n' '不存在的版本不应生成 Release 日志' >&2
  exit 1
fi

printf '%s\n' 'release notes test passed'
