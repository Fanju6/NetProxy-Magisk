package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse

/**
 * [EditorEngine.toggleBlockComment]：块注释切换（无行注释语言的注释切换回退）。
 * 首末非空白行内容头/尾已成对包裹则拆除，否则整块包一层。
 */
class EditorEngineToggleBlockCommentTest {

    private fun EditorEngine.toggle() = toggleBlockComment("<!--", "-->")

    @Test
    fun caretOnlyWrapsCurrentLine() {
        val e = EditorEngine("abc")
        e.setCursor(TextPosition(0, 1))
        e.toggle()
        assertEquals("<!-- abc -->", e.getText())
        assertEquals(TextPosition(0, 6), e.caret) // 光标随开标记插入平移
    }

    @Test
    fun toggleTwiceRoundTrips() {
        val e = EditorEngine("abc")
        e.setCursor(TextPosition(0, 0))
        e.toggle()
        e.toggle()
        assertEquals("abc", e.getText())
    }

    @Test
    fun unwrapWithoutConventionSpaces() {
        val e = EditorEngine("<!--abc-->")
        e.setCursor(TextPosition(0, 0))
        e.toggle()
        assertEquals("abc", e.getText())
    }

    @Test
    fun multiLineSelectionWrapsWholeBlockOnce() {
        val e = EditorEngine("a\nb\nc")
        e.setSelection(TextPosition(0, 0), TextPosition(2, 1))
        e.toggle()
        assertEquals("<!-- a\nb\nc -->", e.getText())
    }

    @Test
    fun multiLineUnwrap() {
        val e = EditorEngine("<!-- a\nb\nc -->")
        e.setSelection(TextPosition(0, 0), TextPosition(2, 6))
        e.toggle()
        assertEquals("a\nb\nc", e.getText())
    }

    @Test
    fun openAfterLeadingWhitespaceCloseBeforeTrailing() {
        val e = EditorEngine("  a  ")
        e.setCursor(TextPosition(0, 3))
        e.toggle()
        assertEquals("  <!-- a -->  ", e.getText())
        e.toggle()
        assertEquals("  a  ", e.getText()) // 首尾空白留在标记外，往返不吞
    }

    @Test
    fun blankEdgeLinesAnchorToContent() {
        // 选区含首尾空白行时，标记落在首/末非空白行上——空白边缘不积累标记。
        val e = EditorEngine("\na\nb\n\n")
        e.setSelection(TextPosition(0, 0), TextPosition(4, 0))
        e.toggle()
        assertEquals("\n<!-- a\nb -->\n\n", e.getText())
    }

    @Test
    fun allBlankIsNoOp() {
        val e = EditorEngine("\n  \n")
        e.setSelection(TextPosition(0, 0), TextPosition(2, 0))
        e.toggle()
        assertEquals("\n  \n", e.getText())
        assertFalse(e.canUndo) // 无编辑不留撤销单元
    }

    @Test
    fun selectionEndAtColumnZeroExcludesThatLine() {
        val e = EditorEngine("a\nb")
        e.setSelection(TextPosition(0, 0), TextPosition(1, 0))
        e.toggle()
        assertEquals("<!-- a -->\nb", e.getText())
    }

    @Test
    fun undoRestoresInOneStepWithSelection() {
        val e = EditorEngine("a\nb")
        e.setSelection(TextPosition(0, 0), TextPosition(1, 1))
        e.toggle()
        e.undo()
        assertEquals("a\nb", e.getText())
        assertEquals(TextPosition(0, 0), e.selStart)
        assertEquals(TextPosition(1, 1), e.selEnd)
    }

    @Test
    fun selectionShiftsWithWrap() {
        val e = EditorEngine("abc")
        e.setSelection(TextPosition(0, 1), TextPosition(0, 2))
        e.toggle()
        assertEquals("<!-- abc -->", e.getText())
        assertEquals(TextPosition(0, 6), e.selStart)
        assertEquals(TextPosition(0, 7), e.selEnd)
    }

    @Test
    fun overlappingMarkersDoNotUnwrap() {
        // "<!-->" 开闭标记共享字符，不算已包裹——包一层而不是错删。
        val e = EditorEngine("<!-->")
        e.setCursor(TextPosition(0, 0))
        e.toggle()
        assertEquals("<!-- <!--> -->", e.getText())
    }

    @Test
    fun emptyBodySingleSpaceUnwrapsToEmpty() {
        // "<!-- -->" 的正文只有那个惯例空格：两侧不得双重认领同一字符。
        val e = EditorEngine("<!-- -->")
        e.setCursor(TextPosition(0, 0))
        e.toggle()
        assertEquals("", e.getText())
    }

    @Test
    fun halfMarkerAddsLayerReversibly() {
        // 只有半边标记 = 未包裹：包一层，再切一次恰好还原。
        val e = EditorEngine("<!-- a")
        e.setCursor(TextPosition(0, 0))
        e.toggle()
        assertEquals("<!-- <!-- a -->", e.getText())
        e.toggle()
        assertEquals("<!-- a", e.getText())
    }
}
