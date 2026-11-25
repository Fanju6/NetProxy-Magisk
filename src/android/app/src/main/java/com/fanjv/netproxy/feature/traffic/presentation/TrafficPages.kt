package com.fanjv.netproxy.feature.traffic.presentation

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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Code
import androidx.compose.material.icons.rounded.List
import androidx.compose.material.icons.rounded.Lock
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.AppBarButton
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.feature.traffic.data.CaptureRecord
import com.fanjv.netproxy.feature.traffic.data.TrafficChangedException
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.glass.GlassTextField
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
    content: @Composable (PaddingValues) -> Unit,
) {
    val navigator = LocalNavigator.current
    Scaffold(topBar = {
        AdaptiveTopAppBar(title = stringResource(title), subtitle = subtitle,
            navigationIcon = { BackIconButton(onClick = navigator::pop) }, actions = actions)
    }, floatingActionButton = floatingActionButton, content = content)
}

@Composable
internal fun TrafficScreen(viewModel: TrafficViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
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
                        onCheckedChange = viewModel::setCapture,
                    )
                    Row(Modifier.fillMaxWidth().padding(start = 22.dp, end = 22.dp, bottom = 16.dp),
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
                CardItem("rules") {
                    TrafficLink(R.string.traffic_rules, R.string.traffic_rules_summary, Icons.Rounded.Code) {
                        navigator.push(Route.TrafficRules)
                    }
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
private fun TrafficLink(title: Int, summary: Int, icon: ImageVector, onClick: () -> Unit) {
    ArrowPreference(title = stringResource(title), summary = stringResource(summary), onClick = onClick,
        startAction = { Icon(icon, null, Modifier.padding(end = 12.dp), tint = MiuixTheme.colorScheme.primary) })
}

@Composable
internal fun TrafficSessionsScreen(viewModel: TrafficViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    var query by rememberSaveable { mutableStateOf("") }
    var method by rememberSaveable { mutableStateOf<String?>(null) }
    var statusClass by rememberSaveable { mutableStateOf<Int?>(null) }
    var tls by rememberSaveable { mutableStateOf<String?>(null) }
    var confirmClear by rememberSaveable { mutableStateOf(false) }
    var filter by rememberSaveable { mutableStateOf<String?>(null) }
    val records = state.records.value.orEmpty()
    val visible = remember(records, query, method, statusClass, tls) {
        records.filter { it.matchesSession(query, method, statusClass, tls) }
    }
    RefreshOnResume(viewModel)
    TopBarMoreMenu(listOf(TopBarMenuAction(stringResource(R.string.traffic_clear), records.isNotEmpty() && !state.busy) {
        confirmClear = true
    }), stringResource(R.string.more_options)) { menu ->
        TrafficFrame(R.string.traffic_records, stringResource(R.string.traffic_record_count, records.size),
            actions = {
                AppBarButton(viewModel::refresh, !state.busy) { Icon(Icons.Rounded.Refresh, stringResource(R.string.traffic_refresh)) }
                menu()
            }) { padding ->
            Column(Modifier.fillMaxSize().padding(padding)) {
                GlassTextField(value = query, onValueChange = { query = it }, label = stringResource(R.string.traffic_search),
                    useLabelAsPlaceholder = true, singleLine = true, modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp))
                Row(Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 10.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    FilterPill(method ?: stringResource(R.string.traffic_method), method != null) { filter = "method" }
                    FilterPill(statusClass?.let { "${it}xx" } ?: stringResource(R.string.traffic_http_status), statusClass != null) { filter = "status" }
                    FilterPill(tls?.let { stringResource(if (it == "decrypted") R.string.traffic_tls_decrypted else R.string.traffic_tls_bypass) }
                        ?: stringResource(R.string.traffic_tls), tls != null) { filter = "tls" }
                }
                val issue = state.records.error ?: state.failure
                if (visible.isEmpty()) ContentStatus(PaddingValues(0.dp), when {
                    issue != null -> trafficError(issue, R.string.traffic_read_failed)
                    records.isNotEmpty() -> stringResource(R.string.traffic_no_matches)
                    else -> stringResource(R.string.traffic_empty)
                }, loading = state.loading)
                else LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(horizontal = 16.dp, vertical = 4.dp),
                    verticalArrangement = Arrangement.spacedBy(5.dp)) {
                    issue?.let { item { TrafficErrorText(it) } }
                    itemsIndexed(visible, key = { index, record -> record.id ?: "$index:${record.time}" }) { _, record ->
                        SessionRow(record) {
                            viewModel.openRecord(record)
                            navigator.push(Route.TrafficInspector(record.id ?: ""))
                        }
                    }
                }
            }
        }
    }
    filter?.let { kind ->
        val all = stringResource(R.string.traffic_filter_all)
        val title = when (kind) {
            "method" -> R.string.traffic_method
            "status" -> R.string.traffic_http_status
            else -> R.string.traffic_tls
        }
        val methods = (listOf("GET", "POST", "PUT", "PATCH", "DELETE", "HEAD") + records.map { it.method }).distinct()
        val values = when (kind) {
            "method" -> methods
            "status" -> listOf("1xx", "2xx", "3xx", "4xx", "5xx")
            else -> listOf(stringResource(R.string.traffic_tls_decrypted), stringResource(R.string.traffic_tls_bypass))
        }
        TrafficOptionDialog(title, listOf(all) + values, { filter = null }) { index ->
            when (kind) {
                "method" -> method = methods.getOrNull(index - 1)
                "status" -> statusClass = index.takeIf { it > 0 }
                else -> tls = listOf("decrypted", "tunnel").getOrNull(index - 1)
            }
            filter = null
        }
    }
    TrafficConfirmation(confirmClear, R.string.traffic_clear, R.string.traffic_clear_prompt,
        { confirmClear = false }, { confirmClear = false; viewModel.clear() })
}

@Composable
private fun SessionRow(record: CaptureRecord, onClick: () -> Unit) {
    val primary = MiuixTheme.colorScheme.primary
    Row(Modifier.fillMaxWidth().background(MiuixTheme.colorScheme.surfaceContainer, RoundedCornerShape(14.dp))
        .clickable(onClick = onClick).padding(horizontal = 12.dp, vertical = 10.dp),
        horizontalArrangement = Arrangement.spacedBy(9.dp), verticalAlignment = Alignment.CenterVertically) {
        StatusBadge(record.method, when (record.method) {
            "GET" -> primary
            "POST", "PUT", "PATCH" -> Color(0xFF18865C)
            "DELETE" -> MiuixTheme.colorScheme.error
            else -> MiuixTheme.colorScheme.onSurfaceVariantSummary
        })
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(1.dp)) {
            Text(record.host, style = MiuixTheme.textStyles.body1, maxLines = 1, overflow = TextOverflow.Ellipsis)
            record.target?.let { Text(it, style = MiuixTheme.textStyles.footnote1.copy(fontFamily = FontFamily.Monospace),
                maxLines = 1, overflow = TextOverflow.Ellipsis, color = MiuixTheme.colorScheme.onSurfaceVariantSummary) }
            Text("${shortTime(record.time)} · ${stringResource(R.string.traffic_milliseconds, record.durationMs)}",
                style = MiuixTheme.textStyles.footnote2, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
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
private fun FilterPill(text: String, selected: Boolean, onClick: () -> Unit) {
    val colors = MiuixTheme.colorScheme
    Text("$text ▾", Modifier.background(if (selected) colors.primary.copy(alpha = 0.15f) else colors.surfaceContainer,
        RoundedCornerShape(16.dp)).clickable(onClick = onClick).padding(horizontal = 13.dp, vertical = 8.dp),
        color = if (selected) colors.primary else colors.onSurface, style = MiuixTheme.textStyles.footnote1)
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
    else -> tls ?: "—"
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
        else -> stringResource(R.string.traffic_api_error, stringResource(fallback), error.resultCode, error.message)
    }
    else -> stringResource(fallback)
}
