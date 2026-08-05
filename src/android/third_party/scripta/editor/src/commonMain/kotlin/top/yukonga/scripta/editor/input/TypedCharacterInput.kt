package top.yukonga.scripta.editor.input

import androidx.compose.ui.input.key.KeyEvent
import top.yukonga.scripta.editor.EditorEngine

/**
 * 平台可打印字符输入回退。桌面上无 IME 参与的普通字符（如 US 键盘直接敲 Latin/数字/符号）经 KeyEvent
 * 到达、不走自管输入会话的 editText（只有 IME 提交/预编辑才走 editText），需在 onKeyEvent 里补插；
 * Android 的硬件键盘字符由自管 IME 的 `EditorInputConnection.sendKeyEvent` 处理，返回 false 放行以避免双插。
 * 返回是否已消费该按键。
 */
expect fun insertTypedCharacter(engine: EditorEngine, event: KeyEvent, readOnly: Boolean): Boolean
