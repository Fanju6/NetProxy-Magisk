package top.yukonga.scripta.editor

import androidx.compose.runtime.saveable.SaverScope
import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** 文档生命周期（B5）：构造播种、无条件 setDocument、进程死亡恢复 Saver。 */
class ControllerDocumentLifecycleTest {

    // --- 构造播种 ---

    @Test
    fun constructorSeedsInitialText() {
        val c = CodeEditorController("hello\nworld")
        assertEquals("hello\nworld", c.getText())
        assertFalse(c.isModified) // 刚打开的文档不脏
        assertFalse(c.canUndo) // 播种不留可撤销历史
    }

    @Test
    fun defaultConstructorIsEmptyClean() {
        val c = CodeEditorController()
        assertEquals("", c.getText())
        assertFalse(c.isModified)
    }

    // --- setDocument ---

    @Test
    fun setDocumentSwapsContentCaretHistoryAndDirty() {
        val c = CodeEditorController("old")
        c.engine.setCursor(TextPosition(0, 3))
        c.engine.insert("X")
        assertTrue(c.isModified)
        c.setDocument("fresh\ndoc")
        assertEquals("fresh\ndoc", c.getText())
        assertEquals(TextPosition(0, 0), c.caret)
        assertFalse(c.isModified)
        assertFalse(c.canUndo)
    }

    @Test
    fun setDocumentWithIdenticalTextStillResets() {
        // 无条件换文档：重开同内容文件也要清历史、光标回文首——不留相等守卫的静默 no-op。
        val c = CodeEditorController("same")
        c.engine.setCursor(TextPosition(0, 4))
        c.engine.insert("X")
        c.undo() // 文本回 "same"，但历史仍在（canRedo）
        assertTrue(c.canRedo)
        c.setDocument("same")
        assertFalse(c.canUndo)
        assertFalse(c.canRedo)
        assertEquals(TextPosition(0, 0), c.caret)
        assertFalse(c.isModified)
    }

    @Test
    fun setDocumentBumpsRevealTickEvenWithCaretAlreadyAtStart() {
        // 手势约定是「拖拽只滚动、不动光标」——光标停 (0,0)、视口在中部是换文档的常态。此时
        // collapseCaret 写入结构相等的选区、快照不失效，视口回顶必须走强制露出通道。
        val c = CodeEditorController("a\nb\nc")
        assertEquals(TextPosition(0, 0), c.caret)
        val t0 = c.engine.revealTick
        c.setDocument("x\ny")
        assertTrue(c.engine.revealTick > t0)
    }

    @Test
    fun setDocumentBumpsContentGeneration() {
        val c = CodeEditorController("a")
        val g = c.engine.contentGeneration
        c.setDocument("b")
        assertTrue(c.engine.contentGeneration > g)
    }

    // --- Saver（进程死亡/旋转恢复）---

    private val scope = SaverScope { true }

    private fun saveOf(c: CodeEditorController): Any? = with(CodeEditorController.Saver) { scope.save(c) }

    @Test
    fun saverRoundTripRestoresTextSelectionAndDirty() {
        val c = CodeEditorController("hello\nworld")
        c.engine.setSelection(TextPosition(1, 4), TextPosition(0, 2)) // 反向选区（保方向）
        c.engine.insert("X") // 弄脏
        c.engine.setSelection(TextPosition(0, 1), TextPosition(0, 3))
        val saved = saveOf(c)
        assertNotNull(saved)
        val r = CodeEditorController.Saver.restore(saved)!!
        assertEquals(c.getText(), r.getText())
        assertEquals(TextPosition(0, 1), r.selection.start)
        assertEquals(TextPosition(0, 3), r.selection.end)
        assertEquals(c.caret, r.caret)
        assertTrue(r.isModified) // 未保存改动的事实跨越进程死亡
    }

    @Test
    fun saverRestoresCleanDocumentAsClean() {
        val c = CodeEditorController("abc")
        val r = CodeEditorController.Saver.restore(saveOf(c)!!)!!
        assertFalse(r.isModified)
    }

    @Test
    fun saverSkipsHugeDocuments() {
        // Bundle ~1MB 事务上限：超限不保存（返回 null），恢复走宿主自己的持久化路径。
        val big = "x".repeat(300_000)
        val c = CodeEditorController(big)
        assertNull(saveOf(c))
    }
}
