package top.yukonga.scripta.editor.menu

import androidx.compose.ui.platform.Clipboard
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import top.yukonga.scripta.editor.EditorEngine
import top.yukonga.scripta.editor.input.plainText
import top.yukonga.scripta.editor.input.plainTextClipEntry

/**
 * 剪切 / 复制 / 粘贴 / 全选的统一执行器：硬键盘、触屏悬浮条、桌面右键菜单三条路都调它，杜绝逻辑重复。
 *
 * [readOnly] 以 lambda 读活值——本执行器可能被固定 key 的手势闭包捕获，直接存布尔会读到过期只读态。
 * 新 [Clipboard] 的收发是 suspend，交 [scope] 跑；复制/剪切把选中文本写入剪贴板，粘贴读回插到光标（替换选区）。
 * 只读时剪切/粘贴为 no-op（改文档的动作被拦），复制/全选仍可用。
 */
internal class EditorClipboardActions(
    private val engine: EditorEngine,
    private val clipboard: Clipboard,
    private val scope: CoroutineScope,
    private val readOnly: () -> Boolean,
) {
    fun perform(action: EditorContextAction) {
        when (action) {
            EditorContextAction.Undo -> if (!readOnly()) engine.undo()

            EditorContextAction.Redo -> if (!readOnly()) engine.redo()

            EditorContextAction.Copy -> engine.selectedText()?.let { txt ->
                scope.launch { clipboard.setClipEntry(plainTextClipEntry(txt)) }
            }

            EditorContextAction.Cut -> if (!readOnly()) engine.selectedText()?.let { txt ->
                scope.launch { clipboard.setClipEntry(plainTextClipEntry(txt)) }
                engine.replaceSelection("")
            }

            EditorContextAction.Paste -> if (!readOnly()) scope.launch {
                clipboard.getClipEntry()?.plainText()?.let { engine.insert(it) }
            }

            EditorContextAction.SelectAll -> engine.selectAll()
        }
    }
}
