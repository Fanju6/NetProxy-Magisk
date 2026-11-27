package com.fanjv.netproxy.feature.routing.presentation

import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.add
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.appBarOverscroll
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.feature.routing.model.LocalRuleSet
import com.fanjv.netproxy.feature.routing.model.RuleField
import com.fanjv.netproxy.feature.routing.model.RuleInputError
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

@Composable
internal fun RoutingPage(
    title: String,
    onBack: () -> Unit,
    actions: @Composable RowScope.() -> Unit = {},
    listState: LazyListState = rememberLazyListState(),
    content: LazyListScope.() -> Unit,
) {
    val scroll = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    Scaffold(
        topBar = {
            BlurredBar(backdrop) {
                AdaptiveTopAppBar(
                    title = title,
                    color = if (backdrop != null) Color.Transparent else MiuixTheme.colorScheme.surface,
                    navigationIcon = { BackIconButton(onClick = onBack) },
                    scrollBehavior = scroll,
                    actions = actions,
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.add(WindowInsets.displayCutout)
            .only(WindowInsetsSides.Horizontal + WindowInsetsSides.Bottom),
    ) { padding ->
        val direction = LocalLayoutDirection.current
        LazyColumn(
            state = listState,
            modifier = Modifier.fillMaxSize().then(if (backdrop == null) Modifier else Modifier.layerBackdrop(backdrop))
                .imePadding().scrollEndHaptic().appBarOverscroll(scroll).nestedScroll(scroll.nestedScrollConnection),
            contentPadding = PaddingValues(
                top = padding.calculateTopPadding() + 6.dp,
                start = padding.calculateStartPadding(direction) + 12.dp,
                end = padding.calculateEndPadding(direction) + 12.dp,
                bottom = padding.calculateBottomPadding() + 24.dp,
            ),
            overscrollEffect = null,
            content = content,
        )
    }
}

@Composable
internal fun RoutingError(
    file: RuleFileState,
    state: RoutingRulesState,
    onReload: () -> Unit,
    backToList: Boolean = false,
    reloadTitle: Int? = null,
) {
    val message = if (file.invalidDocument) stringResource(R.string.routing_invalid_document)
        else file.error.ifBlank { stringResource(R.string.routing_read_failed) }
    Text(message, Modifier.padding(horizontal = 14.dp, vertical = 8.dp),
        color = MiuixTheme.colorScheme.error, style = MiuixTheme.textStyles.body2)
    if (file.requiresReload) ArrowPreference(
        title = stringResource(reloadTitle ?: when {
            backToList -> R.string.routing_back_to_list
            state.draft != null || state.deleting != null -> R.string.routing_reload_draft
            else -> R.string.inbound_reload
        }),
        enabled = !state.isLoading && !state.isSaving,
        onClick = onReload,
    )
}

internal fun LocalRuleSet.title(): Int = when (this) {
    LocalRuleSet.Proxy -> R.string.routing_proxy
    LocalRuleSet.Direct -> R.string.routing_direct
    LocalRuleSet.Block -> R.string.routing_block
    LocalRuleSet.Policy -> R.string.routing_policy_target
}

internal fun RuleField.title(): Int = when (this) {
    RuleField.DomainSuffix -> R.string.routing_domain_suffix
    RuleField.Domain -> R.string.routing_domain
    RuleField.DomainKeyword -> R.string.routing_domain_keyword
    RuleField.DomainRegex -> R.string.routing_domain_regex
    RuleField.IpCidr -> R.string.routing_ip_cidr
    RuleField.Port -> R.string.routing_port
    RuleField.PortRange -> R.string.routing_port_range
    RuleField.PackageName -> R.string.routing_app_condition
}

internal fun RuleField.hint(): Int = when (this) {
    RuleField.DomainSuffix -> R.string.routing_domain_suffix_hint
    RuleField.Domain -> R.string.routing_domain_hint
    RuleField.DomainKeyword -> R.string.routing_domain_keyword_hint
    RuleField.DomainRegex -> R.string.routing_domain_regex_hint
    RuleField.IpCidr -> R.string.routing_ip_cidr_hint
    RuleField.Port -> R.string.routing_port_hint
    RuleField.PortRange -> R.string.routing_port_range_hint
    RuleField.PackageName -> R.string.routing_package_name_hint
}

internal fun RuleInputError.message(): Int = when (this) {
    RuleInputError.Empty -> R.string.routing_error_empty
    RuleInputError.Domain -> R.string.routing_error_domain
    RuleInputError.Port -> R.string.routing_error_port
    RuleInputError.PortRange -> R.string.routing_error_port_range
    RuleInputError.PackageName -> R.string.routing_error_package_name
}
