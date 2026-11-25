package com.fanjv.netproxy.feature.traffic.presentation

import android.content.Intent
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppDialog
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.AppDropdown
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.core.ui.component.dialogTextFieldColors
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import top.yukonga.miuix.kmp.basic.FloatingActionButton
import top.yukonga.miuix.kmp.basic.Icon
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
            groupedCardSection("rules", { stringResource(R.string.traffic_rules) }, rules.mapIndexed { index, rule ->
                CardItem(index.toString()) {
                    ArrowPreference(title = rule.pattern,
                        summary = "${phaseLabel(rule.phase)} · ${stringResource(R.string.traffic_rule_enabled)}",
                        enabled = editable, onClick = { edit(index) })
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
    TopBarMoreMenu(listOf(
        TopBarMenuAction(stringResource(R.string.common_delete), draft.index != null && !state.saving) { confirmDelete = true },
    ), stringResource(R.string.more_options)) { menu ->
        TrafficFrame(if (draft.index == null) R.string.traffic_rule_add else R.string.traffic_rule_edit,
            actions = { if (draft.index != null) menu() }) { padding ->
            Column(Modifier.fillMaxSize().padding(padding).imePadding().verticalScroll(rememberScrollState()).padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp)) {
                AppDropdown(title = stringResource(R.string.traffic_rule_phase),
                    items = listOf(phaseLabel("request"), phaseLabel("response")),
                    selectedIndex = if (rule.phase == "request") 0 else 1, enabled = !state.saving,
                    onSelectedIndexChange = { viewModel.updateRule(rule.copy(phase = if (it == 0) "request" else "response")) })
                GlassTextField(value = rule.pattern, onValueChange = { viewModel.updateRule(rule.copy(pattern = it)) },
                    modifier = Modifier.fillMaxWidth(), singleLine = true, enabled = !state.saving,
                    label = stringResource(R.string.traffic_rule_pattern), colors = dialogTextFieldColors(state.draftError != null))
                GlassTextField(value = rule.script, onValueChange = { viewModel.updateRule(rule.copy(script = it)) },
                    modifier = Modifier.fillMaxWidth(), minLines = 8, maxLines = 16, enabled = !state.saving,
                    label = stringResource(R.string.traffic_rule_script),
                    textStyle = MiuixTheme.textStyles.body2.copy(fontFamily = FontFamily.Monospace),
                    colors = dialogTextFieldColors(state.draftError != null))
                state.draftError?.let { TrafficErrorText(it) }
                AppDialogButton(stringResource(R.string.save_text), { viewModel.saveRule() }, Modifier.fillMaxWidth(),
                    enabled = !state.saving && rule.pattern.isNotBlank() && rule.script.isNotBlank(), role = DialogActionRole.Primary)
            }
        }
    }
    TrafficConfirmation(confirmDelete, R.string.common_delete, R.string.traffic_rule_delete,
        { confirmDelete = false }, { confirmDelete = false; viewModel.saveRule(delete = true) })
}

@Composable
internal fun TrafficHttpsScreen(viewModel: TrafficViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val policy = state.policy.value
    val editable = !state.busy && policy != null && state.policy.error == null
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var confirmTrust by rememberSaveable { mutableStateOf(false) }
    var addHost by rememberSaveable { mutableStateOf(false) }
    var newHost by rememberSaveable { mutableStateOf("") }
    var removeHost by rememberSaveable { mutableStateOf<String?>(null) }
    RefreshOnResume(viewModel)
    LaunchedEffect(Unit) { viewModel.readCertificateStatus() }
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
    TrafficFrame(R.string.traffic_https) { padding ->
        if (policy == null) ContentStatus(padding,
            state.policy.error?.let { trafficError(it, R.string.traffic_https_unavailable) } ?: stringResource(R.string.traffic_https_unavailable),
            loading = state.loading)
        else LazyColumn(Modifier.fillMaxSize().padding(padding).padding(horizontal = 16.dp),
            contentPadding = PaddingValues(bottom = 24.dp)) {
            state.failure?.let { issue -> item { TrafficErrorText(issue) } }
            groupedCardSection("https", { stringResource(R.string.traffic_https) }, listOf(CardItem("switch") {
                SwitchPreference(title = stringResource(R.string.traffic_mitm), summary = stringResource(R.string.traffic_mitm_hint),
                    checked = policy.enabled, enabled = editable, onCheckedChange = viewModel::setMitmEnabled)
            }))
            groupedCardSection("hosts", { stringResource(R.string.traffic_hosts) }, buildList {
                add(CardItem("add") {
                    ArrowPreference(title = stringResource(R.string.traffic_host_add), summary = stringResource(R.string.traffic_hosts_hint),
                        enabled = editable, onClick = { newHost = ""; addHost = true })
                })
                policy.hosts.forEach { host -> add(CardItem("host:$host") {
                    ArrowPreference(title = host, summary = stringResource(R.string.traffic_host_remove),
                        enabled = editable, onClick = { removeHost = host })
                }) }
            })
            groupedCardSection("ca", { stringResource(R.string.traffic_ca_section) }, listOf(
                CardItem("fingerprint") {
                    Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(stringResource(R.string.traffic_ca_fingerprint), style = MiuixTheme.textStyles.subtitle)
                        Text(state.fingerprint ?: stringResource(R.string.traffic_ca_not_ready),
                            style = MiuixTheme.textStyles.footnote1.copy(fontFamily = FontFamily.Monospace))
                    }
                },
                CardItem("export") {
                    ArrowPreference(title = stringResource(R.string.traffic_ca_export), enabled = editable,
                        onClick = { viewModel.prepareCertificate { picker.launch(it) } })
                },
                CardItem("share") {
                    ArrowPreference(title = shareTitle, enabled = editable, onClick = { share() })
                },
                CardItem("trust") {
                    SwitchPreference(title = stringResource(R.string.traffic_trust), checked = policy.systemTrust,
                        summary = stringResource(R.string.traffic_trust_hint), enabled = editable,
                        onCheckedChange = { if (it) confirmTrust = true else viewModel.setSystemTrust(false) })
                },
            ))
        }
    }
    AppDialog(show = addHost, title = stringResource(R.string.traffic_host_add), onDismissRequest = { addHost = false }) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            GlassTextField(value = newHost, onValueChange = { newHost = it }, singleLine = true,
                label = stringResource(R.string.traffic_host), modifier = Modifier.fillMaxWidth())
            ConfirmationActions({ addHost = false }, {
                addHost = false
                viewModel.setHosts(policy?.hosts.orEmpty() + newHost.trim())
            }, enabled = newHost.isNotBlank() && editable)
        }
    }
    TrafficConfirmation(removeHost != null, R.string.traffic_host_remove, R.string.traffic_host_remove_prompt,
        { removeHost = null }, {
            viewModel.setHosts(policy?.hosts.orEmpty().filterNot { it == removeHost })
            removeHost = null
        })
    TrafficConfirmation(confirmTrust, R.string.traffic_trust, R.string.traffic_trust_warning,
        { confirmTrust = false }, { confirmTrust = false; viewModel.setSystemTrust(true) })
}

@Composable
private fun phaseLabel(phase: String) = when (phase) {
    "request" -> stringResource(R.string.traffic_phase_request)
    "response" -> stringResource(R.string.traffic_phase_response)
    else -> phase
}

@Composable
internal fun TrafficOptionDialog(title: Int, options: List<String>, dismiss: () -> Unit, choose: (Int) -> Unit) {
    AppDialog(show = true, title = stringResource(title), onDismissRequest = dismiss) {
        Column(Modifier.heightIn(max = 350.dp).verticalScroll(rememberScrollState())) {
            options.forEachIndexed { index, text -> ArrowPreference(title = text, onClick = { choose(index) }) }
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
private fun ConfirmationActions(dismiss: () -> Unit, confirm: () -> Unit, enabled: Boolean = true) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        AppDialogButton(stringResource(R.string.common_cancel), dismiss, Modifier.weight(1f))
        AppDialogButton(stringResource(R.string.common_confirm), confirm, Modifier.weight(1f), enabled = enabled,
            role = DialogActionRole.Primary)
    }
}
