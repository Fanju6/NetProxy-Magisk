package top.yukonga.scripta.editor.text

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class TextPositionTest {
    @Test
    fun comparesByLineThenColumn() {
        assertTrue(TextPosition(0, 5) < TextPosition(1, 0))
        assertTrue(TextPosition(1, 2) < TextPosition(1, 3))
        assertEquals(TextPosition(2, 4), TextPosition(2, 4))
    }

    @Test
    fun normalizedOrdersEndpoints() {
        val a = TextPosition(3, 1)
        val b = TextPosition(1, 7)
        assertEquals(TextRange(b, a), TextRange(a, b).normalized())
        assertEquals(TextRange(b, a), TextRange(b, a).normalized())
    }

    @Test
    fun cursorIsEmpty() {
        val p = TextPosition(2, 2)
        assertTrue(TextRange.cursor(p).isEmpty)
        assertTrue(!TextRange(p, TextPosition(2, 3)).isEmpty)
    }
}
