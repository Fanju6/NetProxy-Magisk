package top.yukonga.scripta.editor.input

import androidx.compose.ui.input.key.KeyEvent
import top.yukonga.scripta.editor.EditorEngine

// Android：硬件键盘字符由自管 IME 的 EditorInputConnection.sendKeyEvent 处理，此处放行避免双插。
actual fun insertTypedCharacter(engine: EditorEngine, event: KeyEvent, readOnly: Boolean): Boolean = false
