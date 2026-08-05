package top.yukonga.scripta.editor.menu

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.hoverable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsHoveredAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicText
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.draw.dropShadow
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.shadow.Shadow
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.DpOffset
import androidx.compose.ui.unit.IntRect
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupPositionProvider
import androidx.compose.ui.window.PopupProperties
import top.yukonga.scripta.editor.EditorColors
import top.yukonga.scripta.editor.EditorEngine
import top.yukonga.scripta.editor.text.TextPosition
import kotlin.math.roundToInt

private val MENU_GAP = 10.dp     // 悬浮条与选区/光标之间的竖直间距
private val MENU_MARGIN = 8.dp   // 菜单贴窗口边缘的留白（钳制用）
private val MENU_CORNER = 8.dp

/** 菜单外围的小外阴影（Compose 新 [dropShadow]）：浮层与正文分层、有立体感。放在 clip/background 之前 → 画在形状之外。 */
private fun Modifier.menuShadow(shape: Shape): Modifier = dropShadow(
    shape = shape,
    shadow = Shadow(
        radius = 8.dp,
        spread = 0.dp,
        color = Color.Black.copy(alpha = 0.28f),
        offset = DpOffset(0.dp, 2.dp),
    ),
)

/** 选区是否恰好覆盖整篇文档（全选态）——据此从可选项里去掉「全选」。 */
private fun EditorEngine.isAllSelected(): Boolean =
    selStart == TextPosition(0, 0) && selEnd == buffer.endPosition()

/**
 * 触屏文本操作悬浮条 + 粘贴气泡（宿主在窗口级 [Popup]，可浮出编辑器 clipToBounds）。有非空选区时于选区上方浮出
 * 「剪切·复制·粘贴·全选」（按 [readOnly] 与全选态裁剪）；无选区（[show] 由点按落光标触发）时浮出「粘贴·全选」气泡。
 *
 * [show] 由调用方综合「已触发 && 非鼠标 && 未在拖手柄/框选」得出——在此 lambda 内读，只重组本可组合、不牵动编辑器主体。
 * 位置读实时几何 [posRect]（光标/端点的编辑器局部 [x, top, bottom]），经自定义 [PopupPositionProvider] 居中于选区、
 * 上方不够翻到下方并钳进窗口。按钮用 [detectTapGestures] 落点——**不碰 Compose 焦点**，故点菜单不销毁 IME 会话/收键盘。
 */
@Composable
internal fun SelectionActionToolbar(
    engine: EditorEngine,
    colors: EditorColors,
    show: () -> Boolean,
    readOnly: Boolean,
    posRect: (TextPosition) -> FloatArray?,
    onPerform: (EditorContextAction) -> Unit,
) {
    if (!show()) return
    val sel = engine.selection
    val hasSel = !sel.isEmpty
    val actions = visibleTouchActions(
        contextActionAvailability(
            readOnly = readOnly,
            hasSelection = hasSel,
            allSelected = engine.isAllSelected(),
            docNonEmpty = engine.buffer.totalLength() > 0,
        ),
        hasSelection = hasSel,
    )
    if (actions.isEmpty()) return

    val density = LocalDensity.current
    val gapPx = with(density) { MENU_GAP.roundToPx() }
    val marginPx = with(density) { MENU_MARGIN.roundToPx() }
    val start = sel.start
    val end = sel.end
    // 位置在 layout 阶段（calculatePosition）读实时几何——posRect 内含 scrollX/scrollY，若在组合阶段读会订阅逐帧滚动、
    // 使本悬浮条每帧重组（项目纪律：帧率状态只在 draw/layout 读，见 MagnifierOverlay 的 offset{} 锚）。上锚取选区首行顶、
    // 翻下时用末行底；水平单行选区取两端中点、多行/无选区对齐起点。posRect 为 null（行未测量）时退到编辑器左上角兜底。
    val provider = remember(start, end, hasSel, gapPx, marginPx) {
        object : PopupPositionProvider {
            override fun calculatePosition(
                anchorBounds: IntRect,
                windowSize: IntSize,
                layoutDirection: LayoutDirection,
                popupContentSize: IntSize,
            ): IntOffset {
                val startR = posRect(start) ?: return IntOffset(anchorBounds.left, anchorBounds.top)
                val endR = if (hasSel) posRect(end) else startR
                val anchorTopY = startR[1]
                val anchorBottomY = endR?.get(2) ?: startR[2]
                val centerX = when {
                    !hasSel -> startR[0]
                    start.line == end.line && endR != null -> (startR[0] + endR[0]) / 2f
                    else -> startR[0]
                }
                return toolbarTopLeft(
                    anchorCenterX = anchorBounds.left + centerX,
                    anchorTopY = anchorBounds.top + anchorTopY,
                    anchorBottomY = anchorBounds.top + anchorBottomY,
                    contentW = popupContentSize.width,
                    contentH = popupContentSize.height,
                    windowW = windowSize.width,
                    windowH = windowSize.height,
                    gap = gapPx,
                    margin = marginPx,
                )
            }
        }
    }

    Popup(popupPositionProvider = provider, properties = PopupProperties(focusable = false)) {
        Row(
            Modifier
                .menuShadow(RoundedCornerShape(MENU_CORNER))
                .clip(RoundedCornerShape(MENU_CORNER))
                .background(colors.symbolBarBackground)
                .padding(horizontal = 4.dp, vertical = 3.dp),
            horizontalArrangement = Arrangement.spacedBy(2.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            actions.forEach { action ->
                ToolbarButton(label = action.zhLabel, colors = colors) { onPerform(action) }
            }
        }
    }
}

@Composable
private fun ToolbarButton(label: String, colors: EditorColors, onClick: () -> Unit) {
    var pressed by remember { mutableStateOf(false) }
    Box(
        Modifier
            .clip(RoundedCornerShape(6.dp))
            .background(if (pressed) colors.symbolBarPressed else Color.Transparent)
            // 与底部符号条同法：detectTapGestures 落点、不抢焦点 → 点操作不销毁 IME 会话、不收键盘。
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
            .defaultMinSize(minWidth = 44.dp, minHeight = 40.dp)
            .padding(horizontal = 12.dp, vertical = 8.dp),
        contentAlignment = Alignment.Center,
    ) {
        BasicText(text = label, style = TextStyle(color = colors.symbolBarForeground, fontSize = 15.sp))
    }
}

/**
 * 桌面右键上下文菜单（竖排下拉）：右键在编辑器内某点弹出，锚在点击点 [anchor]（编辑器局部像素）。四项常显、
 * 不适用者置灰禁用（符合桌面习惯）；选中某项 / 点菜单外 / Esc 触发 [onDismiss]（[PopupProperties] focusable=true）。
 */
@Composable
internal fun EditorContextMenu(
    engine: EditorEngine,
    colors: EditorColors,
    readOnly: Boolean,
    show: () -> Boolean,
    anchor: () -> Offset?,
    onDismiss: () -> Unit,
    onPerform: (EditorContextAction) -> Unit,
) {
    if (!show()) return
    val at = anchor() ?: return
    val availability = contextActionAvailability(
        readOnly = readOnly,
        hasSelection = !engine.selection.isEmpty,
        allSelected = engine.isAllSelected(),
        docNonEmpty = engine.buffer.totalLength() > 0,
        canUndo = engine.canUndo,
        canRedo = engine.canRedo,
    )

    val density = LocalDensity.current
    val marginPx = with(density) { MENU_MARGIN.roundToPx() }
    val provider = remember(at, marginPx) {
        object : PopupPositionProvider {
            override fun calculatePosition(
                anchorBounds: IntRect,
                windowSize: IntSize,
                layoutDirection: LayoutDirection,
                popupContentSize: IntSize,
            ): IntOffset {
                val x = (anchorBounds.left + at.x.roundToInt())
                    .coerceIn(marginPx, (windowSize.width - popupContentSize.width - marginPx).coerceAtLeast(marginPx))
                val y = (anchorBounds.top + at.y.roundToInt())
                    .coerceIn(marginPx, (windowSize.height - popupContentSize.height - marginPx).coerceAtLeast(marginPx))
                return IntOffset(x, y)
            }
        }
    }

    Popup(
        popupPositionProvider = provider,
        onDismissRequest = onDismiss,
        properties = PopupProperties(focusable = true),
    ) {
        Column(
            Modifier
                .menuShadow(RoundedCornerShape(6.dp))
                .clip(RoundedCornerShape(6.dp))
                .background(colors.symbolBarBackground)
                .padding(vertical = 4.dp)
                .width(IntrinsicSize.Max),
        ) {
            listOf(
                EditorContextAction.Undo, EditorContextAction.Redo,
                EditorContextAction.Cut, EditorContextAction.Copy,
                EditorContextAction.Paste, EditorContextAction.SelectAll,
            ).forEach { action ->
                ContextMenuRow(
                    label = action.zhLabel,
                    enabled = availability.isAvailable(action),
                    colors = colors,
                ) { onPerform(action) }
            }
        }
    }
}

@Composable
private fun ContextMenuRow(label: String, enabled: Boolean, colors: EditorColors, onClick: () -> Unit) {
    val interaction = remember { MutableInteractionSource() }
    val hovered by interaction.collectIsHoveredAsState()
    Box(
        Modifier
            .fillMaxWidth()
            .hoverable(interaction, enabled = enabled)
            .then(
                if (enabled) {
                    Modifier.pointerInput(label) { detectTapGestures(onTap = { onClick() }) }
                } else {
                    Modifier
                }
            )
            .background(if (hovered && enabled) colors.symbolBarPressed else Color.Transparent)
            .defaultMinSize(minWidth = 132.dp, minHeight = 32.dp)
            .padding(horizontal = 14.dp, vertical = 7.dp),
        contentAlignment = Alignment.CenterStart,
    ) {
        BasicText(
            text = label,
            style = TextStyle(
                color = colors.symbolBarForeground.copy(alpha = if (enabled) 1f else 0.38f),
                fontSize = 14.sp,
                textAlign = TextAlign.Start,
            ),
        )
    }
}
