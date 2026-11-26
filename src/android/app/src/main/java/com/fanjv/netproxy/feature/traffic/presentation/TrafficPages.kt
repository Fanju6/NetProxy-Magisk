package com.fanjv.netproxy.feature.traffic.presentation

import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Android
import androidx.compose.material.icons.rounded.Code
import androidx.compose.material.icons.rounded.ExpandLess
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material.icons.rounded.List
import androidx.compose.material.icons.rounded.Lock
import androidx.compose.material.icons.rounded.NetworkCheck
import androidx.compose.material.icons.rounded.PauseCircle
import androidx.compose.material.icons.rounded.PictureInPictureAlt
import androidx.compose.material.icons.rounded.RadioButtonChecked
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.TrackChanges
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.NetProxyApplication
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.AppBarButton
import com.fanjv.netproxy.core.ui.component.AppDialog
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.feature.apps.presentation.AppInfoModel
import com.fanjv.netproxy.feature.apps.presentation.AppIcon
import com.fanjv.netproxy.feature.traffic.data.CaptureRecord
import com.fanjv.netproxy.feature.traffic.data.TrafficChangedException
import com.fanjv.netproxy.feature.traffic.data.responseTypeQuery
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.DropdownDefaults
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.DropdownItem
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.glass.GlassTextField
import top.yukonga.miuix.kmp.popup.WindowDropdownPopup
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter

@Composable
internal fun TrafficFrame(
    title: Int,
    subtitle: String = "",
    actions: @Composable RowScope.() -> Unit = {},
    floatingActionButton: @Composable () -> Unit = {},
    onBack: (() -> Unit)? = null,
    topBarModifier: Modifier = Modifier,
    content: @Composable (PaddingValues) -> Unit,
) {
    val navigator = LocalNavigator.current
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    Scaffold(topBar = {
        BlurredBar(backdrop) {
            AdaptiveTopAppBar(
                title = stringResource(title),
                modifier = topBarModifier,
                subtitle = subtitle,
                color = if (backdrop != null) Color.Transparent else MiuixTheme.colorScheme.surface,
                scrollBehavior = scrollBehavior,
                navigationIcon = { BackIconButton(onClick = onBack ?: navigator::pop) },
                actions = actions,
            )
        }
    }, floatingActionButton = floatingActionButton) { padding ->
        Box(Modifier.fillMaxSize().nestedScroll(scrollBehavior.nestedScrollConnection)
            .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)) {
            content(padding)
        }
    }
}

@Composable
internal fun TrafficScreen(viewModel: TrafficViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val pauses by remember(viewModel) { viewModel.observePauses() }.collectAsStateWithLifecycle(initialValue = null)
    val navigator = LocalNavigator.current
    RefreshOnResume(viewModel)
    TrafficFrame(R.string.traffic_title) { padding ->
        val status = state.status.value
        LazyColumn(Modifier.fillMaxSize().padding(padding).padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)) {
            item {
                Column(
                    Modifier.fillMaxWidth().background(MiuixTheme.colorScheme.surfaceContainer, RoundedCornerShape(24.dp)),
                ) {
                    SwitchPreference(
                        title = stringResource(R.string.traffic_capture),
                        summary = stringResource(when (status?.running) {
                            true -> R.string.traffic_on
                            false -> R.string.traffic_off
                            null -> R.string.traffic_read_failed
                        }),
                        checked = status?.running == true, enabled = status != null && !state.busy,
                        onCheckedChange = viewModel::setCapture, insideMargin = TrafficRowInsets,
                        startAction = { CaptureRowIcon(Icons.Rounded.RadioButtonChecked) },
                    )
                    SwitchPreference(
                        title = stringResource(R.string.traffic_disable_quic),
                        summary = stringResource(R.string.traffic_disable_quic_summary),
                        checked = state.captureAppPolicy?.disableQuic == true,
                        enabled = state.captureAppPolicy?.disableQuic != null && !state.busy,
                        onCheckedChange = viewModel::setCaptureDisableQuic, insideMargin = TrafficRowInsets,
                        startAction = { CaptureRowIcon(Icons.Rounded.NetworkCheck) },
                    )
                    CaptureOverlayPreference(status?.running == true)
                    Row(Modifier.fillMaxWidth().padding(start = 14.dp, end = 14.dp, bottom = 10.dp),
                        horizontalArrangement = Arrangement.SpaceBetween) {
                        StatusBadge(status?.count?.let { stringResource(R.string.traffic_record_count, it) } ?: "—")
                        StatusBadge(stringResource(when (state.policy.value?.enabled) {
                            true -> R.string.traffic_https_active
                            false -> R.string.traffic_https_inactive
                            null -> R.string.traffic_https
                        }))
                    }
                }
            }
            state.failure?.let { issue -> item { TrafficErrorText(issue) } }
            groupedCardSection("tools", { stringResource(R.string.traffic_workspace) }, listOf(
                CardItem("sessions") {
                    TrafficLink(R.string.traffic_records, R.string.traffic_sessions_summary, Icons.Rounded.List) {
                        navigator.push(Route.TrafficSessions)
                    }
                },
                CardItem("apps") {
                    ArrowPreference(title = stringResource(R.string.traffic_capture_apps),
                        summary = stringResource(R.string.traffic_capture_apps_summary),
                        onClick = { navigator.push(Route.TrafficApps) }, enabled = !state.busy,
                        insideMargin = TrafficRowInsets,
                        startAction = { CaptureRowIcon(Icons.Rounded.Android) })
                },
                CardItem("rules") {
                    TrafficLink(R.string.traffic_rules, R.string.traffic_rules_summary, Icons.Rounded.Code) {
                        navigator.push(Route.TrafficRules)
                    }
                },
                CardItem("breakpoints") {
                    ArrowPreference(title = stringResource(R.string.traffic_breakpoints),
                        onClick = { navigator.push(Route.TrafficBreakpoints) }, insideMargin = TrafficRowInsets,
                        startAction = { CaptureRowIcon(Icons.Rounded.TrackChanges) })
                },
                CardItem("paused") {
                    val count = pauses?.getOrNull()?.size ?: 0
                    ArrowPreference(title = if (count > 0) pausedCountLabel(count) else stringResource(R.string.traffic_breakpoints_paused),
                        onClick = { navigator.push(Route.TrafficPaused) }, insideMargin = TrafficRowInsets,
                        startAction = { CaptureRowIcon(Icons.Rounded.PauseCircle) })
                },
                CardItem("https") {
                    TrafficLink(R.string.traffic_https, R.string.traffic_ca_hint, Icons.Rounded.Lock) {
                        navigator.push(Route.TrafficHttps)
                    }
                },
            ))
        }
    }
}

@Composable
private fun CaptureOverlayPreference(capturing: Boolean) {
    val context = LocalContext.current
    val overlay = (context.applicationContext as NetProxyApplication).container.captureOverlay
    var enabled by remember { mutableStateOf(overlay.enabled) }
    var granted by remember { mutableStateOf(Settings.canDrawOverlays(context)) }
    var explain by rememberSaveable { mutableStateOf(false) }
    LifecycleResumeEffect(Unit) {
        granted = Settings.canDrawOverlays(context)
        onPauseOrDispose { }
    }
    LaunchedEffect(capturing, enabled, granted) {
        if (capturing && enabled && !granted && overlay.claimPermissionHint()) explain = true
    }
    SwitchPreference(
        title = stringResource(R.string.traffic_overlay),
        summary = stringResource(R.string.traffic_overlay_summary),
        checked = enabled,
        onCheckedChange = {
            overlay.enabled = it
            enabled = it
        },
        insideMargin = TrafficRowInsets,
        startAction = { CaptureRowIcon(Icons.Rounded.PictureInPictureAlt) },
    )
    if (enabled && !granted) {
        ArrowPreference(title = stringResource(R.string.traffic_overlay_permission),
            onClick = { explain = true }, insideMargin = TrafficRowInsets,
            startAction = { CaptureRowIcon(Icons.Rounded.PictureInPictureAlt) })
    }
    AppDialog(
        show = explain && enabled && !granted,
        title = stringResource(R.string.traffic_overlay_permission),
        summary = stringResource(R.string.traffic_overlay_explain),
        onDismissRequest = { explain = false },
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            AppDialogButton(stringResource(R.string.common_cancel), { explain = false }, Modifier.weight(1f))
            AppDialogButton(stringResource(R.string.traffic_overlay_open_settings), {
                explain = false
                val intent = Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION, Uri.fromParts("package", context.packageName, null))
                runCatching { context.startActivity(intent) }
            }, Modifier.weight(1f), role = DialogActionRole.Primary)
        }
    }
}

@Composable
private fun TrafficLink(title: Int, summary: Int, icon: ImageVector, onClick: () -> Unit) {
    ArrowPreference(title = stringResource(title), summary = stringResource(summary), onClick = onClick,
        insideMargin = TrafficRowInsets,
        startAction = { CaptureRowIcon(icon) })
}

@Composable
internal fun CaptureRowIcon(icon: ImageVector) {
    Icon(icon, null, Modifier.padding(end = 12.dp), tint = MiuixTheme.colorScheme.primary)
}

internal val TrafficRowInsets = PaddingValues(horizontal = 14.dp, vertical = 8.dp)

@Composable
internal fun TrafficSessionsScreen(viewModel: TrafficViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    var query by rememberSaveable { mutableStateOf("") }
    var method by rememberSaveable { mutableStateOf<String?>(null) }
    var statusClass by rememberSaveable { mutableStateOf<Int?>(null) }
    var tls by rememberSaveable { mutableStateOf<String?>(null) }
    var rewrittenOnly by rememberSaveable { mutableStateOf(false) }
    var app by rememberSaveable { mutableStateOf<String?>(null) }
    var outbound by rememberSaveable { mutableStateOf<String?>(null) }
    var confirmClear by rememberSaveable { mutableStateOf(false) }
    var deleteBatch by rememberSaveable { mutableStateOf<String?>(null) }
    var showDomains by rememberSaveable { mutableStateOf(false) }
    var showResponseTypes by rememberSaveable { mutableStateOf(false) }
    var grouping by rememberSaveable { mutableStateOf(SessionGrouping.None) }
    var collapsed by remember(grouping) { mutableStateOf(emptySet<String>()) }
    val records = state.records.value.orEmpty()
    val historical = state.batches.value.orEmpty().filterNot { it.running }
    val batchLabels = historical.map {
        val name = if (it.imported) stringResource(R.string.traffic_har_read_only, it.name.orEmpty()) else it.id.take(6)
        "$name · ${it.startedAt.substringBefore('.').substringBefore('Z').replace('T', ' ')} UTC · " +
            stringResource(R.string.traffic_record_count, it.count)
    }
    LaunchedEffect(viewModel) { viewModel.loadAppCatalog() }
    val methods = remember(records) {
        (listOf("GET", "POST", "PUT", "PATCH", "DELETE", "HEAD") + records.map { it.method }).distinct()
    }
    val apps = remember(records) { records.mapNotNull { it.appKey }.distinct().sorted() }
    val appLabels = apps.map { key -> state.appCatalog[key]?.let { "${it.label} · $key" } ?: key }
    val outbounds = remember(records) {
        records.flatMap { listOf(it.outbound) + it.exit?.chain.orEmpty().map { hop -> hop.tag } }
            .filter { it.isNotBlank() }.distinct().sorted()
    }
    val visible = remember(records, query, method, statusClass, tls, rewrittenOnly, app, outbound) {
        records.filter { it.matchesSession(query, method, statusClass, tls, rewrittenOnly, app, outbound) }
    }
    val groups = remember(visible, grouping) { visible.groupSessions(grouping) }
    val listState = key(state.selectedBatch, state.selectedDomain, state.selectedResponseTypes) { rememberLazyListState() }
    val before = state.page?.before
    val running = state.status.value?.running == true
    LaunchedEffect(listState, before, visible.isNotEmpty(), state.busy, state.loadingOlder, state.olderError) {
        if (before != null && visible.isNotEmpty() && !state.busy && !state.loadingOlder && state.olderError == null) {
            snapshotFlow {
                val layout = listState.layoutInfo
                val last = layout.visibleItemsInfo.lastOrNull()?.index ?: return@snapshotFlow false
                last >= layout.totalItemsCount - 5
            }.first { it }
            viewModel.loadOlder()
        }
    }
    LaunchedEffect(running, state.selectedBatch) {
        if (running && state.selectedBatch == null) while (true) {
            delay(2000)
            viewModel.pollRecords()
        }
    }
    RefreshOnResume(viewModel)
    val groupingActions = SessionGrouping.entries.map { option ->
        val label = stringResource(when (option) {
            SessionGrouping.None -> R.string.traffic_group_none
            SessionGrouping.App -> R.string.traffic_group_app
            SessionGrouping.Domain -> R.string.traffic_group_domain
            SessionGrouping.Outbound -> R.string.traffic_group_outbound
        })
        TopBarMenuAction(label, grouping != option) { grouping = option }
    }
    val batchActions = listOf(
        TopBarMenuAction(stringResource(R.string.traffic_delete_batch), state.selectedBatch != null && !state.busy) {
            deleteBatch = state.selectedBatch
        },
        TopBarMenuAction(stringResource(R.string.traffic_clear),
            (records.isNotEmpty() || state.batches.value.orEmpty().any { it.count > 0 } ||
                (state.status.value?.count ?: 0) > 0 || historical.isNotEmpty()) && !state.busy) { confirmClear = true },
    )
    val archiveActions = trafficArchiveActions(viewModel,
        batch = (state.selectedBatch ?: state.page?.batch)?.let { mapOf("batch" to it) }.orEmpty(),
        filters = buildMap {
            state.selectedDomain?.let { put("host", it) }
            putAll(state.selectedResponseTypes.responseTypeQuery())
            query.trim().takeIf { it.isNotEmpty() }?.let { put("query", it) }
            method?.let { put("method", it) }
            statusClass?.let { put("status_class", it.toString()) }
            tls?.let { put("tls", it) }
            if (rewrittenOnly) put("rewritten", "true")
            app?.let { put("app", it) }
            outbound?.let { put("outbound", it) }
        }, enabled = !state.busy)
    TopBarMoreMenu(groupingActions + batchActions + archiveActions, stringResource(R.string.more_options)) { menu ->
        TrafficFrame(R.string.traffic_records, stringResource(R.string.traffic_record_count, records.size),
            actions = {
                AppBarButton(viewModel::refresh, !state.busy) { Icon(Icons.Rounded.Refresh, stringResource(R.string.traffic_refresh)) }
                menu()
            }) { padding ->
            Column(Modifier.fillMaxSize().padding(padding)) {
                if (historical.isNotEmpty()) Row(Modifier.padding(horizontal = 16.dp)) {
                    FilterDropdown(R.string.traffic_default_batch, historical.map { it.id }, state.selectedBatch, batchLabels,
                        defaultLabel = R.string.traffic_default_batch) { viewModel.selectBatch(it) }
                }
                GlassTextField(value = query, onValueChange = { query = it }, label = stringResource(R.string.traffic_search),
                    useLabelAsPlaceholder = true, singleLine = true, modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp))
                Row(Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 3.dp),
                    horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    val domainLabel = state.selectedDomain ?: stringResource(R.string.traffic_domain_filter)
                    FilterChip("$domainLabel ▾", state.selectedDomain != null, Modifier.clickable(enabled = !state.busy) {
                        showDomains = true
                    })
                    val typesSelected = state.selectedResponseTypes.isNotEmpty()
                    val typeLabel = if (typesSelected) stringResource(R.string.traffic_response_type_selected, state.selectedResponseTypes.size)
                        else stringResource(R.string.traffic_response_type_filter)
                    FilterChip("$typeLabel ▾", typesSelected, Modifier.clickable(enabled = !state.busy) {
                        showResponseTypes = true
                    })
                    FilterDropdown(R.string.traffic_method, methods, method) { method = it }
                    FilterDropdown(R.string.traffic_http_status, (1..5).map { "${it}xx" }, statusClass?.let { "${it}xx" }) {
                        statusClass = it?.firstOrNull()?.digitToInt()
                    }
                    val tlsOptions = listOf("decrypted", "tunnel")
                    val tlsLabels = listOf(stringResource(R.string.traffic_tls_decrypted), stringResource(R.string.traffic_tls_bypass))
                    FilterDropdown(R.string.traffic_tls, tlsOptions, tls, tlsLabels) { tls = it }
                    FilterChip(stringResource(R.string.traffic_rewritten), rewrittenOnly,
                        Modifier.toggleable(rewrittenOnly, role = Role.Checkbox) { rewrittenOnly = it })
                    FilterDropdown(R.string.traffic_app, apps, app, appLabels) { app = it }
                    FilterDropdown(R.string.traffic_outbound, outbounds, outbound) { outbound = it }
                }
                val issue = state.records.error ?: state.failure
                if (visible.isEmpty() && before == null) ContentStatus(PaddingValues(0.dp), when {
                    issue != null -> trafficError(issue, R.string.traffic_read_failed)
                    records.isNotEmpty() || state.selectedDomain != null || state.selectedResponseTypes.isNotEmpty() ->
                        stringResource(R.string.traffic_no_matches)
                    else -> stringResource(R.string.traffic_empty)
                }, loading = state.loading)
                else LazyColumn(Modifier.fillMaxSize(), state = listState, contentPadding = PaddingValues(horizontal = 16.dp, vertical = 4.dp),
                    verticalArrangement = Arrangement.spacedBy(5.dp)) {
                    issue?.let { item { TrafficErrorText(it) } }
                    if (visible.isEmpty()) item {
                        Text(stringResource(R.string.traffic_no_matches), Modifier.padding(vertical = 10.dp))
                    }
                    for ((key, rows) in groups) {
                        val groupKey = "group:${grouping.name}:${key.orEmpty()}"
                        val expanded = groupKey !in collapsed
                        if (grouping != SessionGrouping.None) item(key = groupKey) {
                            val groupApp = if (grouping == SessionGrouping.App) state.appCatalog[key] else null
                            if (groupApp != null) LaunchedEffect(groupApp.iconKey) { viewModel.loadAppIcon(groupApp.iconKey) }
                            val title = groupApp?.let { "${it.label} · $key" } ?: key
                                ?: stringResource(R.string.traffic_group_unknown)
                            SessionGroupHeader(title, rows.size, groupApp, expanded) {
                                collapsed = if (expanded) collapsed + groupKey else collapsed - groupKey
                            }
                        }
                        if (expanded) itemsIndexed(rows, key = { index, record -> record.id ?: "$groupKey:$index:${record.time}" }) { _, record ->
                            val sourceApp = state.appCatalog[record.appRef]
                            if (sourceApp != null) LaunchedEffect(sourceApp.iconKey) { viewModel.loadAppIcon(sourceApp.iconKey) }
                            SessionRow(record, sourceApp) {
                                viewModel.openRecord(record)
                                navigator.push(Route.TrafficInspector(record.id ?: ""))
                            }
                        }
                    }
                    if (before != null) item(key = "load-older") {
                        state.olderError?.let { TrafficErrorText(it) }
                        ArrowPreference(title = stringResource(R.string.traffic_load_older),
                            enabled = !state.busy && !state.loadingOlder, onClick = viewModel::loadOlder)
                    }
                }
            }
        }
    }
    if (showDomains) CaptureDomainPicker(viewModel, state.page?.batch ?: state.selectedBatch, state.selectedDomain,
        allowAll = true, onDismiss = { showDomains = false }, choose = viewModel::selectDomain)
    if (showResponseTypes) CaptureResponseTypePicker(viewModel, state.page?.batch ?: state.selectedBatch, state.selectedDomain,
        state.selectedResponseTypes, onDismiss = { showResponseTypes = false }, choose = viewModel::selectResponseTypes)
    TrafficConfirmation(confirmClear, R.string.traffic_clear, R.string.traffic_clear_prompt,
        { confirmClear = false }, { confirmClear = false; viewModel.clear() })
    TrafficConfirmation(deleteBatch != null, R.string.traffic_delete_batch, R.string.traffic_delete_batch_prompt,
        { deleteBatch = null }, { deleteBatch?.let(viewModel::deleteBatch); deleteBatch = null })
}

@Composable
private fun SessionGroupHeader(title: String, count: Int, app: AppInfoModel?, expanded: Boolean, onClick: () -> Unit) {
    Row(Modifier.fillMaxWidth().background(MiuixTheme.colorScheme.surfaceContainer, RoundedCornerShape(14.dp))
        .clickable(role = Role.Button, onClick = onClick).heightIn(min = 44.dp).padding(horizontal = 10.dp, vertical = 5.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
        if (app != null) AppIcon(app, size = 20.dp)
        SmallTitle(title, Modifier.weight(1f), insideMargin = PaddingValues(0.dp))
        StatusBadge(stringResource(R.string.traffic_record_count, count))
        Icon(if (expanded) Icons.Rounded.ExpandLess else Icons.Rounded.ExpandMore,
            stringResource(if (expanded) R.string.traffic_group_collapse else R.string.traffic_group_expand))
    }
}

@Composable
private fun SessionRow(record: CaptureRecord, app: AppInfoModel?, onClick: () -> Unit) {
    val primary = MiuixTheme.colorScheme.primary
    Row(Modifier.fillMaxWidth().background(MiuixTheme.colorScheme.surfaceContainer, RoundedCornerShape(14.dp))
        .clickable(onClick = onClick).heightIn(min = 44.dp).padding(horizontal = 9.dp, vertical = 5.dp),
        horizontalArrangement = Arrangement.spacedBy(9.dp), verticalAlignment = Alignment.CenterVertically) {
        StatusBadge(record.method, when (record.method) {
            "GET" -> primary
            "POST", "PUT", "PATCH" -> Color(0xFF18865C)
            "DELETE" -> MiuixTheme.colorScheme.error
            else -> MiuixTheme.colorScheme.onSurfaceVariantSummary
        })
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(1.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(5.dp)) {
                Text(record.host, Modifier.weight(1f), style = MiuixTheme.textStyles.footnote1,
                    maxLines = 1, overflow = TextOverflow.Ellipsis)
                if (record.rewritten) Text(stringResource(R.string.traffic_rewritten), color = primary,
                    style = MiuixTheme.textStyles.footnote2, maxLines = 1)
            }
            record.passthroughReason?.let {
                Text(passthroughLabel(it), style = MiuixTheme.textStyles.footnote2, maxLines = 1,
                    overflow = TextOverflow.Ellipsis, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
            }
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(5.dp)) {
                val source = app?.label ?: record.sourceApp?.packageName ?: record.appKey
                if (app != null) AppIcon(app, size = 14.dp)
                Text(listOfNotNull(source, record.target?.takeIf { it.isNotEmpty() }).joinToString(" · "), Modifier.weight(1f),
                    style = MiuixTheme.textStyles.footnote2.copy(fontFamily = FontFamily.Monospace),
                    maxLines = 1, overflow = TextOverflow.Ellipsis, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                Text("${shortTime(record.time)} · ${stringResource(R.string.traffic_milliseconds, record.durationMs)}",
                    style = MiuixTheme.textStyles.footnote2, color = MiuixTheme.colorScheme.onSurfaceVariantSummary, maxLines = 1)
            }
        }
        val tls = record.tls
        if (tls != null) Icon(Icons.Rounded.Lock, tlsLabel(tls), Modifier.padding(end = 2.dp),
            tint = if (tls == "decrypted") primary else MiuixTheme.colorScheme.onSurfaceVariantSummary)
        if (record.status > 0) StatusBadge(record.status.toString(), when (record.status / 100) {
            2 -> Color(0xFF18865C)
            3 -> primary
            4 -> Color(0xFF9B6612)
            5 -> MiuixTheme.colorScheme.error
            else -> MiuixTheme.colorScheme.onSurfaceVariantSummary
        })
    }
}

private fun shortTime(time: String): String = runCatching {
    OffsetDateTime.parse(time).atZoneSameInstant(ZoneId.systemDefault()).format(DateTimeFormatter.ofPattern("HH:mm:ss"))
}.getOrDefault(time)

@Composable
internal fun StatusBadge(text: String, color: Color = MiuixTheme.colorScheme.primary) {
    Text(text, Modifier.background(color.copy(alpha = 0.13f), RoundedCornerShape(8.dp)).padding(horizontal = 7.dp, vertical = 4.dp),
        style = MiuixTheme.textStyles.footnote1.copy(fontWeight = FontWeight.SemiBold), color = color, maxLines = 1)
}

@Composable
private fun FilterDropdown(title: Int, values: List<String>, selected: String?, labels: List<String> = values,
    defaultLabel: Int = R.string.traffic_filter_all, choose: (String?) -> Unit) {
    var open by remember { mutableStateOf(false) }
    val label = labels.getOrNull(values.indexOf(selected)) ?: selected ?: stringResource(title)
    val all = stringResource(defaultLabel)
    Box {
        FilterChip("$label ▾", selected != null, Modifier.clickable { open = true })
        WindowDropdownPopup(
            entry = DropdownEntry(listOf(DropdownItem(text = all, selected = selected == null, onClick = { choose(null) })) +
                values.mapIndexed { index, value ->
                    DropdownItem(text = labels[index], selected = selected == value, onClick = { choose(value) })
                }),
            show = open, onDismiss = { open = false }, maxHeight = 320.dp,
            dropdownColors = DropdownDefaults.dropdownColors(),
        )
    }
}

@Composable
private fun FilterChip(text: String, selected: Boolean, modifier: Modifier) {
    val colors = MiuixTheme.colorScheme
    Box(modifier.heightIn(min = 40.dp), contentAlignment = Alignment.Center) {
        Text(text, Modifier.widthIn(max = 180.dp)
            .background(if (selected) colors.primary.copy(alpha = 0.15f) else colors.surfaceContainer, RoundedCornerShape(10.dp))
            .padding(horizontal = 9.dp, vertical = 4.dp), color = if (selected) colors.primary else colors.onSurface,
            style = MiuixTheme.textStyles.footnote2, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

@Composable
internal fun RefreshOnResume(viewModel: TrafficViewModel) {
    LifecycleResumeEffect(viewModel) {
        viewModel.refresh()
        onPauseOrDispose {}
    }
}

@Composable
internal fun tlsLabel(tls: String?): String = when (tls) {
    "decrypted" -> stringResource(R.string.traffic_tls_decrypted)
    "bypass" -> stringResource(R.string.traffic_tls_bypass)
    "handshake_failed" -> stringResource(R.string.traffic_tls_failed)
    "pinned_skipped" -> stringResource(R.string.traffic_skip_certificate)
    else -> tls ?: "—"
}

@Composable
internal fun passthroughLabel(reason: String): String = when (reason) {
    "certificate_rejected" -> stringResource(R.string.traffic_certificate_rejected)
    "client_ca_untrusted" -> stringResource(R.string.traffic_ca_untrusted)
    "certificate_bypassed" -> stringResource(R.string.traffic_skip_certificate)
    "client_handshake_closed" -> stringResource(R.string.traffic_client_handshake_closed)
    "client_handshake_timeout" -> stringResource(R.string.traffic_client_handshake_timeout)
    "client_handshake_failed" -> stringResource(R.string.traffic_client_handshake_failed)
    "upstream_handshake_timeout" -> stringResource(R.string.traffic_upstream_handshake_timeout)
    "upstream_handshake_failed" -> stringResource(R.string.traffic_upstream_handshake_failed)
    "excluded_host" -> stringResource(R.string.traffic_skip_excluded)
    "host_not_selected" -> stringResource(R.string.traffic_skip_unselected)
    "decryption_disabled" -> stringResource(R.string.traffic_skip_disabled)
    "decryption_unavailable" -> stringResource(R.string.traffic_skip_unavailable)
    "non_tls" -> stringResource(R.string.traffic_skip_non_tls)
    "sniff_timeout" -> stringResource(R.string.traffic_skip_timeout)
    "unsupported_alpn" -> stringResource(R.string.traffic_skip_alpn)
    "alpn_incompatible" -> stringResource(R.string.traffic_alpn_incompatible)
    "alpn_bypassed" -> stringResource(R.string.traffic_alpn_bypassed)
    "tls_hello_unavailable", "tls_host_unavailable", "protocol_unidentified" -> stringResource(R.string.traffic_skip_unidentified)
    else -> reason
}

@Composable
internal fun TrafficErrorText(error: Throwable, fallback: Int = R.string.traffic_operation_failed) {
    Text(trafficError(error, fallback), Modifier.padding(horizontal = 14.dp, vertical = 8.dp), color = MiuixTheme.colorScheme.error)
}

@Composable
internal fun trafficError(error: Throwable, fallback: Int): String = when (error) {
    is TrafficChangedException -> stringResource(R.string.traffic_changed)
    is ModuleApiException -> when (error.resultCode) {
        "traffic.ca_invalid" -> stringResource(R.string.traffic_ca_failed)
        "traffic.record_not_found" -> stringResource(R.string.traffic_detail_expired)
        "traffic.trust_remove_incomplete" -> stringResource(R.string.traffic_trust_remove_incomplete)
        "traffic.replay_core_unavailable" -> stringResource(R.string.traffic_replay_core)
        "traffic.replay_route_failed" -> stringResource(R.string.traffic_replay_route_failed)
        "traffic.replay_invalid" -> stringResource(R.string.traffic_replay_invalid)
        "traffic.breakpoint_gone" -> stringResource(R.string.traffic_breakpoint_gone)
        "traffic.replay_response_failed" -> stringResource(R.string.traffic_replay_failed)
        else -> stringResource(R.string.traffic_api_error, stringResource(fallback), error.resultCode, error.message)
    }
    else -> stringResource(fallback)
}
