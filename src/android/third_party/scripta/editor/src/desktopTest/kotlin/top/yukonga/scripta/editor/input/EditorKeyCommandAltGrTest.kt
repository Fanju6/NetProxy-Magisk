package top.yukonga.scripta.editor.input

import androidx.compose.ui.InternalComposeUiApi
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.KeyEventType
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/**
 * Ctrl 系命令解析不得吞掉 AltGr 组合：Windows 把 AltGr 上报为 Ctrl+Alt（skiko 再把 AltGraph
 * 折进 isAltPressed），AltGr+V 这类「打 @/{ 字符」的按键若被解析成 Paste，字符就永远到不了插入路径。
 * Ctrl+Alt+X 本身也不是 Ctrl+X——带 Alt 的组合不属于 Ctrl 系快捷键。
 */
@OptIn(InternalComposeUiApi::class)
class EditorKeyCommandAltGrTest {

    private fun keyDown(key: Key, ctrl: Boolean = false, alt: Boolean = false) =
        KeyEvent(key = key, type = KeyEventType.KeyDown, isCtrlPressed = ctrl, isAltPressed = alt)

    @Test
    fun ctrlAltDoesNotResolveToPaste() = assertNull(resolveCtrlBased(keyDown(Key.V, ctrl = true, alt = true)))

    @Test
    fun ctrlAltDoesNotResolveToSelectAll() = assertNull(resolveCtrlBased(keyDown(Key.A, ctrl = true, alt = true)))

    @Test
    fun ctrlAltHomeDoesNotResolveToDocStart() {
        // MoveHome 无 Ctrl 也解析（LineStart），带 Alt 时两者都不该出——那可能是 AltGr 层的键。
        assertNull(resolveCtrlBased(keyDown(Key.MoveHome, ctrl = true, alt = true)))
        assertNull(resolveCtrlBased(keyDown(Key.MoveHome, alt = true)))
    }

    @Test
    fun plainCtrlStillResolves() {
        assertEquals(EditorKeyCommand.Paste, resolveCtrlBased(keyDown(Key.V, ctrl = true)))
        assertEquals(EditorKeyCommand.SelectAll, resolveCtrlBased(keyDown(Key.A, ctrl = true)))
    }
}
