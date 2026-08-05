package top.yukonga.scripta.editor.input

import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Rect
import top.yukonga.scripta.editor.EditorEngine

/**
 * 平台输入缝：自管 IME 会话（无 BasicTextField），字符/预编辑喂给同一个 [engine]。Android 走 InputConnection，
 * 桌面走 establishTextInputSession 的富 request。[enabled] 为 false（只读）时不建立会话。
 * [caretRectInEditor] 返回光标在编辑器内容本地坐标系的矩形（含滚动/gutter），供桌面把 IME 候选窗定位到光标处；
 * Android 忽略它（软键盘/候选栏由系统输入法自行定位）。
 */
expect fun Modifier.editorTextInput(
    engine: EditorEngine,
    enabled: Boolean,
    caretRectInEditor: () -> Rect?,
): Modifier
