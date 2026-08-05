package top.yukonga.scripta.editor.text

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue

/** 整篇缓存的字符数上限：超过则不常驻缓存，避免大文件多留一份 N 常驻。 */
private const val CACHE_TEXT_LIMIT = 1_000_000

/**
 * 文档模型的 Compose-aware 薄壳：持 [version] 快照 state、做 CRLF 规整，把全部文本语义委托给纯 [PieceTree]
 * （piece-table + treap，编辑与 offset↔(行,列) 均 O(log n)）。公开接口保持不变，消费者（EditorEngine /
 * 渲染）无需改动；offset↔位置换算也由 piece-tree 内建（不再需要独立的行偏移索引）。
 *
 * 不变量：始终至少一行（空文档 = 一行空串）；行内不含 '\n'（CRLF 在 setText/replace 入口规整）。
 */
class TextBuffer(initialText: String = "") {

    private val tree = PieceTree()

    /** 每次改动 +1；是 Compose 快照 state，供渲染层观测。 */
    var version: Int by mutableStateOf(0)
        private set

    // 整篇拼接缓存：小文档未编辑时复用，省 getText / 相等比较的 O(n) 重建；超过 CACHE_TEXT_LIMIT 的大文档
    // 不缓存（那只是一份多余的 N 常驻）。任何改动都置空，见 replace/setText。
    private var cachedText: String? = null

    init {
        setText(initialText)
    }

    val lineCount: Int get() = tree.lineCount

    fun lineText(line: Int): String = tree.lineContent(line)

    fun lineLength(line: Int): Int = tree.lineLength(line)

    fun endPosition(): TextPosition {
        val last = tree.lineCount - 1
        return TextPosition(last, tree.lineLength(last))
    }

    fun clamp(pos: TextPosition): TextPosition {
        val line = pos.line.coerceIn(0, tree.lineCount - 1)
        val col = pos.column.coerceIn(0, tree.lineLength(line))
        return TextPosition(line, col)
    }

    fun textInRange(range: TextRange): String {
        val r = range.normalized()
        val s = tree.offsetAt(clamp(r.start))
        val e = tree.offsetAt(clamp(r.end))
        return tree.substring(s, e - s)
    }

    fun text(): String {
        cachedText?.let { return it }
        val whole = tree.getText()
        if (whole.length <= CACHE_TEXT_LIMIT) cachedText = whole
        return whole
    }

    // --- offset 边界（IME：引擎用扁平 offset 与输入法对话）----------------------------------
    fun offsetAt(pos: TextPosition): Int = tree.offsetAt(clamp(pos))
    fun positionAt(offset: Int): TextPosition = tree.positionAt(offset)
    fun totalLength(): Int = tree.length

    /** 用 [replacement] 替换 [range]；返回插入文本末尾的光标位置。 */
    fun replace(range: TextRange, replacement: String): TextPosition {
        val r = range.normalized()
        val s = tree.offsetAt(clamp(r.start))
        val e = tree.offsetAt(clamp(r.end))
        val repl = normalizeNewlines(replacement)
        tree.delete(s, e - s)
        tree.insert(s, repl)
        cachedText = null
        version++
        return tree.positionAt(s + repl.length)
    }

    fun setText(text: String) {
        val normalized = normalizeNewlines(text)
        tree.reset(normalized)
        // 小文档预热缓存（省首次 getText 重建）；大文档不缓存、避免多留一份 N。
        cachedText = if (normalized.length <= CACHE_TEXT_LIMIT) normalized else null
        version++
    }

    /** 统一换行 CRLF/CR → LF；无 CR 的常见情况不复制整篇（String.replace 无匹配也会新建一份 N）。 */
    private fun normalizeNewlines(raw: String): String =
        if (raw.indexOf('\r') >= 0) raw.replace("\r\n", "\n").replace("\r", "\n") else raw
}
