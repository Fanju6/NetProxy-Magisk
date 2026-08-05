package top.yukonga.scripta.editor.highlight

/**
 * 行状态链缓存：懒推进「行退出状态」数组，供虚拟化渲染按需取任意行的着色段——请求第 N 行会把
 * 0..N-1 的状态链补齐（每行只分词一次），编辑后经 [invalidate] 把可信前缀截到最早受影响行，
 * 其上游状态不动、下游按需重算。分词本身便宜（纯行内扫描），不再缓存 span 结果；渲染层的
 * layout 缓存以「内容 + spans 相等」判断复用，稳态滚动/闪烁帧根本不会走到这里。
 *
 * 收敛截断：非结构性编辑把旧可信前缀降级为「尾链」而非丢弃——重推进时某行新算的退出状态与
 * 旧存值相等（[LineState] 契约要求结构相等），说明下游整段旧链依然成立，水位线直接跳回尾链末端。
 * 于是在超长文件顶部打字（未改变跨行状态）后跳到文件尾是 O(脏行数) 而非 O(文件行数)。
 */
class HighlightCache(
    private val highlighter: SyntaxHighlighter,
    /**
     * 单行分词上限（UTF-16 字符）：更长的行不分词——空着色段、状态穿透（当作对跨行结构无贡献）。
     * 巨行（几 MB 的 minified 单行）每次编辑都要 O(行长) 重扫，必须有放弃阈值兜底；
     * 默认值与主流编辑器同级（VS Code maxTokenizationLineLength = 20000）。
     */
    private val maxLexedLineLength: Int = 20_000,
) {

    private val exitStates = ArrayList<LineState?>()
    private var valid = 0 // exitStates[0, valid) 可信

    // 尾链 [tailFloor, tailEnd)：旧存退出状态的行内容自失效以来未变、且同属一条旧可信链。
    // tailFloor 兜住脏范围下界——脏范围内即使状态巧合相等也不许跳（那之后的内容真的变了）。
    private var tailFloor = 0
    private var tailEnd = 0

    /** 编辑触及 [line]（及其后）：按最保守语义截断（等价结构性失效——不依赖行号对齐）。 */
    fun invalidateFrom(line: Int) = invalidate(line, Int.MAX_VALUE, structural = true)

    /**
     * 编辑影响了 `[from, endExclusive)`：截断可信前缀。[structural] = 行数变过——行号错位，
     * 旧状态不可再按行号比对；非结构性时保证该范围之外的行内容未变（收敛截断的前提）。
     */
    fun invalidate(from: Int, endExclusive: Int, structural: Boolean) {
        val f = from.coerceAtLeast(0)
        if (structural) {
            tailEnd = 0
            if (f < valid) valid = f
            return
        }
        val e = endExclusive.coerceAtLeast(f)
        if (f < valid) {
            // 旧可信前缀整段成为尾链，比较从最后一条可能脏的行（e-1）起。已有未决尾链时弃旧取新
            // ——两段属不同纪元不可拼接，弃掉只损失优化、不损失正确性。
            tailFloor = e - 1
            tailEnd = valid
            valid = f
        } else if (f < tailEnd) {
            // 编辑落在未决尾链内：被改的行不能再作为跳跃依据，抬高比较下限。
            if (e - 1 > tailFloor) tailFloor = e - 1
        }
    }

    /** 第 [line] 行的着色段；[getLine] 供状态链补齐时取行文本。 */
    fun spansForLine(line: Int, getLine: (Int) -> String): List<HighlightSpan> {
        while (valid < line) advance(getLine)
        if (line == valid) return advance(getLine).spans
        return lexLine(line, getLine).spans
    }

    private fun entryFor(line: Int): LineState? =
        if (line == 0) highlighter.initialState else exitStates[line - 1]

    /** 分词一行；超过 [maxLexedLineLength] 的行不进插件：空 spans、进入状态原样穿透。 */
    private fun lexLine(line: Int, getLine: (Int) -> String): LineHighlight {
        val text = getLine(line)
        val entry = entryFor(line)
        if (text.length > maxLexedLineLength) return LineHighlight(emptyList(), entry)
        return highlighter.highlightLine(text, entry)
    }

    private fun advance(getLine: (Int) -> String): LineHighlight {
        val line = valid
        val comparable = line >= tailFloor && line < tailEnd
        val old = if (comparable) exitStates[line] else null
        val h = lexLine(line, getLine)
        setExit(line, h.exitState)
        valid = if (comparable && h.exitState == old) tailEnd else line + 1
        return h
    }

    private fun setExit(line: Int, state: LineState?) {
        while (exitStates.size <= line) exitStates.add(null)
        exitStates[line] = state
    }
}
