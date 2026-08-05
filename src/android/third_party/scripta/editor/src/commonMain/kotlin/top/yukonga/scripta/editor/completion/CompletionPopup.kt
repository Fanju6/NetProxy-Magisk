package top.yukonga.scripta.editor.completion

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.hoverable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsHoveredAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicText
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.dropShadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.shadow.Shadow
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.DpOffset
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntRect
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupPositionProvider
import androidx.compose.ui.window.PopupProperties
import top.yukonga.scripta.editor.EditorColors
import top.yukonga.scripta.editor.text.TextPosition
import kotlin.math.roundToInt

private val COMPLETION_WIDTH = 260.dp
private val COMPLETION_MAX_LIST_HEIGHT = 132.dp
private val COMPLETION_GAP = 4.dp
private val COMPLETION_MARGIN = 8.dp
private val COMPLETION_CORNER = 8.dp

/** 光标附近的非抢焦点补全列表，软键盘打开时点击候选不会销毁输入会话。 */
@Composable
internal fun CompletionPopup(
    result: CompletionResult,
    selectedIndex: Int,
    caret: TextPosition,
    colors: EditorColors,
    posRect: (TextPosition) -> FloatArray?,
    onSelect: (Int) -> Unit,
    onAccept: (Int) -> Unit,
) {
    if (result.items.isEmpty()) return

    val density = LocalDensity.current
    val gapPx = with(density) { COMPLETION_GAP.roundToPx() }
    val marginPx = with(density) { COMPLETION_MARGIN.roundToPx() }
    val provider = remember(caret, gapPx, marginPx) {
        object : PopupPositionProvider {
            override fun calculatePosition(
                anchorBounds: IntRect,
                windowSize: IntSize,
                layoutDirection: LayoutDirection,
                popupContentSize: IntSize,
            ): IntOffset {
                val rect = posRect(caret) ?: return IntOffset(anchorBounds.left, anchorBounds.top)
                val x = (anchorBounds.left + rect[0].roundToInt()).coerceIn(
                    marginPx,
                    (windowSize.width - popupContentSize.width - marginPx).coerceAtLeast(marginPx),
                )
                val below = anchorBounds.top + rect[2].roundToInt() + gapPx
                val above = anchorBounds.top + rect[1].roundToInt() - popupContentSize.height - gapPx
                val y = if (below + popupContentSize.height <= windowSize.height - marginPx) {
                    below
                } else {
                    above.coerceAtLeast(marginPx)
                }
                return IntOffset(x, y)
            }
        }
    }
    val listState = rememberLazyListState()
    val safeSelectedIndex = selectedIndex.coerceIn(0, result.items.lastIndex)
    LaunchedEffect(safeSelectedIndex) {
        listState.animateScrollToItem(safeSelectedIndex)
    }

    Popup(
        popupPositionProvider = provider,
        properties = PopupProperties(focusable = false),
    ) {
        Column(
            modifier = Modifier
                .dropShadow(
                    shape = RoundedCornerShape(COMPLETION_CORNER),
                    shadow = Shadow(
                        radius = 10.dp,
                        color = Color.Black.copy(alpha = 0.3f),
                        offset = DpOffset(0.dp, 3.dp),
                    ),
                )
                .clip(RoundedCornerShape(COMPLETION_CORNER))
                .background(colors.symbolBarBackground)
                .width(COMPLETION_WIDTH)
                .padding(vertical = 2.dp),
        ) {
            LazyColumn(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = COMPLETION_MAX_LIST_HEIGHT),
                state = listState,
            ) {
                itemsIndexed(result.items) { index, item ->
                    CompletionRow(
                        item = item,
                        selected = index == safeSelectedIndex,
                        colors = colors,
                        onHover = { onSelect(index) },
                        onClick = { onAccept(index) },
                    )
                }
            }

            result.items[safeSelectedIndex].documentation
                ?.takeIf { it.isNotBlank() }
                ?.let { documentation ->
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .background(colors.symbolBarPressed.copy(alpha = 0.5f))
                            .padding(horizontal = 10.dp, vertical = 4.dp),
                    ) {
                        BasicText(
                            text = documentation,
                            style = TextStyle(
                                color = colors.symbolBarForeground.copy(alpha = 0.82f),
                                fontSize = 11.sp,
                                lineHeight = 14.sp,
                            ),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    }
                }
        }
    }
}

@Composable
private fun CompletionRow(
    item: CompletionItem,
    selected: Boolean,
    colors: EditorColors,
    onHover: () -> Unit,
    onClick: () -> Unit,
) {
    val interaction = remember { MutableInteractionSource() }
    val hovered by interaction.collectIsHoveredAsState()
    LaunchedEffect(hovered) {
        if (hovered) onHover()
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .hoverable(interaction)
            .background(if (selected || hovered) colors.symbolBarPressed else Color.Transparent)
            .pointerInput(item) { detectTapGestures(onTap = { onClick() }) }
            .defaultMinSize(minHeight = 34.dp)
            .padding(horizontal = 10.dp, vertical = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        BasicText(
            text = item.label,
            modifier = Modifier.weight(1f),
            style = TextStyle(
                color = colors.symbolBarForeground,
                fontSize = 13.sp,
                fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Normal,
            ),
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        item.detail?.takeIf { it.isNotBlank() }?.let { detail ->
            BasicText(
                text = detail,
                style = TextStyle(
                    color = colors.symbolBarForeground.copy(alpha = 0.58f),
                    fontSize = 11.sp,
                ),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}
