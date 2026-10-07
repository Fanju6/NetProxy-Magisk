package com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.PlatformTextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.LineHeightStyle
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.fanjv.netproxy.R
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.CardDefaults
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.PressFeedbackType

/** 单个节点卡片，只负责展示和把点击事件交给列表容器。 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
internal fun NodeCard(
    title: String,
    summary: String,
    protocol: String,
    latency: String? = null,
    selected: Boolean,
    enabled: Boolean,
    itemSize: Int,
    onClick: () -> Unit,
    onLongClick: (() -> Unit)? = null
) {
    val cornerRadius = when (itemSize) {
        1 -> 12.dp
        2 -> 8.dp
        else -> 16.dp
    }
    val innerPadding = when (itemSize) {
        1 -> 12.dp
        2 -> 8.dp
        else -> 16.dp
    }
    val titleFontSize = when (itemSize) {
        1 -> 13
        2 -> 12
        else -> 14
    }
    val detailFontSize = when (itemSize) {
        1 -> 11
        2 -> 10
        else -> 12
    }
    val lineHeightStyle = LineHeightStyle(
        alignment = LineHeightStyle.Alignment.Center,
        trim = LineHeightStyle.Trim.None
    )
    val titleStyle = MiuixTheme.textStyles.body1.copy(
        fontSize = titleFontSize.sp,
        lineHeight = (titleFontSize + 6).sp,
        platformStyle = PlatformTextStyle(includeFontPadding = false),
        lineHeightStyle = lineHeightStyle
    )
    val detailStyle = MiuixTheme.textStyles.body2.copy(
        fontSize = detailFontSize.sp,
        lineHeight = (detailFontSize + 6).sp,
        platformStyle = PlatformTextStyle(includeFontPadding = false),
        lineHeightStyle = lineHeightStyle
    )
    val valueStyle = detailStyle.copy(fontSize = (detailFontSize - 1).sp)
    val density = LocalDensity.current
    val titleHeight = with(density) { titleStyle.lineHeight.toDp() }
    val detailHeight = with(density) { detailStyle.lineHeight.toDp() }
    val shape = RoundedCornerShape(cornerRadius)
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .graphicsLayer {
                this.shape = shape
                clip = true
            }
            .border(
                width = if (selected) 1.5.dp else 0.dp,
                color = if (selected) colorScheme.primary else Color.Transparent,
                shape = shape
            ),
        cornerRadius = cornerRadius,
        insideMargin = PaddingValues(0.dp),
        colors = CardDefaults.defaultColors(
            color = if (selected) colorScheme.primary.copy(alpha = 0.1f)
            else colorScheme.surfaceContainer
        ),
        onClick = if (enabled) onClick else null,
        onLongPress = if (enabled) onLongClick else null,
        pressFeedbackType = PressFeedbackType.Sink,
        showIndication = true
    ) {
        Column(modifier = Modifier
            .fillMaxWidth()
            .padding(innerPadding)) {
            // 内容槽独立于文字度量，避免中文、emoji 和测速状态改变卡片高度。
            Row(
                modifier = Modifier.height(titleHeight),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = title,
                    modifier = Modifier.weight(1f),
                    color = if (selected) colorScheme.primary else colorScheme.onSurface,
                    style = titleStyle,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
            Spacer(Modifier.height(if (itemSize == 0) 8.dp else if (itemSize == 1) 4.dp else 2.dp))
            Row(
                modifier = Modifier.fillMaxWidth().height(detailHeight),
                verticalAlignment = Alignment.CenterVertically
            ) {
                if (protocol.isNotBlank()) {
                    Text(
                        text = protocol,
                        modifier = Modifier.weight(1f),
                        color = if (selected) colorScheme.primary else colorScheme.onSurfaceVariantActions,
                        style = detailStyle,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
                if (latency != null) {
                    Text(
                        text = latencyLabel(latency),
                        modifier = Modifier.padding(start = 8.dp),
                        color = latencyColor(latency),
                        style = valueStyle.copy(fontWeight = FontWeight.Medium),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                } else if (summary.isNotBlank()) {
                    Text(
                        text = summary,
                        modifier = Modifier
                            .padding(start = 8.dp)
                            .weight(1f),
                        color = colorScheme.onSurfaceVariantSummary,
                        style = valueStyle,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }
        }
    }
}

@Composable
private fun latencyLabel(value: String): String = when (value) {
    "testing..." -> stringResource(R.string.latency_testing)
    "failed" -> stringResource(R.string.latency_failed)
    "timeout" -> stringResource(R.string.latency_timeout)
    else -> "$value ms"
}

@Composable
private fun latencyColor(value: String): Color = when (value) {
    "testing..." -> colorScheme.primary
    "failed", "timeout" -> if (MiuixTheme.isDynamicColor) colorScheme.error else Color(0xFFF72727)
    else -> when (value.toIntOrNull() ?: Int.MAX_VALUE) {
        in 0..799 -> Color(0xFF32A852)
        in 800..1499 -> Color(0xFFE39A20)
        else -> if (MiuixTheme.isDynamicColor) colorScheme.error else Color(0xFFF05252)
    }
}
