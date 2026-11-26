package com.fanjv.netproxy.feature.routing.presentation

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.AppBarButton
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.chromeIcon
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.feature.routing.model.LocalRuleSet
import com.fanjv.netproxy.feature.routing.model.RuleField
import com.fanjv.netproxy.feature.routing.model.SimpleRule
import com.fanjv.netproxy.feature.routing.model.policyRule
import com.fanjv.netproxy.feature.routing.model.simpleRule
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import kotlinx.coroutines.CancellationException
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Add
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme

internal enum class RuleSection(val title: Int) {
    Apps(R.string.routing_section_apps),
    Address(R.string.routing_section_address),
    Advanced(R.string.routing_section_advanced),
}

internal data class RuleListEntry(val identity: RuleIdentity, val match: SimpleRule?, val policyTag: String?)

internal fun RoutingRulesState.groupedRules(): Map<RuleSection, List<RuleListEntry>> = rules.map { identity ->
    val rule = files.getValue(identity.target).document!!.rules[identity.index]
    val policy = rule.policyRule()
    RuleListEntry(identity, policy?.second ?: rule.simpleRule(), policy?.first)
}.groupBy {
    when (it.match?.field) {
        null -> RuleSection.Advanced
        RuleField.PackageName -> RuleSection.Apps
        else -> RuleSection.Address
    }
}

@Composable
internal fun RoutingRulesScreen(onBack: () -> Unit, viewModel: RoutingRulesViewModel = netProxyViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    val groups = remember(state.files) { state.groupedRules() }
    val hasAppRules = RuleSection.Apps in groups
    val nativeOnly = state.files.getValue(LocalRuleSet.Policy).document?.let { it.policyStart == null } == true
    val appNames by produceState(emptyMap<String, String>(), viewModel, hasAppRules) {
        value = if (hasAppRules) {
            try {
                viewModel.installedApps().associate { it.packageName to it.label }
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                emptyMap()
            }
        } else emptyMap()
    }
    LifecycleResumeEffect(viewModel) {
        viewModel.refresh()
        onPauseOrDispose { }
    }

    RoutingPage(
        title = stringResource(R.string.routing_rules),
        onBack = onBack,
        actions = {
            AppBarButton(onClick = { viewModel.openEditor(null, null) }, enabled = state.canAdd) {
                Icon(chromeIcon(MiuixIcons.Add), stringResource(R.string.routing_add))
            }
        },
    ) {
        if (!state.hasDocuments && (state.isLoading || state.files.values.none { it.requiresReload })) {
            item("loading") {
                ContentStatus(PaddingValues(0.dp), "", loading = true, modifier = Modifier.fillParentMaxHeight())
            }
        }
        if (state.isEmpty && !nativeOnly) item("empty") {
            Column(
                Modifier.fillParentMaxHeight().fillMaxWidth().padding(horizontal = 14.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(8.dp, Alignment.CenterVertically),
            ) {
                Text(stringResource(R.string.routing_empty), style = MiuixTheme.textStyles.body1,
                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary, textAlign = TextAlign.Center)
                Text(stringResource(R.string.routing_empty_hint), style = MiuixTheme.textStyles.footnote1,
                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary, textAlign = TextAlign.Center)
            }
        }
        for ((target, file) in state.files) {
            if (!state.showPageError(target)) continue
            item("error:${target.tag}") {
                Text(stringResource(target.title()), Modifier.padding(start = 14.dp, top = 12.dp),
                    style = MiuixTheme.textStyles.body1)
                RoutingError(file, state, onReload = { viewModel.refresh() })
                if (file.invalidDocument) ArrowPreference(
                    title = stringResource(R.string.routing_raw),
                    onClick = { navigator.push(Route.JsonEdit(target.target)) },
                    enabled = !state.isLoading,
                )
            }
        }
        for (section in RuleSection.entries) {
            val rows = groups[section].orEmpty().map { entry ->
                val (target, index) = entry.identity
                CardItem("${target.tag}:$index") {
                    val match = entry.match
                    val value = match?.values?.first()
                    val appName = if (match?.field == RuleField.PackageName) value?.let(appNames::get) else null
                    ArrowPreference(
                        title = (appName ?: value)?.take(100) ?: stringResource(R.string.routing_advanced),
                        summary = listOfNotNull(
                            entry.policyTag ?: stringResource(target.title()),
                            if (section != RuleSection.Apps) match?.field?.let { stringResource(it.title()) } else value?.takeIf { appName != null && appName != it },
                            match?.values?.size?.takeIf { it > 1 }?.let { stringResource(R.string.routing_values_count, it) },
                        ).joinToString(" · "),
                        enabled = state.editable(target),
                        onClick = { viewModel.openEditor(entry.identity, state.files.getValue(target).revision) },
                    )
                }
            }
            val items = if (section == RuleSection.Advanced && nativeOnly) rows + CardItem("native") {
                ArrowPreference(
                    title = stringResource(R.string.routing_raw),
                    summary = stringResource(R.string.routing_policy_order_unsupported),
                    onClick = { navigator.push(Route.JsonEdit(LocalRuleSet.Policy.target)) },
                )
            } else rows
            if (items.isNotEmpty()) groupedCardSection(section.name, { stringResource(section.title) }, items)
        }
    }
    RoutingRuleEditorSheet(state, viewModel, onEditJson = { navigator.push(Route.JsonEdit(it.target)) })
}
