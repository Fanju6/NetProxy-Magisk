package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import kotlin.test.Test
import kotlin.test.assertEquals

/** [EditorEngine.insertNewlineAutoIndent]：回车继承选区起点行的前导空白（截断到起点列）。 */
class EditorEngineAutoIndentTest {

    @Test
    fun inheritsLeadingWhitespaceAtLineEnd() {
        val e = EditorEngine("  ab")
        e.setCursor(TextPosition(0, 4))
        e.insertNewlineAutoIndent()
        assertEquals("  ab\n  ", e.getText())
        assertEquals(TextPosition(1, 2), e.caret)
    }

    @Test
    fun noIndentOnUnindentedLine() {
        val e = EditorEngine("ab")
        e.setCursor(TextPosition(0, 2))
        e.insertNewlineAutoIndent()
        assertEquals("ab\n", e.getText())
        assertEquals(TextPosition(1, 0), e.caret)
    }

    @Test
    fun caretInsideIndentInheritsOnlyItsLeftPart() {
        val e = EditorEngine("    x")
        e.setCursor(TextPosition(0, 2))
        e.insertNewlineAutoIndent()
        assertEquals("  \n    x", e.getText())
        assertEquals(TextPosition(1, 2), e.caret)
    }

    @Test
    fun enterAtLineStartAddsNoIndent() {
        val e = EditorEngine("  ab")
        e.setCursor(TextPosition(0, 0))
        e.insertNewlineAutoIndent()
        assertEquals("\n  ab", e.getText())
        assertEquals(TextPosition(1, 0), e.caret)
    }

    @Test
    fun tabIndentIsInheritedVerbatim() {
        val e = EditorEngine("\tab")
        e.setCursor(TextPosition(0, 3))
        e.insertNewlineAutoIndent()
        assertEquals("\tab\n\t", e.getText())
        assertEquals(TextPosition(1, 1), e.caret)
    }

    @Test
    fun mixedSpaceTabIndentIsInheritedVerbatim() {
        val e = EditorEngine(" \t a")
        e.setCursor(TextPosition(0, 4))
        e.insertNewlineAutoIndent()
        assertEquals(" \t a\n \t ", e.getText())
        assertEquals(TextPosition(1, 3), e.caret)
    }

    @Test
    fun selectionUsesItsStartLineIndentTruncatedToStartColumn() {
        val e = EditorEngine("  abc\nxyz")
        e.setSelection(TextPosition(0, 4), TextPosition(1, 1))
        e.insertNewlineAutoIndent()
        assertEquals("  ab\n  yz", e.getText())
        assertEquals(TextPosition(1, 2), e.caret)
    }

    @Test
    fun emptyDocumentEnterInsertsBareNewline() {
        val e = EditorEngine("")
        e.insertNewlineAutoIndent()
        assertEquals("\n", e.getText())
        assertEquals(TextPosition(1, 0), e.caret)
    }

    @Test
    fun undoRevertsNewlineAndIndentInOneStep() {
        val e = EditorEngine("  ab")
        e.setCursor(TextPosition(0, 4))
        e.insertNewlineAutoIndent()
        e.undo()
        assertEquals("  ab", e.getText())
        assertEquals(TextPosition(0, 4), e.caret)
    }
}
