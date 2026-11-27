package com.fanjv.netproxy.feature.routing.presentation

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
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
import com.fanjv.netproxy.feature.routing.model.simpleRule
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
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
    val reviewingPlacement = draft?.target == LocalRuleSet.Policy && draft.index == null && file?.requiresReload == true
    val placementDocument = file?.document?.takeIf { draft != null && state.needsInsertion(draft) }
    val listState = rememberLazyListState()
    var pickingApps by rememberSaveable { mutableStateOf(false) }
    var pickingPosition by rememberSaveable { mutableStateOf(false) }
    val reload = {
        when {
            reviewingPlacement -> viewModel.reloadPolicyDraft()
            requested != null -> viewModel.closeEditor()
            else -> viewModel.openEditor(null, null, discardDraft = true)
        }
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
                    item("error") {
                        RoutingError(file, state, reload, backToList = requested != null,
                            reloadTitle = R.string.routing_reload_position.takeIf { reviewingPlacement })
                    }
                }

                if (field != null) {
                    val enabled = state.editable(draft.target) && state.deleting == null
                    groupedCardSection("match", { stringResource(R.string.routing_section_match) }, buildList {
                        add(CardItem("kind") {
                            AppDropdown(
                                title = stringResource(R.string.routing_match),
                                items = RuleField.entries.map { stringResource(it.title()) },
                                selectedIndex = field.ordinal,
                                enabled = !state.isSaving,
                                onSelectedIndexChange = { viewModel.updateDraft(RuleField.entries[it], draft.text) },
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
                        if (placementDocument != null) add(CardItem("position") {
                            val position = draft.insertion
                            val anchor = position?.let(placementDocument.rules::getOrNull)
                            ArrowPreference(
                                title = stringResource(R.string.routing_position),
                                summary = when {
                                    position == null -> stringResource(R.string.routing_position_choose)
                                    anchor == null -> stringResource(R.string.routing_position_last)
                                    else -> "${stringResource(R.string.routing_position_before, position + 1)} · ${anchor.positionSummary()}"
                                },
                                enabled = enabled,
                                onClick = { sheetFocus.clearFocus(); pickingPosition = true },
                            )
                            position?.let(placementDocument::earlierCatchAll)?.let { index ->
                                Text(stringResource(R.string.routing_position_shadowed, index + 1),
                                    Modifier.padding(start = 16.dp, end = 16.dp, bottom = 12.dp),
                                    style = MiuixTheme.textStyles.footnote2,
                                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
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

    if (placementDocument != null && file != null) AppDialog(
        show = pickingPosition,
        title = stringResource(R.string.routing_position),
        onDismissRequest = { pickingPosition = false },
    ) {
        val select: (Int) -> Unit = {
            viewModel.selectInsertion(it, file.revision)
            pickingPosition = false
        }
        LazyColumn(Modifier.fillMaxWidth().heightIn(max = 460.dp), overscrollEffect = null) {
            itemsIndexed(placementDocument.rules) { index, rule ->
                BasicComponent(
                    title = stringResource(R.string.routing_position_before, index + 1),
                    summary = rule.positionSummary(),
                    onClick = { select(index) },
                )
            }
            item {
                BasicComponent(
                    title = stringResource(R.string.routing_position_last),
                    onClick = { select(placementDocument.rules.size) },
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

@Composable
private fun JsonElement.positionSummary(): String {
    val rule = this as? JsonObject ?: return stringResource(R.string.routing_advanced)
    fun JsonElement?.labels(): String =
        (if (this is JsonArray) this else listOfNotNull(this))
            .mapNotNull { (it as? JsonPrimitive)?.contentOrNull }.take(2).joinToString(", ").take(46)
    val label = when {
        rule["ip_is_private"] == JsonPrimitive(true) -> stringResource(R.string.routing_private_ip)
        else -> JsonObject(rule - setOf("action", "outbound")).simpleRule()?.values?.first()?.take(46)
            ?: rule["clash_mode"].labels().ifBlank { rule["rule_set"].labels() }
    }
    val action = rule["outbound"].labels().ifBlank { rule["action"].labels() }
    return when {
        label.isNotEmpty() && action.isNotEmpty() -> "$label → $action"
        label.isNotEmpty() -> label
        action.isNotEmpty() -> "${stringResource(R.string.routing_advanced)} · $action"
        else -> stringResource(R.string.routing_advanced)
    }
}
