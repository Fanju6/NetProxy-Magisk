package top.yukonga.scripta.editor.highlight

/**
 * 语义 token 类型：插件产出「类型」、主题把类型映射为样式——换主题不换插件、换插件不换主题。
 * 封闭枚举是刻意的：主题（[SyntaxColors]）对每个类型都是全量映射、没有回退链，
 * 新增成员时穷举 when 会把漏配变成编译错误。
 */
enum class TokenType {
    Comment, Key, String, Number, Boolean, Null, Keyword, Punctuation, Anchor, Tag, Directive,
    Operator, Variable, Property, Function, Type, Escape, Regex, Heading,
}

/**
 * 行内一段着色：[start, end) 列区间（UTF-16 char 列）+ 语义类型。区间须按 start 升序、互不重叠；
 * 未覆盖的空隙走基础前景色。越界区间在落到 AnnotatedString 时被钳到行内（防御，不作为契约依赖）。
 */
data class HighlightSpan(val start: Int, val end: Int, val type: TokenType)

/**
 * 裁剪着色段到列窗口 `[from, until)` 并平移为窗口内坐标（网格长行只测可见列切片时用）；
 * 与窗口无交集的段丢弃。入参须满足 [HighlightSpan] 的升序不重叠契约，输出保持同契约。
 */
fun clipSpansToWindow(spans: List<HighlightSpan>, from: Int, until: Int): List<HighlightSpan> {
    if (spans.isEmpty() || until <= from) return emptyList()
    val out = ArrayList<HighlightSpan>()
    for (s in spans) {
        if (s.end <= from) continue
        if (s.start >= until) break // 升序：后续段只会更靠右
        out.add(HighlightSpan(maxOf(s.start, from) - from, minOf(s.end, until) - from, s.type))
    }
    return out
}

/** 块注释开闭标记对（如 HTML 的 `<!--` / `-->`）。 */
data class BlockComment(val open: String, val close: String)

/**
 * 行进入 / 退出状态：跨行结构（块标量、多行字符串等）经它在行间传递。实现须为不可变值类型
 * （结构相等）——缓存靠「内容 + 进入状态相等」判断可否复用上次分词结果。
 */
interface LineState

/** 一行的分词结果：着色段 + 传给下一行的退出状态（null = 无跨行结构）。 */
class LineHighlight(val spans: List<HighlightSpan>, val exitState: LineState?)

/**
 * 语法高亮插件接口：按行增量分词。实现只依赖「本行文本 + 上一行退出状态」，其余（状态链推进、
 * 缓存、失效、渲染）由编辑器承担——插件保持纯函数即可跨行正确、可单测。
 */
interface SyntaxHighlighter {
    /** 文档首行的进入状态；通常 null（无跨行结构）。 */
    val initialState: LineState? get() = null

    /** 本语言的行注释前缀（注释切换 Ctrl+/ 用，如 YAML 的 `#`）；null = 无行注释、切换回退到 [blockComment]。 */
    val lineCommentPrefix: String? get() = null

    /** 本语言的块注释标记（无行注释语言的注释切换回退，如 HTML 的 `<!-- -->`）；null = 无块注释。 */
    val blockComment: BlockComment? get() = null

    fun highlightLine(text: String, entryState: LineState?): LineHighlight
}
