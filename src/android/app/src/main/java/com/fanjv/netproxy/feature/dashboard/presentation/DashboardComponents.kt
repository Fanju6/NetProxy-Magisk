package com.fanjv.netproxy.feature.dashboard.presentation

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Animatable
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
    downloadHistory: List<Float>,
    uploadHistory: List<Float>,
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
                        if (downloadHistory.isEmpty() && uploadHistory.isEmpty()) {
                            Text(
                                text = stringResource(R.string.collecting_data),
                                style = MiuixTheme.textStyles.body2,
                                color = colorScheme.onSurfaceVariantActions
                            )
                        } else {
                            SpeedChart(
                                downloadHistory = downloadHistory,
                                uploadHistory = uploadHistory,
                                modifier = Modifier.fillMaxSize()
                            )
                        }
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

/** 将最近 40 个采样点绘制为平滑双折线。 */
@Composable
private fun SpeedChart(
    downloadHistory: List<Float>,
    uploadHistory: List<Float>,
    modifier: Modifier = Modifier
) {
    val downloadColor = Color(0xFF2196F3)
    val uploadColor = Color(0xFF4CAF50)
    val animation = remember { Animatable(0f) }
    var previousDownload by remember { mutableStateOf<List<Offset>>(emptyList()) }
    var currentDownload by remember { mutableStateOf<List<Offset>>(emptyList()) }
    var previousUpload by remember { mutableStateOf<List<Offset>>(emptyList()) }
    var currentUpload by remember { mutableStateOf<List<Offset>>(emptyList()) }

    LaunchedEffect(downloadHistory, uploadHistory) {
        previousDownload = currentDownload
        previousUpload = currentUpload
        val maxSpeed = maxOf(
            downloadHistory.maxOrNull() ?: 0f,
            uploadHistory.maxOrNull() ?: 0f,
            100f
        )
        currentDownload = normalizedPoints(downloadHistory, maxSpeed)
        currentUpload = normalizedPoints(uploadHistory, maxSpeed)
        animation.snapTo(0f)
        animation.animateTo(1f, tween(300))
    }

    val progress = animation.value
    val download = interpolate(previousDownload, currentDownload, progress)
    val upload = interpolate(previousUpload, currentUpload, progress)
    Canvas(modifier = modifier) {
        fun drawSeries(points: List<Offset>, color: Color, alpha: Float) {
            if (points.isEmpty()) return
            val line = smoothPath(points, size.width, size.height)
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
            drawPath(path = line, color = color, style = Stroke(width = 2.dp.toPx()))
        }
        drawSeries(download, downloadColor, 0.25f)
        drawSeries(upload, uploadColor, 0.15f)
    }
}

private fun normalizedPoints(history: List<Float>, maxSpeed: Float): List<Offset> =
    history.takeLast(40).mapIndexed { index, speed ->
        Offset(index / 39f, speed / maxSpeed)
    }

private fun interpolate(
    previous: List<Offset>,
    current: List<Offset>,
    progress: Float
): List<Offset> = current.mapIndexed { index, point ->
    val start = previous.getOrNull(index) ?: point
    Offset(
        x = start.x + (point.x - start.x) * progress,
        y = start.y + (point.y - start.y) * progress
    )
}

private fun smoothPath(points: List<Offset>, width: Float, height: Float): Path = Path().apply {
    moveTo(points.first().x * width, (1f - points.first().y) * height)
    for (index in 1 until points.lastIndex) {
        val current = points[index]
        val next = points[index + 1]
        quadraticTo(
            current.x * width,
            (1f - current.y) * height,
            (current.x + next.x) / 2f * width,
            (1f - (current.y + next.y) / 2f) * height
        )
    }
    if (points.size > 1) {
        val last = points.last()
        lineTo(last.x * width, (1f - last.y) * height)
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
