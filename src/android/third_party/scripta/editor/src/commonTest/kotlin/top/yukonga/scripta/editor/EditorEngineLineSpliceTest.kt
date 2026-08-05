package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** [EditorEngine.consumeLineSplices]：行结构变化（行数增减）的按序事件流，供视觉行索引增量 splice。 */
class EditorEngineLineSpliceTest {

    @Test
    fun freshEngineHasNoPendingSplices() {
        // 视图按引擎当前行数直接建索引，不需要（也不能）重放建索引之前的历史。
        val e = EditorEngine("a\nb\nc")
        assertTrue(e.consumeLineSplices().isEmpty())
    }

    @Test
    fun nonStructuralEditProducesNoSplice() {
        val e = EditorEngine("a\nb")
        e.setCursor(TextPosition(1, 1))
        e.insert("x")
        assertTrue(e.consumeLineSplices().isEmpty())
    }

    @Test
    fun newlineInsertProducesInsertSplice() {
        val e = EditorEngine("a\nb\nc")
        e.setCursor(TextPosition(1, 1))
        e.insert("\n")
        assertEquals(listOf(LineSplice(1, 1, 2)), e.consumeLineSplices())
    }

    @Test
    fun multiLineDeleteProducesRemoveSplice() {
        val e = EditorEngine("aa\nbb\ncc")
        e.setSelection(TextPosition(0, 1), TextPosition(2, 1))
        e.insert("")
        assertEquals(listOf(LineSplice(0, 3, 1)), e.consumeLineSplices())
    }

    @Test
    fun consumeClearsThePendingQueue() {
        val e = EditorEngine("a")
        e.setCursor(TextPosition(0, 1))
        e.insert("\n")
        e.consumeLineSplices()
        assertTrue(e.consumeLineSplices().isEmpty())
    }

    @Test
    fun multipleStructuralEditsQueueInOrder() {
        val e = EditorEngine("a\nb")
        e.setCursor(TextPosition(0, 1))
        e.insert("\n") // (0, 1, 2)
        e.setCursor(TextPosition(2, 1))
        e.insert("\n") // (2, 1, 2)：行号是第一次编辑之后的实时行号
        assertEquals(listOf(LineSplice(0, 1, 2), LineSplice(2, 1, 2)), e.consumeLineSplices())
    }

    @Test
    fun undoAndRedoProduceMirrorSplices() {
        val e = EditorEngine("a\nb")
        e.setCursor(TextPosition(0, 1))
        e.insert("\n")
        e.consumeLineSplices()
        e.undo()
        assertEquals(listOf(LineSplice(0, 2, 1)), e.consumeLineSplices())
        e.redo()
        assertEquals(listOf(LineSplice(0, 1, 2)), e.consumeLineSplices())
    }

    @Test
    fun setTextClearsPendingSplices() {
        // setText 换代（contentGeneration++）后视图整个重建索引，残留的旧文档 splice 若被应用会错位。
        val e = EditorEngine("a")
        e.setCursor(TextPosition(0, 1))
        e.insert("\n")
        e.setText("fresh\ndoc")
        assertTrue(e.consumeLineSplices().isEmpty())
    }

    @Test
    fun replaceAllQueuesOneSplicePerStructuralSegment() {
        // 降序应用：先靠后的段。每条 splice 的行号在其应用时刻各自成立。
        val e = EditorEngine("x\ny\nx")
        e.replaceAllOffsetRanges(listOf(0 to 1, 4 to 5), "p\nq")
        assertEquals(listOf(LineSplice(2, 1, 2), LineSplice(0, 1, 2)), e.consumeLineSplices())
    }

    @Test
    fun sameLineCountReplaceProducesNoSplice() {
        val e = EditorEngine("aa\nbb\ncc")
        e.setSelection(TextPosition(1, 0), TextPosition(2, 1))
        e.insert("x\ny") // 删 1 换行、插 1 换行：行数不变
        assertTrue(e.consumeLineSplices().isEmpty())
    }
}
