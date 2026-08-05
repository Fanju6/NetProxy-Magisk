package top.yukonga.scripta.editor.input

import androidx.compose.ui.InternalComposeUiApi
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.KeyEventType
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** 补全快捷键在各平台统一使用 Ctrl+Space，避免占用 macOS 的 Cmd+Space。 */
@OptIn(InternalComposeUiApi::class)
class EditorKeyCommandCompletionTest {
    private fun keyDown(key: Key, ctrl: Boolean = false) =
        KeyEvent(key = key, type = KeyEventType.KeyDown, isCtrlPressed = ctrl)

    @Test
    fun ctrlSpaceResolvesToCompletion() =
        assertEquals(EditorKeyCommand.Completion, resolveCtrlBased(keyDown(Key.Spacebar, ctrl = true)))

    @Test
    fun ctrlSpaceResolvesToCompletionOnMac() =
        assertEquals(EditorKeyCommand.Completion, resolveMacBased(keyDown(Key.Spacebar, ctrl = true)))

    @Test
    fun bareSpaceDoesNotResolve() = assertNull(resolveCtrlBased(keyDown(Key.Spacebar)))
}
