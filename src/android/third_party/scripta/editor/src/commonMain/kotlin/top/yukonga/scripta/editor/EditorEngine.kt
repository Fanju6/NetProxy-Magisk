package top.yukonga.scripta.editor

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import top.yukonga.scripta.editor.text.EditKind
import top.yukonga.scripta.editor.text.SelectionState
import top.yukonga.scripta.editor.text.TextBuffer
import top.yukonga.scripta.editor.text.TextEdit
import top.yukonga.scripta.editor.text.TextPosition
import top.yukonga.scripta.editor.text.TextRange
import top.yukonga.scripta.editor.text.UndoHistory
import top.yukonga.scripta.editor.text.UndoStep

/**
 * 自 [EditorEngine.consumeDirty] 上次消费以来编辑影响的行范围：非结构性时 `[from, endExclusive)`
 * 之外的行内容保证未变；[structural] = 行数变过，行号已错位。
 */
internal data class DirtyRange(val from: Int, val endExclusive: Int, val structural: Boolean)

/**
 * 一次行数变化的替换：起始行 [startLine] 起的 [oldLines] 行被 [newLines] 行取代（都含起始行本身，恒 ≥ 1）。
 * 行号是该编辑应用时刻的实时行号，多条按发生顺序依次应用才成立——与 [DirtyRange] 的并集语义不同，
 * splice 不能合并（后一条的行号建立在前一条已生效的坐标上）。
 */
internal data class LineSplice(val startLine: Int, val oldLines: Int, val newLines: Int)

/**
 * 编辑引擎：持有 TextBuffer + 归一化选择 + 可选 composing 区间，承载全部编辑/选择/IME 语义
 * （由 spike 的 MiniEditorState 推广到 (行,列) 坐标）。offset 只在 IME 边界经 piece-tree（buffer.offsetAt/positionAt）换算。
 *
 * selection 恒为归一化（start ≤ end）；cursor 即 start == end。composing 为 null 表示无输入法预编辑。
 */
class EditorEngine(initialText: String = "") {

    companion object {
        /** 缩进单位：Tab 键插入 / 块缩进 / 反缩进共用的一档宽度。 */
        const val INDENT_UNIT = "    "

        // 自动配对表与闭字符集（跳过闭合判定用；引号开闭同字符、两边都在）。
        private val AUTO_PAIRS = mapOf('(' to ')', '[' to ']', '{' to '}', '"' to '"', '\'' to '\'')
        private val AUTO_PAIR_CLOSERS = AUTO_PAIRS.values.toSet()
    }

    val buffer = TextBuffer()

    // 方向性选区：start=anchor（固定端）、end=head（活动端），允许无序。必须存方向——归一化会丢失它，
    // 只存归一化选区时 Shift+左/上 的第二步会塌成光标、无法继续反向扩选（B1）。单一 state 承载而非
    // anchor/head/selection 三份：selection/caret 皆为派生读，单写点让「selection == normalized(anchor,head)」
    // 按构造成立，同帧读 selection 与 caret 也不会撕裂（出自同一次快照读取）。
    private var directional: TextRange by mutableStateOf(TextRange.cursor(TextPosition(0, 0)))

    /** 归一化选区（start ≤ end；空选区 = 光标）。快照 state 派生：组合中读取自动订阅。 */
    val selection: TextRange get() = directional.normalized()

    private val anchor: TextPosition get() = directional.start
    private val head: TextPosition get() = directional.end

    /** 选区固定端（anchor）。供 controller 的状态保存（Saver）取方向性选区；活动端走 [caret]。 */
    internal val selectionAnchor: TextPosition get() = anchor

    /** 选区活动端（head）：光标闪烁 / keep-in-view 应跟随它。空选区时 head==anchor==光标位置。 */
    val caret: TextPosition get() = head

    /** 强制露出请求计数：编程定位（controller.select/jumpTo、查找导航、跳转行号）bump 它，keep-in-view
     *  把它并入观测值——目标与当前选区相等时快照不失效（结构相等去重），没有本通道「跳到原地」就不会
     *  滚回。刻意不进 setCursor/setSelection 通用路径：手势路径的 clip-gate 行为回归敏感，编程入口够用。 */
    internal var revealTick: Int by mutableStateOf(0)
        private set

    internal fun requestReveal() {
        revealTick++
    }

    var composing: TextRange? by mutableStateOf(null)
        private set

    /** 整篇替换（setText / 换文档 / 打开文件）的代次，编辑不改。视图层据此重置「跨文档累积」的量
     *  （如横向滚动范围 widestSeen），避免旧文档的最宽行残留到新文档。 */
    var contentGeneration: Int by mutableStateOf(0)
        private set

    // 上下移动记忆的目标列：穿过较短行时列被临时夹小，但回到长行仍恢复原列。仅纵向移动保持，
    // 横向移动 / 编辑 / 显式设光标都清空（见 setSelection 与各编辑原语）。
    private var desiredColumn: Int? = null

    val selStart: TextPosition get() = selection.start
    val selEnd: TextPosition get() = selection.end

    /** 是否处于「上下移动记忆目标列」状态。视图层据此在纯纵向导航时不做横向随动——目标列夹变会让横向随动
     *  把视口来回 snap，而此时横向意图本就固定在目标列、视口应保持稳定。横向移动/编辑/点按会清空目标列。 */
    val hasGoalColumn: Boolean get() = desiredColumn != null

    interface ImeListener {
        fun onSelectionChanged(selStart: Int, selEnd: Int, composingStart: Int, composingEnd: Int)
    }

    var imeListener: ImeListener? = null
    var requestShowKeyboard: (() -> Unit)? = null

    /** 整篇替换（setText/换文档）后的平台通知：Android 侧接 restartInput——换文档可能发生在预编辑
     *  进行中，仅靠 updateSelection(composing=-1) 不足以让所有输入法丢弃旧文档的组合状态。 */
    var onDocumentReplaced: (() -> Unit)? = null

    private val history = UndoHistory()

    // 待消费的行结构变化，按发生顺序（见 [LineSplice] 关于为何不能像 dirty 那样并集合并）。
    // 须在 init 之前初始化：构造走 setText，其中会清本队列。
    private val pendingSplices = ArrayList<LineSplice>()

    // 快照 state 供 UI（菜单置灰 / 工具栏按钮）观测；随每次历史变动同步。
    var canUndo: Boolean by mutableStateOf(false)
        private set
    var canRedo: Boolean by mutableStateOf(false)
        private set

    private var batchDepth = 0

    init {
        if (initialText.isNotEmpty()) setText(initialText)
    }

    // --- 批处理 / IME 通知 -------------------------------------------------------------------

    fun beginBatch() {
        batchDepth++
    }

    fun endBatch(): Boolean {
        if (batchDepth > 0) batchDepth--
        if (batchDepth == 0) fireIme()
        return batchDepth > 0
    }

    private fun maybeNotify() {
        if (batchDepth == 0) fireIme()
    }

    fun fireIme() {
        val l = imeListener ?: return
        val s = selectionOffsets()
        val c = composingOffsets()
        l.onSelectionChanged(s.first, s.second, c.first, c.second)
    }

    fun selectionOffsets(): Pair<Int, Int> = buffer.offsetAt(selStart) to buffer.offsetAt(selEnd)

    fun composingOffsets(): Pair<Int, Int> {
        val c = composing ?: return -1 to -1
        return buffer.offsetAt(c.start) to buffer.offsetAt(c.end)
    }

    // --- 文本进出 ----------------------------------------------------------------------------

    fun getText(): String = buffer.text()

    fun setText(text: String) {
        buffer.setText(text)
        collapseCaret(TextPosition(0, 0)) // 打开文件光标停在文首
        composing = null
        desiredColumn = null
        contentGeneration++ // 换文档：视图层据此重置横向范围等跨文档累积量
        history.clear() // 整篇替换 = 换文档，旧文档的编辑历史不再适用
        syncHistory()
        onDocumentReplaced?.invoke() // 平台侧重启输入连接（预编辑中换文档，见声明处）
        markDirty(0, Int.MAX_VALUE, structural = true)
        pendingSplices.clear() // 换代后视图按新文档行数整建行索引，残留旧文档 splice 若被应用会错位
        maybeNotify()
    }

    // --- 撤销 / 重做 -------------------------------------------------------------------------

    fun undo(): Boolean {
        val step = history.undo() ?: return false
        applyStep(step)
        return true
    }

    fun redo(): Boolean {
        val step = history.redo() ?: return false
        applyStep(step)
        return true
    }

    /** 回放一步历史：按序应用增量，再把 anchor/head 恢复到快照（保留选区方向）。 */
    private fun applyStep(step: UndoStep) {
        composing = null
        desiredColumn = null
        for (e in step.edits) {
            val start = buffer.positionAt(e.offset)
            val end = buffer.positionAt(e.offset + e.removed.length)
            markReplaceDirty(start.line, e.removed, e.inserted)
            buffer.replace(TextRange(start, end), e.inserted)
        }
        directional = TextRange(buffer.positionAt(step.selection.anchor), buffer.positionAt(step.selection.head))
        syncHistory()
        maybeNotify()
    }

    private fun syncHistory() {
        canUndo = history.canUndo
        canRedo = history.canRedo
    }

    /** 当前 anchor/head 的扁平 offset 快照（撤销单元的选区端）。 */
    private fun selectionSnapshot() = SelectionState(buffer.offsetAt(anchor), buffer.offsetAt(head))

    /**
     * 执行替换并收集增量：removed 在替换前取原文，inserted 从缓冲回读——回读到的才是真正入档的文本
     * （buffer.replace 会把 CRLF 规整成 LF，直接记入参会与文档不符、重做长度错位）。
     */
    private fun replaceCollecting(range: TextRange, text: String): Pair<TextEdit, TextPosition> {
        val r = range.normalized()
        val start = buffer.clamp(r.start)
        val startOff = buffer.offsetAt(start)
        val removed = buffer.textInRange(r)
        val newCaret = buffer.replace(r, text)
        val inserted = buffer.textInRange(TextRange(start, newCaret))
        markReplaceDirty(start.line, removed, inserted)
        return TextEdit(startOff, removed, inserted) to newCaret
    }

    /** 单码点判定（含代理对）：单字符键入的合并粒度。 */
    private fun isSingleCodePoint(s: String): Boolean =
        s.length == 1 || (s.length == 2 && s[0].isHighSurrogate() && s[1].isLowSurrogate())

    // 自上次 consumeDirty 以来编辑触及的行范围并集 + 是否有行数变化（初始 = 整篇结构性待处理）。
    private var dirtyFrom = 0
    private var dirtyEndEx = Int.MAX_VALUE
    private var dirtyStructural = true
    private var hasDirty = true

    /**
     * 取走「自上次消费以来的编辑影响范围」并复位；无编辑时返回 null。
     * [DirtyRange.structural] = 行数变过（行号错位，按行号索引的缓存尾部不可再比对复用）；
     * 非结构性时 `[from, endExclusive)` 之外的行内容保证未变。多次编辑取范围并集、
     * 结构性按或——非结构性编辑不移动行号，并集在消费前始终对同一套行号成立。
     */
    internal fun consumeDirty(): DirtyRange? {
        if (!hasDirty) return null
        val d = DirtyRange(dirtyFrom, dirtyEndEx, dirtyStructural)
        hasDirty = false
        dirtyFrom = Int.MAX_VALUE
        dirtyEndEx = 0
        dirtyStructural = false
        return d
    }

    /** 取走「自上次消费以来的行结构变化」并清空；无则空表。供视觉行索引增量 splice，免于整表重建。 */
    internal fun consumeLineSplices(): List<LineSplice> {
        if (pendingSplices.isEmpty()) return emptyList()
        val out = pendingSplices.toList()
        pendingSplices.clear()
        return out
    }

    private fun markDirty(fromLine: Int, endExclusive: Int, structural: Boolean) {
        hasDirty = true
        if (fromLine < dirtyFrom) dirtyFrom = fromLine
        if (endExclusive > dirtyEndEx) dirtyEndEx = endExclusive
        if (structural) dirtyStructural = true
    }

    /** 一次替换对行的影响：从起始行到「删除/插入较多者」的行尾界；换行数不等即结构性，并记一条行 splice。 */
    private fun markReplaceDirty(startLine: Int, removed: String, inserted: String) {
        val rb = removed.count { it == '\n' }
        val ib = inserted.count { it == '\n' }
        markDirty(startLine, startLine + maxOf(rb, ib) + 1, structural = rb != ib)
        if (rb != ib) pendingSplices.add(LineSplice(startLine, rb + 1, ib + 1))
    }

    /**
     * 把 [ranges]（扁平 offset 区间 (start, end)，升序且互不重叠）全部替换为 [replacement]，
     * 整个操作为一个撤销单元；返回替换数。降序应用——先替换靠后的区间，前面区间的 offset 不受影响。
     * 光标落在首个（offset 最小）替换段末尾：降序应用后该 offset 已定格。
     */
    fun replaceAllOffsetRanges(ranges: List<Pair<Int, Int>>, replacement: String): Int {
        if (ranges.isEmpty()) return 0
        val selBefore = selectionSnapshot()
        val edits = ArrayList<TextEdit>(ranges.size)
        for ((s, e) in ranges.asReversed()) {
            val (edit, _) = replaceCollecting(TextRange(buffer.positionAt(s), buffer.positionAt(e)), replacement)
            edits.add(edit)
        }
        composing = null
        desiredColumn = null
        collapseCaret(buffer.positionAt(ranges.first().first + edits.last().inserted.length))
        val selAfter = selectionSnapshot()
        history.beginGroup()
        for (edit in edits) history.record(edit, selBefore, selAfter, EditKind.Other)
        history.endGroup()
        syncHistory()
        maybeNotify()
        return ranges.size
    }

    /** Tab 块缩进：选区触及的每个非空行行首插入 [INDENT_UNIT]（光标时只作用当前行）。
     *  整块一个撤销单元；选区端点随文本平移，行首端点钉在行首——反复缩进/反缩进时选区稳定贴边。 */
    fun indentSelectedLines() {
        val edits = affectedLines().mapNotNull { line ->
            if (buffer.lineLength(line) == 0) return@mapNotNull null // 空行不留尾随空白
            val p = buffer.offsetAt(TextPosition(line, 0))
            p to p
        }
        applyLineStartEdits(edits, INDENT_UNIT)
    }

    /** Shift+Tab 反缩进：每行行首删一个缩进级——行首是 Tab 删 1 个 Tab，否则删至多 [INDENT_UNIT] 个空格。 */
    fun outdentSelectedLines() {
        val edits = affectedLines().mapNotNull { line ->
            val text = buffer.lineText(line)
            val k = if (text.startsWith("\t")) 1 else {
                var n = 0
                while (n < INDENT_UNIT.length && n < text.length && text[n] == ' ') n++
                n
            }
            if (k == 0) return@mapNotNull null
            val p = buffer.offsetAt(TextPosition(line, 0))
            p to p + k
        }
        applyLineStartEdits(edits, "")
    }

    /**
     * 行注释切换：选区触及的非空白行全部已带 [prefix] 则逐行取消（删前缀及其后的惯例空格），否则全部
     * 加一层——在各行的**最小缩进列**对齐插入 `"[prefix] "`（深缩进行的注释符与浅行成一列）。空白行
     * 两个方向都跳过、也不参与判定；混合状态加一层，再切一次恰好还原。一个撤销单元、选区端点随文本
     * 平移（复用 [applyLineStartEdits]）。前缀由调用方按语言提供（高亮插件的 lineCommentPrefix）。
     */
    fun toggleLineComment(prefix: String) {
        val lines = affectedLines()
        var minIndent = Int.MAX_VALUE
        var allCommented = true
        var sawContent = false
        for (line in lines) {
            val text = buffer.lineText(line)
            if (text.isBlank()) continue
            sawContent = true
            var i = 0
            while (i < text.length && (text[i] == ' ' || text[i] == '\t')) i++
            if (i < minIndent) minIndent = i
            if (!text.startsWith(prefix, i)) allCommented = false
        }
        if (!sawContent) return
        if (allCommented) {
            val ranges = lines.mapNotNull { line ->
                val text = buffer.lineText(line)
                if (text.isBlank()) return@mapNotNull null
                var i = 0
                while (i < text.length && (text[i] == ' ' || text[i] == '\t')) i++
                var end = i + prefix.length
                if (end < text.length && text[end] == ' ') end++
                val p = buffer.offsetAt(TextPosition(line, 0))
                (p + i) to (p + end)
            }
            applyLineStartEdits(ranges, "")
        } else {
            val ranges = lines.mapNotNull { line ->
                if (buffer.lineText(line).isBlank()) return@mapNotNull null
                buffer.offsetAt(TextPosition(line, minIndent)).let { it to it }
            }
            applyLineStartEdits(ranges, "$prefix ")
        }
    }

    /**
     * 块注释切换：无行注释语言（HTML/CSS 类）的注释切换回退。作用区 = 选区触及行中首、末两个非空白行：
     * 首行内容头带 [open] 且末行内容尾带 [close]（两标记不共享字符）则拆除（连同各自内侧的惯例空格，
     * 同一空格不被两侧重复认领），否则整块包一层——`"[open] "` 插在首行内容头、`" [close]"` 缀在末行
     * 内容尾。全空白 no-op；只有半边标记算未包裹，包一层、再切一次恰好还原。一个撤销单元、端点随文本平移。
     */
    fun toggleBlockComment(open: String, close: String) {
        var first = -1
        var last = -1
        for (line in affectedLines()) {
            if (buffer.lineText(line).isBlank()) continue
            if (first < 0) first = line
            last = line
        }
        if (first < 0) return
        val firstText = buffer.lineText(first)
        val lastText = buffer.lineText(last)
        var i = 0
        while (i < firstText.length && (firstText[i] == ' ' || firstText[i] == '\t')) i++
        var j = lastText.length
        while (j > 0 && (lastText[j - 1] == ' ' || lastText[j - 1] == '\t')) j--
        val openAt = buffer.offsetAt(TextPosition(first, i))
        val closeEnd = buffer.offsetAt(TextPosition(last, j))
        val wrapped = firstText.startsWith(open, i) && lastText.substring(0, j).endsWith(close) &&
            closeEnd - close.length >= openAt + open.length
        if (wrapped) {
            var openEnd = i + open.length
            if (openEnd < firstText.length && firstText[openEnd] == ' ') openEnd++
            var closeStart = j - close.length
            if (closeStart > 0 && lastText[closeStart - 1] == ' ') closeStart--
            val openEndOff = buffer.offsetAt(TextPosition(first, openEnd))
            val closeStartOff = maxOf(buffer.offsetAt(TextPosition(last, closeStart)), openEndOff)
            applyRangeEdits(listOf(Triple(openAt, openEndOff, ""), Triple(closeStartOff, closeEnd, "")))
        } else {
            applyRangeEdits(listOf(Triple(openAt, openAt, "$open "), Triple(closeEnd, closeEnd, " $close")))
        }
    }

    /** 选区触及的行区间；非空选区终点恰在行首时末行不算（选到下一行行首 ≠ 选中那一行）。 */
    private fun affectedLines(): IntRange {
        val s = selStart
        val e = selEnd
        val endLine = if (e.line > s.line && e.column == 0) e.line - 1 else e.line
        return s.line..endLine
    }

    /** [applyRangeEdits] 的同文本便捷层：把 [ranges] 全部替换为同一个 [replacement]。 */
    private fun applyLineStartEdits(ranges: List<Pair<Int, Int>>, replacement: String) =
        applyRangeEdits(ranges.map { (s, e) -> Triple(s, e, replacement) })

    /**
     * 批量区间编辑核心：[edits]（升序、互不重叠的 (start, end, 替换文本)）一次应用，一个撤销单元。
     * 与 [replaceAllOffsetRanges] 的差别在选区语义：不折叠光标，而是把 anchor/head 各自按编辑平移
     * （落在被删区段内的端点贴回编辑点），保持选区方向与覆盖直觉。
     */
    private fun applyRangeEdits(edits: List<Triple<Int, Int, String>>) {
        if (edits.isEmpty()) return
        val selBefore = selectionSnapshot()
        var aOff = selBefore.anchor
        var hOff = selBefore.head
        for ((s, e, replacement) in edits.asReversed()) { // 平移降序累计：晚区间先算，端点与各区间起点的比较
            val removed = e - s                           // 都在原 offset 坐标下成立——升序会让早区间的净增把
            aOff = shiftForEdit(aOff, s, removed, replacement.length) // 两区间之间的端点数值推过晚区间起点、多算一次
            hOff = shiftForEdit(hOff, s, removed, replacement.length)
        }
        val applied = ArrayList<TextEdit>(edits.size)
        for ((s, e, replacement) in edits.asReversed()) { // 降序应用：前面区间的 offset 不受影响
            val (edit, _) = replaceCollecting(TextRange(buffer.positionAt(s), buffer.positionAt(e)), replacement)
            applied.add(edit)
        }
        composing = null
        desiredColumn = null
        directional = TextRange(buffer.positionAt(aOff), buffer.positionAt(hOff))
        val selAfter = SelectionState(aOff, hOff)
        history.beginGroup()
        for (edit in applied) history.record(edit, selBefore, selAfter, EditKind.Other)
        history.endGroup()
        syncHistory()
        maybeNotify()
    }

    /** 单条编辑对端点 offset 的平移：编辑点前不动；落在被删区段内贴回编辑点（含替换文本末）；其后按净增减平移。 */
    private fun shiftForEdit(off: Int, start: Int, removed: Int, inserted: Int): Int = when {
        off <= start -> off
        off <= start + removed -> start + inserted
        else -> off - removed + inserted
    }

    // --- 选择 --------------------------------------------------------------------------------

    fun setSelection(a: TextPosition, b: TextPosition, keepComposing: Boolean = false) {
        directional = TextRange(buffer.clamp(a), buffer.clamp(b))
        if (!keepComposing) composing = null
        desiredColumn = null
        history.breakMerge() // 显式定位是语义断点：其后键入不并入此前的输入单元
        maybeNotify()
    }

    fun setCursor(p: TextPosition) = setSelection(p, p)

    /** 保持 anchor 不动、把 head 移到 [to]，选区随之伸缩（Shift+方向键、手柄 / 拖拽扩选、视觉行上下移动）。 */
    fun extendSelectionTo(to: TextPosition) {
        directional = TextRange(anchor, buffer.clamp(to))
        composing = null
        desiredColumn = null
        history.breakMerge() // 与 setSelection 同理：选区变动后不再并入此前的输入单元
        maybeNotify()
    }

    fun selectAll() = setSelection(TextPosition(0, 0), buffer.endPosition())

    // --- 编辑原语 ----------------------------------------------------------------------------

    internal fun replaceRange(range: TextRange, text: String, kind: EditKind = EditKind.Other) {
        val selBefore = selectionSnapshot()
        val (edit, newCaret) = replaceCollecting(range, text)
        composing = null
        desiredColumn = null
        collapseCaret(newCaret)
        history.record(edit, selBefore, selectionSnapshot(), kind)
        syncHistory()
        maybeNotify()
    }

    fun replaceSelection(text: String) = replaceRange(selection, text)

    /**
     * 回车并继承缩进：把选区替换为「换行 + 新行缩进」，缩进取选区起点行的前导空白（空格/Tab 原样照抄），
     * 并截断到起点列——光标停在缩进区中间时只继承其左侧部分。整段一次 insert，撤销时连缩进一步回退
     * （含换行的键入自成单元）。只属回车键语义：粘贴含换行的文本不经此处——逐行补缩进会改坏粘贴内容。
     */
    fun insertNewlineAutoIndent() {
        val start = selStart
        val lineText = buffer.lineText(start.line)
        val limit = start.column.coerceIn(0, lineText.length)
        var i = 0
        while (i < limit && (lineText[i] == ' ' || lineText[i] == '\t')) i++
        insert("\n" + lineText.substring(0, i))
    }

    fun commitText(text: String, newCursorPosition: Int) {
        val target = (composing ?: selection).normalized()
        val wasComposing = composing != null
        val selBefore = selectionSnapshot()
        val (edit, newCaret) = replaceCollecting(target, text)
        composing = null
        desiredColumn = null
        collapseCaret(cursorAfterInsert(target.start, newCursorPosition, newCaret))
        // 预编辑中的提交并入本次会话单元并封口；否则单码点纯插入按键入合并，多字符（粘贴 / 整词提交）自成单元。
        val kind = when {
            wasComposing -> EditKind.Composing
            edit.removed.isEmpty() && isSingleCodePoint(edit.inserted) -> EditKind.Typing
            else -> EditKind.Other
        }
        history.record(edit, selBefore, selectionSnapshot(), kind)
        if (wasComposing) history.breakMerge() // 提交结束会话：下个会话 / 键入不并入
        syncHistory()
        maybeNotify()
    }

    fun insert(text: String) = commitText(text, 1)

    /** 键入智能开关：自动配对/跳过闭合/成对退格（宿主经 CodeEditor 参数控制）。非 state：只被键入路径读。 */
    var autoClosePairs: Boolean = true

    /**
     * 「键入一个字符」：与 [insert]（原始插入——粘贴/编程写入/IME 提交走它）不同，本原语承载键入的
     * 智能行为，供各键入路径（桌面 KeyEvent、Android 软键盘单字符、符号条）共用：
     * - 开括号 `([{` 成对插入、光标停中间（一次 insert，撤销一步连闭合一起回退）；
     * - 键入的闭字符与光标右侧相同 → 不插入、只越过（跳过闭合，含引号的第二击）；
     * - 引号按意图启发式：行内左侧同引号奇数个（正在串里、这是闭合）或右侧贴字母数字（在词前补引号）
     *   都原样插入，只有「开新串」的位置才成对；
     * - 预编辑中 / 多字符 / 有选区（包裹语义争议大，按普通替换）/ 开关关闭 → 全部退化为 [insert]。
     */
    fun typeCharacter(text: String) {
        if (!autoClosePairs || composing != null || text.length != 1 || !selection.isEmpty) {
            insert(text)
            return
        }
        val ch = text[0]
        val pos = selStart
        val line = buffer.lineText(pos.line)
        val right = if (pos.column < line.length) line[pos.column] else null
        if (right == ch && ch in AUTO_PAIR_CLOSERS) {
            setCursor(TextPosition(pos.line, pos.column + 1)) // 跳过闭合
            return
        }
        val close = AUTO_PAIRS[ch]
        if (close == null) {
            insert(text)
            return
        }
        if (ch == '"' || ch == '\'') {
            var count = 0
            for (i in 0 until pos.column) if (line[i] == ch) count++
            if (count % 2 == 1 || (right != null && right.isLetterOrDigit())) {
                insert(text)
                return
            }
        }
        insert("$ch$close")
        setCursor(TextPosition(selStart.line, selStart.column - 1)) // 光标回到配对中间
    }

    fun backspace() {
        if (!selection.isEmpty) {
            replaceRange(selection, ""); return
        }
        // 光标恰在自动配对正中（左开右闭同源）：整对一起删——留半个孤儿闭合比多删一个字符更烦。
        if (autoClosePairs && composing == null) {
            val line = buffer.lineText(selStart.line)
            val col = selStart.column
            if (col in 1 until line.length && AUTO_PAIRS[line[col - 1]] == line[col]) {
                replaceRange(
                    TextRange(TextPosition(selStart.line, col - 1), TextPosition(selStart.line, col + 1)),
                    "",
                    EditKind.DeleteBackward,
                )
                return
            }
        }
        val prev = previousCodePointPosition(selStart) ?: return
        replaceRange(TextRange(prev, selStart), "", EditKind.DeleteBackward)
    }

    fun deleteForward() {
        if (!selection.isEmpty) {
            replaceRange(selection, ""); return
        }
        val next = nextCodePointPosition(selEnd) ?: return
        replaceRange(TextRange(selEnd, next), "", EditKind.DeleteForward)
    }

    // --- IME composing -----------------------------------------------------------------------

    fun setComposingText(text: String, newCursorPosition: Int) {
        val target = (composing ?: selection).normalized()
        val selBefore = selectionSnapshot()
        val (edit, newCaret) = replaceCollecting(target, text)
        composing = if (text.isEmpty()) null else TextRange(target.start, newCaret)
        desiredColumn = null
        collapseCaret(cursorAfterInsert(target.start, newCursorPosition, newCaret))
        history.record(edit, selBefore, selectionSnapshot(), EditKind.Composing)
        syncHistory()
        maybeNotify()
    }

    fun setComposingRegion(startOffset: Int, endOffset: Int) {
        val lo = minOf(startOffset, endOffset).coerceAtLeast(0)
        val hi = maxOf(startOffset, endOffset).coerceAtMost(buffer.totalLength())
        composing = if (lo == hi) null else TextRange(buffer.positionAt(lo), buffer.positionAt(hi))
        maybeNotify()
    }

    fun finishComposing() {
        if (composing != null) {
            composing = null
            history.breakMerge() // 会话就此定格：其后编辑不再并入本次预编辑单元
            maybeNotify()
        }
    }

    // --- surrounding 删除（IME，offset 语义）--------------------------------------------------

    fun deleteSurroundingText(before: Int, after: Int) {
        val (selS, selE) = selectionOffsets()
        val total = buffer.totalLength()
        val delStart = (selS - before.coerceAtLeast(0)).coerceAtLeast(0)
        val delEnd = (selE + after.coerceAtLeast(0)).coerceAtMost(total)
        val selBefore = selectionSnapshot()
        // 先删尾段（不影响其前的 offset），再删头段。piece-tree 即时一致，无需失效索引。
        val (tailEdit, _) = replaceCollecting(TextRange(buffer.positionAt(selE), buffer.positionAt(delEnd)), "")
        val (headEdit, _) = replaceCollecting(TextRange(buffer.positionAt(delStart), buffer.positionAt(selS)), "")
        composing = null
        desiredColumn = null
        collapseCaret(buffer.positionAt(delStart))
        // 两段删除是同一 IME 请求：归入同一撤销单元（撤销逆序回放、重做按原序回放）。
        val selAfter = selectionSnapshot()
        history.beginGroup()
        history.record(tailEdit, selBefore, selAfter, EditKind.Other)
        history.record(headEdit, selBefore, selAfter, EditKind.Other)
        history.endGroup()
        syncHistory()
        maybeNotify()
    }

    fun deleteSurroundingTextInCodePoints(before: Int, after: Int) {
        val beforeChars = charsForCodePoints(textBeforeCursorString(before * 2 + 2), before, fromEnd = true)
        val afterChars = charsForCodePoints(textAfterCursorString(after * 2 + 2), after, fromEnd = false)
        deleteSurroundingText(beforeChars, afterChars)
    }

    private fun textBeforeCursorString(n: Int): String {
        val off = buffer.offsetAt(selStart)
        val start = (off - n).coerceAtLeast(0)
        return buffer.textInRange(TextRange(buffer.positionAt(start), selStart))
    }

    private fun textAfterCursorString(n: Int): String {
        val off = buffer.offsetAt(selEnd)
        val end = (off + n).coerceAtMost(buffer.totalLength())
        return buffer.textInRange(TextRange(selEnd, buffer.positionAt(end)))
    }

    private fun charsForCodePoints(window: String, codePoints: Int, fromEnd: Boolean): Int {
        var chars = 0
        var cps = 0
        while (cps < codePoints) {
            val idx = if (fromEnd) window.length - chars - 1 else chars
            if (idx < 0 || idx >= window.length) break
            val c = window[idx]
            val pair = if (fromEnd) {
                idx - 1 >= 0 && window[idx - 1].isHighSurrogate() && c.isLowSurrogate()
            } else {
                idx + 1 < window.length && c.isHighSurrogate() && window[idx + 1].isLowSurrogate()
            }
            chars += if (pair) 2 else 1
            cps++
        }
        return chars
    }

    // --- 光标导航 ----------------------------------------------------------------------------

    fun moveCaretHorizontally(dir: Int, extend: Boolean) {
        if (!extend && !selection.isEmpty) {
            setCursor(if (dir < 0) selStart else selEnd)
            return
        }
        val from = head
        val target = if (dir < 0) previousCodePointPosition(from) else nextCodePointPosition(from)
        val to = target ?: from
        if (extend) extendSelectionTo(to) else setCursor(to)
    }

    fun moveCaretVertically(dir: Int, extend: Boolean) {
        val from = head
        val goal = desiredColumn ?: from.column
        val targetLine = (from.line + dir).coerceIn(0, buffer.lineCount - 1)
        val targetCol = goal.coerceAtMost(buffer.lineLength(targetLine))
        val to = TextPosition(targetLine, targetCol)
        if (extend) extendSelectionTo(to) else setCursor(to)
        desiredColumn = goal // setCursor/extendSelectionTo 已清空，这里恢复目标列供连续上下移动
    }

    fun moveCaretToLineStart(extend: Boolean) {
        val to = TextPosition(head.line, 0)
        if (extend) extendSelectionTo(to) else setCursor(to)
    }

    fun moveCaretToLineEnd(extend: Boolean) {
        val to = TextPosition(head.line, buffer.lineLength(head.line))
        if (extend) extendSelectionTo(to) else setCursor(to)
    }

    fun moveCaretToDocStart(extend: Boolean) {
        val to = TextPosition(0, 0)
        if (extend) extendSelectionTo(to) else setCursor(to)
    }

    fun moveCaretToDocEnd(extend: Boolean) {
        val to = buffer.endPosition()
        if (extend) extendSelectionTo(to) else setCursor(to)
    }

    /** 按词移动：从 head 越过一段「同类字符」到其边界（词 / 空白 / 标点各成段，见 [charClass]）；
     *  在行首 / 行末与逐字导航一样跨行。横向移动 → 清目标列（extendSelectionTo/setCursor 已清）。 */
    fun moveCaretByWord(dir: Int, extend: Boolean) {
        val to = wordBoundaryFrom(head, dir)
        if (extend) extendSelectionTo(to) else setCursor(to)
    }

    /** 翻页：按 [dir]*[lines] 行纵向移动，保留 goal column（与 [moveCaretVertically] 同法）。 */
    fun movePage(dir: Int, lines: Int, extend: Boolean) {
        val from = head
        val goal = desiredColumn ?: from.column
        val targetLine = (from.line + dir * lines).coerceIn(0, buffer.lineCount - 1)
        val targetCol = goal.coerceAtMost(buffer.lineLength(targetLine))
        val to = TextPosition(targetLine, targetCol)
        if (extend) extendSelectionTo(to) else setCursor(to)
        desiredColumn = goal // setCursor/extendSelectionTo 已清空，这里恢复目标列供连续翻页
    }

    private fun wordBoundaryFrom(pos: TextPosition, dir: Int): TextPosition {
        val line = buffer.lineText(pos.line)
        return if (dir > 0) {
            if (pos.column >= line.length) {
                if (pos.line < buffer.lineCount - 1) TextPosition(pos.line + 1, 0) else pos
            } else {
                val cls = charClass(line[pos.column])
                var c = pos.column
                while (c < line.length && charClass(line[c]) == cls) c++
                TextPosition(pos.line, c)
            }
        } else {
            if (pos.column <= 0) {
                if (pos.line > 0) TextPosition(pos.line - 1, buffer.lineLength(pos.line - 1)) else pos
            } else {
                val cls = charClass(line[pos.column - 1])
                var c = pos.column
                while (c > 0 && charClass(line[c - 1]) == cls) c--
                TextPosition(pos.line, c)
            }
        }
    }

    // --- 选择文本 / IME getter ---------------------------------------------------------------

    fun selectedText(): String? = if (selection.isEmpty) null else buffer.textInRange(selection)

    fun textBeforeCursor(n: Int): CharSequence = textBeforeCursorString(n.coerceAtLeast(0))

    fun textAfterCursor(n: Int): CharSequence = textAfterCursorString(n.coerceAtLeast(0))

    /**
     * 双击/长按选中的「词」范围：选中当前位置所在的一段连续同类字符——标识符（含 _ 与 $）、空白、标点
     * 各自成段。此前按 isLetterOrDigit 判定，会把 my_variable/$scope 截断，双击空白或标点还返回空选区。
     */
    fun wordRangeAt(pos: TextPosition): TextRange {
        val p = buffer.clamp(pos)
        val line = buffer.lineText(p.line)
        if (line.isEmpty()) return TextRange.cursor(p)
        // 光标停在行尾时按其左侧字符归类。
        val idx = if (p.column < line.length) p.column else p.column - 1
        val cls = charClass(line[idx])
        var s = idx
        var e = idx + 1
        while (s > 0 && charClass(line[s - 1]) == cls) s--
        while (e < line.length && charClass(line[e]) == cls) e++
        return TextRange(TextPosition(p.line, s), TextPosition(p.line, e))
    }

    /**
     * 从锚定词 [anchorWord] 扩展到 [at] 所在词：选区并到两词的最小外包区间，两端吸附词边界。
     * 用于长按选词后的拖拽/边缘自动滚动扩展——即便 [at] 仍落在锚定词内（手指未动）也保留整词、不塌成光标。
     */
    fun selectWordRange(anchorWord: TextRange, at: TextPosition) {
        val cur = wordRangeAt(at)
        setSelection(minOf(anchorWord.start, cur.start), maxOf(anchorWord.end, cur.end))
    }

    /** 字符归类：0=空白，1=词字符（字母数字/_/$），2=标点符号。 */
    private fun charClass(c: Char): Int = when {
        c.isWhitespace() -> 0
        c.isLetterOrDigit() || c == '_' || c == '$' -> 1
        else -> 2
    }

    // --- 内部辅助 ----------------------------------------------------------------------------

    /** 折叠光标到 [p]：编辑原语把选区收敛为插入点时用（composing/desiredColumn 由调用方清）。 */
    private fun collapseCaret(p: TextPosition) {
        directional = TextRange.cursor(p)
    }

    private fun cursorAfterInsert(insertStart: TextPosition, newCursorPosition: Int, insertedEnd: TextPosition): TextPosition {
        if (newCursorPosition == 1) return insertedEnd
        val startOff = buffer.offsetAt(insertStart)
        val endOff = buffer.offsetAt(insertedEnd)
        val raw = if (newCursorPosition > 0) endOff + (newCursorPosition - 1) else startOff + newCursorPosition
        return buffer.positionAt(raw.coerceIn(0, buffer.totalLength()))
    }

    internal fun previousCodePointPosition(pos: TextPosition): TextPosition? {
        if (pos.column > 0) {
            val line = buffer.lineText(pos.line)
            val c = pos.column
            val step = if (c >= 2 && line[c - 1].isLowSurrogate() && line[c - 2].isHighSurrogate()) 2 else 1
            return TextPosition(pos.line, c - step)
        }
        if (pos.line > 0) return TextPosition(pos.line - 1, buffer.lineLength(pos.line - 1))
        return null
    }

    internal fun nextCodePointPosition(pos: TextPosition): TextPosition? {
        val len = buffer.lineLength(pos.line)
        if (pos.column < len) {
            val line = buffer.lineText(pos.line)
            val c = pos.column
            val step = if (c + 1 < len && line[c].isHighSurrogate() && line[c + 1].isLowSurrogate()) 2 else 1
            return TextPosition(pos.line, c + step)
        }
        if (pos.line < buffer.lineCount - 1) return TextPosition(pos.line + 1, 0)
        return null
    }
}
