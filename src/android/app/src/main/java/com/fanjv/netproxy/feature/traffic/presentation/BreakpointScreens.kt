package com.fanjv.netproxy.feature.traffic.presentation

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.PauseCircle
import androidx.compose.material.icons.rounded.TrackChanges
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.AppDropdown
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.feature.traffic.data.CaptureBreakpointAudit
import com.fanjv.netproxy.feature.traffic.data.CaptureBreakpointConfig
import com.fanjv.netproxy.feature.traffic.data.CaptureBreakpointEdit
import com.fanjv.netproxy.feature.traffic.data.CaptureBreakpointRule
import com.fanjv.netproxy.feature.traffic.data.CapturePause
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.Switch
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.glass.GlassTextField
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme

@Composable
internal fun PausedCaptureScreen(viewModel: TrafficViewModel) {
    val navigator = LocalNavigator.current
    val snapshot by remember(viewModel) { viewModel.observePauses() }.collectAsStateWithLifecycle(initialValue = null)
    val pauses = snapshot?.getOrNull().orEmpty()
    TrafficFrame(R.string.traffic_breakpoints_paused) { padding ->
        if (pauses.isEmpty()) {
            val description = if (snapshot?.isFailure == true) R.string.traffic_read_failed else R.string.traffic_breakpoints_no_pauses
            ContentStatus(padding, stringResource(description), loading = snapshot == null)
        } else LazyColumn(Modifier.fillMaxSize().padding(padding), contentPadding = PaddingValues(horizontal = 14.dp)) {
            items(pauses, key = CapturePause::id) { pause ->
                ArrowPreference(title = "${pause.method} · ${breakpointPhase(pause.phase)}", summary = pause.url,
                    onClick = { navigator.push(Route.TrafficPauseEditor(pause.id)) }, insideMargin = TrafficRowInsets,
                    startAction = { CaptureRowIcon(Icons.Rounded.PauseCircle) })
            }
        }
    }
}

@Composable
internal fun PauseEditorScreen(viewModel: TrafficViewModel, id: String) {
    val navigator = LocalNavigator.current
    var pause by remember(id) { mutableStateOf<CapturePause?>(null) }
    var error by remember(id) { mutableStateOf<Throwable?>(null) }
    var busy by remember(id) { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(id) {
        breakpointCall({ pause = viewModel.pausedDetail(id) }, { error = it })
    }
    TrafficFrame(R.string.traffic_breakpoints_paused) { padding ->
        val original = pause
        if (original == null) {
            ContentStatus(padding, error?.let { trafficError(it, R.string.traffic_read_failed) }.orEmpty(), loading = error == null)
            return@TrafficFrame
        }
        fun resolve(edit: CaptureBreakpointEdit?, abort: Boolean) {
            if (busy) return
            busy = true
            scope.launch {
                breakpointCall({ viewModel.resolvePause(id, edit, abort); navigator.pop() }, { error = it })
                busy = false
            }
        }
        PauseFields(original, Modifier.padding(padding), busy, error, ::resolve)
    }
}

@Composable
private fun PauseFields(pause: CapturePause, modifier: Modifier, busy: Boolean, error: Throwable?,
    resolve: (CaptureBreakpointEdit?, Boolean) -> Unit) {
    val request = pause.phase == "request"
    var method by rememberSaveable(pause.id) { mutableStateOf(pause.method) }
    var url by rememberSaveable(pause.id) { mutableStateOf(pause.url) }
    val initialHeaders = remember(pause.id) {
        pause.headers.entries.filterNot { it.key.equals("Content-Length", true) }
            .flatMap { (name, values) -> values.map { "$name: $it" } }.joinToString("\n")
    }
    var headers by rememberSaveable(pause.id) { mutableStateOf(initialHeaders) }
    var body by rememberSaveable(pause.id) { mutableStateOf(pause.body.orEmpty()) }
    var status by rememberSaveable(pause.id) { mutableStateOf(pause.status.toString()) }
    val parsedHeaders = remember(headers) { replayHeaderLines(headers) }
    val actualStatus = status.toIntOrNull()
    val target = url.trim().toHttpUrlOrNull()
    val changedOrigin = target?.let { next ->
        pause.url.toHttpUrlOrNull()?.let { it.scheme != next.scheme || it.host != next.host || it.port != next.port }
    } == true
    val valid = parsedHeaders != null && body.toByteArray().size <= 256 * 1024 &&
        (if (request) method.isNotBlank() && target != null else actualStatus != null && actualStatus in 200..599)
    Column(modifier.fillMaxSize().imePadding().verticalScroll(rememberScrollState()).padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("${breakpointPhase(pause.phase)} · ${pause.url}", maxLines = 2, overflow = TextOverflow.Ellipsis)
        if (request) {
            GlassTextField(method, { method = it }, Modifier.fillMaxWidth(), singleLine = true,
                enabled = !busy, label = stringResource(R.string.traffic_method))
            GlassTextField(url, { url = it }, Modifier.fillMaxWidth(), singleLine = true,
                enabled = !busy, label = stringResource(R.string.traffic_full_url))
            if (changedOrigin) Text(stringResource(R.string.traffic_replay_new_origin))
        } else GlassTextField(status, { status = it }, Modifier.fillMaxWidth(), singleLine = true,
            enabled = !busy, label = stringResource(R.string.traffic_http_status))
        GlassTextField(headers, { headers = it }, Modifier.fillMaxWidth(), minLines = 3, maxLines = 7,
            enabled = !busy, label = stringResource(R.string.traffic_breakpoint_headers))
        if (parsedHeaders == null) Text(stringResource(R.string.traffic_replay_invalid_headers))
        if (pause.body != null) GlassTextField(body, { body = it }, Modifier.fillMaxWidth(), minLines = 3,
            enabled = !busy, label = stringResource(R.string.traffic_body))
        else Text(stringResource(R.string.traffic_breakpoint_body_unavailable))
        error?.let { TrafficErrorText(it) }
        AppDialogButton(stringResource(R.string.traffic_breakpoint_continue), { resolve(null, false) },
            Modifier.fillMaxWidth(), enabled = !busy)
        AppDialogButton(stringResource(R.string.traffic_breakpoint_edit_continue), {
            resolve(CaptureBreakpointEdit(
                method = method.trim().takeIf { request && it != pause.method },
                url = url.trim().takeIf { request && it != pause.url },
                status = actualStatus.takeIf { !request && it != pause.status },
                headers = parsedHeaders.takeIf { headers != initialHeaders },
                body = body.takeIf { pause.body != null && it != pause.body },
            ), false)
        }, Modifier.fillMaxWidth(), enabled = !busy && valid, role = DialogActionRole.Primary)
        AppDialogButton(stringResource(R.string.traffic_breakpoint_abort), { resolve(null, true) },
            Modifier.fillMaxWidth(), enabled = !busy, role = DialogActionRole.Destructive)
    }
}

@Composable
internal fun BreakpointRulesScreen(viewModel: TrafficViewModel) {
    var config by remember { mutableStateOf<CaptureBreakpointConfig?>(null) }
    var error by remember { mutableStateOf<Throwable?>(null) }
    var editing by rememberSaveable { mutableStateOf<Int?>(null) }
    var saving by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(Unit) { breakpointCall({ config = viewModel.breakpointConfig() }, { error = it }) }
    fun save(next: CaptureBreakpointConfig, afterSave: () -> Unit = {}) {
        if (saving) return
        saving = true
        error = null
        scope.launch {
            breakpointCall({ viewModel.saveBreakpoints(next); config = viewModel.breakpointConfig(); afterSave() }, { error = it })
            saving = false
        }
    }
    val loaded = config
    val index = editing
    if (loaded != null && index != null) {
        BackHandler(!saving) { editing = null; error = null }
        BreakpointRuleEditor(loaded.rules.getOrNull(index) ?: CaptureBreakpointRule(), saving, error,
            close = { if (!saving) { editing = null; error = null } },
            delete = if (index >= 0) ({ save(loaded.copy(rules = loaded.rules.filterIndexed { i, _ -> i != index })) { editing = null } }) else null,
            save = { rule ->
                val rules = loaded.rules.toMutableList()
                if (index < 0) rules.add(rule) else rules[index] = rule
                save(loaded.copy(rules = rules)) { editing = null }
            },
        )
        return
    }
    TrafficFrame(R.string.traffic_breakpoints) { padding ->
        if (loaded == null) {
            ContentStatus(padding, error?.let { trafficError(it, R.string.traffic_read_failed) }.orEmpty(), loading = error == null)
            return@TrafficFrame
        }
        var timeout by rememberSaveable(loaded.timeoutSeconds) { mutableStateOf(loaded.timeoutSeconds.toString()) }
        LazyColumn(Modifier.fillMaxSize().padding(padding), contentPadding = PaddingValues(14.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)) {
            item {
                GlassTextField(timeout, { timeout = it }, Modifier.fillMaxWidth(), singleLine = true,
                    enabled = !saving, label = stringResource(R.string.traffic_breakpoint_timeout_seconds))
                val seconds = timeout.toIntOrNull()
                AppDialogButton(stringResource(R.string.save_text), {
                    seconds?.let { save(loaded.copy(timeoutSeconds = it)) }
                }, Modifier.fillMaxWidth(), enabled = !saving && seconds != null && seconds in 1..120 && seconds != loaded.timeoutSeconds)
            }
            item { AppDialogButton(stringResource(R.string.traffic_breakpoint_add), { editing = -1 }, Modifier.fillMaxWidth(), enabled = !saving) }
            error?.let { item { TrafficErrorText(it) } }
            items(loaded.rules.size) { position ->
                val rule = loaded.rules[position]
                val phase = breakpointPhase(rule.phase)
                BasicComponent(enabled = !saving, onClick = { editing = position }, insideMargin = TrafficRowInsets,
                    startAction = { CaptureRowIcon(Icons.Rounded.TrackChanges) },
                    endActions = {
                        Switch(checked = rule.enabled, enabled = !saving, onCheckedChange = { enabled ->
                            save(loaded.copy(rules = loaded.rules.mapIndexed { i, current ->
                                if (i == position) current.copy(enabled = enabled) else current
                            }))
                        })
                    }) {
                    Text(rule.name.ifBlank { "${rule.domains.firstOrNull().orEmpty()} · $phase" },
                        style = MiuixTheme.textStyles.headline1, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    Text((rule.domains + phase).joinToString(" · "), style = MiuixTheme.textStyles.body2,
                        maxLines = 1, overflow = TextOverflow.Ellipsis)
                    Text(rule.pattern, style = MiuixTheme.textStyles.body2, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
        }
    }
}

@Composable
private fun BreakpointRuleEditor(rule: CaptureBreakpointRule, busy: Boolean, error: Throwable?, close: () -> Unit,
    delete: (() -> Unit)?, save: (CaptureBreakpointRule) -> Unit) {
    val phases = listOf("request", "response", "both")
    var title by rememberSaveable { mutableStateOf(rule.name) }
    var domains by rememberSaveable { mutableStateOf(rule.domains.joinToString(", ")) }
    var pattern by rememberSaveable { mutableStateOf(rule.pattern) }
    var phase by rememberSaveable { mutableStateOf(rule.phase) }
    var enabled by rememberSaveable { mutableStateOf(rule.enabled) }
    val parsed = domains.split(',', '\n').map(String::trim).filter(String::isNotEmpty)
    TrafficFrame(R.string.traffic_breakpoints, onBack = close) { padding ->
        Column(Modifier.padding(padding).fillMaxSize().imePadding().verticalScroll(rememberScrollState()).padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)) {
            GlassTextField(title, { title = it }, Modifier.fillMaxWidth(), singleLine = true,
                enabled = !busy, label = stringResource(R.string.traffic_rule_name))
            GlassTextField(domains, { domains = it }, Modifier.fillMaxWidth(), singleLine = true,
                enabled = !busy, label = stringResource(R.string.traffic_rule_domains))
            GlassTextField(pattern, { pattern = it }, Modifier.fillMaxWidth(), singleLine = true,
                enabled = !busy, label = stringResource(R.string.traffic_rule_pattern))
            AppDropdown(title = stringResource(R.string.traffic_rule_phase), items = phases.map { breakpointPhase(it) },
                selectedIndex = phases.indexOf(phase), enabled = !busy, insideMargin = TrafficRowInsets,
                onSelectedIndexChange = { phase = phases[it] })
            SwitchPreference(title = stringResource(R.string.traffic_rule_enabled), checked = enabled,
                enabled = !busy, onCheckedChange = { enabled = it }, insideMargin = TrafficRowInsets)
            error?.let { TrafficErrorText(it) }
            AppDialogButton(stringResource(R.string.save_text), {
                save(CaptureBreakpointRule(parsed, pattern.trim(), phase, enabled, title.trim()))
            }, Modifier.fillMaxWidth(), enabled = !busy && parsed.isNotEmpty() && pattern.isNotBlank())
            delete?.let { AppDialogButton(stringResource(R.string.common_delete), it, Modifier.fillMaxWidth(),
                enabled = !busy, role = DialogActionRole.Destructive) }
        }
    }
}

@Composable
internal fun BreakpointAuditCard(audit: CaptureBreakpointAudit) {
    InspectCard {
        Text(listOf(audit.rule, breakpointPhase(audit.phase), stringResource(when (audit.outcome) {
            "edited" -> R.string.traffic_breakpoint_edited
            "continued" -> R.string.traffic_breakpoint_continued
            "aborted" -> R.string.traffic_breakpoint_aborted
            "timed_out" -> R.string.traffic_breakpoint_timed_out
            "shutdown" -> R.string.traffic_breakpoint_shutdown
            else -> R.string.traffic_breakpoint_canceled
        })).filter(String::isNotBlank).joinToString(" · "))
        if (audit.changes.isNotEmpty()) {
            val labels = audit.changes.map {
                stringResource(when (it) {
                "method" -> R.string.traffic_method
                "url" -> R.string.traffic_full_url
                "status" -> R.string.traffic_http_status
                "headers" -> R.string.traffic_breakpoint_headers
                else -> R.string.traffic_body
                })
            }
            Text(labels.joinToString(" · "))
        }
        if (audit.actor.isNotEmpty()) Text(stringResource(
            if (audit.actor == "operator") R.string.traffic_breakpoint_operator else R.string.traffic_breakpoint_worker,
        ))
        if (audit.originChanged) Text(stringResource(R.string.traffic_breakpoint_origin_changed))
        audit.originalExit?.let { Text(stringResource(R.string.traffic_replay_original_route) + ": " + it.chain.joinToString(" → ") { hop -> hop.tag }) }
        audit.actualExit?.let { Text(stringResource(R.string.traffic_exit_chain) + ": " + it.chain.joinToString(" → ") { hop -> hop.tag }) }
        if (audit.routeChanged) Text(stringResource(R.string.traffic_replay_route_changed))
        if (audit.unattributed) Text(stringResource(R.string.traffic_replay_unattributed))
        if (audit.routeFailure.isNotEmpty()) Text(stringResource(R.string.traffic_replay_route_failed))
    }
}

@Composable
internal fun pausedCountLabel(count: Int) = pluralStringResource(R.plurals.traffic_breakpoint_count, count, count)

@Composable
private fun breakpointPhase(phase: String) = stringResource(when (phase) {
    "request" -> R.string.traffic_phase_request
    "response" -> R.string.traffic_phase_response
    else -> R.string.traffic_breakpoint_both
})

private suspend fun breakpointCall(block: suspend () -> Unit, error: (Throwable) -> Unit) {
    try {
        block()
    } catch (failure: CancellationException) {
        throw failure
    } catch (failure: Exception) {
        error(failure)
    }
}
