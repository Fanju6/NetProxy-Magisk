package top.yukonga.scripta.editor.text

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** [BracketMatcher]：光标邻接括号的配对查找（纯字符扫描、同字符对独立计数、预算上限）。 */
class BracketMatcherTest {

    private fun buf(text: String) = TextBuffer(text)

    private fun pair(text: String, line: Int, col: Int, budget: Int = 20_000): BracketPair? =
        BracketMatcher.findMatch(buf(text), TextPosition(line, col), budget)

    @Test
    fun caretRightOfOpenBracketMatchesForward() {
        // "(" 后紧跟光标……锚定优先级：左侧是开括号 → 也匹配。先测右侧开括号：光标在 "(" 前。
        assertEquals(BracketPair(TextPosition(0, 0), TextPosition(0, 4)), pair("(abc)", 0, 0))
    }

    @Test
    fun caretLeftOfCloseBracketMatchesBackward() {
        // 光标在 ")" 后（刚打完闭合）：左闭优先。
        assertEquals(BracketPair(TextPosition(0, 0), TextPosition(0, 4)), pair("(abc)", 0, 5))
    }

    @Test
    fun leftCloserWinsOverRightOpener() {
        // ")|(": 左侧闭括号与右侧开括号同时邻接 → 左闭优先（刚输入的括号）。
        assertEquals(BracketPair(TextPosition(0, 0), TextPosition(0, 1)), pair("()(x)", 0, 2))
    }

    @Test
    fun nestingOfSameTypeIsBalanced() {
        // "( ( ) )"：外层开括号匹配外层闭括号（"((x)y)z" 里 index 5 的 `)`）。
        assertEquals(BracketPair(TextPosition(0, 0), TextPosition(0, 5)), pair("((x)y)z", 0, 0))
    }

    @Test
    fun differentBracketTypesAreCountedIndependently() {
        // "([)]" 病态交错：( 的配对按同字符计数找到 )，忽略中间的 [。
        assertEquals(BracketPair(TextPosition(0, 0), TextPosition(0, 2)), pair("([)]", 0, 0))
    }

    @Test
    fun matchSpansLines() {
        assertEquals(BracketPair(TextPosition(0, 4), TextPosition(2, 0)), pair("key:{\n  a: 1\n}", 0, 4))
    }

    @Test
    fun unbalancedReturnsNull() {
        assertNull(pair("(abc", 0, 0))
        assertNull(pair("abc)", 0, 4))
    }

    @Test
    fun caretNotAdjacentToBracketReturnsNull() {
        assertNull(pair("(abc)", 0, 2))
        assertNull(pair("plain", 0, 3))
    }

    @Test
    fun quotesAreNotMatched() {
        // 引号配对无方向、歧义大：不做匹配高亮。
        assertNull(pair("\"ab\"", 0, 0))
    }

    @Test
    fun curlyAndSquareWorkToo() {
        assertEquals(BracketPair(TextPosition(0, 0), TextPosition(0, 3)), pair("{ab}", 0, 0))
        assertEquals(BracketPair(TextPosition(0, 0), TextPosition(0, 3)), pair("[ab]", 0, 4))
    }

    @Test
    fun scanBudgetCapsTheSearch() {
        val big = "(" + "x".repeat(500) + ")"
        assertEquals(BracketPair(TextPosition(0, 0), TextPosition(0, 501)), pair(big, 0, 0))
        assertNull(pair(big, 0, 0, budget = 100)) // 预算内找不到 → 放弃（大文档不全文扫）
    }

    @Test
    fun openerAtLeftSideAlsoAnchors() {
        // 光标在 "(" 后（左开）：无左闭无右开时锚左开。
        assertEquals(BracketPair(TextPosition(0, 0), TextPosition(0, 4)), pair("(abc)", 0, 1))
    }

    @Test
    fun closerAtRightSideAlsoAnchors() {
        // 光标在 ")" 前（右闭）：最低优先级锚。
        assertEquals(BracketPair(TextPosition(0, 0), TextPosition(0, 4)), pair("(abc)", 0, 4))
    }
}
