package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * [EditorEngine.indentSelectedLines] / [EditorEngine.outdentSelectedLines]：
 * 选区触及行的块缩进/反缩进，一个撤销单元，选区端点随文本平移（行首端点钉在行首）。
 */
class EditorEngineIndentTest {

    // --- indent ---

    @Test
    fun indentSingleLineSelectionShiftsSelection() {
        val e = EditorEngine("abc\ndef")
        e.setSelection(TextPosition(0, 1), TextPosition(0, 3))
        e.indentSelectedLines()
        assertEquals("    abc\ndef", e.getText())
        assertEquals(TextPosition(0, 5), e.selStart)
        assertEquals(TextPosition(0, 7), e.selEnd)
    }

    @Test
    fun indentSpansEveryTouchedLine() {
        val e = EditorEngine("a\nb\nc")
        e.setSelection(TextPosition(0, 0), TextPosition(2, 1))
        e.indentSelectedLines()
        assertEquals("    a\n    b\n    c", e.getText())
    }

    @Test
    fun indentKeepsLineStartAnchorAtLineStart() {
        val e = EditorEngine("abc\ndef")
        e.setSelection(TextPosition(0, 0), TextPosition(1, 2))
        e.indentSelectedLines()
        assertEquals("    abc\n    def", e.getText())
        assertEquals(TextPosition(0, 0), e.selStart) // 行首端点不动，反复缩进保持贴边
        assertEquals(TextPosition(1, 6), e.selEnd)
    }

    @Test
    fun indentKeepsEndOfLineAnchorOnItsOwnLine() {
        // 行尾端点只吃本行插入的平移：早行的净增不得把它推过下一行行首、误吞下一行的平移量。
        val e = EditorEngine("aaa\nbbb")
        e.setSelection(TextPosition(0, 3), TextPosition(1, 3))
        e.indentSelectedLines()
        assertEquals("    aaa\n    bbb", e.getText())
        assertEquals(TextPosition(0, 7), e.selStart)
        assertEquals(TextPosition(1, 7), e.selEnd)
    }

    @Test
    fun indentExcludesLineWhereSelectionEndsAtColumnZero() {
        val e = EditorEngine("a\nb\nc")
        e.setSelection(TextPosition(0, 0), TextPosition(2, 0)) // 终点在 L2 行首：L2 不算选中
        e.indentSelectedLines()
        assertEquals("    a\n    b\nc", e.getText())
    }

    @Test
    fun indentSkipsEmptyLines() {
        val e = EditorEngine("a\n\nb")
        e.setSelection(TextPosition(0, 0), TextPosition(2, 1))
        e.indentSelectedLines()
        assertEquals("    a\n\n    b", e.getText()) // 空行不留尾随空白
    }

    @Test
    fun indentCaretOnlyAffectsItsLine() {
        val e = EditorEngine("ab\ncd")
        e.setCursor(TextPosition(1, 1))
        e.indentSelectedLines()
        assertEquals("ab\n    cd", e.getText())
        assertEquals(TextPosition(1, 5), e.caret)
    }

    @Test
    fun indentUndoesAsOneUnitRestoringSelection() {
        val e = EditorEngine("a\nb\nc")
        e.setSelection(TextPosition(0, 0), TextPosition(2, 1))
        e.indentSelectedLines()
        e.undo()
        assertEquals("a\nb\nc", e.getText())
        assertEquals(TextPosition(0, 0), e.selStart)
        assertEquals(TextPosition(2, 1), e.selEnd)
    }

    // --- outdent ---

    @Test
    fun outdentRemovesUpToFourSpaces() {
        val e = EditorEngine("    a\n  b\nc")
        e.setSelection(TextPosition(0, 0), TextPosition(2, 1))
        e.outdentSelectedLines()
        assertEquals("a\nb\nc", e.getText()) // 4 空格删 4、2 空格删 2、无空白不动
    }

    @Test
    fun outdentRemovesOneLeadingTab() {
        val e = EditorEngine("\ta\n\t\tb")
        e.setSelection(TextPosition(0, 0), TextPosition(1, 3))
        e.outdentSelectedLines()
        assertEquals("a\n\tb", e.getText())
    }

    @Test
    fun outdentShiftsSelectionEndpoints() {
        val e = EditorEngine("    abc")
        e.setSelection(TextPosition(0, 5), TextPosition(0, 7))
        e.outdentSelectedLines()
        assertEquals("abc", e.getText())
        assertEquals(TextPosition(0, 1), e.selStart)
        assertEquals(TextPosition(0, 3), e.selEnd)
    }

    @Test
    fun outdentClampsEndpointInsideRemovedWhitespace() {
        val e = EditorEngine("    abc")
        e.setSelection(TextPosition(0, 2), TextPosition(0, 6)) // 起点落在被删的空白内
        e.outdentSelectedLines()
        assertEquals("abc", e.getText())
        assertEquals(TextPosition(0, 0), e.selStart) // 贴回行首
        assertEquals(TextPosition(0, 2), e.selEnd)
    }

    @Test
    fun outdentCaretOnlyAffectsItsLine() {
        val e = EditorEngine("    a\n    b")
        e.setCursor(TextPosition(1, 5))
        e.outdentSelectedLines()
        assertEquals("    a\nb", e.getText())
        assertEquals(TextPosition(1, 1), e.caret)
    }

    @Test
    fun outdentExcludesLineWhereSelectionEndsAtColumnZero() {
        val e = EditorEngine("  a\n  b")
        e.setSelection(TextPosition(0, 0), TextPosition(1, 0))
        e.outdentSelectedLines()
        assertEquals("a\n  b", e.getText())
    }

    @Test
    fun outdentNoLeadingWhitespaceIsNoOpWithoutHistoryNoise() {
        val e = EditorEngine("abc")
        e.setCursor(TextPosition(0, 2))
        e.outdentSelectedLines()
        assertEquals("abc", e.getText())
        assertEquals(false, e.canUndo) // 没编辑就不该留下撤销单元
    }

    @Test
    fun outdentUndoesAsOneUnitRestoringSelection() {
        val e = EditorEngine("    a\n    b")
        e.setSelection(TextPosition(0, 4), TextPosition(1, 5))
        e.outdentSelectedLines()
        e.undo()
        assertEquals("    a\n    b", e.getText())
        assertEquals(TextPosition(0, 4), e.selStart)
        assertEquals(TextPosition(1, 5), e.selEnd)
    }
}
