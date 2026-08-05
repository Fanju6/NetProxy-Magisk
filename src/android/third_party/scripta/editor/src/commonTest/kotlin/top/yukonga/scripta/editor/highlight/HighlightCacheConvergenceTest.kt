package top.yukonga.scripta.editor.highlight

import kotlin.test.Test
import kotlin.test.assertTrue

/**
 * 收敛截断：非结构性编辑后，一旦某行新算的退出状态与旧存值相等，水位线直接跳回旧可信位置，
 * 不再逐行重扫下游——在超长文件顶部打字后跳到文件尾不应触发 O(N) 全链重扫。
 */
class HighlightCacheConvergenceTest {

    /** 遇到含 `#` 的行翻转状态，其余行透传；记录每次分词的行文本。 */
    private class RecordingHighlighter : SyntaxHighlighter {
        val lexed = mutableListOf<String>()

        data class Flip(val on: Boolean) : LineState

        override fun highlightLine(text: String, entryState: LineState?): LineHighlight {
            lexed += text
            val on = (entryState as? Flip)?.on ?: false
            val exit = if (text.contains("#")) Flip(!on) else entryState
            val spans = if (on && text.isNotEmpty()) listOf(HighlightSpan(0, text.length, TokenType.String)) else emptyList()
            return LineHighlight(spans, exit)
        }
    }

    private fun docOf(n: Int) = MutableList(n) { "line $it" }

    @Test
    fun unchangedExitConvergesWithoutFullRelex() {
        val h = RecordingHighlighter()
        val cache = HighlightCache(h)
        val doc = docOf(100)
        cache.spansForLine(99) { doc[it] } // 铺满状态链
        h.lexed.clear()
        doc[5] = "line 5 edited" // 无 #：退出状态不变
        cache.invalidate(5, 6, structural = false)
        cache.spansForLine(99) { doc[it] }
        // 只应重扫脏行本身 + 被请求行，而不是 5..99 全链
        assertTrue(h.lexed.size <= 3, "expected convergence, lexed ${h.lexed.size} lines: ${h.lexed}")
    }

    @Test
    fun changedExitRelexesDownstreamAndSpansStayCorrect() {
        val h = RecordingHighlighter()
        val cache = HighlightCache(h)
        val doc = docOf(100)
        cache.spansForLine(99) { doc[it] }
        h.lexed.clear()
        doc[5] = "# flipped" // 翻转：下游全部行的进入状态改变
        cache.invalidate(5, 6, structural = false)
        val spans9 = cache.spansForLine(9) { doc[it] }
        assertTrue(spans9.isNotEmpty(), "line 9 must reflect the flipped state")
        assertTrue(h.lexed.size >= 5, "downstream must actually re-lex, lexed: ${h.lexed.size}")
    }

    @Test
    fun structuralInvalidationNeverConverges() {
        val h = RecordingHighlighter()
        val cache = HighlightCache(h)
        val doc = docOf(100)
        cache.spansForLine(99) { doc[it] }
        h.lexed.clear()
        cache.invalidate(5, 6, structural = true) // 行数变过：旧尾链不可比
        cache.spansForLine(99) { doc[it] }
        assertTrue(h.lexed.size >= 95, "structural invalidation must re-lex the whole chain, lexed: ${h.lexed.size}")
    }

    @Test
    fun multiLineDirtyRangeOnlyConvergesPastItsEnd() {
        val h = RecordingHighlighter()
        val cache = HighlightCache(h)
        val doc = docOf(100)
        cache.spansForLine(99) { doc[it] }
        h.lexed.clear()
        // 脏范围 [5,8)，其中真正翻转状态的是中间那行——若在 5 行就「巧合收敛」会漏掉它。
        doc[6] = "# hidden flip"
        cache.invalidate(5, 8, structural = false)
        val spans9 = cache.spansForLine(9) { doc[it] }
        assertTrue(spans9.isNotEmpty(), "convergence below the dirty range end must not skip the flip at line 6")
    }

    @Test
    fun editInsidePendingTailRaisesTheComparisonFloor() {
        val h = RecordingHighlighter()
        val cache = HighlightCache(h)
        val doc = docOf(100)
        cache.spansForLine(99) { doc[it] }
        h.lexed.clear()
        // 第一次失效挂起尾链 [5,100)，未消费；第二次编辑翻转了尾链内部的行 50。
        doc[5] = "line 5 edited"
        cache.invalidate(5, 6, structural = false)
        doc[50] = "# flipped inside tail"
        cache.invalidate(50, 51, structural = false)
        val spans60 = cache.spansForLine(60) { doc[it] }
        assertTrue(spans60.isNotEmpty(), "line 60 must reflect the flip at line 50 — no convergence below line 50")
    }
}
