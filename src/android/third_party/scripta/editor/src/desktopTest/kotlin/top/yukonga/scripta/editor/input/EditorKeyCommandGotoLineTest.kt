package top.yukonga.scripta.editor.input

import androidx.compose.ui.InternalComposeUiApi
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.KeyEventType
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** 跳转行号快捷键：Ctrl 系 Ctrl+G；macOS Cmd+L（Cmd+G 已属 FindNext）。 */
@OptIn(InternalComposeUiApi::class)
class EditorKeyCommandGotoLineTest {

    private fun keyDown(key: Key, ctrl: Boolean = false, meta: Boolean = false) =
        KeyEvent(key = key, type = KeyEventType.KeyDown, isCtrlPressed = ctrl, isMetaPressed = meta)

    @Test
    fun ctrlGResolvesToGotoLine() =
        assertEquals(EditorKeyCommand.GotoLine, resolveCtrlBased(keyDown(Key.G, ctrl = true)))

    @Test
    fun bareGDoesNotResolve() = assertNull(resolveCtrlBased(keyDown(Key.G)))

    @Test
    fun cmdLResolvesToGotoLineOnMac() =
        assertEquals(EditorKeyCommand.GotoLine, resolveMacBased(keyDown(Key.L, meta = true)))

    @Test
    fun cmdGStaysFindNextOnMac() =
        assertEquals(EditorKeyCommand.FindNext, resolveMacBased(keyDown(Key.G, meta = true)))

    @Test
    fun ctrlLDoesNotResolveOnCtrlPlatforms() = assertNull(resolveCtrlBased(keyDown(Key.L, ctrl = true)))
}
