package top.yukonga.scripta.editor.input

import androidx.compose.ui.awt.awtEventOrNull
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.isAltPressed
import androidx.compose.ui.input.key.isCtrlPressed
import androidx.compose.ui.input.key.isMetaPressed
import androidx.compose.ui.input.key.utf16CodePoint
import org.jetbrains.skiko.hostOs
import top.yukonga.scripta.editor.EditorEngine

/**
 * 可打印字符判定：修饰组合是「打字」还是「快捷键」。
 *
 * 打字修饰放行——[altGraph]（欧洲布局的 `{ } [ ] @ \` 靠 AltGr 层；Windows 把 AltGr 合成 Ctrl+Alt
 * 上报、skiko 又把 AltGraph 折进 isAltPressed，此时的 Ctrl/Alt 都是伪装）；macOS 纯 Option+字符
 * （√ ø π … 的输入方式，Cmd/Ctrl 一旦参与就回到快捷键语义）。其余 Ctrl/Meta/Alt 是快捷键、拒插。
 *
 * 码点关卡与修饰无关：修饰键(Shift 等)/功能键/IME 切换键报 CHAR_UNDEFINED(0xFFFF)，控制字符与
 * 未分配码点同拒——否则按 Shift 切中英、按功能键都会各插一个 U+FFFF（显示为 ？）。
 */
internal fun shouldInsertTypedChar(
    cp: Int,
    ctrl: Boolean,
    meta: Boolean,
    alt: Boolean,
    altGraph: Boolean,
    isMac: Boolean,
): Boolean {
    val shortcutModifierHeld = when {
        altGraph -> false
        isMac && alt && !ctrl && !meta -> false
        else -> ctrl || meta || alt
    }
    if (shortcutModifierHeld) return false
    return !(cp == 0xFFFF || Character.isISOControl(cp) || !Character.isDefined(cp))
}

/**
 * 桌面：可打印字符经 KeyEvent 到达时插入。仅当非只读、无预编辑（预编辑中的按键归 IME）、且修饰组合
 * 判定为打字（见 [shouldInsertTypedChar]）才插入。AltGraph 位只存在于 AWT 原生事件上，须经
 * awtEventOrNull 读取。IME 提交/预编辑的文本走输入会话的 editText、不经此处，故与之无双插。
 * 码点转字符串用 Character.toChars 以正确处理增补平面。
 */
actual fun insertTypedCharacter(engine: EditorEngine, event: KeyEvent, readOnly: Boolean): Boolean {
    if (readOnly) return false
    if (engine.composing != null) return false
    val insertable = shouldInsertTypedChar(
        cp = event.utf16CodePoint,
        ctrl = event.isCtrlPressed,
        meta = event.isMetaPressed,
        alt = event.isAltPressed,
        altGraph = event.awtEventOrNull?.isAltGraphDown == true,
        isMac = hostOs.isMacOS,
    )
    if (!insertable) return false
    engine.typeCharacter(String(Character.toChars(event.utf16CodePoint))) // 键入原语：带自动配对/跳过闭合
    return true
}
