package top.yukonga.scripta.editor.text

import kotlin.random.Random
import kotlin.test.Test
import kotlin.test.assertEquals

class PieceTreeTest {

    // --- 构建 / 只读 ---

    @Test
    fun emptyTreeIsOneEmptyLine() {
        val t = PieceTree("")
        assertEquals(0, t.length)
        assertEquals(1, t.lineCount)
        assertEquals("", t.getText())
        assertEquals("", t.lineContent(0))
        assertEquals(0, t.lineLength(0))
        assertEquals(0, t.offsetAt(TextPosition(0, 0)))
        assertEquals(TextPosition(0, 0), t.positionAt(0))
    }

    @Test
    fun singleLineNoNewline() {
        val t = PieceTree("hello")
        assertEquals(5, t.length)
        assertEquals(1, t.lineCount)
        assertEquals("hello", t.getText())
        assertEquals("hello", t.lineContent(0))
        assertEquals(5, t.lineLength(0))
    }

    @Test
    fun multiLineContentAndLengths() {
        val t = PieceTree("ab\ncde\n\nf")
        assertEquals(4, t.lineCount)
        assertEquals("ab", t.lineContent(0))
        assertEquals("cde", t.lineContent(1))
        assertEquals("", t.lineContent(2))
        assertEquals("f", t.lineContent(3))
        assertEquals(2, t.lineLength(0))
        assertEquals(3, t.lineLength(1))
        assertEquals(0, t.lineLength(2))
        assertEquals(1, t.lineLength(3))
    }

    @Test
    fun trailingNewlineGivesEmptyLastLine() {
        val t = PieceTree("a\n")
        assertEquals(2, t.lineCount)
        assertEquals("a", t.lineContent(0))
        assertEquals("", t.lineContent(1))
    }

    @Test
    fun substringSpansAndClamps() {
        val t = PieceTree("0123456789")
        assertEquals("234", t.substring(2, 3))
        assertEquals("", t.substring(2, 0))
        assertEquals("89", t.substring(8, 100)) // 末端钳制
        assertEquals("", t.substring(100, 5))
    }

    @Test
    fun offsetPositionRoundTrip() {
        val t = PieceTree("ab\ncde\nf")
        // (行,列) -> offset
        assertEquals(0, t.offsetAt(TextPosition(0, 0)))
        assertEquals(3, t.offsetAt(TextPosition(1, 0)))
        assertEquals(6, t.offsetAt(TextPosition(1, 3)))
        assertEquals(7, t.offsetAt(TextPosition(2, 0)))
        // offset -> (行,列)
        assertEquals(TextPosition(0, 2), t.positionAt(2))
        assertEquals(TextPosition(1, 0), t.positionAt(3))
        assertEquals(TextPosition(2, 0), t.positionAt(7))
    }

    // --- 编辑 ---

    @Test
    fun insertAtVariousPositions() {
        val t = PieceTree("hello")
        t.insert(0, ">>")
        assertEquals(">>hello", t.getText())
        t.insert(t.length, "<<")
        assertEquals(">>hello<<", t.getText())
        t.insert(4, "__")
        assertEquals(">>he__llo<<", t.getText())
    }

    @Test
    fun insertNewlinesUpdatesLineCount() {
        val t = PieceTree("ab")
        t.insert(1, "X\nY\nZ")
        assertEquals("aX\nY\nZb", t.getText())
        assertEquals(3, t.lineCount)
        assertEquals("aX", t.lineContent(0))
        assertEquals("Y", t.lineContent(1))
        assertEquals("Zb", t.lineContent(2))
    }

    @Test
    fun deleteWithinAndAcrossLines() {
        val t = PieceTree("hello\nworld\n!")
        t.delete(2, 2) // 删 "ll"
        assertEquals("heo\nworld\n!", t.getText())
        t.delete(3, 1) // 删跨行的 '\n' -> 合并
        assertEquals("heoworld\n!", t.getText())
        assertEquals(2, t.lineCount)
    }

    @Test
    fun deleteEntireDocument() {
        val t = PieceTree("abc\ndef")
        t.delete(0, t.length)
        assertEquals("", t.getText())
        assertEquals(1, t.lineCount)
        assertEquals(0, t.length)
    }

    @Test
    fun interleavedEditsStayConsistent() {
        val t = PieceTree("")
        t.insert(0, "one\ntwo\nthree")
        t.insert(4, "1.5\n")
        assertEquals("one\n1.5\ntwo\nthree", t.getText())
        t.delete(0, 4) // 删 "one\n"
        assertEquals("1.5\ntwo\nthree", t.getText())
        assertEquals(3, t.lineCount)
        assertEquals("1.5", t.lineContent(0))
    }

    @Test
    fun surrogatePairSurvivesRoundTrip() {
        val t = PieceTree("a😀b") // 😀 = 一个 surrogate pair（2 个 UTF-16 unit）
        assertEquals(4, t.length)
        t.insert(1, "X")
        assertEquals("aX😀b", t.getText())
        t.delete(0, 1)
        assertEquals("X😀b", t.getText())
    }

    // --- fuzz 对照朴素 StringBuilder（piece-table 正确性黄金测试） ---

    @Test
    fun fuzzAgainstNaiveReference() {
        val rng = Random(0xF0F0F0)
        val t = PieceTree("")
        val ref = StringBuilder()
        val alphabet = "abc\nde\nZ" // 含换行，制造多行/合并
        repeat(4000) { step ->
            val len = ref.length
            when (rng.nextInt(3)) {
                0, 2 -> { // insert
                    val off = rng.nextInt(len + 1)
                    val s = buildString { repeat(rng.nextInt(1, 7)) { append(alphabet[rng.nextInt(alphabet.length)]) } }
                    t.insert(off, s); ref.insert(off, s)
                }

                1 -> if (len > 0) { // delete
                    val off = rng.nextInt(len)
                    val n = rng.nextInt(1, minOf(8, len - off) + 1)
                    t.delete(off, n); ref.delete(off, off + n)
                }
            }
            if (step % 25 == 0 || step > 3950) {
                val expected = ref.toString()
                assertEquals(expected, t.getText())
                assertEquals(expected.count { it == '\n' } + 1, t.lineCount)
                val line = rng.nextInt(t.lineCount)
                assertEquals(refLineContent(expected, line), t.lineContent(line))
                if (expected.isNotEmpty()) {
                    val o = rng.nextInt(expected.length + 1)
                    val p = t.positionAt(o)
                    assertEquals(refPositionAt(expected, o), p)
                    assertEquals(o, t.offsetAt(p)) // 往返
                }
            }
        }
        assertEquals(ref.toString(), t.getText())
    }

    private fun refLineContent(s: String, line: Int): String = s.split("\n")[line]

    private fun refPositionAt(s: String, off: Int): TextPosition {
        var line = 0
        var lastNl = -1
        for (i in 0 until off) if (s[i] == '\n') {
            line++; lastNl = i
        }
        return TextPosition(line, off - (lastNl + 1))
    }
}
