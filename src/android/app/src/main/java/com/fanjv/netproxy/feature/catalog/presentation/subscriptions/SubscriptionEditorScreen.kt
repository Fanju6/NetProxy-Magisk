package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Check
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.component.AppBarButton
import com.fanjv.netproxy.core.ui.component.AppContentCard
import com.fanjv.netproxy.core.ui.component.AppDropdown as OverlayDropdownPreference
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.AppTopAppBar as TopAppBar
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import com.fanjv.netproxy.core.ui.component.chromeIcon
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.resolve
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

@Composable
internal fun SubscriptionEditorScreen(
    id: String,
    onBack: () -> Unit,
    viewModel: SubscriptionEditorViewModel = netProxyViewModel()
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val draft = state.draft
    val snackbarHostState = rememberAppSnackbarHostState()
    val backdrop = rememberBlurBackdrop()
    val scrollBehavior = MiuixScrollBehavior()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface

    LaunchedEffect(id) { viewModel.load(id) }
    LaunchedEffect(state.saved) {
        if (state.saved) onBack()
    }
    SnackbarNoticeEffect(
        eventId = state.noticeId,
        message = state.error.resolve(),
        isError = state.error !is UiText.Empty,
        hostState = snackbarHostState,
        onConsumed = viewModel::clearError
    )

    Scaffold(
        snackbarHost = { AppSnackbarHost(snackbarHostState) },
        topBar = {
            BlurredBar(backdrop) {
                TopAppBar(
                    color = barColor,
                    title = stringResource(
                        if (state.id.isBlank()) R.string.subscription_editor_add_title
                        else R.string.subscription_editor_edit_title
                    ),
                    scrollBehavior = scrollBehavior,
                    navigationIcon = { BackIconButton(onClick = onBack) },
                    actions = {
                        AppBarButton(
                            enabled = !state.saving && !state.loading,
                            onClick = viewModel::save
                        ) {
                            if (state.saving) {
                                InfiniteProgressIndicator(modifier = Modifier.size(20.dp))
                            } else {
                                Icon(
                                    chromeIcon(Icons.Rounded.Check),
                                    contentDescription = stringResource(R.string.save_text)
                                )
                            }
                        }
                    }
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.only(WindowInsetsSides.Horizontal)
    ) { innerPadding ->
        if (state.loading) {
            Box(
                Modifier
                    .fillMaxSize()
                    .padding(innerPadding), contentAlignment = Alignment.Center
            ) {
                InfiniteProgressIndicator()
            }
        } else {
            LazyColumn(
                modifier = Modifier
                    .fillMaxSize()
                    .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)
                    .nestedScroll(scrollBehavior.nestedScrollConnection)
                    .scrollEndHaptic()
                    .overScrollVertical()
                    .padding(horizontal = 12.dp),
                contentPadding = PaddingValues(
                    top = innerPadding.calculateTopPadding() + 12.dp,
                    bottom = innerPadding.calculateBottomPadding() + 24.dp
                ),
                verticalArrangement = Arrangement.spacedBy(12.dp),
                overscrollEffect = null
            ) {
                item { SectionLabel(stringResource(R.string.subscription_editor_basic_section)) }
                item {
                    TextField(
                        value = draft.name,
                        onValueChange = { value ->
                            viewModel.update { it.copy(name = value) }
                        },
                        label = stringResource(
                            if (state.id.isBlank()) R.string.subscription_editor_name_auto
                            else R.string.subscription_editor_name
                        ),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
                item {
                    TextField(
                        value = draft.url,
                        onValueChange = { value ->
                            viewModel.update { it.copy(url = value) }
                        },
                        label = stringResource(R.string.subscription_editor_url),
                        modifier = Modifier.fillMaxWidth(),
                        minLines = 2,
                        maxLines = 4
                    )
                }
                item {
                    AppContentCard(modifier = Modifier.fillMaxWidth()) {
                        val intervals = subscriptionIntervalOptions(draft.updateIntervalSeconds)
                        val intervalLabels = listOf(
                            stringResource(R.string.subscription_duration_minutes, 15),
                            stringResource(R.string.subscription_duration_hours, 1),
                            stringResource(R.string.subscription_duration_hours, 6),
                            stringResource(R.string.subscription_duration_hours, 12),
                            stringResource(R.string.subscription_duration_hours, 24),
                            stringResource(R.string.subscription_duration_days, 3),
                            stringResource(R.string.subscription_duration_days, 7)
                        ) + if (intervals.size > 7) {
                            listOf(stringResource(R.string.subscription_interval_custom_seconds, draft.updateIntervalSeconds))
                        } else emptyList()
                        OverlayDropdownPreference(
                            title = stringResource(R.string.subscription_editor_update_interval),
                            items = intervalLabels,
                            selectedIndex = intervals.indexOf(draft.updateIntervalSeconds),
                            onSelectedIndexChange = { index ->
                                viewModel.update {
                                    it.copy(updateIntervalSeconds = intervals[index])
                                }
                            }
                        )
                        SwitchPreference(
                            title = stringResource(R.string.subscription_editor_auto_update),
                            checked = draft.autoUpdate,
                            onCheckedChange = { enabled ->
                                viewModel.update { it.copy(autoUpdate = enabled) }
                            }
                        )
                        val viaValues = listOf("auto", "always", "never")
                        OverlayDropdownPreference(
                            title = stringResource(R.string.subscription_editor_update_network),
                            items = listOf(
                                stringResource(R.string.subscription_editor_network_auto),
                                stringResource(R.string.subscription_editor_network_proxy),
                                stringResource(R.string.subscription_editor_network_direct)
                            ),
                            selectedIndex = viaValues.indexOf(draft.updateViaProxy)
                                .coerceAtLeast(0),
                            onSelectedIndexChange = { index ->
                                viewModel.update {
                                    it.copy(updateViaProxy = viaValues[index])
                                }
                            }
                        )
                    }
                }
                item { SectionLabel(stringResource(R.string.subscription_editor_filter_section)) }
                item {
                    TextField(
                        value = draft.include,
                        onValueChange = { value ->
                            viewModel.update { it.copy(include = value) }
                        },
                        label = stringResource(R.string.subscription_editor_include),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
                item {
                    TextField(
                        value = draft.exclude,
                        onValueChange = { value ->
                            viewModel.update { it.copy(exclude = value) }
                        },
                        label = stringResource(R.string.subscription_editor_exclude),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
                item {
                    AppContentCard(modifier = Modifier.fillMaxWidth()) {
                        SwitchPreference(
                            title = stringResource(R.string.subscription_editor_allow_insecure),
                            summary = stringResource(R.string.subscription_editor_allow_insecure_summary),
                            checked = draft.allowInsecure,
                            onCheckedChange = { enabled ->
                                viewModel.update { it.copy(allowInsecure = enabled) }
                            }
                        )
                    }
                }
                item { SectionLabel(stringResource(R.string.subscription_editor_advanced_section)) }
                item {
                    TextField(
                        value = draft.userAgent,
                        onValueChange = { value ->
                            viewModel.update { it.copy(userAgent = value) }
                        },
                        label = stringResource(R.string.subscription_editor_user_agent),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
                item {
                    TextField(
                        value = draft.hwid,
                        onValueChange = { value ->
                            viewModel.update { it.copy(hwid = value) }
                        },
                        label = stringResource(R.string.subscription_editor_hwid),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
                item {
                    TextField(
                        value = state.headersText,
                        onValueChange = viewModel::updateHeaders,
                        label = stringResource(R.string.subscription_editor_headers),
                        modifier = Modifier.fillMaxWidth(),
                        minLines = 3,
                        maxLines = 8
                    )
                }
                item {
                    TextField(
                        value = draft.timeoutSeconds.toString(),
                        onValueChange = { value ->
                            value.toIntOrNull()?.let { seconds ->
                                viewModel.update { it.copy(timeoutSeconds = seconds) }
                            }
                        },
                        label = stringResource(R.string.subscription_editor_timeout),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
            }
        }
    }
}
