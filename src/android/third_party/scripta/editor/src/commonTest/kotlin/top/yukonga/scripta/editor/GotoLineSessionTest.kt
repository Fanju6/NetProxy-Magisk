package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** [GotoLineSession]：跳转行号会话——open 预填当前行、jump 钳制并落光标行首、非数字不动作。 */
class GotoLineSessionTest {

    private fun session(text: String = "a\nb\nc\nd\ne"): Pair<EditorEngine, GotoLineSession> {
        val e = EditorEngine(text)
        return e to GotoLineSession(e)
    }

    @Test
    fun openShowsBarPrefilledWithCurrentLine() {
        val (e, s) = session()
        e.setCursor(TextPosition(2, 1))
        s.open()
        assertTrue(s.visible)
        assertEquals("3", s.input) // 展示 1 基行号
    }

    @Test
    fun jumpMovesCaretToLineStartAndCloses() {
        val (e, s) = session()
        s.open()
        s.input = "4"
        assertTrue(s.jump())
        assertEquals(TextPosition(3, 0), e.caret)
        assertFalse(s.visible)
    }

    @Test
    fun jumpClampsBelowRangeToFirstLine() {
        val (e, s) = session()
        s.open()
        s.input = "0"
        assertTrue(s.jump())
        assertEquals(TextPosition(0, 0), e.caret)
    }

    @Test
    fun jumpClampsAboveRangeToLastLine() {
        val (e, s) = session()
        s.open()
        s.input = "999"
        assertTrue(s.jump())
        assertEquals(TextPosition(4, 0), e.caret)
    }

    @Test
    fun jumpSurvivesOverflowingNumbers() {
        val (e, s) = session()
        s.open()
        s.input = "99999999999999999999" // 超 Long 也不能崩
        assertTrue(s.jump())
        assertEquals(TextPosition(4, 0), e.caret)
    }

    @Test
    fun jumpRejectsNonNumericInputAndStaysOpen() {
        val (e, s) = session()
        e.setCursor(TextPosition(1, 0))
        s.open()
        s.input = ""
        assertFalse(s.jump())
        assertTrue(s.visible)
        assertEquals(TextPosition(1, 0), e.caret)
    }

    @Test
    fun closeHidesBar() {
        val (_, s) = session()
        s.open()
        s.close()
        assertFalse(s.visible)
    }
}
