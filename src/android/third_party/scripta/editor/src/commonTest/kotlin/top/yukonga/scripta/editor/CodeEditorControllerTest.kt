package top.yukonga.scripta.editor

import kotlin.test.Test
import kotlin.test.assertEquals
import top.yukonga.scripta.editor.text.TextPosition

class CodeEditorControllerTest {
    @Test
    fun customSymbolShortcutPairsAndSkipsClosingBracket() {
        val controller = CodeEditorController()
        controller.typeSymbol(EditorSymbol("{"))
        assertEquals("{}", controller.getText())
        assertEquals(TextPosition(0, 1), controller.caret)
        controller.typeSymbol(EditorSymbol("}"))
        assertEquals("{}", controller.getText())
        assertEquals(TextPosition(0, 2), controller.caret)
    }

    @Test
    fun getTextRoundTripsSetDocument() {
        val c = CodeEditorController()
        c.setDocument("hello\nworld")
        assertEquals("hello\nworld", c.getText())
    }

    @Test
    fun controllerExposesEngine() {
        val c = CodeEditorController("abc")
        assertEquals("abc", c.engine.getText())
    }

    @Test
    fun undoRedoDelegateToEngine() {
        val c = CodeEditorController("abc")
        c.engine.insert("X")
        assertEquals(true, c.canUndo)
        assertEquals(true, c.undo())
        assertEquals("abc", c.getText())
        assertEquals(true, c.canRedo)
        assertEquals(true, c.redo())
        assertEquals("Xabc", c.getText())
        assertEquals(false, c.redo())
    }
}
