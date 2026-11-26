package com.fanjv.netproxy.feature.traffic.presentation

import android.content.Intent
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.DeleteOutline
import androidx.compose.material.icons.rounded.FileDownload
import androidx.compose.material.icons.rounded.Lock
import androidx.compose.material.icons.rounded.RemoveCircleOutline
import androidx.compose.material.icons.rounded.Security
import androidx.compose.material.icons.rounded.Share
import androidx.compose.material.icons.rounded.SwapHoriz
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppBarButton
import com.fanjv.netproxy.core.ui.component.AppBottomSheet
import com.fanjv.netproxy.core.ui.component.AppDialog
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.AppDropdown
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.core.ui.component.chromeIcon
import com.fanjv.netproxy.core.ui.component.dialogTextFieldColors
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlin.math.abs
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.FloatingActionButton
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.Switch
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.glass.GlassTextField
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import java.io.File

@Composable
internal fun TrafficRulesScreen(viewModel: TrafficViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    val rules = state.rules.value
    val editable = !state.busy && rules != null && state.rules.error == null
    RefreshOnResume(viewModel)
    fun edit(index: Int? = null) {
        if (editable) {
            viewModel.editRule(index)
            navigator.push(Route.TrafficRuleEditor)
        }
    }
    TrafficFrame(R.string.traffic_rules, floatingActionButton = {
        if (editable) FloatingActionButton(onClick = { edit() }) {
            Icon(Icons.Rounded.Add, stringResource(R.string.traffic_rule_add))
        }
    }) { padding ->
        if (rules.isNullOrEmpty()) ContentStatus(padding,
            state.rules.error?.let { trafficError(it, R.string.traffic_read_failed) } ?: stringResource(R.string.traffic_rules_empty),
            loading = state.loading)
        else LazyColumn(Modifier.fillMaxSize().padding(padding).padding(horizontal = 16.dp),
            contentPadding = PaddingValues(bottom = 88.dp)) {
            state.failure?.let { issue -> item { TrafficErrorText(issue) } }
            groupedCardSection("rules", { stringResource(R.string.traffic_rules) }, rules.mapIndexed { index, rule ->
                CardItem(index.toString()) {
                    val phase = phaseLabel(rule.phase)
                    val title = rule.name.ifBlank { rule.domains.firstOrNull()?.let { "$it · $phase" } ?: phase }
                    val summary = (rule.domains + phase).joinToString(" · ")
                    val switchLabel = stringResource(R.string.traffic_rule_enabled)
                    BasicComponent(
                        enabled = editable, onClick = { edit(index) }, insideMargin = TrafficRowInsets,
                        endActions = {
                            Switch(checked = rule.enabled, enabled = editable,
                                onCheckedChange = { viewModel.setRuleEnabled(index, it) },
                                modifier = Modifier.semantics { contentDescription = "$title · $switchLabel" })
                        },
                    ) {
                        Text(title, style = MiuixTheme.textStyles.headline1, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(summary, style = MiuixTheme.textStyles.body2, color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                            maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(rule.pattern, style = MiuixTheme.textStyles.body2.copy(fontFamily = FontFamily.Monospace),
                            color = MiuixTheme.colorScheme.onSurfaceVariantSummary, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                }
            })
        }
    }
}

@Composable
internal fun TrafficRuleEditorScreen(viewModel: TrafficViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    val draft = state.ruleDraft
    var confirmDelete by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(draft == null) { if (draft == null) navigator.pop() }
    if (draft == null) return
    val rule = draft.rule
    var domains by rememberSaveable(draft.index) { mutableStateOf(rule.domains.joinToString(", ")) }
    var pickDomain by rememberSaveable { mutableStateOf(false) }
    TopBarMoreMenu(listOf(
        TopBarMenuAction(stringResource(R.string.common_delete), draft.index != null && !state.saving) { confirmDelete = true },
    ), stringResource(R.string.more_options)) { menu ->
        TrafficFrame(if (draft.index == null) R.string.traffic_rule_add else R.string.traffic_rule_edit,
            actions = {
                if (draft.index != null) menu()
                AppBarButton({ viewModel.saveRule() }, !state.saving && rule.domains.isNotEmpty() && rule.pattern.isNotBlank() && rule.script.isNotBlank()) {
                    if (state.saving) InfiniteProgressIndicator(modifier = Modifier.size(20.dp))
                    else Icon(chromeIcon(Icons.Rounded.Check), stringResource(R.string.save_text))
                }
            }) { padding ->
            Column(Modifier.fillMaxSize().padding(padding).imePadding().verticalScroll(rememberScrollState()).padding(horizontal = 14.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp)) {
                GlassTextField(value = rule.name, onValueChange = { viewModel.updateRule(rule.copy(name = it)) },
                    modifier = Modifier.fillMaxWidth(), singleLine = true, enabled = !state.saving,
                    label = stringResource(R.string.traffic_rule_name))
                SwitchPreference(title = stringResource(R.string.traffic_rule_enabled), checked = rule.enabled,
                    enabled = !state.saving, onCheckedChange = { viewModel.updateRule(rule.copy(enabled = it)) }, insideMargin = TrafficRowInsets)
                AppDropdown(title = stringResource(R.string.traffic_rule_phase),
                    items = listOf(phaseLabel("request"), phaseLabel("response")),
                    selectedIndex = if (rule.phase == "request") 0 else 1, enabled = !state.saving,
                    insideMargin = TrafficRowInsets,
                    onSelectedIndexChange = { viewModel.updateRule(rule.copy(phase = if (it == 0) "request" else "response")) })
                GlassTextField(value = domains, onValueChange = {
                    domains = it
                    viewModel.updateRule(rule.copy(domains = hostList(it)))
                }, modifier = Modifier.fillMaxWidth(), singleLine = true, enabled = !state.saving,
                    label = stringResource(R.string.traffic_rule_domains))
                ArrowPreference(title = stringResource(R.string.traffic_rule_pick_domain),
                    enabled = !state.saving, onClick = { pickDomain = true }, insideMargin = TrafficRowInsets)
                GlassTextField(value = rule.pattern, onValueChange = { viewModel.updateRule(rule.copy(pattern = it)) },
                    modifier = Modifier.fillMaxWidth(), singleLine = true, enabled = !state.saving,
                    label = stringResource(R.string.traffic_rule_pattern), colors = dialogTextFieldColors(state.draftError != null))
                if (rule.domains.isEmpty()) {
                    Text(stringResource(R.string.traffic_rule_domain_required), style = MiuixTheme.textStyles.footnote1,
                        color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                }
                GlassTextField(value = rule.script, onValueChange = { viewModel.updateRule(rule.copy(script = it)) },
                    modifier = Modifier.fillMaxWidth(), minLines = 8, maxLines = 16, enabled = !state.saving,
                    label = stringResource(R.string.traffic_rule_script),
                    textStyle = MiuixTheme.textStyles.body2.copy(fontFamily = FontFamily.Monospace),
                    colors = dialogTextFieldColors(state.draftError != null))
                state.draftError?.let { TrafficErrorText(it) }
            }
        }
    }
    if (pickDomain) CaptureDomainPicker(viewModel, null, null, allowAll = false, onDismiss = { pickDomain = false }) { host ->
        if (host != null && host !in rule.domains) {
            val updated = rule.domains + host
            domains = updated.joinToString(", ")
            viewModel.updateRule(rule.copy(domains = updated))
        }
    }
    TrafficConfirmation(confirmDelete, R.string.common_delete, R.string.traffic_rule_delete,
        { confirmDelete = false }, { confirmDelete = false; viewModel.saveRule(delete = true) })
}

private fun hostList(value: String) = value.split(',', '\n').map(String::trim).filter(String::isNotEmpty)

@Composable
internal fun TrafficHttpsScreen(viewModel: TrafficViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val policy = state.policy.value
    val editable = !state.busy && policy != null && state.policy.error == null
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var confirmTrust by rememberSaveable { mutableStateOf(false) }
    var confirmRemoveUserCA by rememberSaveable { mutableStateOf(false) }
    var confirmClearPinned by rememberSaveable { mutableStateOf(false) }
    var confirmClearAlpn by rememberSaveable { mutableStateOf(false) }
    var removeBypass by remember { mutableStateOf<BypassItem?>(null) }
    var addHost by rememberSaveable { mutableStateOf(false) }
    var newHost by rememberSaveable { mutableStateOf("") }
    var removeHost by rememberSaveable { mutableStateOf<String?>(null) }
    RefreshOnResume(viewModel)
    LaunchedEffect(Unit) {
        viewModel.readCertificateStatus()
        viewModel.readPinned()
        viewModel.readAlpnBypasses()
    }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("application/x-pem-file")) { uri ->
        val cert = viewModel.takeCertificate()
        if (uri != null && cert != null) scope.launch {
            val saved = withContext(Dispatchers.IO) {
                runCatching { context.contentResolver.openOutputStream(uri, "wt")?.use {
                    it.write(cert.pem.toByteArray(Charsets.US_ASCII))
                } != null }.getOrDefault(false)
            }
            Toast.makeText(context, if (saved) R.string.traffic_ca_saved else R.string.traffic_ca_failed, Toast.LENGTH_LONG).show()
        }
    }
    val shareTitle = stringResource(R.string.traffic_ca_share)
    fun share() = viewModel.prepareCertificate {
        val cert = viewModel.takeCertificate() ?: return@prepareCertificate
        scope.launch {
            val uri = withContext(Dispatchers.IO) {
                runCatching {
                    val dir = File(context.cacheDir, "traffic-ca").apply { mkdirs() }
                    val file = File(dir, cert.filename).apply { writeText(cert.pem, Charsets.US_ASCII) }
                    FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", file)
                }.getOrNull()
            }
            if (uri == null) Toast.makeText(context, R.string.traffic_ca_failed, Toast.LENGTH_LONG).show()
            else context.startActivity(Intent.createChooser(Intent(Intent.ACTION_SEND).apply {
                type = "application/x-pem-file"
                putExtra(Intent.EXTRA_STREAM, uri)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }, shareTitle))
        }
    }
    TopBarMoreMenu(listOf(
        TopBarMenuAction(stringResource(R.string.traffic_ca_export), editable, icon = Icons.Rounded.FileDownload) {
            viewModel.prepareCertificate { picker.launch(it) }
        },
        TopBarMenuAction(shareTitle, editable, icon = Icons.Rounded.Share) { share() },
    ), stringResource(R.string.more_options)) { menu ->
    TrafficFrame(R.string.traffic_https, actions = {
        AppBarButton(onClick = { newHost = ""; addHost = true }, enabled = editable && !addHost) {
            Icon(Icons.Rounded.Add, stringResource(R.string.traffic_host_add))
        }
        menu()
    }) { padding ->
        if (policy == null) ContentStatus(padding,
            state.policy.error?.let { trafficError(it, R.string.traffic_https_unavailable) } ?: stringResource(R.string.traffic_https_unavailable),
            loading = state.loading)
        else LazyColumn(Modifier.fillMaxSize().padding(padding).padding(horizontal = 16.dp),
            contentPadding = PaddingValues(bottom = 24.dp)) {
            state.failure?.let { issue -> item { TrafficErrorText(issue) } }
            item("switch") {
                SwitchPreference(title = stringResource(R.string.traffic_mitm),
                    summary = stringResource(R.string.traffic_mitm_hint) + " " + stringResource(R.string.traffic_user_ca_hint),
                    checked = policy.enabled, enabled = editable, onCheckedChange = viewModel::setMitmEnabled,
                    insideMargin = TrafficRowInsets, startAction = { CaptureRowIcon(Icons.Rounded.Lock) })
            }
            groupedCardSection("hosts", { stringResource(R.string.traffic_hosts) }, buildList {
                if (addHost) add(CardItem("add") {
                    Row(Modifier.padding(horizontal = 8.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                        GlassTextField(value = newHost, onValueChange = { newHost = it }, singleLine = true,
                            label = stringResource(R.string.traffic_host), modifier = Modifier.weight(1f))
                        AppBarButton({ addHost = false }) {
                            Icon(Icons.Rounded.Close, stringResource(R.string.common_cancel))
                        }
                        AppBarButton({
                            addHost = false
                            viewModel.setHosts(policy.hosts.filterNot { it == "*" } + newHost.trim())
                        }, newHost.isNotBlank() && editable) {
                            Icon(chromeIcon(Icons.Rounded.Check), stringResource(R.string.common_confirm))
                        }
                    }
                })
                if (addHost || policy.hosts == listOf("*") || policy.hosts.isEmpty()) add(CardItem("hint") {
                    Text(stringResource(R.string.traffic_hosts_hint), Modifier.padding(TrafficRowInsets),
                        color = MiuixTheme.colorScheme.onSurfaceVariantSummary, style = MiuixTheme.textStyles.footnote1)
                })
                policy.hosts.forEach { host -> add(CardItem("host:$host") {
                    ArrowPreference(title = host, summary = stringResource(R.string.traffic_host_remove),
                        enabled = editable, onClick = { removeHost = host }, insideMargin = TrafficRowInsets)
                }) }
            })
            item("bypass") {
                var bypass by rememberSaveable(policy.bypassHosts) { mutableStateOf(policy.bypassHosts.joinToString(", ")) }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    GlassTextField(value = bypass, onValueChange = { bypass = it }, modifier = Modifier.weight(1f),
                        singleLine = true, enabled = editable, label = stringResource(R.string.traffic_bypass_hosts))
                    AppBarButton({ viewModel.setBypassHosts(hostList(bypass)) }, editable && hostList(bypass) != policy.bypassHosts) {
                        Icon(chromeIcon(Icons.Rounded.Check), stringResource(R.string.save_text))
                    }
                }
            }
            groupedCardSection("pinned", { stringResource(R.string.traffic_pinned_title) }, buildList {
                val pinned = state.pinned.value.orEmpty()
                state.pinned.error?.let { error -> add(CardItem("error") { TrafficErrorText(error) }) }
                if (pinned.isEmpty() && state.pinned.value != null) {
                    add(CardItem("empty") {
                        Text(stringResource(R.string.traffic_pinned_empty), Modifier.padding(TrafficRowInsets))
                    })
                }
                if (pinned.isNotEmpty()) add(CardItem("clear") {
                    ArrowPreference(title = stringResource(R.string.traffic_pinned_clear), enabled = !state.busy,
                        onClick = { confirmClearPinned = true }, insideMargin = TrafficRowInsets,
                        startAction = { CaptureRowIcon(Icons.Rounded.DeleteOutline) })
                })
                pinned.forEach { pin ->
                    add(CardItem("${pin.userId}:${pin.packageName}:${pin.host}") {
                        val item = BypassItem(pin.host, pin.userId, pin.packageName, pin.reason, pin.createdAt)
                        BypassRow(item, !state.busy, item.app) { removeBypass = item }
                    })
                }
            })
            groupedCardSection("alpn", { stringResource(R.string.traffic_alpn_title) }, buildList {
                val bypasses = state.alpnBypasses.value.orEmpty()
                state.alpnBypasses.error?.let { error -> add(CardItem("error") { TrafficErrorText(error) }) }
                if (bypasses.isEmpty() && state.alpnBypasses.value != null) {
                    add(CardItem("empty") {
                        Text(stringResource(R.string.traffic_alpn_empty), Modifier.padding(TrafficRowInsets))
                    })
                }
                if (bypasses.isNotEmpty()) add(CardItem("clear") {
                    ArrowPreference(title = stringResource(R.string.traffic_alpn_clear), enabled = !state.busy,
                        onClick = { confirmClearAlpn = true }, insideMargin = TrafficRowInsets,
                        startAction = { CaptureRowIcon(Icons.Rounded.DeleteOutline) })
                })
                bypasses.forEach { entry ->
                    add(CardItem("${entry.userId}:${entry.packageName}:${entry.host}") {
                        val item = BypassItem(entry.host, entry.userId, entry.packageName, "alpn_incompatible", entry.expiresAt, true)
                        BypassRow(item, !state.busy, item.app + "\n" + stringResource(R.string.traffic_alpn_expires, entry.expiresAt)) {
                            removeBypass = item
                        }
                    })
                }
            })
            groupedCardSection("ca", { stringResource(R.string.traffic_ca_section) }, listOf(
                CardItem("fingerprint") {
                    Column(Modifier.padding(TrafficRowInsets), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Text(stringResource(R.string.traffic_ca_fingerprint), style = MiuixTheme.textStyles.subtitle)
                        Text(state.fingerprint ?: stringResource(R.string.traffic_ca_not_ready),
                            style = MiuixTheme.textStyles.footnote1.copy(fontFamily = FontFamily.Monospace))
                    }
                },
                CardItem("trust") {
                    SwitchPreference(title = stringResource(R.string.traffic_trust), checked = policy.systemTrust,
                        summary = stringResource(R.string.traffic_trust_hint), enabled = editable, insideMargin = TrafficRowInsets,
                        startAction = { CaptureRowIcon(Icons.Rounded.Security) },
                        onCheckedChange = { if (it) confirmTrust = true else viewModel.setSystemTrust(false) })
                },
                CardItem("removeUserCA") {
                    ArrowPreference(title = stringResource(R.string.traffic_user_ca_remove),
                        enabled = editable, onClick = { confirmRemoveUserCA = true }, insideMargin = TrafficRowInsets,
                        startAction = { CaptureRowIcon(Icons.Rounded.RemoveCircleOutline) })
                },
            ))
        }
    }
    }
    TrafficConfirmation(removeHost != null, R.string.traffic_host_remove, R.string.traffic_host_remove_prompt,
        { removeHost = null }, {
            viewModel.setHosts(policy?.hosts.orEmpty().filterNot { it == removeHost })
            removeHost = null
        })
    TrafficConfirmation(confirmTrust, R.string.traffic_trust, R.string.traffic_trust_warning,
        { confirmTrust = false }, { confirmTrust = false; viewModel.setSystemTrust(true) })
    TrafficConfirmation(confirmRemoveUserCA, R.string.traffic_user_ca_remove, R.string.traffic_user_ca_remove_prompt,
        { confirmRemoveUserCA = false }, { confirmRemoveUserCA = false; viewModel.removeUserCertificate() })
    TrafficConfirmation(confirmClearPinned, R.string.traffic_pinned_clear, R.string.traffic_pinned_clear_prompt,
        { confirmClearPinned = false }, { confirmClearPinned = false; viewModel.clearPinned() })
    TrafficConfirmation(confirmClearAlpn, R.string.traffic_alpn_clear, R.string.traffic_alpn_clear_prompt,
        { confirmClearAlpn = false }, { confirmClearAlpn = false; viewModel.clearAlpnBypasses() })
    removeBypass?.let { item ->
        val reason = item.reason?.let { passthroughLabel(it) } ?: stringResource(R.string.traffic_bypass_unknown)
        val time = if (item.alpn && item.time != null) stringResource(R.string.traffic_alpn_expires, item.time)
            else stringResource(R.string.traffic_bypass_created, item.time ?: stringResource(R.string.traffic_bypass_unknown))
        AppDialog(show = true, title = stringResource(R.string.traffic_bypass_remove),
            summary = "${item.host}\n${stringResource(R.string.traffic_bypass_app, item.packageName, item.user)}\n$reason\n$time",
            onDismissRequest = { removeBypass = null }) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                AppDialogButton(stringResource(R.string.common_cancel), { removeBypass = null }, Modifier.weight(1f))
                AppDialogButton(stringResource(R.string.common_delete), {
                    removeBypass = null
                    viewModel.removeBypass(item.host, item.app, item.alpn)
                }, Modifier.weight(1f), role = DialogActionRole.Destructive)
            }
        }
    }
}

private data class BypassItem(
    val host: String,
    val user: Int,
    val packageName: String,
    val reason: String?,
    val time: String?,
    val alpn: Boolean = false,
) {
    val app: String get() = "$user:$packageName"
}

@Composable
private fun BypassRow(item: BypassItem, enabled: Boolean, summary: String, select: () -> Unit) {
    val action by rememberUpdatedState(select)
    Box(Modifier.fillMaxWidth().pointerInput(item, enabled) {
        if (!enabled) return@pointerInput
        var offset = 0f
        detectHorizontalDragGestures(
            onDragStart = { offset = 0f },
            onDragEnd = { if (abs(offset) >= 52.dp.toPx()) action() },
            onHorizontalDrag = { _, distance -> offset += distance },
        )
    }) {
        ArrowPreference(title = item.host, summary = summary, enabled = enabled, onClick = select,
            insideMargin = TrafficRowInsets,
            startAction = { CaptureRowIcon(if (item.alpn) Icons.Rounded.SwapHoriz else Icons.Rounded.Security) })
    }
}

@Composable
private fun phaseLabel(phase: String) = when (phase) {
    "request" -> stringResource(R.string.traffic_phase_request)
    "response" -> stringResource(R.string.traffic_phase_response)
    else -> phase
}

@Composable
internal fun TrafficCopySheet(title: Int, options: List<String>, dismiss: () -> Unit, choose: (Int) -> Unit) {
    AppBottomSheet(show = true, title = stringResource(title), onDismissRequest = dismiss) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 8.dp)) {
            options.forEachIndexed { index, text ->
                ArrowPreference(title = text, onClick = { choose(index) }, insideMargin = TrafficRowInsets)
            }
        }
    }
}

@Composable
internal fun TrafficConfirmation(show: Boolean, title: Int, prompt: Int, dismiss: () -> Unit, confirm: () -> Unit) {
    AppDialog(show = show, title = stringResource(title), summary = stringResource(prompt), onDismissRequest = dismiss) {
        ConfirmationActions(dismiss, confirm)
    }
}

@Composable
private fun ConfirmationActions(dismiss: () -> Unit, confirm: () -> Unit) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        AppDialogButton(stringResource(R.string.common_cancel), dismiss, Modifier.weight(1f))
        AppDialogButton(stringResource(R.string.common_confirm), confirm, Modifier.weight(1f),
            role = DialogActionRole.Primary)
    }
}
