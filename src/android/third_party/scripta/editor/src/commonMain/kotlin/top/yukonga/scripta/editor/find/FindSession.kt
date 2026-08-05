package top.yukonga.scripta.editor.find

import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import top.yukonga.scripta.editor.EditorEngine

/** 某文档行内的一段匹配高亮：[startCol, endCol) + 所属匹配下标（画布据此区分当前匹配强调色）。 */
data class FindSpan(val startCol: Int, val endCol: Int, val matchIndex: Int)

/**
 * 查找 / 替换会话：查找条 UI 与画布高亮共用的状态机。持引擎引用——导航「选中当前匹配」直接走
 * [EditorEngine.setSelection]，视口露出由既有 keep-in-view 机制完成；替换走成组撤销的引擎原语
 * （单处与全部替换各成一个撤销单元）。
 *
 * [refresh] 由视图层在（可见 && 查询/开关/文档版本变化）时调用重算；重算后的当前匹配保持稳定：
 * 选区恰为某匹配时保持之，否则取光标之后的第一个匹配（替换后自然落到下一处待替换项）。
 * 高亮渲染经 [spansForLine] 取行内列区间（跨行匹配已按行裁剪），draw 阶段读、随状态失效重绘。
 */
@Stable
class FindSession internal constructor(private val engine: EditorEngine) {

    var visible: Boolean by mutableStateOf(false)
        private set
    var replaceVisible: Boolean by mutableStateOf(false)
        private set

    var query: String by mutableStateOf("")
    var replacement: String by mutableStateOf("")
    var caseSensitive: Boolean by mutableStateOf(false)
    var wholeWord: Boolean by mutableStateOf(false)
    var useRegex: Boolean by mutableStateOf(false)

    var result: FindResult by mutableStateOf(FindResult.Empty)
        private set
    var activeIndex: Int by mutableIntStateOf(-1)
        private set

    // 行 → 行内匹配段（跨行匹配已裁剪成逐行段）。快照 state：画布 draw 阶段读，refresh 换表即重绘。
    private var spansByLine: Map<Int, List<FindSpan>> by mutableStateOf(emptyMap())

    fun open(withReplace: Boolean) {
        visible = true
        // 两种模式互斥：纯查找不提供替换能力，打开时收起已展开的替换行。
        replaceVisible = withReplace
        // 有单行选区时以选中文本预填查询（多行选区不预填——查询框是单行语义）。
        val selected = engine.selectedText()
        if (!selected.isNullOrEmpty() && !selected.contains('\n')) query = selected
    }

    fun close() {
        visible = false
        replaceVisible = false
        result = FindResult.Empty
        activeIndex = -1
        spansByLine = emptyMap()
    }

    fun refresh() {
        if (!visible || query.isEmpty()) {
            result = FindResult.Empty
            activeIndex = -1
            spansByLine = emptyMap()
            return
        }
        val r = findAllMatches(engine.getText(), FindQuery(query, caseSensitive, wholeWord, useRegex))
        result = r
        spansByLine = buildSpans(r.matches)
        val ms = r.matches
        if (ms.isEmpty()) {
            activeIndex = -1
            return
        }
        // 当前匹配稳定化：选区恰为某匹配 → 保持；否则取光标（选区尾）之后的第一个（环绕）。
        val (selS, selE) = engine.selectionOffsets()
        val exact = ms.indexOfFirst { it.start == selS && it.end == selE }
        activeIndex = exact.takeIf { it >= 0 } ?: (nextMatchIndex(ms, selE, forward = true) ?: -1)
    }

    fun next() = step(forward = true)

    fun prev() = step(forward = false)

    private fun step(forward: Boolean) {
        val ms = result.matches
        if (ms.isEmpty()) return
        val (selS, selE) = engine.selectionOffsets()
        val from = if (forward) selE else selS
        val i = nextMatchIndex(ms, from, forward) ?: return
        activate(i)
    }

    /** 选中第 [i] 个匹配并滚动露出（选区未变时——如 refresh 后重激活同一匹配——经强制通道露出）。 */
    private fun activate(i: Int) {
        val m = result.matches.getOrNull(i) ?: return
        activeIndex = i
        engine.setSelection(engine.buffer.positionAt(m.start), engine.buffer.positionAt(m.end))
        engine.requestReveal()
    }

    /**
     * 选区恰为当前匹配时替换之并自动选中下一处；否则只定位选中当前匹配（先看清、再次操作才真改文档）。
     */
    fun replaceCurrent() {
        val ms = result.matches
        val i = activeIndex
        if (i !in ms.indices) return
        val m = ms[i]
        val (selS, selE) = engine.selectionOffsets()
        if (selS == m.start && selE == m.end) {
            engine.replaceAllOffsetRanges(listOf(m.start to m.end), replacement)
            refresh() // 光标在替换段末 → 当前匹配落到下一处
            if (activeIndex >= 0) activate(activeIndex)
        } else {
            activate(i)
        }
    }

    /** 全部替换（一个撤销单元）；返回替换数。 */
    fun replaceAll(): Int {
        val ms = result.matches
        if (ms.isEmpty()) return 0
        val n = engine.replaceAllOffsetRanges(ms.map { it.start to it.end }, replacement)
        refresh()
        return n
    }

    fun spansForLine(line: Int): List<FindSpan> = spansByLine[line] ?: emptyList()

    private fun buildSpans(matches: List<MatchRange>): Map<Int, List<FindSpan>> {
        if (matches.isEmpty()) return emptyMap()
        val map = HashMap<Int, MutableList<FindSpan>>()
        matches.forEachIndexed { idx, m ->
            val s = engine.buffer.positionAt(m.start)
            val e = engine.buffer.positionAt(m.end)
            for (line in s.line..e.line) {
                val cS = if (line == s.line) s.column else 0
                val cE = if (line == e.line) e.column else engine.buffer.lineLength(line)
                if (cE > cS || s.line != e.line) {
                    map.getOrPut(line) { mutableListOf() }.add(FindSpan(cS, cE, idx))
                }
            }
        }
        return map
    }
}
