package top.yukonga.scripta.editor.render

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.util.lerp
import top.yukonga.scripta.editor.EditorColors

// thumb 尺寸：静止态要够粗——发丝线在高密度屏上传达不了「可抓」；且要离物理右缘足够远——贴边的
// 可见条会让手指瞄准点落进厂商边缘防误触带，按下事件被系统吞掉、热区再大也救不回。拖拽态再展宽。
private val THUMB_IDLE_WIDTH = 5.dp
private val THUMB_ACTIVE_WIDTH = 8.dp
private val THUMB_EDGE_MARGIN = 4.dp

/**
 * 滚动条叠层：右缘纵向 thumb（可抓；抓取/拖拽由 CodeEditor 的手势层驱动，这里只画）。独立轻量
 * Canvas——淡出动画只重绘本层，不整层重录主画布。**不画横向条**：横向上界由 widestSeen 派生
 * （只增不减、只含已测量过的可见行），真正的最长行可能从未入测——按低估的假上界画比例/位置是
 * 误导，宁缺。纵向无此问题：行数精确，softWrap 的高度估算也只影响像素映射（拖拽已改行空间反解），
 * thumb 比例随测量收敛。
 *
 * draw 阶段读活值：alpha 驱动显隐与重绘唤醒（显示状态机写 Animatable），滚动量/上界驱动几何。
 * alpha ≤ 0 早退安全：唤醒不依赖本层订阅滚动量，由状态机的 alpha 写入触发。拖拽期 thumb 位置用
 * [thumbTopOverridePx]（跟手），不从 scrollY 反推——softWrap 下可见测量会抬高 maxScroll，反推会让
 * thumb 从指下溜走。
 */
@Composable
internal fun ScrollbarOverlay(
    colors: EditorColors,
    alpha: () -> Float,
    dragging: () -> Boolean,
    thumbTopOverridePx: () -> Float,
    scrollY: () -> Float,
    maxScrollY: () -> Float,
    minThumbPx: Float,
    modifier: Modifier = Modifier,
) {
    // 按住加宽的过渡：宽度与活动色由同一动画分数驱动（瞬变突兀）。组合期读 dragging() 只在拖拽
    // 沿重组本小层；动画分数逐帧由 draw 读、只重绘。
    val activeFraction by animateFloatAsState(
        targetValue = if (dragging()) 1f else 0f,
        animationSpec = tween(durationMillis = 140),
        label = "scrollbarActive",
    )
    Canvas(modifier) {
        val a = alpha()
        if (a <= 0f) return@Canvas
        val vh = size.height
        val vw = size.width
        val marginPx = THUMB_EDGE_MARGIN.toPx()

        val maxY = maxScrollY()
        val thumbH = ScrollbarMath.thumbHeight(vh, maxY, minThumbPx)
        if (thumbH > 0f) {
            val drag = dragging()
            val override = thumbTopOverridePx()
            val top = if (drag && override >= 0f) override.coerceIn(0f, vh - thumbH)
            else ScrollbarMath.thumbTop(vh, maxY, thumbH, scrollY())
            val f = activeFraction
            val w = lerp(THUMB_IDLE_WIDTH.toPx(), THUMB_ACTIVE_WIDTH.toPx(), f)
            drawRoundRect(
                color = lerp(colors.scrollbarThumb, colors.scrollbarThumbActive, f),
                topLeft = Offset(vw - w - marginPx, top),
                size = Size(w, thumbH),
                cornerRadius = CornerRadius(w / 2f),
                alpha = a,
            )
        }
    }
}
