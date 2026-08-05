package top.yukonga.scripta.editor.input

import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.isAltPressed
import androidx.compose.ui.input.key.isCtrlPressed
import androidx.compose.ui.input.key.isMetaPressed
import androidx.compose.ui.input.key.isShiftPressed
import androidx.compose.ui.input.key.key

/** 语义编辑命令：把平台各异的按键组合归一，供 onKeyEvent 派发（导航命令的 Shift 由调用方另读，决定是否扩选；
 *  撤销/重做的 Shift 参与命令区分，在此读取）。 */
enum class EditorKeyCommand {
    SelectAll, Copy, Cut, Paste, Undo, Redo,
    Find, Replace, FindNext, FindPrev, GotoLine, Completion, ToggleComment,
    WordLeft, WordRight, LineStart, LineEnd, DocStart, DocEnd, PageUp, PageDown,
}

/** 平台主修饰键映射（Windows/Linux/Android，硬件键盘）：Ctrl 系。
 *  Alt 参与的组合一律不解析：Ctrl+Alt+X 不是 Ctrl+X；更要紧的是 Windows 把 AltGr 上报为 Ctrl+Alt
 *  （skiko 再把 AltGraph 折进 isAltPressed），欧洲布局按 AltGr+V 打 `@` 若被解析成 Paste，
 *  字符就永远到不了插入路径（insertTypedCharacter）。 */
internal fun resolveCtrlBased(e: KeyEvent): EditorKeyCommand? {
    if (e.isAltPressed) return null
    val ctrl = e.isCtrlPressed
    return when (e.key) {
        Key.A -> if (ctrl) EditorKeyCommand.SelectAll else null
        Key.C -> if (ctrl) EditorKeyCommand.Copy else null
        Key.X -> if (ctrl) EditorKeyCommand.Cut else null
        Key.V -> if (ctrl) EditorKeyCommand.Paste else null
        Key.Z -> if (ctrl) (if (e.isShiftPressed) EditorKeyCommand.Redo else EditorKeyCommand.Undo) else null
        Key.Y -> if (ctrl) EditorKeyCommand.Redo else null
        Key.F -> if (ctrl) EditorKeyCommand.Find else null
        Key.H -> if (ctrl) EditorKeyCommand.Replace else null
        Key.G -> if (ctrl) EditorKeyCommand.GotoLine else null
        Key.Spacebar -> if (ctrl) EditorKeyCommand.Completion else null
        Key.Slash -> if (ctrl) EditorKeyCommand.ToggleComment else null
        Key.F3 -> if (e.isShiftPressed) EditorKeyCommand.FindPrev else EditorKeyCommand.FindNext
        Key.DirectionLeft -> if (ctrl) EditorKeyCommand.WordLeft else null
        Key.DirectionRight -> if (ctrl) EditorKeyCommand.WordRight else null
        Key.MoveHome -> if (ctrl) EditorKeyCommand.DocStart else EditorKeyCommand.LineStart
        Key.MoveEnd -> if (ctrl) EditorKeyCommand.DocEnd else EditorKeyCommand.LineEnd
        Key.PageUp -> EditorKeyCommand.PageUp
        Key.PageDown -> EditorKeyCommand.PageDown
        else -> null
    }
}

/** macOS 映射：Cmd(Meta) 剪贴板/行/文档，Opt(Alt) 按词。约定，未经真机验证。 */
internal fun resolveMacBased(e: KeyEvent): EditorKeyCommand? {
    val cmd = e.isMetaPressed
    val opt = e.isAltPressed
    return when (e.key) {
        Key.A -> if (cmd) EditorKeyCommand.SelectAll else null
        Key.C -> if (cmd) EditorKeyCommand.Copy else null
        Key.X -> if (cmd) EditorKeyCommand.Cut else null
        Key.V -> if (cmd) EditorKeyCommand.Paste else null
        Key.Z -> if (cmd) (if (e.isShiftPressed) EditorKeyCommand.Redo else EditorKeyCommand.Undo) else null
        Key.F -> when {
            cmd && opt -> EditorKeyCommand.Replace
            cmd -> EditorKeyCommand.Find
            else -> null
        }
        Key.G -> if (cmd) (if (e.isShiftPressed) EditorKeyCommand.FindPrev else EditorKeyCommand.FindNext) else null
        Key.L -> if (cmd) EditorKeyCommand.GotoLine else null
        Key.Spacebar -> if (e.isCtrlPressed) EditorKeyCommand.Completion else null
        Key.Slash -> if (cmd) EditorKeyCommand.ToggleComment else null
        Key.DirectionLeft -> when {
            opt -> EditorKeyCommand.WordLeft
            cmd -> EditorKeyCommand.LineStart
            else -> null
        }
        Key.DirectionRight -> when {
            opt -> EditorKeyCommand.WordRight
            cmd -> EditorKeyCommand.LineEnd
            else -> null
        }
        Key.DirectionUp -> if (cmd) EditorKeyCommand.DocStart else null
        Key.DirectionDown -> if (cmd) EditorKeyCommand.DocEnd else null
        Key.MoveHome -> EditorKeyCommand.DocStart
        Key.MoveEnd -> EditorKeyCommand.DocEnd
        Key.PageUp -> EditorKeyCommand.PageUp
        Key.PageDown -> EditorKeyCommand.PageDown
        else -> null
    }
}

/**
 * 把一个 KeyEvent 解析成语义命令（仅 KeyDown；解析失败返回 null，交回 onKeyEvent 的平台无关分支）。
 * Shift 不影响命令本身（只在调用方决定是否扩选），故此处不读 Shift。
 */
expect fun resolveEditorKeyCommand(event: KeyEvent): EditorKeyCommand?
