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
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.core.ui.component.deferredTopPadding
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
import top.yukonga.miuix.kmp.basic.TabRow
import top.yukonga.miuix.kmp.basic.TabRowDefaults
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
    val navigator = LocalNavigator.current
    val scrollBehavior = MiuixScrollBehavior()
    val dynamicTopPadding = remember(scrollBehavior) {
        { 12.dp * (1f - scrollBehavior.state.collapsedFraction) }
    }
    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else MiuixTheme.colorScheme.surface
    val rawEdit = { navigator.push(Route.JsonEdit(state.selected.target)) }
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
                        IconButton(onClick = { viewModel.edit() }, enabled = state.editable) {
                            Icon(MiuixIcons.Add, stringResource(R.string.routing_add), tint = MiuixTheme.colorScheme.onSurface)
                        }
                        var expanded by remember { mutableStateOf(false) }
                        TopBarMoreMenu(expanded, { expanded = it }, listOf(
                            TopBarMenuAction(stringResource(R.string.routing_raw), !state.isSaving, rawEdit),
                            TopBarMenuAction(stringResource(R.string.inbound_reload), !state.isSaving) { viewModel.refresh() },
                        ), stringResource(R.string.more_actions))
                    },
                    bottomContent = {
                        Column(
                            Modifier.fillMaxWidth()
                                .padding(horizontal = 12.dp)
                                .padding(bottom = 6.dp)
                                .deferredTopPadding(dynamicTopPadding),
                        ) {
                            TabRow(
                                tabs = LocalRuleSet.entries.map { stringResource(it.title()) },
                                selectedTabIndex = state.selected.ordinal,
                                onTabSelected = { viewModel.select(LocalRuleSet.entries[it]) },
                                modifier = Modifier.fillMaxWidth(),
                                colors = TabRowDefaults.tabRowColors(backgroundColor = barColor),
                                height = 40.dp,
                            )
                        }
                    },
                )
            }
        },
    ) { padding ->
        val layoutDirection = LocalLayoutDirection.current
        if (state.document == null && !state.invalidDocument && state.draft == null) {
            ContentStatus(padding, stringResource(R.string.routing_read_failed), loading = state.isLoading || state.error.isEmpty())
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
            if ((state.error.isNotEmpty() || state.requiresReload) && state.draft == null && state.deleting == null) {
                item("error") { RuleError(state, reload = { viewModel.refresh(discardDraft = true) }) }
            }
            if (state.document == null) {
                if (state.invalidDocument) item("raw") {
                    TextButton(stringResource(R.string.routing_raw), rawEdit, Modifier.fillMaxWidth())
                }
            } else {
                val rules = state.document!!.rules
                if (rules.isNotEmpty()) groupedCardSection(
                    "routing_rules",
                    { stringResource(R.string.routing_matches) },
                    rules.mapIndexed { index, rule -> CardItem(index.toString()) {
                        val simple = remember(rule) { rule.simpleRule() }
                        ArrowPreference(
                            title = simple?.values?.first()?.take(100) ?: stringResource(R.string.routing_advanced),
                            summary = simple?.let {
                                stringResource(it.field.title()) + if (it.values.size > 1) " · " + stringResource(R.string.routing_values_count, it.values.size) else ""
                            } ?: stringResource(R.string.routing_advanced_hint),
                            onClick = { viewModel.edit(index) },
                            holdDownState = state.draft?.index == index,
                        )
                    } },
                    titleTopPadding = 8.dp,
                )
                if (rules.isEmpty()) item("empty") {
                    Box(Modifier.fillMaxWidth().fillParentMaxHeight(0.7f), contentAlignment = Alignment.Center) {
                        Column(Modifier.padding(horizontal = 14.dp),
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(stringResource(R.string.routing_empty), style = MiuixTheme.textStyles.body1,
                                color = MiuixTheme.colorScheme.onSurfaceVariantSummary, textAlign = TextAlign.Center)
                            Text(stringResource(R.string.routing_empty_hint), style = MiuixTheme.textStyles.footnote1,
                                color = MiuixTheme.colorScheme.onSurfaceVariantSummary, textAlign = TextAlign.Center)
                        }
                    }
                }
            }
            item("bottom") { Spacer(Modifier.height(24.dp).navigationBarsPadding()) }
        }
    }

    var retainedDraft by remember { mutableStateOf<RuleDraft?>(null) }
    val draft = state.draft ?: retainedDraft
    SideEffect { if (state.draft != null) retainedDraft = state.draft }
    OverlayDialog(show = state.draft != null, title = stringResource(if (draft?.index == null) R.string.routing_add else R.string.routing_edit),
        onDismissRequest = viewModel::dismiss, onDismissFinished = { retainedDraft = null }) {
        if (draft != null) {
            val textState = rememberTextFieldState(draft.text)
            LaunchedEffect(textState, draft.field) {
                draft.field?.let { field ->
                    snapshotFlow { textState.text.toString() }.collect { viewModel.updateDraft(field, it) }
                }
            }
            Column(Modifier.heightIn(max = 500.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Column(Modifier.weight(1f, fill = false).verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(12.dp)) {
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
                    if (state.error.isNotEmpty() || state.requiresReload) RuleError(state) { viewModel.refresh(discardDraft = true) }
                }
                RuleButtons(state.isSaving, viewModel::dismiss,
                    confirm = { if (draft.field != null) {
                        viewModel.updateDraft(draft.field, textState.text.toString())
                        viewModel.save()
                    } else { viewModel.dismiss(); rawEdit() } },
                    enabled = state.editable,
                    confirmLabel = stringResource(if (draft.field != null) R.string.save_text else R.string.routing_raw))
                draft.index?.let { index ->
                    TextButton(stringResource(R.string.common_delete), { viewModel.requestDelete(index) },
                        Modifier.fillMaxWidth(), enabled = state.editable,
                        colors = ButtonDefaults.textButtonColors(textColor = MiuixTheme.colorScheme.error))
                }
            }
        }
    }
    OverlayDialog(show = state.deleting != null, title = stringResource(R.string.routing_delete), summary = stringResource(R.string.routing_delete_hint),
        onDismissRequest = viewModel::dismiss) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (state.error.isNotEmpty() || state.requiresReload) RuleError(state) { viewModel.refresh(discardDraft = true) }
            RuleButtons(state.isSaving, viewModel::dismiss,
                confirm = viewModel::delete, enabled = state.editable, confirmLabel = stringResource(R.string.common_delete))
        }
    }
}

@Composable
private fun RuleError(state: RoutingRulesState, reload: () -> Unit) {
    Column(Modifier.padding(vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        val message = when {
            state.invalidDocument -> stringResource(R.string.routing_invalid_document)
            state.requiresReload && state.document != null -> stringResource(R.string.inbound_conflict)
            else -> state.error
        }
        Text(listOf(message, state.error).filter(String::isNotBlank).distinct().joinToString("\n"),
            style = MiuixTheme.textStyles.body2, color = MiuixTheme.colorScheme.error)
        if (state.requiresReload) TextButton(stringResource(if (state.draft != null) R.string.routing_reload_draft else R.string.inbound_reload),
            reload, Modifier.fillMaxWidth(), enabled = !state.isLoading)
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
