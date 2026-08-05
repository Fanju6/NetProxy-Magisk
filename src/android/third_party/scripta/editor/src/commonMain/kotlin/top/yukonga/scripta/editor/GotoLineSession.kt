package top.yukonga.scripta.editor

import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import top.yukonga.scripta.editor.text.TextPosition

/**
 * 跳转行号会话：停靠条 UI 的状态机（与查找条同族：停靠进布局、不用 focusable Popup）。
 * [open] 以当前行号（1 基）预填输入——字段挂载态全选，直接键入即覆盖、直接确认即原地不动；
 * [jump] 把合法数字钳制到 [1, 行数] 后光标落该行行首（视口露出走既有 keep-in-view），随后关条。
 */
@Stable
class GotoLineSession internal constructor(private val engine: EditorEngine) {

    var visible: Boolean by mutableStateOf(false)
        private set

    var input: String by mutableStateOf("")

    fun open() {
        input = (engine.caret.line + 1).toString()
        visible = true
    }

    fun close() {
        visible = false
    }

    /** 解析并跳转；非数字（空/溢出之外的垃圾）返回 false、条保持打开。溢出按超界钳到末行。 */
    fun jump(): Boolean {
        val text = input.trim()
        if (text.isEmpty() || text.any { !it.isDigit() }) return false
        val n = text.toLongOrNull() ?: Long.MAX_VALUE // 全数字但超 Long：按极大值钳到末行
        val line = n.coerceIn(1L, engine.buffer.lineCount.toLong()).toInt() - 1
        engine.setCursor(TextPosition(line, 0))
        engine.requestReveal() // 跳到「当前行」时选区不变、快照不失效，须经强制通道露出
        close()
        return true
    }
}
