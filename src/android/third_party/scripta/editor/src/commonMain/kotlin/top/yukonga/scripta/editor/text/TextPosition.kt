package top.yukonga.scripta.editor.text

/** 文档内的一个 (行, 列) 位置。列是该行内的 UTF-16 char 下标。 */
data class TextPosition(val line: Int, val column: Int) : Comparable<TextPosition> {
    override fun compareTo(other: TextPosition): Int =
        if (line != other.line) line.compareTo(other.line) else column.compareTo(other.column)
}

/** 一段区间（选择）。start/end 未必有序，用 [normalized] 归一。 */
data class TextRange(val start: TextPosition, val end: TextPosition) {
    val isEmpty: Boolean get() = start == end

    fun normalized(): TextRange = if (start <= end) this else TextRange(end, start)

    companion object {
        fun cursor(p: TextPosition): TextRange = TextRange(p, p)
    }
}
