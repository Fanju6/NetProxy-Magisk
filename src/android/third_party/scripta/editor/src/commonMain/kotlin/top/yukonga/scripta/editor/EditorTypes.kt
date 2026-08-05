package top.yukonga.scripta.editor

import androidx.compose.runtime.Immutable
import androidx.compose.ui.graphics.Color
import top.yukonga.scripta.editor.highlight.SyntaxColors

/** 文档的换行风格：载入时检测（`\r\n` 与孤 `\n` 多数决；孤 `\r` 不计票，平手/无换行回落 LF），
 *  编辑不改变。编辑器内部恒以 LF 规范形工作；保存用 [CodeEditorController.getText] 的带参重载按此还原。 */
enum class LineEnding { LF, CRLF }

/** The language the editor is configured for: selects the built-in highlighter plugin (PlainText = none). A custom [top.yukonga.scripta.editor.highlight.SyntaxHighlighter] passed to [CodeEditor] takes precedence. */
enum class EditorLanguage {
    PlainText,
    Yaml,
}

/**
 * 行号的横向锚定方式。
 *
 * [PinnedToScreen]：行号钉在屏幕左侧，不随横向滚动移动；正文之后重绘不透明 gutter 条盖住滚进来的内容，
 * 保证任意横向滚动下数字都清晰。
 *
 * [PinnedToLine]：行号连同其 gutter 底色属于该行坐标系，随内容一起横向滚动——滚到右侧时一并从左边滑出屏幕。
 *
 * 两种模式共用同一 gutter 宽度与正文原点，切换不引起文本重排，命中/滚动几何一致。
 */
enum class LineNumberMode {
    PinnedToScreen,
    PinnedToLine,
}

/** Colors used by the editor surface. Kept flat and [Immutable] so it stays a stable Compose input. */
@Immutable
data class EditorColors(
    val background: Color,
    val foreground: Color,
    val gutterBackground: Color,
    val gutterForeground: Color,
    val cursor: Color,
    val selection: Color,
    /** Caret / selection drag handle (teardrop) color. */
    val handle: Color,
    /** 底部符号条：背景 / 键文字 / 按下高亮。带默认值，不破坏既有构造。 */
    val symbolBarBackground: Color = Color(0xFF2A2A2C),
    val symbolBarForeground: Color = Color(0xFFCFCFCF),
    val symbolBarPressed: Color = Color(0x33FFFFFF),
    /** 查找命中的底色 / 当前命中的强调底色（画在选区之下）。带默认值，不破坏既有构造。 */
    val findMatch: Color = Color(0x4DFFC107),
    val findMatchActive: Color = Color(0x80FF6F00),
    /** 无选区时当前行的整行底色。带默认值（与深色 gutter 同值，沿用旧视觉），不破坏既有构造。 */
    val currentLine: Color = Color(0xFF252526),

    /** 匹配括号底色：光标邻接 `()[]{}` 时点亮该括号及其配对方。半透明中性灰、双主题通用。 */
    val bracketMatch: Color = Color(0x4D909090),

    /** 滚动条 thumb（静止指示态）。半透明中性灰：深/浅底上都可辨，默认即适配双主题。 */
    val scrollbarThumb: Color = Color(0x66808080),

    /** 滚动条 thumb（拖拽加宽态）。 */
    val scrollbarThumbActive: Color = Color(0xB38C8C8C),
    /** 语法高亮样式（token 类型 → 颜色 + 可选粗细/斜体/装饰线，逐槽位可自定义）。带默认值，不破坏既有构造。 */
    val syntax: SyntaxColors = SyntaxColors.Dark,
) {
    companion object {
        /** A neutral dark default so the scaffold renders visibly before a theme is wired up. */
        val Default: EditorColors = EditorColors(
            background = Color(0xFF1E1E1E),
            foreground = Color(0xFFE0E0E0),
            gutterBackground = Color(0xFF252526),
            gutterForeground = Color(0xFF858585),
            cursor = Color(0xFFAEAFAD),
            selection = Color(0x553A6DA0),
            handle = Color(0xFF277AF7),
            currentLine = Color(0xFF252526),
        )

        /** Light counterpart to [Default]; same field set, tuned for a white background. */
        val Light: EditorColors = EditorColors(
            background = Color(0xFFFFFFFF),
            foreground = Color(0xFF1F1F1F),
            gutterBackground = Color(0xFFF3F3F3),
            gutterForeground = Color(0xFF9AA0A6),
            cursor = Color(0xFF1F1F1F),
            selection = Color(0x553B82F6),
            handle = Color(0xFF277AF7),
            symbolBarBackground = Color(0xFFECECEC),
            symbolBarForeground = Color(0xFF3C3C3C),
            symbolBarPressed = Color(0x14000000),
            currentLine = Color(0xFFF3F3F3),
            syntax = SyntaxColors.Light,
        )
    }
}
