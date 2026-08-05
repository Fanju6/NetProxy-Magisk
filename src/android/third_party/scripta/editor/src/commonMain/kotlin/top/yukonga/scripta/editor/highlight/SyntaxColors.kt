package top.yukonga.scripta.editor.highlight

import androidx.compose.runtime.Immutable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration

/**
 * 单个 token 类型的渲染样式：颜色必选，粗细 / 斜体 / 装饰线可选（null = 不覆盖基础文本样式）。
 * 粗斜体只影响常规行——它们与基础样式共用同一次测量布局，命中测试自洽；网格快路径本就不走高亮。
 */
@Immutable
data class TokenStyle(
    val color: Color,
    val fontWeight: FontWeight? = null,
    val fontStyle: FontStyle? = null,
    val textDecoration: TextDecoration? = null,
) {
    fun toSpanStyle(): SpanStyle = SpanStyle(
        color = color,
        fontWeight = fontWeight,
        fontStyle = fontStyle,
        textDecoration = textDecoration,
    )
}

/**
 * 语法高亮配色：[TokenType] → [TokenStyle] 的完整映射，每个语义类型一个可自定义的槽位。
 * 插件只产出类型、不产出样式——换主题不用换插件。扁平 [Immutable] data class，
 * 与 [top.yukonga.scripta.editor.EditorColors] 同法保持稳定 Compose 输入。
 * 后加的槽位一律带默认值（取深色预设值），既有调用点不随枚举扩张而破坏。
 */
@Immutable
data class SyntaxColors(
    val comment: TokenStyle,
    val key: TokenStyle,
    val string: TokenStyle,
    val number: TokenStyle,
    val boolean: TokenStyle,
    val nullLike: TokenStyle,
    val keyword: TokenStyle,
    val punctuation: TokenStyle,
    val anchor: TokenStyle,
    val tag: TokenStyle,
    val directive: TokenStyle,
    val operator: TokenStyle = TokenStyle(Color(0xFFD4D4D4)),
    val variable: TokenStyle = TokenStyle(Color(0xFF9CDCFE)),
    val property: TokenStyle = TokenStyle(Color(0xFF9CDCFE)),
    val function: TokenStyle = TokenStyle(Color(0xFFDCDCAA)),
    val type: TokenStyle = TokenStyle(Color(0xFF4EC9B0)),
    val escape: TokenStyle = TokenStyle(Color(0xFFD7BA7D)),
    val regex: TokenStyle = TokenStyle(Color(0xFFD16969)),
    val heading: TokenStyle = TokenStyle(Color(0xFF569CD6), fontWeight = FontWeight.Bold),
) {
    fun styleFor(type: TokenType): TokenStyle = when (type) {
        TokenType.Comment -> comment
        TokenType.Key -> key
        TokenType.String -> string
        TokenType.Number -> number
        TokenType.Boolean -> boolean
        TokenType.Null -> nullLike
        TokenType.Keyword -> keyword
        TokenType.Punctuation -> punctuation
        TokenType.Anchor -> anchor
        TokenType.Tag -> tag
        TokenType.Directive -> directive
        TokenType.Operator -> operator
        TokenType.Variable -> variable
        TokenType.Property -> property
        TokenType.Function -> function
        TokenType.Type -> this.type
        TokenType.Escape -> escape
        TokenType.Regex -> regex
        TokenType.Heading -> heading
    }

    companion object {
        /** 深色底默认配色。 */
        val Dark = SyntaxColors(
            comment = TokenStyle(Color(0xFF6A9955)),
            key = TokenStyle(Color(0xFF9CDCFE)),
            string = TokenStyle(Color(0xFFCE9178)),
            number = TokenStyle(Color(0xFFB5CEA8)),
            boolean = TokenStyle(Color(0xFF569CD6)),
            nullLike = TokenStyle(Color(0xFF569CD6)),
            keyword = TokenStyle(Color(0xFFC586C0)),
            punctuation = TokenStyle(Color(0xFF9B9B9B)),
            anchor = TokenStyle(Color(0xFF4EC9B0)),
            tag = TokenStyle(Color(0xFF4EC9B0)),
            directive = TokenStyle(Color(0xFFC586C0)),
        )

        /** 浅色底默认配色。 */
        val Light = SyntaxColors(
            comment = TokenStyle(Color(0xFF008000)),
            key = TokenStyle(Color(0xFF0451A5)),
            string = TokenStyle(Color(0xFFA31515)),
            number = TokenStyle(Color(0xFF098658)),
            boolean = TokenStyle(Color(0xFF0000FF)),
            nullLike = TokenStyle(Color(0xFF0000FF)),
            keyword = TokenStyle(Color(0xFFAF00DB)),
            punctuation = TokenStyle(Color(0xFF7B7B7B)),
            anchor = TokenStyle(Color(0xFF267F99)),
            tag = TokenStyle(Color(0xFF267F99)),
            directive = TokenStyle(Color(0xFFAF00DB)),
            operator = TokenStyle(Color(0xFF000000)),
            variable = TokenStyle(Color(0xFF001080)),
            property = TokenStyle(Color(0xFF001080)),
            function = TokenStyle(Color(0xFF795E26)),
            type = TokenStyle(Color(0xFF267F99)),
            escape = TokenStyle(Color(0xFFEE0000)),
            regex = TokenStyle(Color(0xFF811F3F)),
            heading = TokenStyle(Color(0xFF800000), fontWeight = FontWeight.Bold),
        )
    }
}

/**
 * 把着色段落到 [AnnotatedString]（越界段钳到内容内）；无段时不建 builder。
 * [colorOnly]：只落颜色、丢弃粗细/斜体/装饰线——网格长行的等宽算术假设基础字宽，
 * 颜色不改度量而字重可能改，故其切片测量走此模式。
 */
fun highlightedText(
    content: String,
    spans: List<HighlightSpan>,
    colors: SyntaxColors,
    colorOnly: Boolean = false,
): AnnotatedString {
    if (spans.isEmpty()) return AnnotatedString(content)
    return buildAnnotatedString {
        append(content)
        for (s in spans) {
            val st = s.start.coerceIn(0, content.length)
            val en = s.end.coerceIn(st, content.length)
            if (en > st) {
                val style = colors.styleFor(s.type)
                addStyle(if (colorOnly) SpanStyle(color = style.color) else style.toSpanStyle(), st, en)
            }
        }
    }
}
