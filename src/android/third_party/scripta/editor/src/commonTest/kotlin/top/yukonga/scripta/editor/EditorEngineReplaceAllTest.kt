package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse

class EditorEngineReplaceAllTest {

    @Test
    fun replacesAllRangesAndReportsCount() {
        val e = EditorEngine("cat dog cat dog cat")
        // 三处 cat：offset 0-3、8-11、16-19。
        val n = e.replaceAllOffsetRanges(listOf(0 to 3, 8 to 11, 16 to 19), "bird")
        assertEquals(3, n)
        assertEquals("bird dog bird dog bird", e.getText())
    }

    @Test
    fun replacementWithDifferentLengthKeepsLaterRangesCorrect() {
        val e = EditorEngine("aa bb aa bb aa")
        e.replaceAllOffsetRanges(listOf(0 to 2, 6 to 8, 12 to 14), "XXXX")
        assertEquals("XXXX bb XXXX bb XXXX", e.getText())
    }

    @Test
    fun replaceAllIsOneUndoUnit() {
        val e = EditorEngine("cat dog cat")
        e.replaceAllOffsetRanges(listOf(0 to 3, 8 to 11), "bird")
        assertEquals("bird dog bird", e.getText())
        e.undo()
        assertEquals("cat dog cat", e.getText())
        assertFalse(e.canUndo)
        e.redo()
        assertEquals("bird dog bird", e.getText())
    }

    @Test
    fun emptyRangesIsNoop() {
        val e = EditorEngine("abc")
        assertEquals(0, e.replaceAllOffsetRanges(emptyList(), "x"))
        assertEquals("abc", e.getText())
        assertFalse(e.canUndo)
    }

    @Test
    fun caretLandsAtEndOfFirstReplacement() {
        val e = EditorEngine("cat dog cat")
        e.replaceAllOffsetRanges(listOf(0 to 3, 8 to 11), "bird")
        assertEquals(TextPosition(0, 4), e.caret)
    }

    @Test
    fun replaceAllAcrossLinesUndoRestores() {
        val e = EditorEngine("x\nx\nx")
        e.replaceAllOffsetRanges(listOf(0 to 1, 2 to 3, 4 to 5), "yy")
        assertEquals("yy\nyy\nyy", e.getText())
        e.undo()
        assertEquals("x\nx\nx", e.getText())
    }
}
