package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import top.yukonga.scripta.editor.text.TextRange
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** Controller 观测出口（B1+B2）：selection/caret 读取、documentVersion/isModified/markSaved、isComposing。 */
class ControllerObservationTest {

    private fun controller(text: String): CodeEditorController = CodeEditorController(text)

    // --- B2 selection / caret ---

    @Test
    fun selectionIsNormalizedAndCaretIsActiveEnd() {
        val c = controller("hello\nworld")
        c.engine.setSelection(TextPosition(0, 2), TextPosition(1, 3))
        assertEquals(TextRange(TextPosition(0, 2), TextPosition(1, 3)), c.selection)
        assertEquals(TextPosition(1, 3), c.caret)
    }

    @Test
    fun reverseSelectionKeepsCaretAtItsMovingEnd() {
        // 反向选择（head 在前）：selection 仍归一化，caret 是移动的那端（start 侧）。
        val c = controller("hello\nworld")
        c.engine.setSelection(TextPosition(1, 3), TextPosition(0, 2))
        assertEquals(TextRange(TextPosition(0, 2), TextPosition(1, 3)), c.selection)
        assertEquals(TextPosition(0, 2), c.caret)
    }

    @Test
    fun caretFollowsTyping() {
        val c = controller("ab")
        c.engine.setCursor(TextPosition(0, 1))
        c.engine.insert("X")
        assertEquals(TextPosition(0, 2), c.caret)
        assertTrue(c.selection.isEmpty)
    }

    // --- B1 documentVersion / isModified / markSaved ---

    @Test
    fun documentVersionAdvancesOnEdit() {
        val c = controller("ab")
        val v0 = c.documentVersion
        c.engine.insert("X")
        assertTrue(c.documentVersion != v0)
    }

    @Test
    fun freshControllerIsNotModified() {
        assertFalse(CodeEditorController().isModified)
    }

    @Test
    fun editSetsModifiedAndMarkSavedClearsIt() {
        val c = controller("ab")
        c.engine.insert("X")
        assertTrue(c.isModified)
        c.markSaved(c.documentVersion)
        assertFalse(c.isModified)
    }

    @Test
    fun markSavedWithStaleVersionKeepsModified() {
        // 异步保存竞态方向：写盘期间的键入不能被误标为已保存。
        val c = controller("ab")
        val vAtSave = c.documentVersion
        c.engine.insert("X") // 「写盘期间」的编辑
        c.markSaved(vAtSave)
        assertTrue(c.isModified)
    }

    @Test
    fun undoBackToSavedPointStaysConservativelyModified() {
        // 钉死文档化的保守语义：按 version 比较，undo 回保存点不回落。
        val c = controller("ab")
        c.markSaved(c.documentVersion)
        c.engine.insert("X")
        c.undo()
        assertTrue(c.isModified)
    }

    // --- B1 isComposing ---

    @Test
    fun composingIsObservable() {
        val c = controller("")
        assertFalse(c.isComposing)
        c.engine.setComposingText("ni", 1)
        assertTrue(c.isComposing)
        c.engine.finishComposing()
        assertFalse(c.isComposing)
    }
}
