package com.fanjv.netproxy.feature.logs.presentation

import android.content.ClipData
import android.content.Intent
import android.net.Uri
import android.os.ParcelFileDescriptor
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowForward
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.FileProvider
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppContentCard
import com.fanjv.netproxy.core.ui.component.AppDialog
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.core.ui.component.ManagerMenu
import com.fanjv.netproxy.core.ui.component.appBarOverscroll
import com.fanjv.netproxy.core.ui.component.deferredTopPadding
import com.fanjv.netproxy.core.ui.component.groupedCardItems
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.theme.isInDarkTheme
import com.fanjv.netproxy.feature.logs.data.LogItem
import com.fanjv.netproxy.feature.logs.data.LogLevel
import com.fanjv.netproxy.feature.logs.data.LogType
import com.fanjv.netproxy.feature.logs.data.OutboundFlow
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.DropdownItem
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.SnackbarDuration
import top.yukonga.miuix.kmp.glass.GlassTabRow
import top.yukonga.miuix.kmp.glass.GlassTabRowDefaults
import top.yukonga.miuix.kmp.basic.Text
import com.fanjv.netproxy.core.ui.component.AppTopAppBar as TopAppBar
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

/** 日志页：查看并导出服务与内核日志。 */
@Composable
internal fun LogsScreen(
    viewModel: LogsViewModel = com.fanjv.netproxy.core.di.netProxyViewModel(),
    onBack: () -> Unit
) {
    val logsState by viewModel.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val snackbarHostState = rememberAppSnackbarHostState()
    val listState = rememberLazyListState()
    val sendLogTitle = stringResource(R.string.send_log)
    val logSavedMessage = stringResource(R.string.log_saved)
    val clearSuccessMessage = stringResource(R.string.clear_logs_success)
    val clearFailedMessage = stringResource(R.string.clear_logs_failed)
    val saveLocationFailed = stringResource(R.string.log_save_location_failed)
    val saveEmptyFailed = stringResource(R.string.log_save_empty_failed)
    val saveFailed = stringResource(R.string.log_save_failed)
    val shareFailed = stringResource(R.string.log_share_failed)
    val exportFailed = stringResource(R.string.log_export_failed)
    val shareFailedDetail = stringResource(R.string.log_share_failed_detail)

    fun showMessage(message: String, isError: Boolean = false) {
        scope.launch {
            snackbarHostState.showSnackbar(
                message = message,
                withDismissAction = isError,
                duration = if (isError) SnackbarDuration.Long else SnackbarDuration.Short
            )
        }
    }

    var selectedTabIndex by remember { mutableIntStateOf(0) }
    var isCardView by remember { mutableStateOf(true) }
    var clearTarget by remember { mutableStateOf<LogType?>(null) }

    val currentType = remember(selectedTabIndex) {
        when (selectedTabIndex) {
            0 -> LogType.SERVICE
            else -> LogType.KERNEL
        }
    }

    LaunchedEffect(currentType) {
        viewModel.refresh(currentType)
    }

    val content = logsState[currentType]
    val logs = content.entries
    LaunchedEffect(currentType, content.error) {
        if (content.error.isNotEmpty() && logs.isNotEmpty()) showMessage(content.error, isError = true)
    }

    // 导出保存启动器
    val exportLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.CreateDocument("application/gzip")
    ) { uri: Uri? ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            try {
                withContext(Dispatchers.IO) {
                    val logFile = viewModel.createReport()
                    try {
                        val descriptor = context.contentResolver.openFileDescriptor(uri, "rwt")
                            ?: error(saveLocationFailed)
                        val copiedBytes =
                            ParcelFileDescriptor.AutoCloseOutputStream(descriptor).use { output ->
                                logFile.inputStream().use { input ->
                                    input.copyTo(output).also { output.flush() }
                                }
                            }
                        check(copiedBytes == logFile.length() && copiedBytes > 0L) {
                            saveEmptyFailed
                        }
                    } finally {
                        logFile.delete()
                    }
                }
                showMessage(logSavedMessage)
            } catch (e: Exception) {
                if (e is CancellationException) throw e
                showMessage(e.message ?: saveFailed, isError = true)
            }
        }
    }

    val tabs = listOf(
        stringResource(R.string.service_logs),
        stringResource(R.string.kernel_logs)
    )

    val scrollBehavior = MiuixScrollBehavior()
    val dynamicTopPadding = remember(scrollBehavior) {
        { 12.dp * (1f - scrollBehavior.state.collapsedFraction) }
    }
    val backdrop = rememberBlurBackdrop()
    val blurActive = backdrop != null
    val barColor = if (blurActive) Color.Transparent else MiuixTheme.colorScheme.surface

    fun shareLogs() {
        scope.launch {
            try {
                val logFile = withContext(Dispatchers.IO) { viewModel.createReport() }
                val uri = FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", logFile)
                val intent = Intent(Intent.ACTION_SEND).apply {
                    type = "application/gzip"
                    putExtra(Intent.EXTRA_STREAM, uri)
                    clipData = ClipData.newRawUri(sendLogTitle, uri)
                    addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                }
                context.startActivity(Intent.createChooser(intent, sendLogTitle))
            } catch (e: Exception) {
                if (e is CancellationException) throw e
                showMessage(e.message ?: shareFailedDetail.format(shareFailed), isError = true)
            }
        }
    }

    fun saveLogs() {
        try {
            val timestamp = java.time.format.DateTimeFormatter.ofPattern("yyyyMMdd_HHmm")
                .format(java.time.LocalDateTime.now())
            exportLauncher.launch("NetProxy_Logs_$timestamp.tar.gz")
        } catch (e: Exception) {
            showMessage(exportFailed.format(e.message ?: saveFailed), isError = true)
        }
    }

    val canChangeView = !content.loading && (logs.isNotEmpty() || content.error.isEmpty())
    val canScroll = logs.isNotEmpty()
    ManagerMenu(
        entries = listOf(
            DropdownEntry(
                items = listOf(
                    DropdownItem(
                        text = stringResource(if (isCardView) R.string.raw_view else R.string.card_view),
                        enabled = canChangeView,
                        onClick = { isCardView = !isCardView },
                    ),
                    DropdownItem(
                        text = stringResource(R.string.log_jump),
                        enabled = canScroll,
                        children = listOf(
                            DropdownItem(
                                text = stringResource(R.string.scroll_to_top),
                                onClick = { scope.launch { listState.animateScrollToItem(0) } },
                            ),
                            DropdownItem(
                                text = stringResource(R.string.scroll_to_bottom),
                                onClick = {
                                    if (logs.isNotEmpty()) scope.launch { listState.animateScrollToItem(logs.lastIndex) }
                                },
                            ),
                        ),
                    ),
                    DropdownItem(
                        text = stringResource(R.string.log_export),
                        children = listOf(
                            DropdownItem(text = stringResource(R.string.send_log), onClick = ::shareLogs),
                            DropdownItem(text = stringResource(R.string.save_log), onClick = ::saveLogs),
                        ),
                    ),
                ),
            ),
            DropdownEntry(
                items = listOf(
                    DropdownItem(
                        text = stringResource(R.string.clear_logs_now),
                        enabled = canChangeView,
                        onClick = { clearTarget = currentType },
                    ),
                ),
            ),
        ),
        description = stringResource(R.string.more_options),
    ) { moreButton ->
        Scaffold(
            snackbarHost = { AppSnackbarHost(snackbarHostState) },
            topBar = {
                BlurredBar(backdrop) {
                    TopAppBar(
                        color = barColor,
                        title = stringResource(R.string.logs),
                        navigationIcon = {
                            BackIconButton(onClick = onBack)
                        },
                        actions = { moreButton() },
                        scrollBehavior = scrollBehavior,
                        bottomContent = {
                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(horizontal = 12.dp)
                                    .padding(bottom = 6.dp)
                                    .deferredTopPadding(dynamicTopPadding)
                            ) {
                                GlassTabRow(
                                    tabs = tabs,
                                    selectedIndex = selectedTabIndex,
                                    onSelect = { selectedTabIndex = it },
                                    backdrop = null,
                                    modifier = Modifier.fillMaxWidth(),
                                    colors = GlassTabRowDefaults.primaryColors().copy(
                                        selectedContentColor = MiuixTheme.colorScheme.onPrimary,
                                    ),
                                )
                            }
                        }
                    )
                }
            },
        ) { innerPadding ->
            val layoutDirection = LocalLayoutDirection.current
            LazyColumn(
                state = listState,
                modifier = Modifier
                    .fillMaxSize()
                    .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)
                    .scrollEndHaptic()
                    .appBarOverscroll(scrollBehavior)
                    .nestedScroll(scrollBehavior.nestedScrollConnection),
                contentPadding = PaddingValues(
                    top = innerPadding.calculateTopPadding() + 6.dp,
                    start = innerPadding.calculateStartPadding(layoutDirection) + 12.dp,
                    end = innerPadding.calculateEndPadding(layoutDirection) + 12.dp,
                    bottom = innerPadding.calculateBottomPadding()
                ),
                overscrollEffect = null
            ) {
                if (logs.isEmpty()) {
                    item {
                        ContentStatus(
                            padding = PaddingValues(0.dp),
                            text = stringResource(if (content.error.isNotEmpty()) R.string.logs_read_failed else R.string.no_logs),
                            loading = content.loading,
                            modifier = Modifier.fillParentMaxSize(),
                        )
                    }
                } else {
                    if (isCardView) {
                        items(logs) { item ->
                            LogItemCard(item = item, type = currentType)
                        }
                    } else {
                        rawLogItems(logs = logs)
                    }
                    item {
                        Spacer(modifier = Modifier.height(16.dp))
                    }
                }
            }
        }
    }

    AppDialog(
        show = clearTarget != null,
        title = stringResource(R.string.clear_logs_now),
        onDismissRequest = { clearTarget = null },
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            AppDialogButton(
                text = stringResource(R.string.common_cancel),
                modifier = Modifier.weight(1f),
                onClick = { clearTarget = null },
            )
            AppDialogButton(
                text = stringResource(R.string.common_confirm),
                modifier = Modifier.weight(1f),
                role = DialogActionRole.Destructive,
                onClick = {
                    val target = clearTarget
                    clearTarget = null
                    if (target != null) {
                        viewModel.clear(target) { success ->
                            showMessage(
                                if (success) clearSuccessMessage else clearFailedMessage,
                                isError = !success,
                            )
                        }
                    }
                },
            )
        }
    }
}

@Composable
fun LogItemCard(item: LogItem, type: LogType) {
    val service = type == LogType.SERVICE
    AppContentCard(
        modifier = Modifier
            .fillMaxWidth()
            .padding(bottom = 12.dp),
        insideMargin = PaddingValues(12.dp)
    ) {
        Column {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Row(
                    modifier = Modifier.weight(1f),
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    if (service && item.result.isNotEmpty()) {
                        NativeResultBadge(item.result)
                    } else {
                        LogLevelBadge(level = item.level)
                    }
                    if (service) {
                        val component = nativeComponentLabel(item.tag)
                        val event = nativeEventLabel(item.event)
                        if (component.isNotEmpty()) {
                            LogBadge(component, LogBadgeTone.NEUTRAL, Modifier.weight(1f, fill = false))
                        }
                        if (event.isNotEmpty() && event != component) {
                            LogBadge(event, LogBadgeTone.NEUTRAL, Modifier.weight(2f, fill = false))
                        }
                    } else if (item.tag.isNotEmpty()) {
                        Text(
                            text = item.tag,
                            fontSize = 12.sp,
                            fontWeight = FontWeight.Bold,
                            color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f, fill = false)
                        )
                    }
                }
                if (item.timestamp.isNotEmpty()) {
                    Spacer(modifier = Modifier.width(8.dp))
                    Text(
                        text = item.timestamp,
                        fontSize = 11.sp,
                        color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                        maxLines = 1
                    )
                }
            }

            if (item.connectionId != null) {
                Spacer(modifier = Modifier.height(6.dp))
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    LogBadge(text = "#${item.connectionId}", tone = LogBadgeTone.CONNECTION)
                    if (item.latency != null) {
                        LatencyBadge(latency = item.latency, latencyMs = item.latencyMs)
                    }
                }
            }

            Spacer(modifier = Modifier.height(8.dp))

            if (!service && item.outboundFlow != null) {
                OutboundFlowView(flow = item.outboundFlow)
            } else {
                Text(
                    text = item.message,
                    fontSize = 13.sp,
                    color = MiuixTheme.colorScheme.onSurface,
                    lineHeight = 18.sp
                )
            }
            if (service && item.errorCode.isNotEmpty()) {
                Spacer(modifier = Modifier.height(4.dp))
                Text(
                    text = item.errorCode,
                    style = MiuixTheme.textStyles.footnote2,
                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                )
            }
        }
    }
}

@Composable
private fun nativeComponentLabel(component: String): String =
    when (component) {
        "service" -> stringResource(R.string.log_component_service)
        "worker" -> stringResource(R.string.log_component_worker)
        "subscription" -> stringResource(R.string.subscriptions)
        "node" -> stringResource(R.string.nodes)
        "mode" -> stringResource(R.string.log_component_mode)
        "app" -> stringResource(R.string.apps)
        "config" -> stringResource(R.string.log_component_config)
        "network" -> stringResource(R.string.log_component_network)
        "module" -> stringResource(R.string.log_component_module)
        else -> component
    }

@Composable
private fun nativeEventLabel(event: String): String =
    when (event) {
        "service.start" -> stringResource(R.string.log_event_start)
        "service.stop" -> stringResource(R.string.log_event_stop)
        "service.reload" -> stringResource(R.string.log_event_reload)
        "worker.start", "worker.run" -> "Worker"
        "network.watch" -> stringResource(R.string.log_event_network_watch)
        "network.read" -> stringResource(R.string.log_event_network_read)
        "network.policy" -> stringResource(R.string.log_event_network_policy)
        "subscription.add" -> stringResource(R.string.log_event_subscription_add)
        "subscription.edit" -> stringResource(R.string.subscription_editor_edit_title)
        "subscription.update" -> stringResource(R.string.log_event_subscription_update)
        "subscription.update-all" -> stringResource(R.string.log_event_subscription_update_all)
        "subscription.remove" -> stringResource(R.string.log_event_subscription_remove)
        "subscription.runtime-sync" -> stringResource(R.string.log_event_runtime_sync)
        "subscription.effect" -> stringResource(R.string.log_event_effect)
        "subscription.schedule" -> stringResource(R.string.log_event_schedule)
        "node.append" -> stringResource(R.string.node_add)
        "node.import" -> stringResource(R.string.log_event_node_import)
        "node.edit" -> stringResource(R.string.node_edit)
        "node.remove" -> stringResource(R.string.node_delete)
        "node.select", "node.selection" -> stringResource(R.string.log_event_node_select)
        "mode.apply" -> stringResource(R.string.log_event_mode_apply)
        "app-policy.update" -> stringResource(R.string.log_event_app_policy)
        "config.apply" -> stringResource(R.string.log_event_config_apply)
        "config.rollback" -> stringResource(R.string.log_event_config_rollback)
        "config.validate" -> stringResource(R.string.log_event_config_validate)
        "module.boot" -> stringResource(R.string.log_event_boot)
        "module.update" -> stringResource(R.string.log_event_module_update)
        else -> event
    }

@Composable
private fun NativeResultBadge(result: String) {
    val (label, tone) = when (result) {
        "success", "recovered" -> stringResource(R.string.log_result_success) to LogBadgeTone.SUCCESS
        "failed", "forced" -> stringResource(R.string.log_result_failed) to LogBadgeTone.ERROR
        "persisted" -> stringResource(R.string.log_result_persisted) to LogBadgeTone.WARNING
        "fallback" -> stringResource(R.string.log_result_fallback) to LogBadgeTone.WARNING
        "partial" -> stringResource(R.string.log_result_partial) to LogBadgeTone.WARNING
        "started" -> stringResource(R.string.log_result_started) to LogBadgeTone.NEUTRAL
        "already-running" -> stringResource(R.string.log_result_running) to LogBadgeTone.INFO
        "stopped" -> stringResource(R.string.log_result_stopped) to LogBadgeTone.NEUTRAL
        "skipped" -> stringResource(R.string.log_result_skipped) to LogBadgeTone.NEUTRAL
        else -> result to LogBadgeTone.NEUTRAL
    }
    LogBadge(text = label, tone = tone)
}

private enum class LogBadgeTone {
    INFO, SUCCESS, WARNING, ERROR, NEUTRAL, CONNECTION, PRIMARY
}

@Composable
private fun logBadgeColors(tone: LogBadgeTone): Pair<Color, Color> {
    val isDark = isInDarkTheme()
    return when (tone) {
        LogBadgeTone.INFO -> if (isDark) {
            Color(0xFF0D47A1).copy(alpha = 0.3f) to Color(0xFF64B5F6)
        } else {
            Color(0xFFE3F2FD) to Color(0xFF1976D2)
        }
        LogBadgeTone.SUCCESS -> if (isDark) {
            Color(0xFF1B5E20).copy(alpha = 0.3f) to Color(0xFF81C784)
        } else {
            Color(0xFFE8F5E9) to Color(0xFF2E7D32)
        }
        LogBadgeTone.WARNING -> if (isDark) {
            Color(0xFFE65100).copy(alpha = 0.3f) to Color(0xFFFFB74D)
        } else {
            Color(0xFFFFF3E0) to Color(0xFFE65100)
        }
        LogBadgeTone.ERROR -> if (isDark) {
            Color(0xFFB71C1C).copy(alpha = 0.3f) to Color(0xFFE57373)
        } else {
            Color(0xFFFFEBEE) to Color(0xFFC62828)
        }
        LogBadgeTone.NEUTRAL -> MiuixTheme.colorScheme.run {
            onSurface.copy(alpha = 0.08f) to onSurfaceVariantSummary
        }
        LogBadgeTone.CONNECTION -> if (isDark) {
            Color(0xFF4A148C).copy(alpha = 0.3f) to Color(0xFFBA68C8)
        } else {
            Color(0xFFF3E5F5) to Color(0xFF7B1FA2)
        }
        LogBadgeTone.PRIMARY -> MiuixTheme.colorScheme.primary.let { color ->
            color.copy(alpha = if (isDark) 0.3f else 0.1f) to color
        }
    }
}

@Composable
private fun LogBadge(text: String, tone: LogBadgeTone, modifier: Modifier = Modifier) {
    val (backgroundColor, textColor) = logBadgeColors(tone)
    Box(
        modifier = modifier
            .background(backgroundColor, RoundedCornerShape(4.dp))
            .padding(horizontal = 6.dp, vertical = 2.dp)
    ) {
        Text(
            text = text,
            fontSize = 10.sp,
            fontWeight = FontWeight.Bold,
            color = textColor,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

@Composable
private fun LatencyBadge(latency: String, latencyMs: Int?) {
    val tone = when {
        latencyMs == null -> LogBadgeTone.NEUTRAL
        latencyMs < 800 -> LogBadgeTone.SUCCESS
        latencyMs < 1500 -> LogBadgeTone.WARNING
        else -> LogBadgeTone.ERROR
    }
    LogBadge(text = latency, tone = tone)
}

@Composable
fun LogLevelBadge(level: LogLevel) {
    val tone = when (level) {
        LogLevel.INFO -> LogBadgeTone.INFO
        LogLevel.WARN -> LogBadgeTone.WARNING
        LogLevel.ERROR -> LogBadgeTone.ERROR
        LogLevel.DEBUG -> LogBadgeTone.SUCCESS
        LogLevel.UNKNOWN -> LogBadgeTone.NEUTRAL
    }
    LogBadge(text = level.name, tone = tone)
}

@Composable
fun OutboundFlowView(flow: OutboundFlow) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .background(
                MiuixTheme.colorScheme.surfaceContainer,
                RoundedCornerShape(8.dp)
            )
            .padding(10.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = stringResource(R.string.source_label),
                    fontSize = 10.sp,
                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary
                )
                Text(
                    text = flow.source,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium,
                    color = MiuixTheme.colorScheme.onSurface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }

            Icon(
                imageVector = Icons.AutoMirrored.Rounded.ArrowForward,
                contentDescription = null,
                tint = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                modifier = Modifier
                    .size(16.dp)
                    .padding(horizontal = 2.dp)
            )

            Column(
                modifier = Modifier.weight(1f),
                horizontalAlignment = Alignment.End
            ) {
                Text(
                    text = stringResource(R.string.destination_label),
                    fontSize = 10.sp,
                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary
                )
                Text(
                    text = flow.target,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium,
                    color = MiuixTheme.colorScheme.onSurface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
        }

        Spacer(modifier = Modifier.height(6.dp))

        val outboundTone = when (flow.outbound.lowercase()) {
            "direct" -> LogBadgeTone.SUCCESS
            "block", "reject" -> LogBadgeTone.ERROR
            else -> LogBadgeTone.PRIMARY
        }

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.End
        ) {
            LogBadge(text = flow.outbound.uppercase(), tone = outboundTone)
        }
    }
}

private fun LazyListScope.rawLogItems(logs: List<LogItem>) {
    groupedCardItems(
        keyPrefix = "raw-log",
        items = logs.mapIndexed { index, item ->
            CardItem(key = index.toString()) {
                Text(
                    text = item.rawLine,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 11.sp,
                    color = MiuixTheme.colorScheme.onSurface,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(
                            start = 12.dp,
                            end = 12.dp,
                            top = if (index == 0) 13.dp else 1.dp,
                            bottom = if (index == logs.lastIndex) 13.dp else 1.dp,
                        )
                )
            }
        }
    )
}
