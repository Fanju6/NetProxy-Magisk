package top.yukonga.scripta.editor.highlight

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** 网格长行接高亮的三块地基：span 列窗口裁剪、仅取颜色的样式落地、超长行分词上限。 */
class GridHighlightTest {

    // --- clipSpansToWindow -----------------------------------------------------------------

    @Test
    fun clipKeepsShiftsAndDropsSpans() {
        val spans = listOf(
            HighlightSpan(0, 10, TokenType.Key),      // 全在窗口左外
            HighlightSpan(10, 40, TokenType.String),  // 跨左边界
            HighlightSpan(50, 60, TokenType.Number),  // 全在窗口内
            HighlightSpan(90, 130, TokenType.Comment), // 跨右边界
            HighlightSpan(150, 160, TokenType.Boolean), // 全在窗口右外
        )
        val out = clipSpansToWindow(spans, 32, 128)
        assertEquals(
            listOf(
                HighlightSpan(0, 8, TokenType.String),   // 40-32=8
                HighlightSpan(18, 28, TokenType.Number),
                HighlightSpan(58, 96, TokenType.Comment), // 130 钳到 128 → 96
            ),
            out,
        )
    }

    @Test
    fun clipReturnsEmptyWhenNoOverlap() {
        val spans = listOf(HighlightSpan(0, 5, TokenType.Key))
        assertTrue(clipSpansToWindow(spans, 100, 200).isEmpty())
        assertTrue(clipSpansToWindow(emptyList(), 0, 10).isEmpty())
    }

    // --- highlightedText colorOnly ----------------------------------------------------------

    @Test
    fun colorOnlyDropsWeightAndStyleButKeepsColor() {
        val colors = SyntaxColors.Dark.copy(
            comment = TokenStyle(Color.Green, fontWeight = FontWeight.Bold, fontStyle = FontStyle.Italic),
        )
        val text = highlightedText("ab", listOf(HighlightSpan(0, 2, TokenType.Comment)), colors, colorOnly = true)
        assertEquals(Color.Green, text.spanStyles[0].item.color)
        assertNull(text.spanStyles[0].item.fontWeight)
        assertNull(text.spanStyles[0].item.fontStyle)
    }

    // --- 分词上限 ----------------------------------------------------------------------------

    private class RecordingHighlighter : SyntaxHighlighter {
        val lexed = mutableListOf<Int>() // 记录被分词行的文本长度

        data class Flip(val on: Boolean) : LineState

        override fun highlightLine(text: String, entryState: LineState?): LineHighlight {
            lexed += text.length
            val on = (entryState as? Flip)?.on ?: false
            val exit = if (text.contains("#")) Flip(!on) else entryState
            val spans = if (on && text.isNotEmpty()) listOf(HighlightSpan(0, text.length, TokenType.String)) else emptyList()
            return LineHighlight(spans, exit)
        }
    }

    @Test
    fun overlongLineIsNotLexedAndStatePassesThrough() {
        val h = RecordingHighlighter()
        val cache = HighlightCache(h, maxLexedLineLength = 100)
        val doc = listOf("# flip", "x".repeat(101), "after")
        assertTrue(cache.spansForLine(1) { doc[it] }.isEmpty()) // 超长行：空 spans
        assertTrue(101 !in h.lexed, "overlong line must not reach the lexer")
        // 状态穿透：# 行翻转出的状态穿过超长行，第 2 行仍按 on=true 着色。
        assertTrue(cache.spansForLine(2) { doc[it] }.isNotEmpty())
    }

    @Test
    fun defaultCapAllowsTwentyThousandChars() {
        val h = RecordingHighlighter()
        val cache = HighlightCache(h)
        val doc = listOf("x".repeat(20_000), "x".repeat(20_001))
        cache.spansForLine(1) { doc[it] }
        assertTrue(20_000 in h.lexed, "a 20k line is still lexed by default")
        assertTrue(20_001 !in h.lexed, "beyond the default cap is skipped")
    }
}
