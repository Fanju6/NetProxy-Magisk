package com.fanjv.netproxy.feature.traffic.presentation

import android.content.ClipData
import android.content.ClipDescription
import android.content.ClipboardManager
import android.content.Context
import android.os.Build
import android.os.PersistableBundle
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.feature.traffic.data.CapturedMessage
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.glass.GlassTabRow
import top.yukonga.miuix.kmp.theme.MiuixTheme

@Composable
internal fun TrafficInspectorScreen(id: String, viewModel: TrafficViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val selection = state.selectedRecord?.takeIf { it.record.id.orEmpty() == id }
    val cachedRecord = state.records.value?.firstOrNull { it.id == id }
    val record = selection?.record
    val detail = selection?.detail?.http
    val context = LocalContext.current
    var tab by rememberSaveable(id) { mutableIntStateOf(0) }
    LaunchedEffect(id, selection == null, cachedRecord) {
        if (selection == null) {
            if (cachedRecord != null) viewModel.openRecord(cachedRecord)
            else if (state.records.value == null) viewModel.refresh()
        }
    }
    val copyUrl = stringResource(R.string.traffic_copy_field, stringResource(R.string.traffic_full_url))
    val copyCurl = stringResource(R.string.traffic_copy_curl)
    TopBarMoreMenu(listOf(
        TopBarMenuAction(copyUrl, detail != null) { detail?.let { copyCapture(context, copyUrl, it.url) } },
        TopBarMenuAction(copyCurl, detail != null) {
            detail?.let { copyCapture(context, copyCurl, asCurl(record?.method.orEmpty(), it)) }
        },
    ), stringResource(R.string.more_options)) { menu ->
        TrafficFrame(R.string.traffic_record_detail, record?.host.orEmpty(), actions = { menu() }) { padding ->
            if (record == null) {
                ContentStatus(padding, stringResource(R.string.traffic_detail_expired), loading = state.loading)
                return@TrafficFrame
            }
            Column(Modifier.fillMaxSize().padding(padding)) {
                GlassTabRow(tabs = listOf(stringResource(R.string.traffic_overview), stringResource(R.string.traffic_request_detail),
                    stringResource(R.string.traffic_response_detail)), selectedIndex = tab, onSelect = { tab = it },
                    backdrop = null, modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp))
                LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(16.dp),
                    verticalArrangement = Arrangement.spacedBy(14.dp)) {
                    if (tab == 0) {
                        detail?.let { item {
                            InspectCard {
                                Text(stringResource(R.string.traffic_full_url), style = MiuixTheme.textStyles.subtitle)
                                CodeText(it.url)
                                if (it.urlTruncated) TruncatedNotice()
                            }
                        } }
                        item {
                            InspectCard {
                                InspectRow(stringResource(R.string.traffic_method), record.method)
                                InspectRow(stringResource(R.string.traffic_http_status), record.status.takeIf { it > 0 }?.toString() ?: "—")
                                InspectRow(stringResource(R.string.traffic_time), record.time)
                                InspectRow(stringResource(R.string.traffic_duration), stringResource(R.string.traffic_milliseconds, record.durationMs))
                                InspectRow(stringResource(R.string.traffic_tls), tlsLabel(record.tls))
                                InspectRow(stringResource(R.string.traffic_inbound), "netproxy-in")
                                InspectRow(stringResource(R.string.traffic_outbound), record.outbound)
                                InspectRow(stringResource(R.string.traffic_rewrite_result), record.rewrite ?: "—")
                            }
                        }
                    }
                    when {
                        selection.loading -> item { InfiniteProgressIndicator() }
                        selection.error != null -> item { TrafficErrorText(selection.error, R.string.traffic_detail_failed) }
                        detail == null -> item { Text(stringResource(R.string.traffic_metadata_only)) }
                        tab == 1 || tab == 2 -> item { MessageInspector(if (tab == 1) detail.request else detail.response) }
                    }
                }
            }
        }
    }
}

@Composable
private fun MessageInspector(message: CapturedMessage) {
    val context = LocalContext.current
    val headersTitle = stringResource(R.string.traffic_headers)
    val bodyTitle = stringResource(R.string.traffic_body)
    val copyHeaders = stringResource(R.string.traffic_copy_field, headersTitle)
    val copyBody = stringResource(R.string.traffic_copy_field, bodyTitle)
    val fields = remember(message.headers) { message.headers.toSortedMap(String.CASE_INSENSITIVE_ORDER) }
    InspectCard {
        Text(headersTitle, style = MiuixTheme.textStyles.subtitle)
        if (fields.isEmpty()) Text(stringResource(R.string.traffic_payload_empty))
        fields.forEach { (key, entries) -> entries.forEach { InspectRow(key, it, monospace = true) } }
        if (message.headersTruncated) TruncatedNotice()
        if (fields.isNotEmpty()) AppDialogButton(copyHeaders, {
            copyCapture(context, headersTitle, fields.flatMap { (key, values) -> values.map { "$key: $it" } }.joinToString("\n"))
        })
    }
    val body = message.body
    val formatted = remember(body) { if (body.format == "text") displayBody(body.data, body.contentType) else body.data }
    val isJson = body.contentType?.substringBefore(';')?.let { it == "application/json" || it.endsWith("+json") } == true
    Column(Modifier.padding(top = 14.dp)) {
        InspectCard {
            Text(bodyTitle, style = MiuixTheme.textStyles.subtitle)
            body.contentType?.let { InspectRow(stringResource(R.string.traffic_content_type), it) }
            body.contentEncoding?.let { InspectRow(stringResource(R.string.traffic_content_encoding), it) }
            if (body.format == "base64") StatusBadge(stringResource(R.string.traffic_body_base64))
            if (body.truncated) TruncatedNotice()
            if (formatted.isEmpty()) Text(stringResource(R.string.traffic_payload_empty)) else {
                CodeText(formatted, highlight = isJson && body.format == "text")
                AppDialogButton(copyBody, { copyCapture(context, bodyTitle, body.data) })
            }
        }
    }
}

@Composable
private fun InspectCard(content: @Composable () -> Unit) {
    Column(Modifier.fillMaxWidth().background(MiuixTheme.colorScheme.surfaceContainer, RoundedCornerShape(18.dp)).padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(9.dp)) { content() }
}

@Composable
private fun InspectRow(key: String, value: String, monospace: Boolean = false) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(key, Modifier.weight(0.40f), style = MiuixTheme.textStyles.footnote1, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
        Text(value, Modifier.weight(0.60f), style = if (monospace) MiuixTheme.textStyles.footnote1.copy(fontFamily = FontFamily.Monospace)
            else MiuixTheme.textStyles.footnote1)
    }
}

private val jsonKey = Regex("""(?m)^\s*"[^"\n]+":""")

@Composable
private fun CodeText(text: String, highlight: Boolean = false) {
    val color = MiuixTheme.colorScheme.primary
    val styled = remember(text, highlight, color) {
        buildAnnotatedString {
            if (!highlight || text.length > 16384) append(text) else {
                var offset = 0
                jsonKey.findAll(text).forEach {
                    append(text.substring(offset, it.range.first))
                    withStyle(SpanStyle(color = color)) { append(it.value) }
                    offset = it.range.last + 1
                }
                append(text.substring(offset))
            }
        }
    }
    Text(styled, style = MiuixTheme.textStyles.footnote1.copy(fontFamily = FontFamily.Monospace))
}

@Composable
private fun TruncatedNotice() = StatusBadge(stringResource(R.string.traffic_truncated), MiuixTheme.colorScheme.error)

internal fun copyCapture(context: Context, label: String, data: String) {
    val clip = ClipData.newPlainText(label, data)
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
        clip.description.extras = PersistableBundle().apply { putBoolean(ClipDescription.EXTRA_IS_SENSITIVE, true) }
    }
    context.getSystemService(ClipboardManager::class.java).setPrimaryClip(clip)
}
