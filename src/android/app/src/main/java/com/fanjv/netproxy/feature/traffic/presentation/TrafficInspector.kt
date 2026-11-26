package com.fanjv.netproxy.feature.traffic.presentation

import android.content.ClipData
import android.content.ClipDescription
import android.content.ClipboardManager
import android.content.Context
import android.os.Build
import android.os.PersistableBundle
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.Code
import androidx.compose.material.icons.rounded.ContentCopy
import androidx.compose.material.icons.rounded.KeyboardArrowDown
import androidx.compose.material.icons.rounded.KeyboardArrowUp
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.Alignment
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.feature.apps.data.AndroidApp
import com.fanjv.netproxy.feature.apps.data.AppIconCache
import com.fanjv.netproxy.feature.traffic.data.CaptureRecord
import com.fanjv.netproxy.feature.traffic.data.CapturedBody
import com.fanjv.netproxy.feature.traffic.data.CapturedMessage
import com.fanjv.netproxy.feature.traffic.data.RewriteMatch
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.theme.MiuixTheme

@Composable
internal fun TrafficInspectorScreen(id: String, viewModel: TrafficViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val selection = state.selectedRecord?.takeIf { it.record.id.orEmpty() == id }
    val cachedRecord = state.records.value?.firstOrNull { it.id == id }
    val record = selection?.record
    val detail = selection?.detail?.http
    val context = LocalContext.current
    val navigator = LocalNavigator.current
    var tab by rememberSaveable(id) { mutableIntStateOf(0) }
    var expanded by rememberSaveable(id, tab) { mutableStateOf(false) }
    var pretty by rememberSaveable(id, tab) { mutableStateOf(true) }
    var headerName by remember(id, tab) { mutableStateOf<String?>(null) }
    var headerValue by remember(id, tab) { mutableStateOf("") }
    val message = when (tab) { 1 -> detail?.request; 2 -> detail?.response; else -> null }
    val body = message?.body
    val json = body?.format == "text" && isJsonBody(body.contentType)
    val formatted = remember(body, expanded) {
        if (!expanded) null else body?.let { if (it.format == "base64") captureHex(it.data) else displayBody(it.data, it.contentType) }
    }
    val chunks = remember(expanded, body, pretty) {
        if (!expanded || body == null || body.data.isEmpty()) emptyList()
        else bodyChunks(if (pretty) formatted ?: body.data else body.data)
    }
    val auditIndices = remember(detail, state.rules.value) {
        detail?.rewrites.orEmpty().map { state.rules.value?.indexOfCapture(it) }
    }
    LaunchedEffect(id, selection == null, cachedRecord) {
        if (selection == null) {
            if (cachedRecord != null) viewModel.openRecord(cachedRecord)
            else if (state.records.value == null) viewModel.refresh()
        }
    }
    val copyUrl = stringResource(R.string.traffic_copy_field, stringResource(R.string.traffic_full_url))
    val copyCurl = stringResource(R.string.traffic_copy_curl)
    headerName?.let { name ->
        TrafficCopySheet(R.string.traffic_header_copy, listOf(
            stringResource(R.string.traffic_copy_name), stringResource(R.string.traffic_copy_value),
            stringResource(R.string.traffic_copy_line),
        ), { headerName = null }) { index ->
            copyCapture(context, name, listOf(name, headerValue, "$name: $headerValue")[index])
            headerName = null
        }
    }
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
                InspectorTabs(tabs = listOf(stringResource(R.string.traffic_overview), stringResource(R.string.traffic_request_detail),
                    stringResource(R.string.traffic_response_detail), stringResource(R.string.traffic_rewrite_tab)),
                    selected = tab, select = { tab = it })
                LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(horizontal = 14.dp, vertical = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (tab == 0) {
                        detail?.websocket?.let { websocket ->
                            item {
                                InspectCard {
                                    Text(stringResource(R.string.traffic_ws_frames), style = MiuixTheme.textStyles.subtitle)
                                    if (websocket.truncated) TruncatedNotice()
                                }
                            }
                            items(websocket.frames.size) { index ->
                                val frame = websocket.frames[index]
                                InspectCard {
                                    val direction = stringResource(if (frame.direction == "send") R.string.traffic_ws_sent else R.string.traffic_ws_received)
                                    InspectRow(direction, "0x${frame.opcode.toString(16)} · ${frame.size} B")
                                    frame.text?.let { CodeText(it) }
                                    if (frame.truncated) TruncatedNotice()
                                }
                            }
                        }
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
                                record.passthroughReason?.let {
                                    InspectRow(stringResource(R.string.traffic_skip_reason), passthroughLabel(it))
                                }
                                InspectRow(stringResource(R.string.traffic_inbound), record.exit?.inbound ?: "netproxy-in")
                                InspectRow(stringResource(R.string.traffic_outbound), record.outbound)
                                record.exit?.let { exit ->
                                    if (exit.chain.isNotEmpty()) InspectRow(stringResource(R.string.traffic_exit_chain),
                                        exit.chain.joinToString(" → ") { "${it.tag} (${it.type})" })
                                    exit.remote?.let { InspectRow(stringResource(R.string.traffic_remote), it) }
                                }
                                InspectSourceApp(record, selection.app)
                                record.exit?.process?.let { InspectRow(stringResource(R.string.traffic_process), it) }
                                InspectRow(stringResource(R.string.traffic_rewrite_result), when {
                                    record.rewritten -> stringResource(R.string.traffic_rewritten)
                                    record.rewrite != null -> stringResource(R.string.traffic_rule_unchanged)
                                    else -> "—"
                                })
                            }
                        }
                    }
                    when {
                        selection.loading -> item { InfiniteProgressIndicator() }
                        selection.error != null -> item { TrafficErrorText(selection.error, R.string.traffic_detail_failed) }
                        detail == null -> item { Text(stringResource(R.string.traffic_metadata_only)) }
                        message != null -> {
                            item {
                                MessageHeaders(message) { name, value -> headerName = name; headerValue = value }
                            }
                            body?.let {
                                if (it.format == "binary" || it.blob) {
                                    item("media-$tab") { TrafficMedia(id, if (tab == 1) "request" else "response", it, viewModel) }
                                } else {
                                    item { BodyHeader(it, expanded, { expanded = !expanded }, formatted != it.data, pretty, { pretty = !pretty }) }
                                    bodyContent(chunks, json && pretty)
                                }
                            }
                        }
                        tab == 3 -> {
                            if (detail.rewrites.isEmpty()) item { Text(stringResource(R.string.traffic_no_rewrites)) }
                            items(detail.rewrites.size) { index ->
                                RewriteAuditCard(detail.rewrites[index], record.status,
                                    canEdit = auditIndices[index] != null && !state.busy) {
                                    auditIndices[index]?.let {
                                        viewModel.editRule(it)
                                        navigator.push(Route.TrafficRuleEditor)
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun InspectSourceApp(record: CaptureRecord, app: AndroidApp?) {
    (record.sourceApp?.packageName ?: record.exit?.packageName)?.let { packageName ->
        val bitmap = app?.let {
            remember(it.iconKey, AppIconCache.revision) { AppIconCache.cachedIcon(it.iconKey) }
        }
        val appText = if (app == null || app.label == packageName) packageName else "${app.label}\n$packageName"
        InspectRow(stringResource(R.string.traffic_app), appText) {
            bitmap?.let {
                Image(it, null, Modifier.align(Alignment.CenterVertically).padding(start = 6.dp)
                    .size(36.dp).clip(RoundedCornerShape(9.dp)))
            }
        }
    }
    (record.sourceApp?.uid ?: record.exit?.uid)?.let { InspectRow("UID", it.toString()) }
    record.sourceApp?.userId?.let {
        InspectRow(stringResource(R.string.traffic_app_user), it.toString())
    }
}

@Composable
private fun InspectorTabs(tabs: List<String>, selected: Int, select: (Int) -> Unit) {
    val colors = MiuixTheme.colorScheme
    BoxWithConstraints(Modifier.fillMaxWidth()) {
        Row(
            Modifier.horizontalScroll(rememberScrollState()).widthIn(min = maxWidth).selectableGroup(),
            horizontalArrangement = Arrangement.SpaceEvenly,
        ) {
            tabs.forEachIndexed { index, title ->
                val active = index == selected
                Box(
                    Modifier.height(44.dp).widthIn(min = 64.dp)
                        .selectable(active, role = Role.Tab) { select(index) },
                    contentAlignment = Alignment.Center,
                ) {
                    Text(title, Modifier.padding(horizontal = 12.dp), style = MiuixTheme.textStyles.footnote1,
                        color = if (active) colors.primary else colors.onSurfaceVariantSummary, maxLines = 1)
                    if (active) Box(Modifier.matchParentSize(), contentAlignment = Alignment.BottomCenter) {
                        Box(Modifier.fillMaxWidth().height(2.dp).padding(horizontal = 12.dp)
                            .background(colors.primary, RoundedCornerShape(1.dp)))
                    }
                }
            }
        }
    }
}

@Composable
private fun MessageHeaders(message: CapturedMessage, onHeader: (String, String) -> Unit) {
    val context = LocalContext.current
    val headersTitle = stringResource(R.string.traffic_headers)
    val copyHeaders = stringResource(R.string.traffic_copy_field, headersTitle)
    val fields = remember(message.headers) { message.headers.toSortedMap(String.CASE_INSENSITIVE_ORDER) }
    InspectCard {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(headersTitle, Modifier.weight(1f), style = MiuixTheme.textStyles.subtitle)
            if (fields.isNotEmpty()) InspectAction(Icons.Rounded.ContentCopy, copyHeaders) {
                copyCapture(context, headersTitle, fields.flatMap { (key, values) -> values.map { "$key: $it" } }.joinToString("\n"))
            }
        }
        if (fields.isEmpty()) Text(stringResource(R.string.traffic_payload_empty))
        else Text(stringResource(R.string.traffic_header_copy_hint), style = MiuixTheme.textStyles.footnote1,
            color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
        fields.forEach { (key, entries) -> entries.forEach {
            InspectRow(key, it, monospace = true, modifier = Modifier.clickable(
                onClickLabel = stringResource(R.string.traffic_header_copy),
            ) { onHeader(key, it) })
        } }
        if (message.headersTruncated) TruncatedNotice()
    }
}

@Composable
private fun BodyHeader(body: CapturedBody, expanded: Boolean, toggle: () -> Unit, formattable: Boolean, pretty: Boolean, format: () -> Unit) {
    val context = LocalContext.current
    val title = stringResource(R.string.traffic_body)
    val hasData = body.data.isNotEmpty()
    val expandLabel = stringResource(if (expanded) R.string.traffic_hide_body else R.string.traffic_show_body)
    val meta = listOfNotNull(
        body.contentType?.substringBefore(';'),
        body.size?.let { stringResource(R.string.traffic_bytes, it) },
        body.contentEncoding,
        if (body.decoded) stringResource(R.string.traffic_body_decoded) else null,
        if (body.format == "base64") stringResource(R.string.traffic_body_base64) else null,
        if (body.truncated) stringResource(R.string.traffic_truncated) else null,
        if (!hasData) stringResource(R.string.traffic_payload_empty) else null,
    ).joinToString(" · ")
    InspectCard {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Row(
                Modifier.weight(1f).heightIn(min = 40.dp)
                    .then(if (hasData) Modifier.clickable(onClickLabel = expandLabel, onClick = toggle) else Modifier),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(Modifier.weight(1f)) {
                    Text(title, style = MiuixTheme.textStyles.subtitle)
                    if (meta.isNotEmpty()) Text(meta, style = MiuixTheme.textStyles.footnote2,
                        maxLines = 2, overflow = TextOverflow.Ellipsis,
                        color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                }
                if (hasData) Icon(if (expanded) Icons.Rounded.KeyboardArrowUp else Icons.Rounded.KeyboardArrowDown,
                    null, Modifier.size(18.dp), tint = MiuixTheme.colorScheme.onSurfaceVariantSummary)
            }
            if (hasData && expanded && formattable) InspectAction(Icons.Rounded.Code,
                stringResource(if (pretty) R.string.traffic_show_raw else R.string.traffic_show_pretty), format)
            if (hasData) InspectAction(Icons.Rounded.ContentCopy, stringResource(R.string.traffic_copy_field, title)) {
                copyCapture(context, title, body.data)
            }
        }
        if (body.truncated) body.capturedSize?.let {
            InspectRow(stringResource(R.string.traffic_captured_size), stringResource(R.string.traffic_bytes, it.toLong()))
        }
    }
}

@Composable
private fun InspectAction(icon: ImageVector, label: String, action: () -> Unit) {
    Box(Modifier.size(40.dp).clip(RoundedCornerShape(12.dp)).clickable(role = Role.Button, onClick = action),
        contentAlignment = Alignment.Center) {
        Icon(icon, label, Modifier.size(18.dp), tint = MiuixTheme.colorScheme.primary)
    }
}

private fun LazyListScope.bodyContent(chunks: List<String>, highlight: Boolean) {
    items(chunks) { chunk ->
        InspectCard { CodeText(chunk, highlight = highlight) }
    }
}

@Composable
private fun RewriteAuditCard(match: RewriteMatch, status: Int, canEdit: Boolean, edit: () -> Unit) {
    InspectCard {
        val editLabel = stringResource(R.string.traffic_rule_edit)
        Row(Modifier.fillMaxWidth().then(if (canEdit) Modifier.heightIn(min = 40.dp)
            .clickable(onClickLabel = editLabel, onClick = edit) else Modifier),
            verticalAlignment = Alignment.CenterVertically) {
            Text(stringResource(R.string.traffic_matched_rule, match.ruleIndex + 1), Modifier.weight(1f),
                style = MiuixTheme.textStyles.subtitle)
            if (canEdit) Icon(Icons.AutoMirrored.Rounded.KeyboardArrowRight, null, Modifier.size(18.dp),
                tint = MiuixTheme.colorScheme.primary)
        }
        CodeText(match.pattern)
        InspectRow(stringResource(R.string.traffic_rule_phase), stringResource(
            if (match.phase == "request") R.string.traffic_request_detail else R.string.traffic_response_detail))
        StatusBadge(stringResource(when (match.state) {
            "applied" -> R.string.traffic_rewritten
            "shadowed" -> R.string.traffic_rule_shadowed
            "not_reached" -> R.string.traffic_rule_not_reached
            "error" -> R.string.traffic_rule_error
            else -> R.string.traffic_rule_unchanged
        }))
        if (match.state !in setOf("shadowed", "not_reached")) InspectRow(
            stringResource(R.string.traffic_duration), stringResource(R.string.traffic_microseconds, match.durationUs))
        match.error?.let { error -> InspectRow(stringResource(R.string.traffic_rule_error), stringResource(when (error) {
            "deadline" -> R.string.traffic_rule_deadline
            "busy" -> R.string.traffic_rule_busy
            "body_unavailable" -> R.string.traffic_rule_body_unavailable
            "invalid_result" -> R.string.traffic_rule_invalid_result
            else -> R.string.traffic_rule_script_error
        })) }
        match.originalStatus?.takeIf { it > 0 }?.let { before ->
            InspectRow(stringResource(R.string.traffic_http_status), "$before → $status")
        }
        if ("reject" in match.actions) Text(stringResource(R.string.traffic_rule_rejected))
        match.redirect?.let { InspectRow(stringResource(R.string.traffic_rule_redirect), it) }
        match.body?.let { InspectRow(stringResource(R.string.traffic_body),
            "${stringResource(R.string.traffic_bytes, it.before.toLong())} → ${stringResource(R.string.traffic_bytes, it.after.toLong())}") }
        match.headers.forEach { change ->
            val before = change.before.joinToString("\n").ifEmpty { "—" }
            val after = change.after.joinToString("\n").ifEmpty { "—" }
            InspectRow(change.name, "$before → $after", monospace = true)
            if (change.truncated) TruncatedNotice()
        }
        if (!canEdit) Text(stringResource(R.string.traffic_rule_unavailable), style = MiuixTheme.textStyles.footnote1)
    }
}

@Composable
internal fun InspectCard(content: @Composable () -> Unit) {
    Column(Modifier.fillMaxWidth().background(MiuixTheme.colorScheme.surfaceContainer, RoundedCornerShape(14.dp))
        .padding(horizontal = 12.dp, vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) { content() }
}

@Composable
private fun InspectRow(
    key: String,
    value: String,
    modifier: Modifier = Modifier,
    monospace: Boolean = false,
    trailing: @Composable RowScope.() -> Unit = {},
) {
    val style = MiuixTheme.textStyles.footnote1.copy(lineHeight = 16.sp)
    Row(modifier.fillMaxWidth().padding(vertical = 1.dp), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(key, Modifier.fillMaxWidth(0.40f), style = style, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
        Text(value, Modifier.weight(1f), textAlign = TextAlign.Start,
            style = if (monospace) style.copy(fontFamily = FontFamily.Monospace) else style)
        trailing()
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
