package com.fanjv.netproxy.feature.traffic.presentation

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Check
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppBarButton
import com.fanjv.netproxy.core.ui.component.AppContentCard
import com.fanjv.netproxy.core.ui.component.AppDropdown
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.ManagerMenu
import com.fanjv.netproxy.feature.apps.presentation.AppInfoModel
import com.fanjv.netproxy.feature.apps.presentation.AppItem
import com.fanjv.netproxy.navigation.LocalNavigator
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.DropdownItem
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.glass.GlassSearchOverlay
import top.yukonga.miuix.kmp.glass.GlassSearchOverlayDefaults
import top.yukonga.miuix.kmp.glass.glassSearchBarTransition
import top.yukonga.miuix.kmp.glass.rememberGlassSearchBarAlpha
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.glass.Close
import top.yukonga.miuix.kmp.icon.glass.Search

@Composable
internal fun TrafficAppsScreen(viewModel: TrafficViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    val draft = state.appDraft
    val selected = draft?.selected?.toSet()
    val modes = listOf("all", "blacklist", "whitelist")
    val mode = draft?.effectiveMode ?: "all"
    var searching by rememberSaveable { mutableStateOf(false) }
    var showSystem by rememberSaveable { mutableStateOf(false) }
    val searchListState = rememberLazyListState()
    val pageAlpha by rememberGlassSearchBarAlpha(searching)
    val groups = remember(state.appCatalog, selected, showSystem) {
        captureAppGroups(state.appCatalog, selected.orEmpty(), showSystem)
    }
    LaunchedEffect(Unit) { viewModel.openAppPicker() }
    val leave = {
        if (!state.saving) {
            viewModel.editCaptureApps(null)
            navigator.pop()
        }
    }
    BackHandler(enabled = !searching, onBack = leave)
    val ready = !state.busy && draft != null
    val editable = ready && mode != "all"
    fun toggle(app: AppInfoModel) {
        if (!editable || selected == null) return
        viewModel.editCaptureApps(if (app.id in selected) selected - app.id else selected + app.id)
    }
    val menu = listOf(DropdownEntry(items = listOf(
        DropdownItem(text = stringResource(R.string.show_system_apps), selected = showSystem,
            onClick = { showSystem = !showSystem }),
        DropdownItem(text = stringResource(R.string.traffic_apps_select_all), onClick = {
            if (editable && selected != null) viewModel.editCaptureApps(selected + groups.values.flatten().map { it.id })
        }),
        DropdownItem(text = stringResource(R.string.traffic_apps_clear_selection), onClick = {
            if (editable) viewModel.editCaptureApps(emptySet())
        }),
    )))
    ManagerMenu(menu, stringResource(R.string.more_actions)) { moreButton ->
        Box(Modifier.fillMaxSize()) {
            TrafficFrame(R.string.traffic_capture_apps, onBack = leave,
                topBarModifier = Modifier.glassSearchBarTransition(searching) { pageAlpha }, actions = {
                AppBarButton(onClick = { searching = true }, enabled = editable) {
                    Icon(MiuixIcons.Glass.Search, stringResource(R.string.search))
                }
                AppBarButton(onClick = { viewModel.saveCaptureApps(navigator::pop) }, enabled = ready) {
                    Icon(Icons.Rounded.Check, stringResource(R.string.common_confirm))
                }
                if (editable) moreButton()
            }) { padding ->
                if (selected == null) {
                    ContentStatus(padding, state.failure?.let { trafficError(it, R.string.traffic_read_failed) }.orEmpty(),
                        loading = state.failure == null)
                } else {
                    Column(Modifier.fillMaxSize().padding(padding)) {
                        AppContentCard(Modifier.padding(horizontal = 12.dp)) {
                            AppDropdown(
                                title = stringResource(R.string.traffic_capture_app_mode),
                                items = listOf(
                                    stringResource(R.string.traffic_filter_all),
                                    stringResource(R.string.app_proxy_mode_blacklist),
                                    stringResource(R.string.app_proxy_mode_whitelist),
                                ),
                                selectedIndex = modes.indexOf(mode).coerceAtLeast(0),
                                enabled = ready,
                                onSelectedIndexChange = { viewModel.setCaptureAppMode(modes[it]) },
                            )
                        }
                        LazyColumn(Modifier.fillMaxSize()) {
                            item("summary") {
                                val summary = when (mode) {
                                    "blacklist" -> R.string.traffic_capture_app_blacklist_summary
                                    "whitelist" -> R.string.traffic_capture_app_whitelist_summary
                                    else -> R.string.traffic_capture_app_all_summary
                                }
                                Text(stringResource(summary), Modifier.padding(16.dp))
                            }
                            state.failure?.let { item("error") { TrafficErrorText(it) } }
                            if (mode != "all") captureAppRows(groups, selected, editable, viewModel::loadAppIcon, ::toggle)
                        }
                    }
                }
            }
            GlassSearchOverlay(
                expanded = searching, onDismissRequest = { searching = false },
                label = stringResource(R.string.search),
                horizontalInsetsModifier = Modifier.windowInsetsPadding(WindowInsets.displayCutout.only(WindowInsetsSides.Horizontal)),
                searchIcon = MiuixIcons.Glass.Search,
                closeIcon = MiuixIcons.Glass.Close,
                scrollOffset = { if (searchListState.firstVisibleItemIndex > 0) Float.MAX_VALUE
                    else searchListState.firstVisibleItemScrollOffset.toFloat() },
            ) { query ->
                val results = remember(groups, query) {
                    groups.mapValues { (_, apps) -> apps.filter { it.label.contains(query, ignoreCase = true) ||
                        it.id.contains(query, ignoreCase = true) } }.filterValues { it.isNotEmpty() }
                }
                LaunchedEffect(query) { searchListState.scrollToItem(0) }
                val top = WindowInsets.statusBars.asPaddingValues().calculateTopPadding() +
                    GlassSearchOverlayDefaults.BarHeight + GlassSearchOverlayDefaults.VerticalPadding * 2 + 12.dp
                LazyColumn(Modifier.fillMaxSize(), state = searchListState,
                    contentPadding = PaddingValues(top = top, bottom = 24.dp)) {
                    if (results.isEmpty() && query.isNotBlank()) item("empty") {
                        Text(stringResource(R.string.search_no_results), Modifier.padding(24.dp))
                    }
                    captureAppRows(results, selected.orEmpty(), editable, viewModel::loadAppIcon, ::toggle)
                }
            }
        }
    }
}

private fun LazyListScope.captureAppRows(
    groups: Map<String, List<AppInfoModel>>,
    selected: Set<String>,
    editable: Boolean,
    loadIcon: suspend (String) -> Unit,
    toggle: (AppInfoModel) -> Unit,
) {
    for ((user, apps) in groups) {
        item("user-$user") { SmallTitle(text = "${stringResource(R.string.traffic_app_user)} $user") }
        items(apps, key = AppInfoModel::id, contentType = { "app_item" }) { app ->
            LaunchedEffect(app.iconKey) { loadIcon(app.iconKey) }
            AppItem(app, showPackageName = true, isProxied = app.id in selected, spacing = 10.dp, enabled = editable) {
                toggle(app)
            }
        }
    }
}

internal fun captureAppGroups(
    catalog: Map<String, AppInfoModel>,
    selected: Set<String>,
    showSystem: Boolean,
): Map<String, List<AppInfoModel>> =
    (catalog.values + selected.filterNot(catalog::containsKey).map {
        AppInfoModel(it.substringAfter(':'), it.substringAfter(':'), it.substringBefore(':'))
    }).filter { showSystem || !it.isSystem || it.id in selected }
        .sortedWith(compareBy({ it.userId.toIntOrNull() }, { it.label.lowercase() }, { it.packageName }))
        .groupBy { it.userId }
