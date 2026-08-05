package top.yukonga.scripta.editor.input

import androidx.compose.ui.InternalComposeUiApi
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.KeyEventType
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** 注释切换快捷键：Ctrl 系 Ctrl+/；macOS Cmd+/。 */
@OptIn(InternalComposeUiApi::class)
class EditorKeyCommandToggleCommentTest {

    private fun keyDown(key: Key, ctrl: Boolean = false, meta: Boolean = false) =
        KeyEvent(key = key, type = KeyEventType.KeyDown, isCtrlPressed = ctrl, isMetaPressed = meta)

    @Test
    fun ctrlSlashResolvesToToggleComment() =
        assertEquals(EditorKeyCommand.ToggleComment, resolveCtrlBased(keyDown(Key.Slash, ctrl = true)))

    @Test
    fun bareSlashDoesNotResolve() = assertNull(resolveCtrlBased(keyDown(Key.Slash)))

    @Test
    fun cmdSlashResolvesOnMac() =
        assertEquals(EditorKeyCommand.ToggleComment, resolveMacBased(keyDown(Key.Slash, meta = true)))
}
