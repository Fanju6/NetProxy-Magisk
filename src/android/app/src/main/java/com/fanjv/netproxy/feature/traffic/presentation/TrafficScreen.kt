package com.fanjv.netproxy.feature.traffic.presentation

import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.AppBarButton
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.appBarOverscroll
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.feature.traffic.data.CaptureRecord
import com.fanjv.netproxy.feature.traffic.data.TrafficChangedException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import java.io.IOException

@Composable
internal fun TrafficScreen(onBack: () -> Unit, viewModel: TrafficViewModel = netProxyViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val backdrop = rememberBlurBackdrop()
    val scroll = MiuixScrollBehavior()
    var detail by remember { mutableStateOf<CaptureRecord?>(null) }
    var confirmClear by remember { mutableStateOf(false) }
    var confirmTrust by remember { mutableStateOf(false) }
    val certificatePicker = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("application/x-pem-file")) { uri ->
        val cert = viewModel.takeCertificate()
        if (uri != null && cert != null) scope.launch {
            val saved = withContext(Dispatchers.IO) {
                try {
                    context.contentResolver.openOutputStream(uri, "wt")?.use {
                        it.write(cert.pem.toByteArray(Charsets.US_ASCII))
                    } != null
                } catch (_: IOException) {
                    false
                } catch (_: SecurityException) {
                    false
                }
            }
            Toast.makeText(context, if (saved) R.string.traffic_ca_saved else R.string.traffic_ca_failed, Toast.LENGTH_LONG).show()
        }
    }

    LifecycleResumeEffect(Unit) {
        viewModel.refresh()
        onPauseOrDispose {}
    }
    Scaffold(topBar = {
        BlurredBar(backdrop) {
            AdaptiveTopAppBar(
                title = stringResource(R.string.traffic_title),
                color = if (backdrop != null) Color.Transparent else MiuixTheme.colorScheme.surface,
                scrollBehavior = scroll,
                navigationIcon = { BackIconButton(onClick = onBack) },
                actions = {
                    AppBarButton(onClick = viewModel::refresh, enabled = !state.busy) {
                        Icon(Icons.Rounded.Refresh, stringResource(R.string.traffic_refresh))
                    }
                },
            )
        }
    }) { padding ->
        Box(Modifier.fillMaxSize().then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)) {
            val status = state.status.value
            if (status == null) {
                ContentStatus(padding, state.status.error?.let { trafficError(it, R.string.traffic_read_failed) }
                    ?: stringResource(R.string.traffic_read_failed), loading = state.loading)
                return@Box
            }
            LazyColumn(
                modifier = Modifier.fillMaxSize().appBarOverscroll(scroll).nestedScroll(scroll.nestedScrollConnection)
                    .padding(horizontal = 12.dp),
                contentPadding = padding,
                overscrollEffect = null,
            ) {
                groupedCardSection("capture", { stringResource(R.string.traffic_capture) }, buildList {
                    add(CardItem("switch") {
                        SwitchPreference(title = stringResource(R.string.traffic_capture), checked = status.running,
                            enabled = !state.busy && state.status.error == null,
                            summary = stringResource(if (status.running) R.string.traffic_on else R.string.traffic_off) +
                                "\n" + stringResource(R.string.traffic_record_count, status.count),
                            onCheckedChange = viewModel::setCapture)
                    })
                    add(CardItem("clear") {
                        ArrowPreference(title = stringResource(R.string.traffic_clear), enabled = !state.busy && status.count > 0,
                            onClick = { confirmClear = true })
                    })
                })
                state.status.error?.let { issue -> item("statusError") { TrafficErrorText(issue, R.string.traffic_read_failed) } }
                state.failure?.let { issue -> item("mutationError") { TrafficErrorText(issue) } }

                groupedCardSection("rules", { stringResource(R.string.traffic_rules) }, buildList {
                    add(CardItem("new") {
                        ArrowPreference(title = stringResource(R.string.traffic_rule_add),
                            enabled = !state.busy && state.rules.value != null && state.rules.error == null,
                            onClick = { viewModel.editRule() })
                    })
                    state.rules.value.orEmpty().forEachIndexed { index, rule ->
                        add(CardItem("rule:$index") {
                            ArrowPreference(title = rule.pattern, summary = phaseLabel(rule.phase),
                                enabled = !state.busy && state.rules.error == null, onClick = { viewModel.editRule(index) })
                        })
                    }
                })
                state.rules.error?.let { issue -> item("rulesError") { TrafficErrorText(issue, R.string.traffic_read_failed) } }

                groupedCardSection("https", { stringResource(R.string.traffic_https) }, buildList {
                    state.policy.value?.let { policy ->
                        val editable = !state.busy && state.policy.error == null
                        add(CardItem("decrypt") {
                            SwitchPreference(title = stringResource(R.string.traffic_mitm), checked = policy.enabled,
                                enabled = editable, summary = stringResource(R.string.traffic_mitm_hint),
                                onCheckedChange = viewModel::setMitmEnabled)
                        })
                        add(CardItem("hosts") {
                            ArrowPreference(title = stringResource(R.string.traffic_hosts),
                                summary = stringResource(R.string.traffic_hosts_count, policy.hosts.size),
                                enabled = editable, onClick = viewModel::editHosts)
                        })
                        add(CardItem("certificate") {
                            ArrowPreference(title = stringResource(R.string.traffic_ca_export),
                                summary = stringResource(R.string.traffic_ca_hint), enabled = editable,
                                onClick = { viewModel.prepareCertificate { certificatePicker.launch(it) } })
                        })
                        add(CardItem("trust") {
                            SwitchPreference(title = stringResource(R.string.traffic_trust), checked = policy.systemTrust,
                                summary = stringResource(R.string.traffic_trust_hint), enabled = editable,
                                onCheckedChange = { checked -> if (checked) confirmTrust = true else viewModel.setSystemTrust(false) })
                        })
                    }
                })
                state.policy.error?.let { issue -> item("policyError") { TrafficErrorText(issue, R.string.traffic_https_unavailable) } }

                val records = state.records.value
                groupedCardSection("records", { stringResource(R.string.traffic_records) }, records.orEmpty().mapIndexed { index, record ->
                    CardItem("record:$index:${record.time}") {
                        ArrowPreference(
                            title = "${record.method} ${record.host}",
                            summary = stringResource(R.string.traffic_record_summary, record.status.takeIf { it > 0 }?.toString() ?: "—", record.outbound, record.time),
                            onClick = { detail = record },
                        )
                    }
                }, outerBottomPadding = 16.dp)
                val recordsError = state.records.error
                if (recordsError != null) item("recordsError") { TrafficErrorText(recordsError, R.string.traffic_read_failed) }
                else if (records?.isEmpty() == true) item("empty") {
                    Text(stringResource(R.string.traffic_empty), Modifier.padding(PaddingValues(16.dp)),
                        color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                }
            }
        }
    }

    detail?.let { RecordDetails(it) { detail = null } }
    state.ruleDraft?.let { RewriteRuleDialog(it, state.saving, state.draftError, viewModel) }
    state.hostsDraft?.let { MitmHostsDialog(it, state.saving, state.draftError, viewModel) }
    TrafficConfirmation(confirmClear, R.string.traffic_clear, R.string.traffic_clear_prompt,
        dismiss = { confirmClear = false }, confirm = { confirmClear = false; viewModel.clear() })
    TrafficConfirmation(confirmTrust, R.string.traffic_trust, R.string.traffic_trust_warning,
        dismiss = { confirmTrust = false }, confirm = { confirmTrust = false; viewModel.setSystemTrust(true) })
}

@Composable
internal fun TrafficErrorText(error: Throwable, fallback: Int = R.string.traffic_operation_failed) {
    Text(trafficError(error, fallback), Modifier.padding(horizontal = 14.dp, vertical = 8.dp),
        color = MiuixTheme.colorScheme.error)
}

@Composable
private fun trafficError(error: Throwable, fallback: Int): String = when (error) {
    is TrafficChangedException -> stringResource(R.string.traffic_changed)
    is ModuleApiException -> when (error.resultCode) {
        "traffic.ca_invalid" -> stringResource(R.string.traffic_ca_failed)
        "traffic.trust_remove_incomplete" -> stringResource(R.string.traffic_trust_remove_incomplete)
        else -> stringResource(R.string.traffic_api_error, stringResource(fallback), error.resultCode, error.message)
    }
    else -> stringResource(fallback)
}
