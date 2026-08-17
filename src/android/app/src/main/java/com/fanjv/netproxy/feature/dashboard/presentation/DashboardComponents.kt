package com.fanjv.netproxy.feature.dashboard.presentation

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.tween
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Download
import androidx.compose.material.icons.rounded.Upload
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.R
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme

/** 服务状态、实时速率与最近流量趋势。 */
@Composable
internal fun SpeedChartCard(
    downloadSpeed: String,
    uploadSpeed: String,
    trafficSamples: List<TrafficSample>,
    statusSummary: String,
    isRunning: Boolean,
    serviceControlEnabled: Boolean,
    modifier: Modifier = Modifier,
    onToggleService: () -> Unit = {}
) {
    Card(modifier = modifier.fillMaxWidth()) {
        Column {
            SwitchPreference(
                title = stringResource(R.string.service_status),
                summary = statusSummary,
                checked = isRunning,
                onCheckedChange = { if (serviceControlEnabled) onToggleService() }
            )
            AnimatedVisibility(
                visible = isRunning,
                enter = expandVertically() + fadeIn(),
                exit = shrinkVertically() + fadeOut()
            ) {
                Column {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 16.dp)
                            .padding(bottom = 7.dp),
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        SpeedLabel(Icons.Rounded.Download, downloadSpeed, Color(0xFF2196F3))
                        SpeedLabel(Icons.Rounded.Upload, uploadSpeed, Color(0xFF4CAF50))
                    }
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(112.dp)
                            .padding(horizontal = 16.dp)
                            .padding(bottom = 15.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        SpeedChart(
                            trafficSamples = trafficSamples,
                            modifier = Modifier.fillMaxSize()
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun SpeedLabel(icon: ImageVector, value: String, color: Color) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Icon(icon, null, modifier = Modifier.size(14.dp), tint = color)
        Spacer(Modifier.width(4.dp))
        Text(
            text = value,
            style = MiuixTheme.textStyles.body2,
            color = colorScheme.onSurfaceVariantActions
        )
    }
}

/** 将最近一段流量采样绘制为双折线。 */
@Composable
private fun SpeedChart(
    trafficSamples: List<TrafficSample>,
    modifier: Modifier = Modifier
) {
    val downloadColor = Color(0xFF2196F3)
    val uploadColor = Color(0xFF4CAF50)
    val geometry = remember(trafficSamples) { trafficChartGeometry(trafficSamples) }
    var fromGeometry by remember { mutableStateOf(geometry) }
    var targetGeometry by remember { mutableStateOf(geometry) }
    val animationProgress = remember { Animatable(1f) }
    LaunchedEffect(geometry) {
        fromGeometry = targetGeometry
        targetGeometry = geometry
        animationProgress.snapTo(0f)
        animationProgress.animateTo(
            targetValue = 1f,
            animationSpec = tween(durationMillis = 700, easing = FastOutSlowInEasing)
        )
    }
    val animatedGeometry = interpolateTrafficChartGeometry(
        from = fromGeometry,
        to = targetGeometry,
        progress = animationProgress.value
    )
    val gridColor = colorScheme.onSurfaceVariantActions.copy(alpha = 0.12f)
    Canvas(modifier = modifier) {
        for (line in 1..3) {
            val y = size.height * line / 4f
            drawLine(
                color = gridColor,
                start = Offset(0f, y),
                end = Offset(size.width, y),
                strokeWidth = 1.dp.toPx()
            )
        }

        fun drawSeries(points: List<Offset>, color: Color, alpha: Float) {
            if (points.isEmpty()) return
            val line = Path().apply {
                val first = points.first()
                moveTo(first.x * size.width, (1f - first.y) * size.height)
                for (index in 1 until points.lastIndex) {
                    val current = points[index]
                    val next = points[index + 1]
                    val midpointX = (current.x + next.x) / 2f
                    val midpointY = (current.y + next.y) / 2f
                    quadraticTo(
                        current.x * size.width,
                        (1f - current.y) * size.height,
                        midpointX * size.width,
                        (1f - midpointY) * size.height
                    )
                }
                if (points.size > 1) {
                    val last = points.last()
                    lineTo(last.x * size.width, (1f - last.y) * size.height)
                }
            }
            val fill = Path().apply {
                addPath(line)
                lineTo(points.last().x * size.width, size.height)
                lineTo(points.first().x * size.width, size.height)
                close()
            }
            drawPath(
                path = fill,
                brush = Brush.verticalGradient(listOf(color.copy(alpha = alpha), Color.Transparent))
            )
            drawPath(
                path = line,
                color = color,
                style = Stroke(
                    width = 2.dp.toPx(),
                    cap = StrokeCap.Round,
                    join = StrokeJoin.Round
                )
            )
        }
        drawSeries(animatedGeometry.download, downloadColor, 0.25f)
        drawSeries(animatedGeometry.upload, uploadColor, 0.15f)
    }
}

/** 仪表盘中的单行摘要。 */
@Composable
internal fun InfoRow(title: String, content: String, icon: ImageVector) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(16.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(icon, null, modifier = Modifier.size(20.dp), tint = colorScheme.primary)
            Spacer(Modifier.width(12.dp))
            Text(title, style = MiuixTheme.textStyles.body1)
        }
        Text(
            content,
            style = MiuixTheme.textStyles.body2,
            color = colorScheme.onSurfaceVariantActions
        )
    }
}
