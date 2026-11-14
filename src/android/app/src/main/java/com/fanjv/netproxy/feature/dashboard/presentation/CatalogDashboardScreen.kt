package com.fanjv.netproxy.feature.dashboard.presentation

import android.icu.text.MeasureFormat
import android.icu.util.Measure
import android.icu.util.MeasureUnit

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
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.module.ModuleConnectionState
import com.fanjv.netproxy.core.ui.component.AppContentCard
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.LocalModuleAvailability
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import com.fanjv.netproxy.core.ui.component.WarningCard
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.component.appBarOverscroll
import com.fanjv.netproxy.core.ui.resolve
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import com.fanjv.netproxy.core.ui.component.AppTopAppBar as TopAppBar
import top.yukonga.miuix.kmp.blur.layerBackdrop
import com.fanjv.netproxy.core.ui.component.AppDropdown as OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.scrollEndHaptic
import kotlin.math.roundToInt

/** 新仪表盘：完整就绪时间以模块状态机为准，运行数据来自本机控制接口。 */
@Composable
internal fun CatalogDashboardScreen(
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
    isActive: Boolean = true,
    onReconnect: () -> Unit = {},
    viewModel: CatalogDashboardViewModel = netProxyViewModel()
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val availability = LocalModuleAvailability.current
    LaunchedEffect(availability) { viewModel.setAvailability(availability) }
    val snackbarHostState = rememberAppSnackbarHostState()
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface
    val noModuleMessage =
        if (availability?.state == ModuleConnectionState.UNTRUSTED) stringResource(R.string.module_identity_failed)
        else "${stringResource(R.string.no_module_title)}\n${stringResource(R.string.no_module_summary)}"

    LifecycleResumeEffect(isActive) {
        viewModel.setVisible(isActive)
        onPauseOrDispose { if (isActive) viewModel.setVisible(false) }
    }

    SnackbarNoticeEffect(
        eventId = state.noticeId,
        message = state.notice.resolve(),
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
                    title = stringResource(R.string.dashboard),
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
                    .appBarOverscroll(scrollBehavior)
                    .nestedScroll(scrollBehavior.nestedScrollConnection)
                    .padding(horizontal = 12.dp),
                contentPadding = innerPadding,
                overscrollEffect = null
            ) {
                if (state.connection != null && !state.connected) {
                    item {
                        DashboardWarning(noModuleMessage, onReconnect)
                    }
                } else if (state.serviceState == "failed" && state.serviceError.isNotBlank()) {
                    item { DashboardWarning(state.serviceError) }
                }

                item {
                    val serviceSummary = when {
                        state.isStarting -> stringResource(R.string.service_starting)
                        state.isStopping -> stringResource(R.string.service_stopping)
                        state.isReady -> stringResource(
                            R.string.dashboard_service_up,
                            formatUptime(state.uptimeSeconds)
                        )
                        state.serviceState == "failed" -> stringResource(R.string.dashboard_service_failed)
                        else -> stringResource(R.string.dashboard_service_stopped)
                    }
                    SpeedChartCard(
                        downloadSpeed = formatRate(state.downloadBytesPerSecond),
                        uploadSpeed = formatRate(state.uploadBytesPerSecond),
                        trafficSamples = state.trafficSamples,
                        statusSummary = serviceSummary,
                        isRunning = state.isReady,
                        serviceControlEnabled = availability?.available == true && state.connected &&
                            !state.loading &&
                            !state.isServiceControlBusy,
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
                    AppContentCard(modifier = Modifier.fillMaxWidth()) {
                        InfoRow(
                            title = stringResource(R.string.dashboard_lan_ip),
                            content = state.internalIp.takeUnless { it.isBlank() || it == "--" }
                                ?: stringResource(R.string.dashboard_lan_ip_unavailable),
                        )
                        InfoRow(
                            title = stringResource(R.string.dashboard_total_traffic),
                            content = stringResource(
                                R.string.dashboard_traffic,
                                formatBytes(state.uploadTotal),
                                formatBytes(state.downloadTotal)
                            ),
                        )
                    }
                }

                item { Spacer(Modifier.height(12.dp)) }
                item {
                    AppContentCard(modifier = Modifier.fillMaxWidth()) {
                        val modeValues = state.availableOutboundModes
                        val modeLabels = modeValues.map { mode ->
                            when (mode) {
                                "Rule" -> stringResource(R.string.dashboard_mode_rule)
                                "Proxy" -> stringResource(R.string.dashboard_mode_proxy)
                                "Direct" -> stringResource(R.string.dashboard_mode_direct)
                                "RuleAllowAds" -> stringResource(R.string.dashboard_mode_rule_allow_ads)
                                else -> mode
                            }
                        }
                        val selectedModeIndex = modeValues.indexOf(state.outboundMode)
                        val modeItems = if (selectedModeIndex >= 0) {
                            modeLabels
                        } else {
                            listOf(
                                if (state.outboundMode == "unknown") stringResource(R.string.dashboard_mode_unknown)
                                else state.outboundMode
                            ) + modeLabels
                        }
                        OverlayDropdownPreference(
                            title = stringResource(R.string.dashboard_outbound_mode),
                            enabled = availability?.available == true && !state.loading,
                            items = modeItems,
                            selectedIndex = if (selectedModeIndex >= 0) selectedModeIndex else 0,
                            onSelectedIndexChange = { index ->
                                val actualIndex = if (selectedModeIndex >= 0) index else index - 1
                                modeValues.getOrNull(actualIndex)?.let(viewModel::setMode)
                            }
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
        )
        ResourceUsageCard(
            modifier = Modifier.weight(1f),
            title = stringResource(R.string.mem_usage),
            value = "%.1f%%".format(memoryUsage),
        )
    }
}

@Composable
private fun ResourceUsageCard(
    modifier: Modifier,
    title: String,
    value: String,
) {
    AppContentCard(
        modifier = modifier,
        insideMargin = PaddingValues(16.dp)
    ) {
        Text(
            text = title,
            style = MiuixTheme.textStyles.body2,
            color = colorScheme.onSurface,
        )
        Text(
            text = value,
            modifier = Modifier.padding(top = 8.dp),
            style = MiuixTheme.textStyles.headline1,
            color = colorScheme.onSurfaceVariantSummary,
        )
    }
}

@Composable
private fun DashboardWarning(message: String, onClick: (() -> Unit)? = null) {
    WarningCard(
        message = message,
        onClick = onClick,
        modifier = Modifier
            .padding(top = 12.dp)
            .fillMaxWidth(),
    )
}

@Composable
private fun formatUptime(seconds: Long): String {
    val locale = LocalConfiguration.current.locales[0]
    val formatter = remember(locale) { MeasureFormat.getInstance(locale, MeasureFormat.FormatWidth.WIDE) }
    val hours = seconds / 3600
    val minutes = seconds % 3600 / 60
    return when {
        hours >= 24 && hours % 24 != 0L -> formatter.formatMeasures(
            Measure(hours / 24, MeasureUnit.DAY), Measure(hours % 24, MeasureUnit.HOUR),
        )
        hours >= 24 -> formatter.formatMeasures(Measure(hours / 24, MeasureUnit.DAY))
        hours > 0 && minutes > 0 -> formatter.formatMeasures(
            Measure(hours, MeasureUnit.HOUR), Measure(minutes, MeasureUnit.MINUTE),
        )
        hours > 0 -> formatter.formatMeasures(Measure(hours, MeasureUnit.HOUR))
        minutes > 0 -> formatter.formatMeasures(Measure(minutes, MeasureUnit.MINUTE))
        else -> formatter.formatMeasures(Measure(seconds.coerceAtLeast(0), MeasureUnit.SECOND))
    }
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
