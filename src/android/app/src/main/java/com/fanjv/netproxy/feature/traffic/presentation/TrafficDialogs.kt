package com.fanjv.netproxy.feature.traffic.presentation

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppDialog
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.AppDropdown
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.core.ui.component.dialogTextFieldColors
import com.fanjv.netproxy.feature.traffic.data.CaptureRecord
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.glass.GlassTextField
import top.yukonga.miuix.kmp.theme.MiuixTheme

@Composable
internal fun RecordDetails(record: CaptureRecord, dismiss: () -> Unit) {
    AppDialog(show = true, title = stringResource(R.string.traffic_record_detail), onDismissRequest = dismiss) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Column(Modifier.heightIn(max = 340.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp)) {
                RecordField(R.string.traffic_method, record.method)
                RecordField(R.string.traffic_host, record.host)
                RecordField(R.string.traffic_outbound, record.outbound)
                RecordField(R.string.traffic_http_status, record.status.takeIf { it > 0 }?.toString())
                RecordField(R.string.traffic_time, record.time)
                RecordField(R.string.traffic_duration, stringResource(R.string.traffic_milliseconds, record.durationMs))
                RecordField(R.string.traffic_tls, when (record.tls) {
                    "decrypted" -> stringResource(R.string.traffic_tls_decrypted)
                    "bypass" -> stringResource(R.string.traffic_tls_bypass)
                    "handshake_failed" -> stringResource(R.string.traffic_tls_failed)
                    else -> record.tls
                })
                RecordField(R.string.traffic_rewrite_result, record.rewrite)
                Text(stringResource(R.string.traffic_metadata_only), color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
            }
            AppDialogButton(stringResource(R.string.common_confirm), dismiss, Modifier.fillMaxWidth())
        }
    }
}

@Composable
private fun RecordField(@StringRes label: Int, value: String?) {
    Text(stringResource(R.string.traffic_detail_field, stringResource(label), value?.takeIf(String::isNotBlank) ?: "—"))
}

@Composable
internal fun RewriteRuleDialog(draft: RuleDraft, saving: Boolean, error: Throwable?, viewModel: TrafficViewModel) {
    var confirmDelete by rememberSaveable(draft.index) { mutableStateOf(false) }
    val rule = draft.rule
    AppDialog(show = !confirmDelete,
        title = stringResource(if (draft.index == null) R.string.traffic_rule_add else R.string.traffic_rule_edit),
        onDismissRequest = viewModel::dismissDraft) {
        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Column(Modifier.heightIn(max = 350.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp)) {
                AppDropdown(title = stringResource(R.string.traffic_rule_phase),
                    items = listOf(phaseLabel("request"), phaseLabel("response")),
                    selectedIndex = if (rule.phase == "request") 0 else 1,
                    enabled = !saving,
                    onSelectedIndexChange = { viewModel.updateRule(rule.copy(phase = if (it == 0) "request" else "response")) })
                GlassTextField(value = rule.pattern, onValueChange = { viewModel.updateRule(rule.copy(pattern = it)) },
                    colors = dialogTextFieldColors(error != null), label = stringResource(R.string.traffic_rule_pattern),
                    modifier = Modifier.fillMaxWidth(), maxLines = 1, enabled = !saving)
                GlassTextField(value = rule.script, onValueChange = { viewModel.updateRule(rule.copy(script = it)) },
                    colors = dialogTextFieldColors(error != null), label = stringResource(R.string.traffic_rule_script),
                    modifier = Modifier.fillMaxWidth(), minLines = 3, maxLines = 7, enabled = !saving)
                error?.let { TrafficErrorText(it) }
            }
            DialogButtons(saving = saving, dismiss = viewModel::dismissDraft, confirm = { viewModel.saveRule() },
                canSave = rule.pattern.isNotBlank() && rule.script.isNotBlank())
            if (draft.index != null) AppDialogButton(stringResource(R.string.common_delete), { confirmDelete = true },
                Modifier.fillMaxWidth(), enabled = !saving, role = DialogActionRole.Destructive)
        }
    }
    TrafficConfirmation(confirmDelete, R.string.common_delete, R.string.traffic_rule_delete,
        dismiss = { confirmDelete = false }, confirm = { confirmDelete = false; viewModel.saveRule(delete = true) })
}

@Composable
internal fun phaseLabel(phase: String): String = when (phase) {
    "request" -> stringResource(R.string.traffic_phase_request)
    "response" -> stringResource(R.string.traffic_phase_response)
    else -> phase
}

@Composable
internal fun MitmHostsDialog(draft: HostsDraft, saving: Boolean, error: Throwable?, viewModel: TrafficViewModel) {
    AppDialog(show = true, title = stringResource(R.string.traffic_hosts),
        summary = stringResource(R.string.traffic_hosts_hint), onDismissRequest = viewModel::dismissDraft) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            GlassTextField(value = draft.text, onValueChange = viewModel::updateHosts,
                colors = dialogTextFieldColors(error != null), label = stringResource(R.string.traffic_hosts),
                modifier = Modifier.fillMaxWidth(), minLines = 3, maxLines = 7, enabled = !saving)
            error?.let { TrafficErrorText(it) }
            DialogButtons(saving = saving, dismiss = viewModel::dismissDraft, confirm = viewModel::saveHosts)
        }
    }
}

@Composable
internal fun TrafficConfirmation(show: Boolean, @StringRes title: Int, @StringRes prompt: Int,
                                 dismiss: () -> Unit, confirm: () -> Unit) {
    AppDialog(show = show, title = stringResource(title), summary = stringResource(prompt), onDismissRequest = dismiss) {
        DialogButtons(dismiss = dismiss, confirm = confirm, label = R.string.common_confirm, role = DialogActionRole.Destructive)
    }
}

@Composable
private fun DialogButtons(
    dismiss: () -> Unit,
    confirm: () -> Unit,
    saving: Boolean = false,
    canSave: Boolean = true,
    @StringRes label: Int = R.string.save_text,
    role: DialogActionRole = DialogActionRole.Primary,
) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        AppDialogButton(stringResource(R.string.common_cancel), dismiss, Modifier.weight(1f), enabled = !saving)
        AppDialogButton(stringResource(label), confirm, Modifier.weight(1f), enabled = !saving && canSave, role = role)
    }
}
