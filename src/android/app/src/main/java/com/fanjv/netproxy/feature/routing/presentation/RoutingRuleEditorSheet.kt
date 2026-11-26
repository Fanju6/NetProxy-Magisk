package com.fanjv.netproxy.feature.routing.presentation

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.Alignment
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppBarButton
import com.fanjv.netproxy.core.ui.component.AppBottomSheet
import com.fanjv.netproxy.core.ui.component.AppDialog
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.AppDropdown
import com.fanjv.netproxy.core.ui.component.AppSheetMoreMenu
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.core.ui.component.chromeIcon
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.core.ui.component.dialogTextFieldColors
import com.fanjv.netproxy.feature.routing.model.LocalRuleSet
import com.fanjv.netproxy.feature.routing.model.RuleField
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.DropdownItem
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.glass.GlassTextField
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Close
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme

private val matchFields = listOf(RuleField.PackageName) + RuleField.entries.filterNot { it == RuleField.PackageName }

@Composable
internal fun RoutingRuleEditorSheet(
    state: RoutingRulesState,
    viewModel: RoutingRulesViewModel,
    onEditJson: (LocalRuleSet) -> Unit,
) {
    val editor = state.editor ?: return
    val requested = editor.identity
    val draft = state.draft
    val identity = draft?.index?.let { RuleIdentity(draft.target, it) }
    val field = draft?.field
    val file = (draft?.target ?: requested?.target)?.let(state.files::getValue)
    val listState = rememberLazyListState()
    var pickingApps by rememberSaveable { mutableStateOf(false) }
    val reload = {
        if (requested != null) viewModel.closeEditor() else viewModel.openEditor(null, null, discardDraft = true)
    }
    val rawEdit: (LocalRuleSet) -> Unit = { target ->
        viewModel.dismiss()
        onEditJson(target)
    }
    LaunchedEffect(state.inputError, file?.error) {
        if (state.inputError != null || file?.error?.isNotEmpty() == true) listState.scrollToItem(0)
    }

    AppBottomSheet(
        show = editor.visible,
        title = stringResource(if (requested == null) R.string.routing_add else R.string.routing_edit),
        onDismissRequest = viewModel::closeEditor,
        allowDismiss = !state.isSaving,
        onDismissFinished = viewModel::dismiss,
        startAction = {
            AppBarButton(onClick = viewModel::closeEditor, enabled = !state.isSaving) {
                Icon(chromeIcon(MiuixIcons.Close), stringResource(R.string.common_cancel))
            }
        },
        endAction = identity?.let { rule ->
            {
                AppSheetMoreMenu(
                    DropdownEntry(listOf(DropdownItem(
                        text = stringResource(R.string.routing_delete_action),
                        enabled = editor.visible && state.editable(rule.target),
                        onClick = { viewModel.requestDelete(rule) },
                    ))),
                    stringResource(R.string.more_actions),
                )
            }
        },
    ) {
        val sheetFocus = LocalFocusManager.current
        Column(Modifier.fillMaxWidth()) {
            LazyColumn(
                modifier = Modifier.weight(1f, fill = false).fillMaxWidth(),
                state = listState,
                contentPadding = PaddingValues(bottom = 12.dp),
            ) {
                if (draft == null) {
                    item("unavailable") {
                        if (state.isLoading) {
                            Box(Modifier.fillMaxWidth().padding(vertical = 24.dp), contentAlignment = Alignment.Center) {
                                InfiniteProgressIndicator()
                            }
                        } else if (file?.requiresReload == true) {
                            RoutingError(file, state, reload, backToList = requested != null)
                            if (file.invalidDocument && requested != null) ArrowPreference(
                                title = stringResource(R.string.routing_raw), onClick = { rawEdit(requested.target) },
                            )
                        } else {
                            ArrowPreference(
                                title = stringResource(if (requested != null) R.string.routing_back_to_list else R.string.inbound_reload),
                                summary = stringResource(if (requested != null) R.string.routing_rule_changed else R.string.routing_read_failed),
                                onClick = reload,
                            )
                        }
                    }
                    return@LazyColumn
                }
                if (file != null && (file.error.isNotEmpty() || file.requiresReload)) {
                    item("error") { RoutingError(file, state, reload, backToList = requested != null) }
                }

                if (field != null) {
                    val enabled = state.editable(draft.target) && state.deleting == null
                    groupedCardSection("match", { stringResource(R.string.routing_section_match) }, buildList {
                        add(CardItem("kind") {
                            AppDropdown(
                                title = stringResource(R.string.routing_match),
                                items = matchFields.map { stringResource(it.title()) },
                                selectedIndex = matchFields.indexOf(field),
                                enabled = !state.isSaving,
                                onSelectedIndexChange = { viewModel.updateDraft(matchFields[it], draft.text) },
                            )
                        })
                        if (field == RuleField.PackageName) add(CardItem("choose_apps") {
                            val count = remember(draft.text) { field.values(draft.text).size }
                            ArrowPreference(
                                title = stringResource(R.string.routing_pick_apps),
                                summary = if (count > 1) stringResource(R.string.routing_values_count, count) else null,
                                enabled = !state.isSaving,
                                onClick = { sheetFocus.clearFocus(); pickingApps = true },
                            )
                        })
                        add(CardItem("values") {
                            GlassTextField(
                                value = draft.text,
                                onValueChange = { viewModel.updateDraft(field, it) },
                                modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp),
                                label = stringResource(if (field == RuleField.PackageName) R.string.routing_package_name else R.string.routing_values),
                                minLines = 2,
                                maxLines = 5,
                                enabled = !state.isSaving,
                                colors = dialogTextFieldColors(isError = state.inputError != null),
                                keyboardOptions = KeyboardOptions(autoCorrectEnabled = false,
                                    keyboardType = if (field == RuleField.DomainKeyword) KeyboardType.Text else KeyboardType.Ascii),
                            )
                            val error = state.inputError
                            Text(
                                stringResource(error?.message() ?: field.hint()), Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
                                color = if (error != null) MiuixTheme.colorScheme.error else MiuixTheme.colorScheme.onSurfaceVariantSummary,
                                style = MiuixTheme.textStyles.footnote2,
                            )
                        })
                    }, titleTopPadding = 8.dp)
                    groupedCardSection("action", { stringResource(R.string.routing_section_action) }, buildList {
                        add(CardItem("target") {
                            if (identity == null) AppDropdown(
                                title = stringResource(R.string.routing_type),
                                items = LocalRuleSet.entries.map { stringResource(it.title()) },
                                selectedIndex = draft.target.ordinal,
                                enabled = !state.isSaving,
                                onSelectedIndexChange = { viewModel.selectTarget(LocalRuleSet.entries[it]) },
                            ) else BasicComponent(title = stringResource(R.string.routing_type), summary = stringResource(draft.target.title()))
                        })
                        if (draft.target == LocalRuleSet.Policy) add(CardItem("group") {
                            when {
                                file?.document?.policyStart == null -> ArrowPreference(
                                    title = stringResource(R.string.routing_raw),
                                    summary = stringResource(R.string.routing_policy_order_unsupported),
                                    enabled = !state.isLoading && !state.isSaving,
                                    onClick = { rawEdit(draft.target) },
                                )
                                state.policyTargets.isEmpty() -> BasicComponent(
                                    title = stringResource(R.string.routing_policy_group),
                                    summary = stringResource(R.string.routing_policy_no_groups),
                                )
                                else -> {
                                    val selected = state.policyTargets.indexOf(draft.policyTag)
                                    AppDropdown(
                                        title = stringResource(R.string.routing_policy_group),
                                        items = listOf(draft.policyTag?.takeIf { selected < 0 }
                                            ?: stringResource(R.string.routing_policy_choose)) + state.policyTargets,
                                        selectedIndex = selected + 1,
                                        enabled = enabled,
                                        onSelectedIndexChange = { state.policyTargets.getOrNull(it - 1)?.let(viewModel::selectPolicy) },
                                    )
                                }
                            }
                        })
                    })
                } else {
                    groupedCardSection("advanced", { stringResource(R.string.routing_section_advanced) }, listOf(CardItem("raw") {
                        ArrowPreference(
                            title = stringResource(R.string.routing_raw),
                            summary = stringResource(R.string.routing_advanced_hint),
                            enabled = !state.isSaving,
                            onClick = { rawEdit(draft.target) },
                        )
                    }), titleTopPadding = 8.dp)
                }
            }
            if (draft != null && field != null) {
                AppDialogButton(
                    text = stringResource(if (state.isSaving) R.string.routing_saving else R.string.save_text),
                    onClick = { sheetFocus.clearFocus(); viewModel.save() },
                    modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp),
                    enabled = editor.visible && state.deleting == null && state.canSave(draft),
                    role = DialogActionRole.Primary,
                )
            }
        }
    }

    if (field == RuleField.PackageName && draft != null) AppDialog(
        show = pickingApps,
        title = stringResource(R.string.routing_pick_apps),
        onDismissRequest = { pickingApps = false },
    ) {
        RoutingAppPicker(draft.text, viewModel, onCancel = { pickingApps = false }, onDone = {
            viewModel.updateDraft(RuleField.PackageName, it)
            pickingApps = false
        })
    }
    val deleting = state.deleting
    AppDialog(
        show = deleting != null,
        title = stringResource(R.string.routing_delete),
        summary = stringResource(R.string.routing_delete_hint),
        onDismissRequest = viewModel::cancelDelete,
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            val deletionFile = deleting?.target?.let(state.files::getValue)
            if (deletionFile != null && (deletionFile.error.isNotEmpty() || deletionFile.requiresReload)) {
                RoutingError(deletionFile, state, reload, backToList = true)
            }
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                AppDialogButton(stringResource(R.string.common_cancel), viewModel::cancelDelete,
                    Modifier.weight(1f), enabled = !state.isSaving)
                AppDialogButton(stringResource(R.string.common_delete), viewModel::delete, Modifier.weight(1f),
                    enabled = deleting?.target?.let(state::editable) == true, role = DialogActionRole.Destructive)
            }
        }
    }
}
