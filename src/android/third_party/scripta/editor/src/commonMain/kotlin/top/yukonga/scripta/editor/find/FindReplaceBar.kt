package top.yukonga.scripta.editor.find

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicText
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isShiftPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.LineHeightStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import top.yukonga.scripta.editor.EditorColors
import top.yukonga.scripta.editor.editorNoFontPaddingStyle

/**
 * 停靠式查找 / 替换条：嵌在编辑器根 Column 顶部、占自己的布局行——**不是** Popup 浮层。
 * Android 上 focusable Popup 会吞掉浮层外的全部触摸（编辑区/工具栏点不动），而查找条里的输入框
 * 又必须可获焦收 IME，两者不可兼得；停靠进主窗口布局则输入、触摸互不干扰，文本区随开合上下让位。
 *
 * 键：查询框 Enter=下一个、Shift+Enter=上一个、Esc=关闭回焦编辑器；替换框 Enter=替换当前。
 */
@Composable
internal fun FindReplaceBar(
    session: FindSession,
    colors: EditorColors,
    readOnly: Boolean,
    onRequestEditorFocus: () -> Unit,
) {
    if (!session.visible) return
    val queryFocus = remember { FocusRequester() }
    fun closeAndRefocus() {
        session.close()
        onRequestEditorFocus()
    }
    Column(
        Modifier
            .fillMaxWidth()
            .background(colors.symbolBarBackground)
            // 上下使用相同留白，让输入框与操作按钮在工具栏中保持视觉居中。
            .padding(horizontal = 6.dp, vertical = 4.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy(4.dp), verticalAlignment = Alignment.CenterVertically) {
            FindField(
                value = session.query,
                onValueChange = { session.query = it },
                placeholder = "查找",
                colors = colors,
                // 弹性宽度：固定尺寸的计数/开关/按钮先占位，输入框吸收剩余宽度——窄屏（或大字体缩放）
                // 下被压缩的是输入框，行尾的 ✕ 等控件在任何屏宽都可见。
                modifier = Modifier
                    .weight(1f)
                    .focusRequester(queryFocus)
                    .onPreviewKeyEvent { ev ->
                        if (ev.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                        when (ev.key) {
                            Key.Enter, Key.NumPadEnter -> {
                                if (ev.isShiftPressed) session.prev() else session.next(); true
                            }

                            Key.Escape -> {
                                closeAndRefocus(); true
                            }

                            else -> false
                        }
                    },
                onImeSearch = { session.next() },
            )
            BasicText(
                text = counterText(session),
                style = TextStyle(color = colors.symbolBarForeground.copy(alpha = 0.75f), fontSize = 12.sp),
                maxLines = 1,
            )
            ToggleChip("Aa", session.caseSensitive, colors) { session.caseSensitive = it }
            ToggleChip("词", session.wholeWord, colors) { session.wholeWord = it }
            ToggleChip(".*", session.useRegex, colors) { session.useRegex = it }
            ActionChip("↑", colors) { session.prev() }
            ActionChip("↓", colors) { session.next() }
            ActionChip("✕", colors) { closeAndRefocus() }
        }
        if (session.replaceVisible && !readOnly) {
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp), verticalAlignment = Alignment.CenterVertically) {
                FindField(
                    value = session.replacement,
                    onValueChange = { session.replacement = it },
                    placeholder = "替换为",
                    colors = colors,
                    // 与查询框同理：弹性宽度，替换按钮恒可见。
                    modifier = Modifier.weight(1f).onPreviewKeyEvent { ev ->
                        if (ev.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                        when (ev.key) {
                            Key.Enter, Key.NumPadEnter -> {
                                session.replaceCurrent(); true
                            }

                            Key.Escape -> {
                                closeAndRefocus(); true
                            }

                            else -> false
                        }
                    },
                    onImeSearch = { session.replaceCurrent() },
                )
                ActionChip("替换", colors) { session.replaceCurrent() }
                ActionChip("全部替换", colors) { session.replaceAll() }
            }
        }
    }
    // 挂载后聚焦查询框。
    LaunchedEffect(Unit) { queryFocus.requestFocus() }
}

private fun counterText(session: FindSession): String = when {
    session.query.isEmpty() -> ""
    session.result.invalidPattern -> "无效正则"
    session.result.matches.isEmpty() -> "无结果"
    else -> {
        val plus = if (session.result.limitHit) "+" else ""
        "${session.activeIndex + 1}/${session.result.matches.size}$plus"
    }
}

@Composable
internal fun FindField(
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    colors: EditorColors,
    modifier: Modifier = Modifier,
    keyboardType: KeyboardType = KeyboardType.Text,
    onImeSearch: () -> Unit,
) {
    // 选区归字段内部管，文本与外部 String 同步。挂载初值全选：查找条关闭重开（字段重挂载）时
    // 上次的查询词处于全选态——直接输入即覆盖、直接 Enter 即复用。
    var fieldValue by remember { mutableStateOf(TextFieldValue(value, TextRange(0, value.length))) }
    if (fieldValue.text != value) {
        // 外部改写文本（打开时按选区预填等）：同步并全选，语义同重开。用户键入不走这里
        //（onValueChange 已把外部值改成一致）。
        fieldValue = TextFieldValue(value, TextRange(0, value.length))
    }
    // 固定行高 + Trim.None 单行也生效 + 居中 + Android 关 includeFontPadding：CJK 回退字体垂直度量
    // 大于 Latin，不固定行高时输入中文会让字段行盒长高、整个输入框跳高（正文行高同一问题、同一修法）。
    // 占位符共用此样式——占位符本身是中文，否则空态（CJK 高）与输入英文（Latin 高）之间也会跳。
    val fieldLineStyle = remember {
        LineHeightStyle(alignment = LineHeightStyle.Alignment.Center, trim = LineHeightStyle.Trim.None)
    }
    val fieldPlatformStyle = remember { editorNoFontPaddingStyle() }
    Box(
        modifier
            // 宽度交给调用方（Row 内 weight 弹性分配），只兜一个可点/可读的下限。
            .widthIn(min = 72.dp)
            .clip(RoundedCornerShape(4.dp))
            .background(colors.background.copy(alpha = 0.6f))
            .padding(horizontal = 8.dp, vertical = 4.dp),
        contentAlignment = Alignment.CenterStart,
    ) {
        if (value.isEmpty()) {
            BasicText(
                text = placeholder,
                style = TextStyle(
                    color = colors.symbolBarForeground.copy(alpha = 0.45f),
                    fontSize = 13.sp,
                    lineHeight = 20.sp,
                    lineHeightStyle = fieldLineStyle,
                    platformStyle = fieldPlatformStyle,
                ),
            )
        }
        BasicTextField(
            value = fieldValue,
            onValueChange = {
                fieldValue = it
                if (it.text != value) onValueChange(it.text)
            },
            singleLine = true,
            textStyle = TextStyle(
                color = colors.foreground,
                fontSize = 13.sp,
                fontFamily = FontFamily.Monospace,
                lineHeight = 20.sp,
                lineHeightStyle = fieldLineStyle,
                platformStyle = fieldPlatformStyle,
            ),
            cursorBrush = SolidColor(colors.cursor),
            keyboardOptions = KeyboardOptions(
                imeAction = ImeAction.Search,
                keyboardType = keyboardType,
                autoCorrectEnabled = false,
            ),
            keyboardActions = KeyboardActions(onSearch = { onImeSearch() }),
            // 撑满外框：空文本的字段本身只有光标宽，点框内空白必须也能聚焦。
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

/** 开关小片（大小写 / 整词 / 正则）：亮起表示开。 */
@Composable
private fun ToggleChip(label: String, on: Boolean, colors: EditorColors, onToggle: (Boolean) -> Unit) {
    Box(
        Modifier
            .clip(RoundedCornerShape(4.dp))
            .background(if (on) colors.symbolBarPressed else Color.Transparent)
            .pointerInput(label, on) { detectTapGestures(onTap = { onToggle(!on) }) }
            .defaultMinSize(minWidth = 28.dp, minHeight = 28.dp)
            .padding(horizontal = 6.dp, vertical = 5.dp),
        contentAlignment = Alignment.Center,
    ) {
        BasicText(
            text = label,
            style = TextStyle(
                color = colors.symbolBarForeground.copy(alpha = if (on) 1f else 0.6f),
                fontSize = 12.sp,
            ),
        )
    }
}

/** 动作小片（上一个 / 下一个 / 关闭 / 替换 / 跳转）：按下反馈同符号条键。 */
@Composable
internal fun ActionChip(label: String, colors: EditorColors, onClick: () -> Unit) {
    var pressed by remember { mutableStateOf(false) }
    Box(
        Modifier
            .clip(RoundedCornerShape(4.dp))
            .background(if (pressed) colors.symbolBarPressed else Color.Transparent)
            .pointerInput(label) {
                detectTapGestures(
                    onPress = {
                        pressed = true
                        tryAwaitRelease()
                        pressed = false
                    },
                    onTap = { onClick() },
                )
            }
            .defaultMinSize(minWidth = 28.dp, minHeight = 28.dp)
            .padding(horizontal = 6.dp, vertical = 5.dp),
        contentAlignment = Alignment.Center,
    ) {
        BasicText(text = label, style = TextStyle(color = colors.symbolBarForeground, fontSize = 12.sp))
    }
}
