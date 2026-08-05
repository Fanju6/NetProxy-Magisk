package top.yukonga.scripta.editor.find

import top.yukonga.scripta.editor.EditorEngine
import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class FindSessionTest {

    private fun session(text: String): Pair<EditorEngine, FindSession> {
        val e = EditorEngine(text)
        return e to FindSession(e)
    }

    @Test
    fun openPrefillsQueryFromSingleLineSelection() {
        val (e, s) = session("hello world")
        e.setSelection(TextPosition(0, 0), TextPosition(0, 5))
        s.open(withReplace = false)
        assertEquals("hello", s.query)
        assertTrue(s.visible)
    }

    @Test
    fun openKeepsQueryWhenSelectionEmpty() {
        val (_, s) = session("hello")
        s.query = "prev"
        s.open(withReplace = false)
        assertEquals("prev", s.query)
    }

    @Test
    fun openModesAreExclusive() {
        val (_, s) = session("hello")
        s.open(withReplace = true)
        assertTrue(s.replaceVisible)
        // 纯查找模式不提供替换能力：已展开的替换行必须收起。
        s.open(withReplace = false)
        assertTrue(s.visible)
        assertFalse(s.replaceVisible)
        s.open(withReplace = true)
        assertTrue(s.replaceVisible)
    }

    @Test
    fun refreshComputesMatchesAndActiveFromCaret() {
        val (e, s) = session("cat dog cat dog cat")
        e.setCursor(TextPosition(0, 5)) // 光标在第一个 cat 之后
        s.open(withReplace = false)
        s.query = "cat"
        s.refresh()
        assertEquals(3, s.result.matches.size)
        assertEquals(1, s.activeIndex) // 光标之后的第一个匹配
    }

    @Test
    fun nextSelectsActiveMatchAndAdvances() {
        val (e, s) = session("cat dog cat dog cat")
        s.open(withReplace = false)
        s.query = "cat"
        s.refresh()
        s.next() // 从光标 (0,0) 起：选中第 0 个
        assertEquals(0, s.activeIndex)
        assertEquals(TextPosition(0, 0), e.selStart)
        assertEquals(TextPosition(0, 3), e.selEnd)
        s.next()
        assertEquals(1, s.activeIndex)
        assertEquals(TextPosition(0, 8), e.selStart)
        s.next()
        assertEquals(2, s.activeIndex)
        s.next() // 环绕回第 0 个
        assertEquals(0, s.activeIndex)
    }

    @Test
    fun prevNavigatesBackwardsWithWrap() {
        val (e, s) = session("cat dog cat")
        s.open(withReplace = false)
        s.query = "cat"
        s.refresh()
        s.prev() // 光标 (0,0) 之前无匹配 → 环绕到最后一个
        assertEquals(1, s.activeIndex)
        assertEquals(TextPosition(0, 8), e.selStart)
        s.prev()
        assertEquals(0, s.activeIndex)
    }

    @Test
    fun refreshKeepsActiveWhenSelectionIsAMatch() {
        val (e, s) = session("cat dog cat dog cat")
        s.open(withReplace = false)
        s.query = "cat"
        s.refresh()
        s.next(); s.next() // 选中第 1 个匹配
        s.refresh() // 文档未变的重算（如切开关再切回）不应漂移
        assertEquals(1, s.activeIndex)
    }

    @Test
    fun replaceCurrentReplacesSelectedMatchAndSelectsNext() {
        val (e, s) = session("cat dog cat dog cat")
        s.open(withReplace = true)
        s.query = "cat"
        s.refresh()
        s.next() // 选中第 0 个
        s.replacement = "bird"
        s.replaceCurrent()
        assertEquals("bird dog cat dog cat", e.getText())
        assertEquals(2, s.result.matches.size)
        // 自动选中下一处待替换匹配。
        assertEquals(TextPosition(0, 9), e.selStart)
        assertEquals(TextPosition(0, 12), e.selEnd)
    }

    @Test
    fun replaceCurrentWhenSelectionNotOnMatchOnlySelects() {
        val (e, s) = session("cat dog cat")
        s.open(withReplace = true)
        s.query = "cat"
        s.refresh()
        s.replacement = "bird"
        s.replaceCurrent() // 选区不在匹配上：只定位选中，不改文本
        assertEquals("cat dog cat", e.getText())
        assertFalse(e.selection.isEmpty)
    }

    @Test
    fun replaceAllReplacesEverythingAsOneUndo() {
        val (e, s) = session("cat dog cat dog cat")
        s.open(withReplace = true)
        s.query = "cat"
        s.refresh()
        s.replacement = "bird"
        val n = s.replaceAll()
        assertEquals(3, n)
        assertEquals("bird dog bird dog bird", e.getText())
        assertEquals(0, s.result.matches.size)
        e.undo()
        assertEquals("cat dog cat dog cat", e.getText())
    }

    @Test
    fun closeClearsHighlightsButKeepsQuery() {
        val (_, s) = session("cat")
        s.open(withReplace = false)
        s.query = "cat"
        s.refresh()
        assertTrue(s.result.matches.isNotEmpty())
        s.close()
        assertFalse(s.visible)
        assertTrue(s.result.matches.isEmpty())
        assertEquals("cat", s.query)
        assertTrue(s.spansForLine(0).isEmpty())
    }

    @Test
    fun spansForLineClipMultiLineMatches() {
        val (_, s) = session("ab\ncd")
        s.open(withReplace = false)
        s.query = "b\nc"
        s.refresh()
        assertEquals(1, s.result.matches.size)
        val l0 = s.spansForLine(0)
        val l1 = s.spansForLine(1)
        assertEquals(listOf(FindSpan(1, 2, 0)), l0) // 行 0 的 b
        assertEquals(listOf(FindSpan(0, 1, 0)), l1) // 行 1 的 c
    }

    @Test
    fun spansCoverEachOccurrenceWithMatchIndex() {
        val (_, s) = session("x y x")
        s.open(withReplace = false)
        s.query = "x"
        s.refresh()
        assertEquals(listOf(FindSpan(0, 1, 0), FindSpan(4, 5, 1)), s.spansForLine(0))
    }

    @Test
    fun editingDocThenRefreshRecomputes() {
        val (e, s) = session("cat cat")
        s.open(withReplace = false)
        s.query = "cat"
        s.refresh()
        assertEquals(2, s.result.matches.size)
        e.setCursor(TextPosition(0, 7))
        e.insert(" cat")
        s.refresh()
        assertEquals(3, s.result.matches.size)
    }
}
