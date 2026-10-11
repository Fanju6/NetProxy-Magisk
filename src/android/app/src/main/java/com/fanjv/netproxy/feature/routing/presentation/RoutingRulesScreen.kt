package com.fanjv.netproxy.feature.routing.presentation

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.input.TextFieldLineLimits
import androidx.compose.foundation.text.input.rememberTextFieldState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.feature.routing.model.LocalRuleSet
import com.fanjv.netproxy.feature.routing.model.RuleField
import com.fanjv.netproxy.feature.routing.model.RuleInputError
import com.fanjv.netproxy.feature.routing.model.simpleRule
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Add
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

@Composable
internal fun RoutingRulesScreen(
    onBack: () -> Unit,
    viewModel: RoutingRulesViewModel = netProxyViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val rules = remember(state.files) { state.rules }
    val navigator = LocalNavigator.current
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else MiuixTheme.colorScheme.surface
    val rawEdit: (LocalRuleSet) -> Unit = { target -> viewModel.dismiss(); navigator.push(Route.JsonEdit(target.target)) }
    LifecycleResumeEffect(Unit) {
        viewModel.refresh()
        onPauseOrDispose { }
    }
    BackHandler(state.isSaving) { }

    Scaffold(
        topBar = {
            BlurredBar(backdrop) {
                AdaptiveTopAppBar(
                    title = stringResource(R.string.routing_rules),
                    color = barColor,
                    navigationIcon = { BackIconButton { if (!state.isSaving) onBack() } },
                    scrollBehavior = scrollBehavior,
                    actions = {
                        IconButton(onClick = viewModel::add, enabled = state.canAdd) {
                            Icon(MiuixIcons.Add, stringResource(R.string.routing_add), tint = MiuixTheme.colorScheme.onSurface)
                        }
                    },
                )
            }
        },
    ) { padding ->
        val layoutDirection = LocalLayoutDirection.current
        if (!state.hasDocuments && (state.isLoading || state.files.values.none { it.requiresReload })) {
            ContentStatus(padding, stringResource(R.string.routing_read_failed), loading = true)
        } else if (!state.hasDocuments) {
            Box(
                Modifier.fillMaxSize()
                    .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)
                    .padding(padding).padding(horizontal = 26.dp),
                contentAlignment = Alignment.Center,
            ) {
                Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    ContentStatus(PaddingValues(0.dp), stringResource(R.string.routing_read_failed), modifier = Modifier.heightIn(min = 64.dp))
                    LocalRuleSet.entries.forEach { target ->
                        RuleFileError(target, state, reload = { viewModel.refresh(discardDraft = true) }, rawEdit = rawEdit)
                    }
                }
            }
        } else if (state.isEmpty) {
            Column(
                Modifier.fillMaxSize()
                    .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)
                    .padding(padding).padding(horizontal = 26.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(8.dp, Alignment.CenterVertically),
            ) {
                Text(stringResource(R.string.routing_empty), style = MiuixTheme.textStyles.body1,
                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary, textAlign = TextAlign.Center)
                Text(stringResource(R.string.routing_empty_hint), style = MiuixTheme.textStyles.footnote1,
                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary, textAlign = TextAlign.Center)
            }
        } else LazyColumn(
            modifier = Modifier.fillMaxSize()
                .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)
                .scrollEndHaptic().overScrollVertical().nestedScroll(scrollBehavior.nestedScrollConnection),
            contentPadding = PaddingValues(
                top = padding.calculateTopPadding() + 6.dp,
                start = padding.calculateStartPadding(layoutDirection) + 12.dp,
                end = padding.calculateEndPadding(layoutDirection) + 12.dp,
                bottom = padding.calculateBottomPadding(),
            ),
            overscrollEffect = null,
        ) {
            LocalRuleSet.entries.forEach { target ->
                val file = state.files.getValue(target)
                if (file.error.isNotEmpty() || file.requiresReload) item("error:${target.tag}") {
                    RuleFileError(target, state, reload = { viewModel.refresh(discardDraft = true) }, rawEdit = rawEdit)
                }
            }
            if (rules.isNotEmpty()) groupedCardSection(
                "routing_rules",
                { stringResource(R.string.routing_matches) },
                rules.map { identity -> CardItem("${identity.target.tag}:${identity.index}") {
                    val rule = state.files.getValue(identity.target).document!!.rules[identity.index]
                    val simple = remember(rule) { rule.simpleRule() }
                    ArrowPreference(
                        title = simple?.values?.first()?.take(100) ?: stringResource(R.string.routing_advanced),
                        summary = stringResource(identity.target.title()) + " · " + (simple?.let {
                            stringResource(it.field.title()) + if (it.values.size > 1) " · " + stringResource(R.string.routing_values_count, it.values.size) else ""
                        } ?: stringResource(R.string.routing_advanced_hint)),
                        onClick = { viewModel.edit(identity) },
                        enabled = state.editable(identity.target),
                        holdDownState = state.draft?.let { it.target == identity.target && it.index == identity.index } == true,
                    )
                } },
                titleTopPadding = 8.dp,
            )
            item("bottom") { Spacer(Modifier.height(24.dp).navigationBarsPadding()) }
        }
    }

    var retainedDraft by remember { mutableStateOf<RuleDraft?>(null) }
    val draft = state.draft ?: retainedDraft
    SideEffect { if (state.draft != null) retainedDraft = state.draft }
    OverlayDialog(show = state.draft != null, title = stringResource(if (draft?.index == null) R.string.routing_add else R.string.routing_edit),
        onDismissRequest = viewModel::dismiss, onDismissFinished = { retainedDraft = null }) {
        if (draft != null) {
            val file = state.files.getValue(draft.target)
            val textState = rememberTextFieldState(draft.text)
            LaunchedEffect(textState, draft.field) {
                draft.field?.let { field ->
                    snapshotFlow { textState.text.toString() }.collect { viewModel.updateDraft(field, it) }
                }
            }
            Column(Modifier.heightIn(max = 500.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Column(Modifier.weight(1f, fill = false).verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    if (draft.index == null) OverlayDropdownPreference(title = stringResource(R.string.routing_type),
                        items = LocalRuleSet.entries.map { stringResource(it.title()) }, selectedIndex = draft.target.ordinal,
                        enabled = !state.isSaving,
                        insideMargin = PaddingValues(vertical = 12.dp),
                        onSelectedIndexChange = { viewModel.selectTarget(LocalRuleSet.entries[it]) })
                    else Text(stringResource(R.string.routing_type) + ": " + stringResource(draft.target.title()),
                        style = MiuixTheme.textStyles.body2)
                    val field = draft.field
                    if (field != null) {
                        OverlayDropdownPreference(title = stringResource(R.string.routing_match),
                            items = RuleField.entries.map { stringResource(it.title()) }, selectedIndex = field.ordinal,
                            enabled = !state.isSaving,
                            insideMargin = PaddingValues(vertical = 12.dp),
                            onSelectedIndexChange = { viewModel.updateDraft(RuleField.entries[it], textState.text.toString()) })
                        TextField(state = textState,
                            label = stringResource(R.string.routing_values), lineLimits = TextFieldLineLimits.MultiLine(3, 6),
                            keyboardOptions = KeyboardOptions(autoCorrectEnabled = false,
                                keyboardType = if (field == RuleField.DomainKeyword) KeyboardType.Text else KeyboardType.Ascii),
                            modifier = Modifier.fillMaxWidth(), enabled = !state.isSaving)
                        Text(stringResource(field.hint()), style = MiuixTheme.textStyles.footnote2,
                            color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                    } else {
                        Text(stringResource(R.string.routing_advanced_hint), style = MiuixTheme.textStyles.body2,
                            color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                    }
                    state.inputError?.let { Text(stringResource(it.message()), color = MiuixTheme.colorScheme.error, style = MiuixTheme.textStyles.body2) }
                    if (file.error.isNotEmpty() || file.requiresReload) RuleError(file, state, reload = { viewModel.refresh(discardDraft = true) })
                    if (file.invalidDocument) TextButton(stringResource(R.string.routing_raw), { rawEdit(draft.target) },
                        Modifier.fillMaxWidth(), enabled = !state.isLoading && !state.isSaving)
                }
                RuleButtons(state.isSaving, viewModel::dismiss,
                    confirm = { if (draft.field != null) {
                        viewModel.updateDraft(draft.field, textState.text.toString())
                        viewModel.save()
                    } else { rawEdit(draft.target) } },
                    enabled = state.editable(draft.target),
                    confirmLabel = stringResource(if (draft.field != null) R.string.save_text else R.string.routing_raw))
                draft.index?.let { index ->
                    TextButton(stringResource(R.string.common_delete), { viewModel.requestDelete(RuleIdentity(draft.target, index)) },
                        Modifier.fillMaxWidth(), enabled = state.editable(draft.target),
                        colors = ButtonDefaults.textButtonColors(textColor = MiuixTheme.colorScheme.error))
                }
            }
        }
    }
    OverlayDialog(show = state.deleting != null, title = stringResource(R.string.routing_delete),
        summary = listOfNotNull(state.deleting?.target?.let { stringResource(it.title()) }, stringResource(R.string.routing_delete_hint)).joinToString("\n"),
        onDismissRequest = viewModel::dismiss) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            val target = state.deleting?.target
            val file = target?.let { state.files.getValue(it) }
            if (file != null && (file.error.isNotEmpty() || file.requiresReload)) RuleError(file, state, reload = { viewModel.refresh(discardDraft = true) })
            RuleButtons(state.isSaving, viewModel::dismiss,
                confirm = viewModel::delete, enabled = target?.let(state::editable) == true, confirmLabel = stringResource(R.string.common_delete))
        }
    }
}

@Composable
private fun RuleFileError(target: LocalRuleSet, state: RoutingRulesState, reload: () -> Unit, rawEdit: (LocalRuleSet) -> Unit) {
    val file = state.files.getValue(target)
    Column(Modifier.padding(vertical = 8.dp)) {
        Text(stringResource(target.title()), style = MiuixTheme.textStyles.body1)
        RuleError(file, state, reload)
        if (file.invalidDocument) TextButton(stringResource(R.string.routing_raw), { rawEdit(target) },
            Modifier.fillMaxWidth(), enabled = !state.isLoading && !state.isSaving)
    }
}

@Composable
private fun RuleError(file: RuleFileState, state: RoutingRulesState, reload: () -> Unit) {
    Column(Modifier.padding(vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        val message = when {
            file.invalidDocument -> stringResource(R.string.routing_invalid_document)
            file.error.isNotEmpty() -> file.error
            else -> stringResource(R.string.routing_read_failed)
        }
        Text(listOf(message, file.error).filter(String::isNotBlank).distinct().joinToString("\n"),
            style = MiuixTheme.textStyles.body2, color = MiuixTheme.colorScheme.error)
        if (file.requiresReload) TextButton(stringResource(if (state.draft != null || state.deleting != null) R.string.routing_reload_draft else R.string.inbound_reload),
            reload, Modifier.fillMaxWidth(), enabled = !state.isLoading && !state.isSaving)
    }
}

@Composable
private fun RuleButtons(saving: Boolean, cancel: () -> Unit, confirm: () -> Unit, enabled: Boolean, confirmLabel: String) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(20.dp), verticalAlignment = Alignment.CenterVertically) {
        TextButton(stringResource(android.R.string.cancel), cancel, Modifier.weight(1f), enabled = !saving)
        if (saving) Box(Modifier.weight(1f), contentAlignment = Alignment.Center) { InfiniteProgressIndicator(Modifier.size(24.dp)) }
        else TextButton(confirmLabel, confirm, Modifier.weight(1f), enabled = enabled, colors = ButtonDefaults.textButtonColorsPrimary())
    }
}

private fun LocalRuleSet.title(): Int = when (this) {
    LocalRuleSet.Proxy -> R.string.routing_proxy
    LocalRuleSet.Direct -> R.string.routing_direct
    LocalRuleSet.Block -> R.string.routing_block
}

private fun RuleField.title(): Int = when (this) {
    RuleField.DomainSuffix -> R.string.routing_domain_suffix
    RuleField.Domain -> R.string.routing_domain
    RuleField.DomainKeyword -> R.string.routing_domain_keyword
    RuleField.DomainRegex -> R.string.routing_domain_regex
    RuleField.IpCidr -> R.string.routing_ip_cidr
    RuleField.Port -> R.string.routing_port
    RuleField.PortRange -> R.string.routing_port_range
}

private fun RuleField.hint(): Int = when (this) {
    RuleField.DomainSuffix -> R.string.routing_domain_suffix_hint
    RuleField.Domain -> R.string.routing_domain_hint
    RuleField.DomainKeyword -> R.string.routing_domain_keyword_hint
    RuleField.DomainRegex -> R.string.routing_domain_regex_hint
    RuleField.IpCidr -> R.string.routing_ip_cidr_hint
    RuleField.Port -> R.string.routing_port_hint
    RuleField.PortRange -> R.string.routing_port_range_hint
}

private fun RuleInputError.message(): Int = when (this) {
    RuleInputError.Empty -> R.string.routing_error_empty
    RuleInputError.Domain -> R.string.routing_error_domain
    RuleInputError.Port -> R.string.routing_error_port
    RuleInputError.PortRange -> R.string.routing_error_port_range
}
