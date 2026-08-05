package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class EditorEngineUndoTest {

    @Test
    fun freshEngineCannotUndoOrRedo() {
        val e = EditorEngine("abc")
        assertFalse(e.canUndo)
        assertFalse(e.canRedo)
        assertFalse(e.undo())
        assertFalse(e.redo())
    }

    @Test
    fun typedRunUndoesAsOneUnit() {
        val e = EditorEngine("")
        e.insert("a"); e.insert("b"); e.insert("c")
        assertTrue(e.canUndo)
        assertTrue(e.undo())
        assertEquals("", e.getText())
        assertEquals(TextPosition(0, 0), e.caret)
        assertFalse(e.canUndo)
    }

    @Test
    fun redoRestoresTypedRunAndCaret() {
        val e = EditorEngine("")
        e.insert("a"); e.insert("b")
        e.undo()
        assertTrue(e.redo())
        assertEquals("ab", e.getText())
        assertEquals(TextPosition(0, 2), e.caret)
        assertFalse(e.canRedo)
    }

    @Test
    fun cursorMoveBreaksTypingMerge() {
        val e = EditorEngine("")
        e.insert("a"); e.insert("b")
        e.setCursor(TextPosition(0, 2)) // 光标动作打断合并（位置未变也算显式定位）
        e.insert("c")
        e.undo()
        assertEquals("ab", e.getText())
        e.undo()
        assertEquals("", e.getText())
    }

    @Test
    fun newlineIsItsOwnUndoUnit() {
        val e = EditorEngine("")
        e.insert("a")
        e.insert("\n")
        e.insert("b")
        e.undo()
        assertEquals("a\n", e.getText())
        e.undo()
        assertEquals("a", e.getText())
        e.undo()
        assertEquals("", e.getText())
    }

    @Test
    fun backspaceRunUndoesAsOneUnit() {
        val e = EditorEngine("hello")
        e.setCursor(TextPosition(0, 5))
        e.backspace(); e.backspace(); e.backspace()
        assertEquals("he", e.getText())
        e.undo()
        assertEquals("hello", e.getText())
        assertEquals(TextPosition(0, 5), e.caret)
        assertFalse(e.canUndo)
    }

    @Test
    fun deleteForwardRunUndoesAsOneUnit() {
        val e = EditorEngine("hello")
        e.setCursor(TextPosition(0, 2))
        e.deleteForward(); e.deleteForward(); e.deleteForward()
        assertEquals("he", e.getText())
        e.undo()
        assertEquals("hello", e.getText())
        assertEquals(TextPosition(0, 2), e.caret)
    }

    @Test
    fun selectionReplaceUndoRestoresSelection() {
        val e = EditorEngine("hello world")
        e.setSelection(TextPosition(0, 0), TextPosition(0, 5))
        e.insert("HI")
        assertEquals("HI world", e.getText())
        e.undo()
        assertEquals("hello world", e.getText())
        assertEquals(TextPosition(0, 0), e.selStart)
        assertEquals(TextPosition(0, 5), e.selEnd)
    }

    @Test
    fun reversedSelectionUndoRestoresHeadAtAnchorSide() {
        val e = EditorEngine("hello")
        e.setSelection(TextPosition(0, 4), TextPosition(0, 1)) // anchor=4, head=1（反向选区）
        e.insert("X")
        e.undo()
        assertEquals("hello", e.getText())
        assertEquals(TextPosition(0, 1), e.caret) // head 恢复到反向端
        assertEquals(TextPosition(0, 1), e.selStart)
        assertEquals(TextPosition(0, 4), e.selEnd)
    }

    @Test
    fun composingSessionUndoesAsOneUnit() {
        val e = EditorEngine("")
        e.setComposingText("n", 1)
        e.setComposingText("ni", 1)
        e.commitText("你", 1)
        assertEquals("你", e.getText())
        e.undo()
        assertEquals("", e.getText())
        assertEquals(TextPosition(0, 0), e.caret)
        assertFalse(e.canUndo)
        e.redo()
        assertEquals("你", e.getText())
        assertEquals(TextPosition(0, 1), e.caret)
    }

    @Test
    fun separateCommitsAreSeparateUnits() {
        val e = EditorEngine("")
        e.setComposingText("ni", 1)
        e.commitText("你", 1)
        e.setComposingText("hao", 1)
        e.commitText("好", 1)
        assertEquals("你好", e.getText())
        e.undo()
        assertEquals("你", e.getText())
        e.undo()
        assertEquals("", e.getText())
    }

    @Test
    fun deleteSurroundingTextUndoesAsOneUnit() {
        val e = EditorEngine("abcdef")
        e.setCursor(TextPosition(0, 3))
        e.deleteSurroundingText(2, 2) // 删 bc 与 de → "af"
        assertEquals("af", e.getText())
        e.undo()
        assertEquals("abcdef", e.getText())
        assertEquals(TextPosition(0, 3), e.caret)
        assertFalse(e.canUndo)
    }

    @Test
    fun setTextClearsHistory() {
        val e = EditorEngine("")
        e.insert("a")
        e.setText("fresh")
        assertFalse(e.canUndo)
        assertFalse(e.redo())
    }

    @Test
    fun newEditClearsRedo() {
        val e = EditorEngine("")
        e.insert("a")
        e.undo()
        assertTrue(e.canRedo)
        e.insert("b")
        assertFalse(e.canRedo)
        assertEquals("b", e.getText())
    }

    @Test
    fun undoAcrossLinesRestoresPositions() {
        val e = EditorEngine("line1\nline2\nline3")
        e.setSelection(TextPosition(0, 2), TextPosition(2, 2))
        e.insert("X")
        assertEquals("liXne3", e.getText())
        e.undo()
        assertEquals("line1\nline2\nline3", e.getText())
        assertEquals(TextPosition(0, 2), e.selStart)
        assertEquals(TextPosition(2, 2), e.selEnd)
    }

    @Test
    fun undoDuringComposingDropsComposingState() {
        val e = EditorEngine("")
        e.insert("a")
        e.setComposingText("x", 1)
        // 预编辑挂起时撤销：先撤掉未提交的预编辑单元，composing 状态清空。
        e.undo()
        assertEquals("a", e.getText())
        assertEquals(null, e.composing)
    }

    @Test
    fun pasteLikeMultiCharInsertIsOwnUnit() {
        val e = EditorEngine("")
        e.insert("a")
        e.insert("pasted text") // 多字符一次插入不并入前面的键入
        e.undo()
        assertEquals("a", e.getText())
        e.undo()
        assertEquals("", e.getText())
    }
}
