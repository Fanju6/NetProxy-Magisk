package top.yukonga.scripta.editor

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicText
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * 底部符号条上的一个键。[label] 为显示文字，[value] 为插入到文档的实际文本（默认与 [label] 相同）。
 * 二者分离是为了让「Tab」这类键显示短名却插入不同内容（如 4 个空格），并允许调用方完全自定义两者。
 */
@Immutable
data class EditorSymbol(val label: String, val value: String = label)

/** 符号快捷栏在编辑器中的停靠位置。 */
enum class SymbolBarPosition {
    Bottom,
    End,
}

/**
 * 底部符号条的默认键集：Tab（插入 4 空格，与编辑器 Tab 键一致）+ 代码/YAML 常用符号。
 * 通过 `CodeEditor(symbols = ...)` 传入自定义列表可整体替换。
 */
val DefaultEditorSymbols: List<EditorSymbol> = listOf(
    EditorSymbol("Tab", "    "),
    EditorSymbol(":"), EditorSymbol("="),
    EditorSymbol("{"), EditorSymbol("}"),
    EditorSymbol("["), EditorSymbol("]"),
    EditorSymbol("("), EditorSymbol(")"),
    EditorSymbol("<"), EditorSymbol(">"),
    EditorSymbol("\""), EditorSymbol("'"),
    EditorSymbol("/"), EditorSymbol("\\"),
    EditorSymbol("|"), EditorSymbol("&"),
    EditorSymbol("+"), EditorSymbol("-"),
    EditorSymbol("*"), EditorSymbol("#"),
    EditorSymbol("_"),
)

/**
 * 常驻编辑器底部的符号快捷条：横向可滚，每键点按即把 [EditorSymbol.value] 交给 [onSymbol] 插入。
 *
 * 键的点按用 [detectTapGestures]（**不触碰 Compose 焦点**）而非 clickable——编辑器的 IME 会话绑定在编辑区
 * 的焦点上，一旦符号键抢走焦点会话即被销毁、软键盘随之收起。这里不动焦点，故打开输入法时点符号键键盘不闪退。
 *
 * [windowInsets] 为底部安全区（导航栏 / 键盘中较大者）：**背景铺满整条（含 inset 区）、键按 inset 抬到系统栏之上**。
 * 于是键盘收起时小白条坐在符号条底色上（沉浸一致）、键盘弹出时整条骑在键盘正上方，两种情形边缘配色都对。
 */
@Composable
internal fun SymbolBar(
    symbols: List<EditorSymbol>,
    colors: EditorColors,
    onSymbol: (EditorSymbol) -> Unit,
    windowInsets: WindowInsets,
    modifier: Modifier = Modifier,
) {
    val divider = colors.symbolBarForeground.copy(alpha = 0.12f)
    Row(
        modifier
            .fillMaxWidth()
            .background(colors.symbolBarBackground) // 背景先铺满整条（含下方 inset 区）→ 铺到屏幕边缘
            .drawBehind { drawLine(divider, Offset(0f, 0f), Offset(size.width, 0f), 1f) } // 与文本区分隔的顶部细线
            .windowInsetsPadding(windowInsets) // 再把键抬到导航栏/键盘之上（背景已铺、只挪内容）
            .horizontalScroll(rememberScrollState())
            .padding(horizontal = 4.dp, vertical = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        symbols.forEach { symbol -> SymbolKey(symbol, colors, onSymbol) }
    }
}

/**
 * 停靠在编辑器右侧的纵向符号快捷栏。符号较多时可纵向滚动，不占用底部操作区。
 */
@Composable
internal fun VerticalSymbolBar(
    symbols: List<EditorSymbol>,
    colors: EditorColors,
    onSymbol: (EditorSymbol) -> Unit,
    windowInsets: WindowInsets?,
    modifier: Modifier = Modifier,
) {
    val divider = colors.symbolBarForeground.copy(alpha = 0.12f)
    Column(
        modifier
            .width(28.dp)
            .fillMaxHeight()
            .background(colors.symbolBarBackground)
            .drawBehind { drawLine(divider, Offset(0f, 0f), Offset(0f, size.height), 1f) }
            .then(if (windowInsets != null) Modifier.windowInsetsPadding(windowInsets) else Modifier)
            .verticalScroll(rememberScrollState())
            .padding(vertical = 1.dp),
        verticalArrangement = Arrangement.spacedBy(1.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        symbols.forEach { symbol -> SymbolKey(symbol, colors, onSymbol, compact = true) }
    }
}

/** 为宿主提供与符号栏一致的底部背景、分隔线和系统栏避让。 */
@Composable
internal fun EditorBottomBar(
    colors: EditorColors,
    windowInsets: WindowInsets,
    content: @Composable () -> Unit,
) {
    val divider = colors.symbolBarForeground.copy(alpha = 0.12f)
    Box(
        Modifier
            .fillMaxWidth()
            .background(colors.symbolBarBackground)
            .drawBehind { drawLine(divider, Offset(0f, 0f), Offset(size.width, 0f), 1f) }
            .windowInsetsPadding(windowInsets),
    ) {
        content()
    }
}

@Composable
private fun SymbolKey(
    symbol: EditorSymbol,
    colors: EditorColors,
    onSymbol: (EditorSymbol) -> Unit,
    compact: Boolean = false,
) {
    var pressed by remember { mutableStateOf(false) }
    Box(
        Modifier
            .clip(RoundedCornerShape(6.dp))
            .background(if (pressed) colors.symbolBarPressed else Color.Transparent)
            .pointerInput(symbol) {
                detectTapGestures(
                    onPress = {
                        pressed = true
                        tryAwaitRelease()
                        pressed = false
                    },
                    onTap = { onSymbol(symbol) },
                )
            }
            .defaultMinSize(
                minWidth = if (compact) 26.dp else 40.dp,
                minHeight = if (compact) 28.dp else 38.dp,
            )
            .padding(
                horizontal = if (compact) 1.dp else 10.dp,
                vertical = if (compact) 1.dp else 6.dp,
            ),
        contentAlignment = Alignment.Center,
    ) {
        BasicText(
            text = symbol.label,
            style = TextStyle(
                color = colors.symbolBarForeground,
                fontSize = if (compact) 13.sp else 15.sp,
                fontFamily = FontFamily.Monospace,
            ),
            maxLines = 1,
            softWrap = false,
        )
    }
}
