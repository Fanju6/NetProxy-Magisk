package com.fanjv.netproxy.core.ui.component

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithCache
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.RoundRect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.compositeOver
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipRect
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.CardDefaults
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.PressFeedbackType

@Composable
internal fun AppContentCard(
    modifier: Modifier = Modifier,
    cornerRadius: Dp = CardDefaults.CornerRadius,
    insideMargin: PaddingValues = CardDefaults.InsideMargin,
    selected: Boolean = false,
    onClick: (() -> Unit)? = null,
    onLongPress: (() -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    val radius = if (useGlassChrome) cornerRadius + 4.dp else cornerRadius
    val surface = colorScheme.surfaceContainer
    val fill = when {
        selected && useGlassChrome -> colorScheme.primary.copy(alpha = 0.14f).compositeOver(surface)
        selected -> colorScheme.primary.copy(alpha = 0.10f)
        else -> surface
    }
    val rim = when {
        selected -> colorScheme.primary.copy(alpha = if (useGlassChrome) 0.55f else 1f)
        useGlassChrome -> contentCardRim()
        else -> Color.Transparent
    }
    Card(
        modifier = if (rim == Color.Transparent) modifier else modifier.border(
            width = if (useGlassChrome) 0.5.dp else 1.5.dp,
            color = rim,
            shape = RoundedCornerShape(radius),
        ),
        cornerRadius = radius,
        insideMargin = insideMargin,
        colors = CardDefaults.defaultColors(color = fill),
        pressFeedbackType = if (onClick != null || onLongPress != null) PressFeedbackType.Sink else PressFeedbackType.None,
        showIndication = true,
        onClick = onClick,
        onLongPress = onLongPress,
        content = content,
    )
}

@Composable
internal fun contentCardRim(): Color = colorScheme.onSurface.copy(alpha = 0.09f)

internal fun Modifier.contentCardSegmentRim(radius: Dp, first: Boolean, last: Boolean, color: Color): Modifier =
    drawWithCache {
        val width = 0.5.dp.toPx()
        val inset = width / 2
        val corner = CornerRadius((radius.toPx() - inset).coerceAtLeast(0f))
        val path = Path().apply {
            addRoundRect(RoundRect(
                left = inset,
                top = if (first) inset else -width,
                right = size.width - inset,
                bottom = if (last) size.height - inset else size.height + width,
                topLeftCornerRadius = if (first) corner else CornerRadius.Zero,
                topRightCornerRadius = if (first) corner else CornerRadius.Zero,
                bottomLeftCornerRadius = if (last) corner else CornerRadius.Zero,
                bottomRightCornerRadius = if (last) corner else CornerRadius.Zero,
            ))
        }
        onDrawWithContent {
            drawContent()
            // Open edges only draw the sides, so consecutive lazy rows have no divider.
            if (size.width > width && size.height > width) {
                clipRect { drawPath(path, color, style = Stroke(width)) }
            }
        }
    }
