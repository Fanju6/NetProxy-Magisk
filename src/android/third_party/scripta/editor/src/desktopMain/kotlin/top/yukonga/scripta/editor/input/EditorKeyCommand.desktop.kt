package top.yukonga.scripta.editor.input

import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.type
import org.jetbrains.skiko.hostOs

// 桌面按运行时 OS 分流：macOS 用 Cmd/Opt 约定，其余用 Ctrl 系。macOS 约定未经真机验证。
actual fun resolveEditorKeyCommand(event: KeyEvent): EditorKeyCommand? {
    if (event.type != KeyEventType.KeyDown) return null
    return if (hostOs.isMacOS) resolveMacBased(event) else resolveCtrlBased(event)
}
