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
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.add
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
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
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.FileProvider
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.deferredTopPadding
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.theme.LocalEnableBlur
import com.fanjv.netproxy.feature.logs.data.LogItem
import com.fanjv.netproxy.feature.logs.data.LogLevel
import com.fanjv.netproxy.feature.logs.data.LogType
import com.fanjv.netproxy.feature.logs.data.OutboundFlow
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.DropdownImpl
import top.yukonga.miuix.kmp.basic.HorizontalDivider
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.ListPopupColumn
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.SnackbarDuration
import top.yukonga.miuix.kmp.basic.TabRow
import top.yukonga.miuix.kmp.basic.TabRowDefaults
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Download
import top.yukonga.miuix.kmp.icon.extended.MoreCircle
import top.yukonga.miuix.kmp.icon.extended.Refresh
import top.yukonga.miuix.kmp.icon.extended.Share
import top.yukonga.miuix.kmp.overlay.OverlayListPopup
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
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
    var showMoreMenu by remember { mutableStateOf(false) }

    val currentType = remember(selectedTabIndex) {
        when (selectedTabIndex) {
            0 -> LogType.SERVICE
            else -> LogType.KERNEL
        }
    }

    LaunchedEffect(currentType) {
        viewModel.refresh(currentType)
    }

    val logs = remember(
        logsState.serviceLogs,
        logsState.kernelLogs,
        selectedTabIndex
    ) {
        when (selectedTabIndex) {
            0 -> logsState.serviceLogs
            else -> logsState.kernelLogs
        }
    }

    // 导出保存启动器
    val exportLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.CreateDocument("application/gzip")
    ) { uri: Uri? ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            try {
                val logFile = withContext(Dispatchers.IO) {
                    viewModel.createReport()
                }
                withContext(Dispatchers.IO) {
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
                }
                showMessage(logSavedMessage)
            } catch (e: Exception) {
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
    val enableBlur = LocalEnableBlur.current
    val backdrop = rememberBlurBackdrop(enableBlur)
    val blurActive = backdrop != null
    val barColor = if (blurActive) Color.Transparent else MiuixTheme.colorScheme.surface

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
                    actions = {
                            IconButton(onClick = { viewModel.refresh(currentType) }) {
                                Icon(
                                    imageVector = MiuixIcons.Refresh,
                                    contentDescription = stringResource(R.string.refresh_logs),
                                    tint = MiuixTheme.colorScheme.onSurface
                                )
                            }
                            IconButton(onClick = {
                                scope.launch {
                                    try {
                                        val logFile = withContext(Dispatchers.IO) {
                                            viewModel.createReport()
                                        }
                                        val uri = FileProvider.getUriForFile(
                                            context,
                                            "${context.packageName}.fileprovider",
                                            logFile
                                        )
                                        val intent = Intent(Intent.ACTION_SEND).apply {
                                            type = "application/gzip"
                                            putExtra(Intent.EXTRA_STREAM, uri)
                                            clipData = ClipData.newRawUri(sendLogTitle, uri)
                                            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                                        }
                                        context.startActivity(
                                            Intent.createChooser(
                                                intent,
                                                sendLogTitle
                                            )
                                        )
                                    } catch (e: Exception) {
                                        showMessage(
                                            e.message ?: shareFailedDetail.format(shareFailed),
                                            isError = true
                                        )
                                    }
                                }
                            }) {
                                Icon(
                                    imageVector = MiuixIcons.Share,
                                    contentDescription = stringResource(R.string.send_log),
                                    tint = MiuixTheme.colorScheme.onSurface
                                )
                            }
                            IconButton(onClick = {
                                try {
                                    val timestamp =
                                        java.time.format.DateTimeFormatter.ofPattern("yyyyMMdd_HHmm")
                                            .format(java.time.LocalDateTime.now())
                                    exportLauncher.launch("NetProxy_Logs_$timestamp.tar.gz")
                                } catch (e: Exception) {
                                    showMessage(
                                        exportFailed.format(e.message ?: saveFailed),
                                        isError = true
                                    )
                                }
                            }) {
                                Icon(
                                    imageVector = MiuixIcons.Download,
                                    contentDescription = stringResource(R.string.save_log),
                                    tint = MiuixTheme.colorScheme.onSurface
                                )
                            }
                            Box {
                                IconButton(onClick = { showMoreMenu = true }) {
                                    Icon(
                                        imageVector = MiuixIcons.MoreCircle,
                                        contentDescription = stringResource(R.string.more_options),
                                        tint = MiuixTheme.colorScheme.onSurface
                                    )
                                }
                                OverlayListPopup(
                                    show = showMoreMenu,
                                    onDismissRequest = { showMoreMenu = false }
                                ) {
                                    ListPopupColumn {
                                        DropdownImpl(
                                            text = stringResource(if (isCardView) R.string.raw_view else R.string.card_view),
                                            optionSize = 4,
                                            isSelected = false,
                                            index = 0,
                                            onSelectedIndexChange = {
                                                isCardView = !isCardView
                                                showMoreMenu = false
                                            }
                                        )
                                        HorizontalDivider(
                                            modifier = Modifier.padding(
                                                horizontal = 20.dp,
                                                vertical = 4.dp
                                            ),
                                            thickness = 1.5.dp
                                        )
                                        DropdownImpl(
                                            text = stringResource(R.string.scroll_to_top),
                                            optionSize = 4,
                                            isSelected = false,
                                            index = 1,
                                            onSelectedIndexChange = {
                                                scope.launch {
                                                    listState.animateScrollToItem(0)
                                                }
                                                showMoreMenu = false
                                            }
                                        )
                                        DropdownImpl(
                                            text = stringResource(R.string.scroll_to_bottom),
                                            optionSize = 4,
                                            isSelected = false,
                                            index = 2,
                                            onSelectedIndexChange = {
                                                scope.launch {
                                                    if (logs.isNotEmpty()) {
                                                        val targetIndex =
                                                            if (isCardView) logs.size - 1 else 0
                                                        listState.animateScrollToItem(targetIndex)
                                                    }
                                                }
                                                showMoreMenu = false
                                            }
                                        )
                                        HorizontalDivider(
                                            modifier = Modifier.padding(
                                                horizontal = 20.dp,
                                                vertical = 4.dp
                                            ),
                                            thickness = 1.5.dp
                                        )
                                        DropdownImpl(
                                            text = stringResource(R.string.clear_logs_now),
                                            optionSize = 4,
                                            isSelected = false,
                                            index = 3,
                                            onSelectedIndexChange = {
                                                viewModel.clear(currentType) { success ->
                                                    showMessage(
                                                        if (success) clearSuccessMessage else clearFailedMessage,
                                                        isError = !success
                                                    )
                                                }
                                                showMoreMenu = false
                                            }
                                        )
                                    }
                                }
                            }
                    },
                    scrollBehavior = scrollBehavior,
                    bottomContent = {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 12.dp)
                                .padding(bottom = 6.dp)
                                .deferredTopPadding(dynamicTopPadding)
                        ) {
                            TabRow(
                                tabs = tabs,
                                selectedTabIndex = selectedTabIndex,
                                onTabSelected = { selectedTabIndex = it },
                                modifier = Modifier.fillMaxWidth(),
                                colors = TabRowDefaults.tabRowColors(
                                    backgroundColor = barColor
                                ),
                                height = 40.dp
                            )
                        }
                    }
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.add(WindowInsets.displayCutout)
            .only(WindowInsetsSides.Horizontal)
    ) { innerPadding ->
        val layoutDirection = LocalLayoutDirection.current
        Box(modifier = if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier) {
            LazyColumn(
                state = listState,
                modifier = Modifier
                    .fillMaxSize()
                    .scrollEndHaptic()
                    .overScrollVertical()
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
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .fillParentMaxHeight(0.7f),
                            contentAlignment = Alignment.Center
                        ) {
                            Text(
                                text = stringResource(R.string.no_logs),
                                color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                                fontSize = 15.sp,
                                textAlign = TextAlign.Center
                            )
                        }
                    }
                } else {
                    if (isCardView) {
                        items(logs) { item ->
                            LogItemCard(item = item, type = currentType)
                        }
                    } else {
                        item {
                            RawLogsCard(logs = logs)
                        }
                    }
                }

                item {
                    Spacer(modifier = Modifier.height(16.dp))
                }
            }
        }
    }
}

@Composable
fun LogItemCard(item: LogItem, type: LogType) {
    var connId: String? = null
    var latency: String? = null
    var cleanTag = item.tag
    var cleanMessage = item.message

    if (type == LogType.KERNEL) {
        val tagMatch = "^\\[(\\d+)(?:\\s+([^]\\s]+))?\\s*]\\s*(.*)$".toRegex().find(item.tag)
        if (tagMatch != null) {
            connId = tagMatch.groupValues[1]
            latency = tagMatch.groupValues[2].takeIf { it.isNotEmpty() }
            cleanTag = tagMatch.groupValues[3]
        } else {
            val msgMatch =
                "^\\[(\\d+)(?:\\s+([^]\\s]+))?\\s*]\\s*(.*)$".toRegex().find(item.message)
            if (msgMatch != null) {
                connId = msgMatch.groupValues[1]
                latency = msgMatch.groupValues[2].takeIf { it.isNotEmpty() }
                cleanMessage = msgMatch.groupValues[3]
            }
        }
    }

    Card(
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
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    LogLevelBadge(level = item.level)
                    if (cleanTag.isNotEmpty()) {
                        Spacer(modifier = Modifier.width(8.dp))
                        if (type == LogType.SERVICE) {
                            NativeComponentBadge(component = cleanTag)
                        } else {
                            Text(
                                text = cleanTag,
                                fontSize = 12.sp,
                                fontWeight = FontWeight.Bold,
                                color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                modifier = Modifier.weight(1f, fill = false)
                            )
                        }
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

            if (type == LogType.SERVICE && (item.event.isNotEmpty() || item.result.isNotEmpty())) {
                Spacer(modifier = Modifier.height(6.dp))
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    if (item.event.isNotEmpty()) {
                        NativeEventBadge(event = item.event)
                    }
                    if (item.result.isNotEmpty()) {
                        NativeResultBadge(result = item.result)
                    }
                }
            }

            if (type == LogType.SERVICE && item.errorCode.isNotEmpty()) {
                Spacer(modifier = Modifier.height(6.dp))
                NativeErrorCodeBadge(errorCode = item.errorCode)
            }

            if (connId != null) {
                Spacer(modifier = Modifier.height(6.dp))
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    ConnectionBadge(id = connId)
                    if (latency != null) {
                        LatencyBadge(latency = latency)
                    }
                }
            }

            Spacer(modifier = Modifier.height(8.dp))

            if (type == LogType.KERNEL && item.outboundFlow != null) {
                OutboundFlowView(flow = item.outboundFlow)
            } else {
                Text(
                    text = cleanMessage,
                    fontSize = 13.sp,
                    color = MiuixTheme.colorScheme.onSurface,
                    lineHeight = 18.sp
                )
            }
        }
    }
}

@Composable
fun NativeComponentBadge(component: String) {
    val isDark = androidx.compose.foundation.isSystemInDarkTheme()
    val label = when (component) {
        "service" -> "服务"
        "worker" -> "后台"
        "subscription" -> "订阅"
        "node" -> "节点"
        "mode" -> "模式"
        "app" -> "应用"
        "config" -> "配置"
        "network" -> "网络"
        "module" -> "模块"
        else -> component
    }
    NativeBadge(
        text = label,
        backgroundColor = if (isDark) Color(0xFF283593).copy(alpha = 0.3f) else Color(0xFFE8EAF6),
        textColor = if (isDark) Color(0xFF9FA8DA) else Color(0xFF3949AB)
    )
}

@Composable
fun NativeEventBadge(event: String) {
    val isDark = androidx.compose.foundation.isSystemInDarkTheme()
    val label = when (event) {
        "service.start" -> "启动"
        "service.stop" -> "停止"
        "service.reload" -> "重载"
        "worker.start", "worker.run" -> "Worker"
        "network.watch" -> "网络监听"
        "network.read" -> "网络读取"
        "network.policy" -> "网络策略"
        "subscription.add" -> "添加订阅"
        "subscription.edit" -> "编辑订阅"
        "subscription.update" -> "更新订阅"
        "subscription.update-all" -> "更新全部"
        "subscription.remove" -> "删除订阅"
        "subscription.runtime-sync" -> "运行时同步"
        "subscription.effect" -> "订阅副作用"
        "subscription.schedule" -> "订阅调度"
        "node.append" -> "添加节点"
        "node.import" -> "导入节点"
        "node.edit" -> "编辑节点"
        "node.remove" -> "删除节点"
        "node.select", "node.selection" -> "选择节点"
        "mode.apply" -> "切换模式"
        "app-policy.update" -> "应用策略"
        "config.apply" -> "保存配置"
        "config.validate" -> "校验配置"
        "module.boot" -> "开机流程"
        "module.update" -> "模块更新"
        else -> event
    }
    NativeBadge(
        text = label,
        backgroundColor = if (isDark) Color(0xFF37474F).copy(alpha = 0.3f) else Color(0xFFECEFF1),
        textColor = if (isDark) Color(0xFFB0BEC5) else Color(0xFF546E7A)
    )
}

@Composable
fun NativeResultBadge(result: String) {
    val isDark = androidx.compose.foundation.isSystemInDarkTheme()
    val (label, backgroundColor, textColor) = when (result) {
        "success", "recovered" -> if (isDark) {
            Triple("成功", Color(0xFF1B5E20).copy(alpha = 0.3f), Color(0xFF81C784))
        } else {
            Triple("成功", Color(0xFFE8F5E9), Color(0xFF2E7D32))
        }

        "failed", "forced" -> if (isDark) {
            Triple("失败", Color(0xFFB71C1C).copy(alpha = 0.3f), Color(0xFFE57373))
        } else {
            Triple("失败", Color(0xFFFFEBEE), Color(0xFFC62828))
        }

        "persisted", "fallback" -> if (isDark) {
            Triple(
                if (result == "persisted") "已保存" else "已回退",
                Color(0xFFE65100).copy(alpha = 0.3f),
                Color(0xFFFFB74D)
            )
        } else {
            Triple(
                if (result == "persisted") "已保存" else "已回退",
                Color(0xFFFFF3E0),
                Color(0xFFE65100)
            )
        }

        "started", "already-running" -> if (isDark) {
            Triple(
                if (result == "started") "进行中" else "已运行",
                Color(0xFF0D47A1).copy(alpha = 0.3f),
                Color(0xFF64B5F6)
            )
        } else {
            Triple(
                if (result == "started") "进行中" else "已运行",
                Color(0xFFE3F2FD),
                Color(0xFF1976D2)
            )
        }

        "stopped", "skipped" -> if (isDark) {
            Triple(
                if (result == "stopped") "已停止" else "已跳过",
                Color(0xFF37474F).copy(alpha = 0.3f),
                Color(0xFFB0BEC5)
            )
        } else {
            Triple(
                if (result == "stopped") "已停止" else "已跳过",
                Color(0xFFF5F5F5),
                Color(0xFF616161)
            )
        }

        else -> if (isDark) {
            Triple(result, Color(0xFF37474F).copy(alpha = 0.3f), Color(0xFFB0BEC5))
        } else {
            Triple(result, Color(0xFFF5F5F5), Color(0xFF616161))
        }
    }
    NativeBadge(text = label, backgroundColor = backgroundColor, textColor = textColor)
}

@Composable
private fun NativeBadge(text: String, backgroundColor: Color, textColor: Color) {
    Box(
        modifier = Modifier
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
private fun NativeErrorCodeBadge(errorCode: String) {
    val isDark = androidx.compose.foundation.isSystemInDarkTheme()
    NativeBadge(
        text = errorCode,
        backgroundColor = if (isDark) Color(0xFF4A1414).copy(alpha = 0.45f) else Color(0xFFFFEBEE),
        textColor = if (isDark) Color(0xFFEF9A9A) else Color(0xFFC62828)
    )
}

@Composable
fun ConnectionBadge(id: String) {
    val isDark = androidx.compose.foundation.isSystemInDarkTheme()
    val backgroundColor = if (isDark) Color(0xFF4A148C).copy(alpha = 0.3f) else Color(0xFFF3E5F5)
    val textColor = if (isDark) Color(0xFFBA68C8) else Color(0xFF7B1FA2)

    Box(
        modifier = Modifier
            .background(backgroundColor, RoundedCornerShape(4.dp))
            .padding(horizontal = 6.dp, vertical = 2.dp)
    ) {
        Text(
            text = "#$id",
            fontSize = 10.sp,
            fontWeight = FontWeight.Bold,
            color = textColor
        )
    }
}

@Composable
fun LatencyBadge(latency: String) {
    val isDark = androidx.compose.foundation.isSystemInDarkTheme()
    val ms = parseLatencyMs(latency)
    val (backgroundColor, textColor) = when {
        ms < 800 -> if (isDark) Color(0xFF1B5E20).copy(alpha = 0.3f) to Color(0xFF81C784) else Color(
            0xFFE8F5E9
        ) to Color(0xFF2E7D32)

        ms < 1500 -> if (isDark) Color(0xFFE65100).copy(alpha = 0.3f) to Color(0xFFFFB74D) else Color(
            0xFFFFF3E0
        ) to Color(0xFFE65100)

        else -> if (isDark) Color(0xFFB71C1C).copy(alpha = 0.3f) to Color(0xFFE57373) else Color(
            0xFFFFEBEE
        ) to Color(0xFFC62828)
    }

    Box(
        modifier = Modifier
            .background(backgroundColor, RoundedCornerShape(4.dp))
            .padding(horizontal = 6.dp, vertical = 2.dp)
    ) {
        Text(
            text = latency,
            style = MiuixTheme.textStyles.footnote2.copy(
                fontSize = 10.sp,
                fontWeight = FontWeight.Bold
            ),
            color = textColor
        )
    }
}

fun parseLatencyMs(duration: String): Int {
    return try {
        val numberPart = duration.filter { it.isDigit() || it == '.' }
        if (duration.endsWith("ms", ignoreCase = true)) {
            numberPart.toDoubleOrNull()?.toInt() ?: 0
        } else if (duration.endsWith("s", ignoreCase = true)) {
            ((numberPart.toDoubleOrNull() ?: 0.0) * 1000).toInt()
        } else {
            0
        }
    } catch (_: Exception) {
        0
    }
}

@Composable
fun LogLevelBadge(level: LogLevel) {
    val isDark = androidx.compose.foundation.isSystemInDarkTheme()
    val (backgroundColor, textColor) = when (level) {
        LogLevel.INFO -> if (isDark) Color(0xFF0D47A1).copy(alpha = 0.3f) to Color(0xFF64B5F6) else Color(
            0xFFE3F2FD
        ) to Color(0xFF1976D2)

        LogLevel.WARN -> if (isDark) Color(0xFFE65100).copy(alpha = 0.3f) to Color(0xFFFFB74D) else Color(
            0xFFFFF3E0
        ) to Color(0xFFE65100)

        LogLevel.ERROR -> if (isDark) Color(0xFFB71C1C).copy(alpha = 0.3f) to Color(0xFFE57373) else Color(
            0xFFFFEBEE
        ) to Color(0xFFC62828)

        LogLevel.DEBUG -> if (isDark) Color(0xFF1B5E20).copy(alpha = 0.3f) to Color(0xFF81C784) else Color(
            0xFFE8F5E9
        ) to Color(0xFF2E7D32)

        LogLevel.UNKNOWN -> if (isDark) Color(0xFF37474F).copy(alpha = 0.3f) to Color(0xFFB0BEC5) else Color(
            0xFFF5F5F5
        ) to Color(0xFF616161)
    }

    Box(
        modifier = Modifier
            .background(backgroundColor, RoundedCornerShape(4.dp))
            .padding(horizontal = 6.dp, vertical = 2.dp)
    ) {
        Text(
            text = level.name,
            fontSize = 10.sp,
            fontWeight = FontWeight.Bold,
            color = textColor
        )
    }
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

        val isDark = androidx.compose.foundation.isSystemInDarkTheme()
        val outboundColor = when (flow.outbound.lowercase()) {
            "direct" -> if (isDark) Color(0xFF81C784) else Color(0xFF2E7D32)
            "block", "reject" -> if (isDark) Color(0xFFE57373) else Color(0xFFC62828)
            else -> MiuixTheme.colorScheme.primary
        }
        val outboundBg = when (flow.outbound.lowercase()) {
            "direct" -> if (isDark) Color(0xFF1B5E20).copy(alpha = 0.3f) else Color(0xFFE8F5E9)
            "block", "reject" -> if (isDark) Color(0xFFB71C1C).copy(alpha = 0.3f) else Color(
                0xFFFFEBEE
            )

            else -> MiuixTheme.colorScheme.primary.copy(alpha = if (isDark) 0.3f else 0.1f)
        }

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.End
        ) {
            Box(
                modifier = Modifier
                    .background(outboundBg, RoundedCornerShape(4.dp))
                    .padding(horizontal = 6.dp, vertical = 2.dp)
            ) {
                Text(
                    text = flow.outbound.uppercase(),
                    fontSize = 10.sp,
                    fontWeight = FontWeight.Bold,
                    color = outboundColor
                )
            }
        }
    }
}

@Composable
fun RawLogsCard(logs: List<LogItem>) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        insideMargin = PaddingValues(12.dp)
    ) {
        Column {
            logs.forEach { item ->
                Text(
                    text = item.rawLine,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 11.sp,
                    color = MiuixTheme.colorScheme.onSurface,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(vertical = 1.dp)
                )
            }
        }
    }
}
