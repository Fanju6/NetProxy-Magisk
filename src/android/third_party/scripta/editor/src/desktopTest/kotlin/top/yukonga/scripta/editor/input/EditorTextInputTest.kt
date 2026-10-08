@file:OptIn(androidx.compose.ui.ExperimentalComposeUiApi::class)

package top.yukonga.scripta.editor.input

import androidx.compose.ui.text.TextRange
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import top.yukonga.scripta.editor.EditorEngine

class EditorTextInputTest {
    @Test fun textAndSelectionTrackCurrentDocumentWithUtf16Offsets() {
        val engine = EditorEngine("a😀中\nend")
        val request = ScriptaImeRequest(engine) { null }
        assertEquals("a😀中\nend", request.state.text)
        request.editText { setSelection(3, 1) }
        assertEquals(TextRange(3, 1), request.state.selection)
        request.editText { commitText("X", 1) }
        assertEquals("aX中\nend", request.state.text)
        assertEquals(request.state.text, request.value().text)
        request.editText { setSelection(-5, 100) }
        assertEquals(TextRange(0, engine.buffer.totalLength()), request.state.selection)
    }

    @Test fun composingRegionSurvivesSelectionAndCommitsInPlace() {
        val engine = EditorEngine("abcde")
        val request = ScriptaImeRequest(engine) { null }
        request.editText {
            setComposingRegion(3, 1)
            setSelection(2, 2)
        }
        assertEquals(TextRange(1, 3), request.state.composition)
        assertEquals(TextRange(2, 2), request.state.selection)
        request.editText { setComposingText("中", 1) }
        assertEquals("a中de", request.state.text)
        assertEquals(TextRange(1, 2), request.state.composition)
        request.editText { finishComposingText() }
        assertNull(request.state.composition)
        assertEquals("a中de", request.state.text)
        request.editText { setComposingRegion(100, 200) }
        assertNull(request.state.composition)
    }
}
