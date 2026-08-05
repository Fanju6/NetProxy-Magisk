package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class EditorEngineDirtyLineTest {

    @Test
    fun freshEngineIsDirtyStructuralFromTop() {
        val e = EditorEngine("a\nb")
        val d = e.consumeDirty()
        assertEquals(0, d!!.from)
        assertTrue(d.structural)
        assertNull(e.consumeDirty()) // 消费后复位为「无脏」
    }

    @Test
    fun singleCharInsertIsNonStructuralSingleLine() {
        val e = EditorEngine("a\nb\nc")
        e.consumeDirty()
        e.setCursor(TextPosition(2, 1))
        e.insert("x")
        val d = e.consumeDirty()!!
        assertEquals(2, d.from)
        assertEquals(3, d.endExclusive)
        assertFalse(d.structural)
    }

    @Test
    fun newlineInsertIsStructural() {
        val e = EditorEngine("a\nb\nc")
        e.consumeDirty()
        e.setCursor(TextPosition(1, 1))
        e.insert("\n")
        val d = e.consumeDirty()!!
        assertEquals(1, d.from)
        assertTrue(d.structural)
    }

    @Test
    fun sameLineCountReplaceIsNonStructuralRange() {
        // 选中跨两行的区域，替换为同样含 1 个换行的文本：行数不变 → 非结构性，范围盖满两行。
        val e = EditorEngine("aa\nbb\ncc")
        e.consumeDirty()
        e.setSelection(TextPosition(1, 0), TextPosition(2, 1))
        e.insert("x\ny")
        val d = e.consumeDirty()!!
        assertEquals(1, d.from)
        assertEquals(3, d.endExclusive)
        assertFalse(d.structural)
    }

    @Test
    fun multiLineDeleteIsStructural() {
        val e = EditorEngine("aa\nbb\ncc")
        e.consumeDirty()
        e.setSelection(TextPosition(0, 1), TextPosition(2, 1))
        e.insert("")
        val d = e.consumeDirty()!!
        assertEquals(0, d.from)
        assertTrue(d.structural)
    }

    @Test
    fun multipleEditsUnionRangeAndOrStructural() {
        val e = EditorEngine("a\nb\nc")
        e.consumeDirty()
        e.setCursor(TextPosition(2, 1)); e.insert("x")
        e.setCursor(TextPosition(0, 1)); e.insert("y")
        val d = e.consumeDirty()!!
        assertEquals(0, d.from)
        assertEquals(3, d.endExclusive)
        assertFalse(d.structural)
    }

    @Test
    fun structuralEditPoisonsTheUnion() {
        val e = EditorEngine("a\nb\nc")
        e.consumeDirty()
        e.setCursor(TextPosition(2, 1)); e.insert("x")
        e.setCursor(TextPosition(0, 1)); e.insert("\n")
        val d = e.consumeDirty()!!
        assertEquals(0, d.from)
        assertTrue(d.structural)
    }

    @Test
    fun undoOfNewlineIsStructural() {
        val e = EditorEngine("a\nb\nc")
        e.setCursor(TextPosition(1, 1))
        e.insert("\n")
        e.consumeDirty()
        e.undo()
        val d = e.consumeDirty()!!
        assertEquals(1, d.from)
        assertTrue(d.structural)
    }

    @Test
    fun undoOfTypingIsNonStructural() {
        val e = EditorEngine("a\nb\nc")
        e.setCursor(TextPosition(1, 1))
        e.insert("z")
        e.consumeDirty()
        e.undo()
        val d = e.consumeDirty()!!
        assertEquals(1, d.from)
        assertEquals(2, d.endExclusive)
        assertFalse(d.structural)
    }

    @Test
    fun replaceAllSingleCharsIsNonStructuralUnion() {
        val e = EditorEngine("x\ny\nx")
        e.consumeDirty()
        e.replaceAllOffsetRanges(listOf(0 to 1, 4 to 5), "z")
        val d = e.consumeDirty()!!
        assertEquals(0, d.from)
        assertEquals(3, d.endExclusive)
        assertFalse(d.structural)
    }

    @Test
    fun setTextIsStructuralFromTop() {
        val e = EditorEngine("a")
        e.setCursor(TextPosition(0, 1))
        e.consumeDirty()
        e.setText("fresh\ndoc")
        val d = e.consumeDirty()!!
        assertEquals(0, d.from)
        assertTrue(d.structural)
    }
}
