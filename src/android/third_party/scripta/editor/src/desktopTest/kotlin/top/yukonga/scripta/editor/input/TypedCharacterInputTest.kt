package top.yukonga.scripta.editor.input

import androidx.compose.ui.InternalComposeUiApi
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.KeyEventType
import top.yukonga.scripta.editor.EditorEngine
import java.awt.event.InputEvent
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * [shouldInsertTypedChar] 判定矩阵：AltGr（含 Windows 的 Ctrl+Alt 伪装）与 macOS 纯 Option
 * 是打字修饰、放行；真快捷键修饰（Ctrl/Meta/独立 Alt）拒绝；无字符码点一律拒绝。
 */
class TypedCharacterInputTest {

    private fun decide(
        cp: Int = 'a'.code,
        ctrl: Boolean = false,
        meta: Boolean = false,
        alt: Boolean = false,
        altGraph: Boolean = false,
        isMac: Boolean = false,
    ) = shouldInsertTypedChar(cp, ctrl, meta, alt, altGraph, isMac)

    @Test
    fun plainPrintableInserts() = assertTrue(decide())

    @Test
    fun ctrlIsShortcutModifier() = assertFalse(decide(ctrl = true))

    @Test
    fun metaIsShortcutModifier() = assertFalse(decide(meta = true))

    @Test
    fun standaloneAltIsShortcutModifierOffMac() = assertFalse(decide(alt = true))

    @Test
    fun windowsAltGrDisguisedAsCtrlAltInserts() {
        // Windows 把 AltGr 合成 Ctrl+Alt 上报，AWT 额外给出 AltGraph 位——这是打字，不是快捷键。
        assertTrue(decide(cp = '@'.code, ctrl = true, alt = true, altGraph = true))
    }

    @Test
    fun linuxAltGrFoldedIntoAltInserts() {
        // skiko 的 isAltPressed 把 AltGraph 折叠进来（X11 上 AltGr 不带 Ctrl）。
        assertTrue(decide(cp = '{'.code, alt = true, altGraph = true))
    }

    @Test
    fun macPlainOptionInserts() {
        // macOS Option+字符就是输入特殊字符（√ ø π …）的方式。
        assertTrue(decide(cp = 0x221A /* √ */, alt = true, isMac = true))
    }

    @Test
    fun macCmdOptionIsShortcutChord() = assertFalse(decide(alt = true, meta = true, isMac = true))

    @Test
    fun macCtrlOptionIsShortcutChord() = assertFalse(decide(alt = true, ctrl = true, isMac = true))

    @Test
    fun charUndefinedRejectedEvenWithAltGraph() {
        // AltGr+功能键 / 死键按下时 keyChar 是 CHAR_UNDEFINED。
        assertFalse(decide(cp = 0xFFFF, ctrl = true, alt = true, altGraph = true))
    }

    @Test
    fun controlCharacterRejected() = assertFalse(decide(cp = '\b'.code))

    @Test
    fun undefinedCodePointRejected() = assertFalse(decide(cp = 0x000E0000))

    @OptIn(InternalComposeUiApi::class)
    @Test
    fun altGraphFlagIsReadFromTheAwtNativeEvent() {
        // 端到端接线：AltGraph 位只存在于 AWT 原生事件上（Compose 修饰位把它折进 isAltPressed），
        // actual 实现须经 awtEventOrNull 读取，否则 Windows AltGr 组合会被当成 Ctrl+Alt 快捷键拒插。
        val engine = EditorEngine("")
        val awt = java.awt.event.KeyEvent(
            object : java.awt.Component() {},
            java.awt.event.KeyEvent.KEY_PRESSED,
            0L,
            InputEvent.CTRL_DOWN_MASK or InputEvent.ALT_DOWN_MASK or InputEvent.ALT_GRAPH_DOWN_MASK,
            java.awt.event.KeyEvent.VK_UNDEFINED,
            '@',
        )
        val ev = KeyEvent(
            key = Key.A,
            type = KeyEventType.KeyDown,
            codePoint = '@'.code,
            isCtrlPressed = true,
            isAltPressed = true,
            nativeEvent = awt,
        )
        assertTrue(insertTypedCharacter(engine, ev, readOnly = false))
        assertEquals("@", engine.getText())
    }
}
