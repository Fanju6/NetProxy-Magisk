package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals

/** [EditorEngine.typeCharacter]（键入智能：自动配对/跳过闭合）与成对退格。 */
class EditorEngineAutoPairTest {

    @Test
    fun typingOpenBracketInsertsPairWithCaretBetween() {
        val e = EditorEngine("")
        e.typeCharacter("(")
        assertEquals("()", e.getText())
        assertEquals(TextPosition(0, 1), e.caret)
    }

    @Test
    fun allBracketTypesPair() {
        val e = EditorEngine("")
        e.typeCharacter("{")
        e.typeCharacter("[")
        assertEquals("{[]}", e.getText())
        assertEquals(TextPosition(0, 2), e.caret)
    }

    @Test
    fun typingCloserOverExistingCloserSkips() {
        val e = EditorEngine("()")
        e.setCursor(TextPosition(0, 1))
        e.typeCharacter(")")
        assertEquals("()", e.getText()) // 不重复插入
        assertEquals(TextPosition(0, 2), e.caret) // 只越过
    }

    @Test
    fun quoteOpensPairInFreshContext() {
        val e = EditorEngine("key: ")
        e.setCursor(TextPosition(0, 5))
        e.typeCharacter("\"")
        assertEquals("key: \"\"", e.getText())
        assertEquals(TextPosition(0, 6), e.caret)
    }

    @Test
    fun secondQuoteSkipsOverAutoClosed() {
        val e = EditorEngine("\"\"")
        e.setCursor(TextPosition(0, 1))
        e.typeCharacter("\"")
        assertEquals("\"\"", e.getText())
        assertEquals(TextPosition(0, 2), e.caret)
    }

    @Test
    fun quoteAfterOddCountClosesPlainly() {
        // 行内左侧已有奇数个同引号 = 正在字符串里、本次是闭合意图：原样插入、不再成对。
        val e = EditorEngine("\"abc")
        e.setCursor(TextPosition(0, 4))
        e.typeCharacter("\"")
        assertEquals("\"abc\"", e.getText())
        assertEquals(TextPosition(0, 5), e.caret)
    }

    @Test
    fun quoteBeforeWordDoesNotPair() {
        // 右侧贴着字母（在词前补引号）：成对会凭空多出一个闭引号。
        val e = EditorEngine("word")
        e.setCursor(TextPosition(0, 0))
        e.typeCharacter("\"")
        assertEquals("\"word", e.getText())
    }

    @Test
    fun selectionTypingReplacesWithoutPairing() {
        val e = EditorEngine("abc")
        e.setSelection(TextPosition(0, 0), TextPosition(0, 3))
        e.typeCharacter("(")
        assertEquals("(", e.getText())
    }

    @Test
    fun multiCharFallsBackToPlainInsert() {
        val e = EditorEngine("")
        e.typeCharacter("    ") // 符号条 Tab
        assertEquals("    ", e.getText())
    }

    @Test
    fun disabledFlagTypesPlainly() {
        val e = EditorEngine("")
        e.autoClosePairs = false
        e.typeCharacter("(")
        assertEquals("(", e.getText())
    }

    @Test
    fun undoRemovesTheWholeInsertedPair() {
        val e = EditorEngine("x")
        e.setCursor(TextPosition(0, 1))
        e.typeCharacter("(")
        e.undo()
        assertEquals("x", e.getText())
    }

    // --- 成对退格 ---

    @Test
    fun backspaceBetweenAutoPairDeletesBoth() {
        val e = EditorEngine("()")
        e.setCursor(TextPosition(0, 1))
        e.backspace()
        assertEquals("", e.getText())
    }

    @Test
    fun backspaceBetweenQuotePairDeletesBoth() {
        val e = EditorEngine("\"\"")
        e.setCursor(TextPosition(0, 1))
        e.backspace()
        assertEquals("", e.getText())
    }

    @Test
    fun backspaceNotBetweenPairDeletesOneChar() {
        val e = EditorEngine("(a)")
        e.setCursor(TextPosition(0, 2))
        e.backspace()
        assertEquals("()", e.getText())
    }

    @Test
    fun backspacePairDisabledDeletesOneChar() {
        val e = EditorEngine("()")
        e.autoClosePairs = false
        e.setCursor(TextPosition(0, 1))
        e.backspace()
        assertEquals(")", e.getText())
    }
}
