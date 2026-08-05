package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse

/** [EditorEngine.toggleLineComment]：行注释切换（全已注释则取消、否则全部加一层；最小缩进列对齐）。 */
class EditorEngineToggleCommentTest {

    @Test
    fun singleUncommentedLineGetsPrefix() {
        val e = EditorEngine("abc")
        e.setCursor(TextPosition(0, 2))
        e.toggleLineComment("#")
        assertEquals("# abc", e.getText())
        assertEquals(TextPosition(0, 4), e.caret) // 光标随插入平移
    }

    @Test
    fun singleCommentedLineLosesPrefixAndSpace() {
        val e = EditorEngine("# abc")
        e.setCursor(TextPosition(0, 4))
        e.toggleLineComment("#")
        assertEquals("abc", e.getText())
        assertEquals(TextPosition(0, 2), e.caret)
    }

    @Test
    fun prefixWithoutTrailingSpaceAlsoRemoved() {
        val e = EditorEngine("#abc")
        e.setCursor(TextPosition(0, 0))
        e.toggleLineComment("#")
        assertEquals("abc", e.getText())
    }

    @Test
    fun multiLineAddsAtMinimumIndentColumn() {
        // 最小缩进列（2）对齐插入：深缩进行的 # 不贴自己行首、与浅行对齐成一列。
        val e = EditorEngine("  a\n    b")
        e.setSelection(TextPosition(0, 0), TextPosition(1, 5))
        e.toggleLineComment("#")
        assertEquals("  # a\n  #   b", e.getText())
    }

    @Test
    fun multiLineAllCommentedUncomments() {
        val e = EditorEngine("  # a\n  # b")
        e.setSelection(TextPosition(0, 0), TextPosition(1, 5))
        e.toggleLineComment("#")
        assertEquals("  a\n  b", e.getText())
    }

    @Test
    fun mixedStateAddsALayerToAll() {
        // 混合时全部加一层（可逆：再切一次恰好回到混合原状）。
        val e = EditorEngine("# a\nb")
        e.setSelection(TextPosition(0, 0), TextPosition(1, 1))
        e.toggleLineComment("#")
        assertEquals("# # a\n# b", e.getText())
        e.toggleLineComment("#")
        assertEquals("# a\nb", e.getText())
    }

    @Test
    fun emptyLinesAreSkippedBothWays() {
        val e = EditorEngine("a\n\nb")
        e.setSelection(TextPosition(0, 0), TextPosition(2, 1))
        e.toggleLineComment("#")
        assertEquals("# a\n\n# b", e.getText()) // 空行不加前缀、也不参与"是否全已注释"判定
    }

    @Test
    fun allEmptyLinesIsNoOp() {
        val e = EditorEngine("\n\n")
        e.setSelection(TextPosition(0, 0), TextPosition(2, 0))
        e.toggleLineComment("#")
        assertEquals("\n\n", e.getText())
        assertFalse(e.canUndo) // 无编辑不留撤销单元
    }

    @Test
    fun selectionEndsAtColumnZeroExcludesThatLine() {
        val e = EditorEngine("a\nb")
        e.setSelection(TextPosition(0, 0), TextPosition(1, 0))
        e.toggleLineComment("#")
        assertEquals("# a\nb", e.getText())
    }

    @Test
    fun undoRestoresInOneStepWithSelection() {
        val e = EditorEngine("a\nb")
        e.setSelection(TextPosition(0, 0), TextPosition(1, 1))
        e.toggleLineComment("#")
        e.undo()
        assertEquals("a\nb", e.getText())
        assertEquals(TextPosition(0, 0), e.selStart)
        assertEquals(TextPosition(1, 1), e.selEnd)
    }

    @Test
    fun selectionSurvivesToggleShifted() {
        val e = EditorEngine("aa\nbb")
        e.setSelection(TextPosition(0, 1), TextPosition(1, 2))
        e.toggleLineComment("#")
        assertEquals("# aa\n# bb", e.getText())
        assertEquals(TextPosition(0, 3), e.selStart)
        assertEquals(TextPosition(1, 4), e.selEnd)
    }

    @Test
    fun endOfLineAnchorStaysOnItsOwnLine() {
        // 行尾端点只吃本行前缀的平移：早行的净增不得把它推过下一行行首、误吞下一行的平移量。
        val e = EditorEngine("aa\nbb")
        e.setSelection(TextPosition(0, 2), TextPosition(1, 2))
        e.toggleLineComment("#")
        assertEquals("# aa\n# bb", e.getText())
        assertEquals(TextPosition(0, 4), e.selStart)
        assertEquals(TextPosition(1, 4), e.selEnd)
    }
}
