package com.fanjv.netproxy.feature.dashboard.presentation

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.AltRoute
import androidx.compose.material.icons.rounded.DataUsage
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.material.icons.rounded.Memory
import androidx.compose.material.icons.rounded.Router
import androidx.compose.material.icons.rounded.Storage
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import com.fanjv.netproxy.core.ui.component.WarningCard
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic
import kotlin.math.roundToInt

/** 新仪表盘：完整就绪时间以模块状态机为准，运行数据来自本机控制接口。 */
@Composable
internal fun CatalogDashboardScreen(
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
    isActive: Boolean = true,
    onNavigateToNodes: () -> Unit = {},
    viewModel: CatalogDashboardViewModel = netProxyViewModel()
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbarHostState = rememberAppSnackbarHostState()
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface
    val noRootMessage =
        "${stringResource(R.string.no_root_title)}\n${stringResource(R.string.no_root_summary)}"
    val noModuleMessage =
        "${stringResource(R.string.no_module_title)}\n${stringResource(R.string.no_module_summary)}"

    DisposableEffect(isActive) {
        viewModel.setVisible(isActive)
        onDispose { if (isActive) viewModel.setVisible(false) }
    }

    SnackbarNoticeEffect(
        eventId = state.noticeId,
        message = state.notice,
        isError = false,
        hostState = snackbarHostState,
        onConsumed = viewModel::clearNotice
    )

    Scaffold(
        snackbarHost = {
            AppSnackbarHost(snackbarHostState, Modifier.padding(bottom = bottomPadding))
        },
        topBar = {
            BlurredBar(backdrop) {
                TopAppBar(
                    color = barColor,
                    title = "仪表盘",
                    scrollBehavior = scrollBehavior
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.only(WindowInsetsSides.Horizontal)
    ) { innerPadding ->
        Box(modifier = if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier) {
            LazyColumn(
                modifier = Modifier
                    .fillMaxSize()
                    .scrollEndHaptic()
                    .overScrollVertical()
                    .nestedScroll(scrollBehavior.nestedScrollConnection)
                    .padding(horizontal = 12.dp),
                contentPadding = innerPadding,
                overscrollEffect = null
            ) {
                if (state.rootChecked && !state.rootGranted) {
                    item {
                        DashboardWarning(noRootMessage)
                    }
                } else if (state.rootChecked && !state.moduleInstalled) {
                    item {
                        DashboardWarning(noModuleMessage)
                    }
                } else if (state.serviceState == "failed" && state.serviceError.isNotBlank()) {
                    item { DashboardWarning(state.serviceError) }
                }

                item {
                    val serviceSummary = when {
                        state.isStarting -> stringResource(R.string.service_starting)
                        state.isStopping -> stringResource(R.string.service_stopping)
                        state.isReady -> "UP: ${formatUptime(state.uptimeSeconds)}"
                        else -> stringResource(R.string.service_stopped)
                    }
                    SpeedChartCard(
                        downloadSpeed = formatRate(state.downloadBytesPerSecond),
                        uploadSpeed = formatRate(state.uploadBytesPerSecond),
                        downloadHistory = state.downloadHistory,
                        uploadHistory = state.uploadHistory,
                        statusSummary = serviceSummary,
                        isRunning = state.isReady,
                        serviceControlEnabled = !state.isServiceControlBusy,
                        modifier = Modifier.padding(top = 12.dp),
                        onToggleService = viewModel::toggleService
                    )
                }

                item { Spacer(Modifier.height(12.dp)) }
                item {
                    SystemStatsSection(
                        cpuUsage = state.cpuUsage,
                        memoryUsage = state.memoryUsage
                    )
                }

                item { Spacer(Modifier.height(12.dp)) }
                item {
                    Card(modifier = Modifier.fillMaxWidth()) {
                        InfoRow(
                            title = "内网 IP",
                            content = state.internalIp,
                            icon = Icons.Rounded.Router
                        )
                        InfoRow(
                            title = "累计流量",
                            content = "↑ ${formatBytes(state.uploadTotal)}   ↓ ${formatBytes(state.downloadTotal)}",
                            icon = Icons.Rounded.DataUsage
                        )
                    }
                }

                item { Spacer(Modifier.height(12.dp)) }
                item {
                    Card(modifier = Modifier.fillMaxWidth()) {
                        val modeValues = listOf("rule", "global", "direct", "AllowAds")
                        OverlayDropdownPreference(
                            title = "出站模式",
                            items = listOf("规则", "全局", "直连", "允许广告"),
                            selectedIndex = modeValues.indexOf(state.outboundMode).coerceAtLeast(0),
                            startAction = {
                                Icon(
                                    imageVector = Icons.AutoMirrored.Rounded.AltRoute,
                                    contentDescription = null,
                                    modifier = Modifier.padding(end = 12.dp),
                                    tint = colorScheme.primary
                                )
                            },
                            onSelectedIndexChange = { index -> viewModel.setMode(modeValues[index]) }
                        )
                        ArrowPreference(
                            title = "当前节点",
                            summary = state.currentNode.ifBlank { "未选择" },
                            startAction = {
                                Icon(
                                    imageVector = Icons.Rounded.Router,
                                    contentDescription = null,
                                    modifier = Modifier.padding(end = 12.dp),
                                    tint = colorScheme.primary
                                )
                            },
                            onClick = onNavigateToNodes
                        )
                    }
                }
                item { Spacer(Modifier.height(80.dp + bottomPadding)) }
            }
        }
    }
}

@Composable
private fun SystemStatsSection(cpuUsage: Float, memoryUsage: Float) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        ResourceUsageCard(
            modifier = Modifier.weight(1f),
            title = stringResource(R.string.cpu_usage),
            value = "%.1f%%".format(cpuUsage),
            icon = Icons.Rounded.Memory
        )
        ResourceUsageCard(
            modifier = Modifier.weight(1f),
            title = stringResource(R.string.mem_usage),
            value = "%.1f%%".format(memoryUsage),
            icon = Icons.Rounded.Storage
        )
    }
}

@Composable
private fun ResourceUsageCard(
    modifier: Modifier,
    title: String,
    value: String,
    icon: androidx.compose.ui.graphics.vector.ImageVector
) {
    Card(
        modifier = modifier,
        insideMargin = PaddingValues(16.dp)
    ) {
        Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
            Icon(
                imageVector = icon,
                contentDescription = null,
                tint = colorScheme.primary
            )
            Text(
                text = title,
                modifier = Modifier.padding(start = 8.dp),
                style = MiuixTheme.textStyles.body2,
                color = colorScheme.onSurfaceVariantSummary
            )
        }
        Text(
            text = value,
            modifier = Modifier.padding(top = 8.dp),
            style = MiuixTheme.textStyles.headline1
        )
    }
}

@Composable
private fun DashboardWarning(message: String) {
    WarningCard(
        message = message,
        modifier = Modifier
            .padding(top = 12.dp)
            .fillMaxWidth(),
        action = {
            Icon(
                Icons.Rounded.ErrorOutline,
                contentDescription = null,
                modifier = Modifier.padding(start = 16.dp),
                tint = if (MiuixTheme.isDynamicColor) {
                    colorScheme.onErrorContainer
                } else {
                    Color(0xFFF72727)
                }
            )
        }
    )
}

private fun formatUptime(seconds: Long): String {
    val hours = seconds / 3600
    val minutes = seconds % 3600 / 60
    val remaining = seconds % 60
    return "%02d:%02d:%02d".format(hours, minutes, remaining)
}

private fun formatRate(bytes: Long): String = "${formatBytes(bytes)}/s"

private fun formatBytes(bytes: Long): String {
    if (bytes < 1024) return "$bytes B"
    val units = arrayOf("KB", "MB", "GB", "TB")
    var value = bytes.toDouble()
    var index = -1
    do {
        value /= 1024.0
        index++
    } while (value >= 1024.0 && index < units.lastIndex)
    return if (value >= 100) "${value.roundToInt()} ${units[index]}"
    else "%.1f %s".format(value, units[index])
}
