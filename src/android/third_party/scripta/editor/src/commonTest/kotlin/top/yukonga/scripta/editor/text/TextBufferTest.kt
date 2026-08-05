package top.yukonga.scripta.editor.text

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotSame
import kotlin.test.assertSame

class TextBufferTest {
    @Test
    fun splitsInitialTextIntoLines() {
        val b = TextBuffer("a\nbb\nccc")
        assertEquals(3, b.lineCount)
        assertEquals("bb", b.lineText(1))
        assertEquals(3, b.lineLength(2))
        assertEquals(TextPosition(2, 3), b.endPosition())
    }

    @Test
    fun emptyBufferIsOneEmptyLine() {
        val b = TextBuffer("")
        assertEquals(1, b.lineCount)
        assertEquals("", b.lineText(0))
        assertEquals(TextPosition(0, 0), b.endPosition())
    }

    @Test
    fun replaceWithinOneLine() {
        val b = TextBuffer("hello world")
        val caret = b.replace(TextRange(TextPosition(0, 0), TextPosition(0, 5)), "HI")
        assertEquals("HI world", b.lineText(0))
        assertEquals(TextPosition(0, 2), caret)
    }

    @Test
    fun insertingNewlineSplitsLine() {
        val b = TextBuffer("abcd")
        val caret = b.replace(TextRange.cursor(TextPosition(0, 2)), "\n")
        assertEquals(2, b.lineCount)
        assertEquals("ab", b.lineText(0))
        assertEquals("cd", b.lineText(1))
        assertEquals(TextPosition(1, 0), caret)
    }

    @Test
    fun replaceAcrossLinesJoins() {
        val b = TextBuffer("line1\nline2\nline3")
        val caret = b.replace(TextRange(TextPosition(0, 2), TextPosition(2, 2)), "X")
        assertEquals(1, b.lineCount)
        assertEquals("liXne3", b.lineText(0))
        assertEquals(TextPosition(0, 3), caret)
    }

    @Test
    fun textInRangeSpansNewlines() {
        val b = TextBuffer("ab\ncd\nef")
        assertEquals("b\ncd\ne", b.textInRange(TextRange(TextPosition(0, 1), TextPosition(2, 1))))
    }

    @Test
    fun textJoinsWithNewline() {
        val b = TextBuffer("x\ny")
        assertEquals("x\ny", b.text())
    }

    @Test
    fun textIsCachedUntilEdited() {
        // 小文档未编辑时复用同一构建结果，省 getText / 相等比较等低频路径的 O(n) 重建。
        val b = TextBuffer("a\nb\nc")
        val first = b.text()
        assertEquals("a\nb\nc", first)
        assertSame(first, b.text()) // 未编辑：命中缓存，返回同一实例

        b.replace(TextRange.cursor(TextPosition(0, 1)), "X") // 编辑使缓存失效
        val afterEdit = b.text()
        assertNotSame(first, afterEdit)
        assertEquals("aX\nb\nc", afterEdit)

        b.setText("hello") // setText 同样失效
        assertEquals("hello", b.text())
    }

    @Test
    fun largeTextNotCachedToAvoidResidentCopy() {
        // 超过缓存上限的大文档不常驻整篇：每次 text() 重建，避免多留一份 N（IME 已窗口化、不再高频拉全篇）。
        val big = "a".repeat(1_100_000) // > CACHE_TEXT_LIMIT (1_000_000)
        val b = TextBuffer(big)
        val first = b.text()
        assertEquals(big.length, first.length)
        assertNotSame(first, b.text()) // 未缓存：两次不同实例
    }

    @Test
    fun versionBumpsOnEdit() {
        val b = TextBuffer("a")
        val v0 = b.version
        b.replace(TextRange.cursor(b.endPosition()), "b")
        assertEquals(v0 + 1, b.version)
    }

    @Test
    fun replaceNormalizesCrlf() {
        // 粘贴/提交 Windows 文本走 replace->splitToLines，应统一换行、不留裸 CR。
        val b = TextBuffer("")
        b.replace(TextRange.cursor(TextPosition(0, 0)), "a\r\nb\rc")
        assertEquals(3, b.lineCount)
        assertEquals("a", b.lineText(0))
        assertEquals("b", b.lineText(1))
        assertEquals("c", b.lineText(2))
        assertEquals("a\nb\nc", b.text())
    }

    @Test
    fun setTextNormalizesCrlf() {
        val b = TextBuffer("x")
        b.setText("p\r\nq")
        assertEquals(2, b.lineCount)
        assertEquals("p", b.lineText(0))
        assertEquals("q", b.lineText(1))
    }
}
