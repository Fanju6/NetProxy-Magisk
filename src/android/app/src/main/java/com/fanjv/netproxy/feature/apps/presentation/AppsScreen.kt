package com.fanjv.netproxy.feature.apps.presentation

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.basicMarquee
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.add
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.requiredSize
import androidx.compose.foundation.layout.requiredWidth
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Android
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.CancellationException
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.flowWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.ManagerMenu
import top.yukonga.miuix.kmp.glass.GlassSearchOverlay
import top.yukonga.miuix.kmp.glass.GlassSearchOverlayDefaults
import top.yukonga.miuix.kmp.glass.glassSearchBarTransition
import top.yukonga.miuix.kmp.glass.rememberGlassSearchBarAlpha
import top.yukonga.miuix.kmp.icon.glass.Search
import top.yukonga.miuix.kmp.icon.glass.Close
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.DropdownItem
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.ui.graphics.graphicsLayer
import com.fanjv.netproxy.core.ui.component.StatusTag
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.component.rememberCommitOnLeave
import com.fanjv.netproxy.core.ui.component.deferredTopPadding
import com.fanjv.netproxy.feature.apps.data.AppIconCache
import top.yukonga.miuix.kmp.basic.BasicComponent
import com.fanjv.netproxy.core.ui.component.AppContentCard
import top.yukonga.miuix.kmp.basic.Icon
import com.fanjv.netproxy.core.ui.component.AppBarButton as IconButton
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.PullToRefresh
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import com.fanjv.netproxy.core.ui.component.AppTopAppBar as TopAppBar
import top.yukonga.miuix.kmp.basic.rememberPullToRefreshState
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import com.fanjv.netproxy.core.ui.component.AppDropdown as OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import com.fanjv.netproxy.core.ui.component.appBarOverscroll
import top.yukonga.miuix.kmp.utils.scrollEndHaptic


@Composable
fun AppIcon(
    app: AppInfoModel,
    modifier: Modifier = Modifier,
    size: androidx.compose.ui.unit.Dp = 40.dp,
) {
    val iconRevision = AppIconCache.revision
    var icon by remember(app.id) {
        mutableStateOf(AppIconCache.cachedIcon(app.iconKey))
    }

    LaunchedEffect(app.iconKey, iconRevision) {
        AppIconCache.cachedIcon(app.iconKey)?.let { icon = it }
    }

    Box(
        modifier = modifier
            .requiredSize(size)
            .clip(RoundedCornerShape(10.dp)),
        contentAlignment = androidx.compose.ui.Alignment.Center
    ) {
        if (icon != null) {
            androidx.compose.foundation.Image(
                bitmap = icon!!,
                contentDescription = null,
                modifier = Modifier.requiredSize(size),
                contentScale = ContentScale.Fit
            )
        } else {
            Icon(
                imageVector = Icons.Rounded.Android,
                contentDescription = null,
                modifier = Modifier.requiredSize(size * 0.6f),
                tint = colorScheme.onBackground.copy(alpha = 0.5f)
            )
        }
    }
}

/** App policy stays on the page; the official overlay owns search interaction. */
@Composable
internal fun AppsScreen(
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
    viewModel: AppsViewModel = netProxyViewModel(),
    onBack: (() -> Unit)? = null,
) {
    val apps by viewModel.state.collectAsStateWithLifecycle()
    var searching by rememberSaveable { mutableStateOf(false) }
    val listState = rememberLazyListState()
    val searchListState = rememberLazyListState()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    for (currentList in listOf(listState, searchListState)) {
        LaunchedEffect(currentList, apps.masterAppList, lifecycle) {
            snapshotFlow {
                val active = (currentList === listState) != searching
                (if (active) currentList.layoutInfo.visibleItemsInfo.mapNotNull { it.key as? String } else emptyList()) to AppIconCache.generation
            }.flowWithLifecycle(lifecycle, Lifecycle.State.RESUMED)
                .distinctUntilChanged().collectLatest { (visible, _) ->
                    try { viewModel.loadIcons(visible) }
                    catch (error: CancellationException) { throw error }
                    catch (_: Exception) { }
                }
        }
    }
    val resources = LocalConfiguration.current
    LaunchedEffect(resources.locales.toLanguageTags()) { viewModel.load(force = apps.hasLoadedApps) }
    LaunchedEffect(searching) { if (!searching) viewModel.updateSearch("") }
    val commitOnLeave = rememberCommitOnLeave(viewModel::requestPolicyFlush)
    val leave: () -> Unit = { commitOnLeave { onBack?.invoke() } }
    BackHandler(enabled = onBack != null && !searching &&
        (apps.hasPendingPolicy || apps.requiresPolicyReload)) { leave() }
    val scroll = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val pageAlpha by rememberGlassSearchBarAlpha(searching)
    val allApps = remember(apps.allApps, apps.proxiedApps, apps.appSelectedFirst,
        apps.appReverseSort, apps.appProxyEnabled) { apps.orderedApps() }
    val results = remember(apps.searchResults, apps.proxiedApps, apps.appSelectedFirst,
        apps.appReverseSort, apps.appProxyEnabled) { apps.orderedApps(apps.searchResults) }
    val entries = listOf(DropdownEntry(items = listOf(
        DropdownItem(text = stringResource(R.string.show_system_apps), selected = apps.showSystemApps,
            onClick = { viewModel.setShowSystemApps(!apps.showSystemApps) }),
        DropdownItem(text = stringResource(R.string.app_selected_first), selected = apps.appSelectedFirst,
            onClick = { viewModel.setSelectedFirst(!apps.appSelectedFirst) }),
        DropdownItem(text = stringResource(R.string.app_reverse_sort), selected = apps.appReverseSort,
            onClick = { viewModel.setReverseSort(!apps.appReverseSort) }),
        DropdownItem(text = stringResource(R.string.app_show_package_name), selected = apps.appShowPackageName,
            onClick = { viewModel.setShowPackageName(!apps.appShowPackageName) }),
    )))
    ManagerMenu(entries, stringResource(R.string.more_actions)) { moreButton ->
        Box(Modifier.fillMaxSize()) {
            Scaffold(
                topBar = {
                    BlurredBar(backdrop) {
                        TopAppBar(
                            title = stringResource(R.string.proxy_apps),
                            color = Color.Transparent,
                            modifier = Modifier.glassSearchBarTransition(searching) { pageAlpha },
                            scrollBehavior = scroll,
                            navigationIcon = { onBack?.let { BackIconButton(onClick = leave) } },
                            actions = {
                                IconButton(onClick = { searching = true }, enabled = apps.hasLoadedApps) {
                                    Icon(MiuixIcons.Glass.Search, stringResource(R.string.search), tint = colorScheme.onSurface)
                                }
                                if (apps.hasLoadedApps && apps.appProxyEnabled) moreButton()
                            },
                            bottomContent = {
                                if (apps.hasLoadedApps) {
                                    Box(Modifier.deferredTopPadding {
                                        8.dp + 4.dp * (1f - scroll.state.collapsedFraction)
                                    }) {
                                        AppProxyMode(apps, viewModel::setProxySettings)
                                    }
                                }
                            },
                        )
                    }
                },
                contentWindowInsets = WindowInsets.systemBars.add(WindowInsets.displayCutout).only(WindowInsetsSides.Horizontal),
            ) { padding ->
                val direction = LocalLayoutDirection.current
                Box(Modifier.fillMaxSize().graphicsLayer { alpha = pageAlpha }
                    .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)) {
                    if (!apps.hasLoadedApps) {
                        Box(Modifier.fillMaxSize().padding(padding), contentAlignment = androidx.compose.ui.Alignment.Center) {
                            InfiniteProgressIndicator()
                        }
                    } else {
                        PullToRefresh(
                            isRefreshing = apps.isLoadingApps,
                            onRefresh = { viewModel.load(force = true) },
                            pullToRefreshState = rememberPullToRefreshState(),
                            refreshTexts = listOf(stringResource(R.string.refresh_pulling), stringResource(R.string.refresh_release),
                                stringResource(R.string.refresh_refresh), stringResource(R.string.refresh_complete)),
                            contentPadding = padding,
                        ) {
                            LazyColumn(state = listState, modifier = Modifier.fillMaxSize().scrollEndHaptic()
                                .appBarOverscroll(scroll).nestedScroll(scroll.nestedScrollConnection),
                                contentPadding = PaddingValues(top = padding.calculateTopPadding(),
                                    start = padding.calculateStartPadding(direction), end = padding.calculateEndPadding(direction),
                                    bottom = bottomPadding + 80.dp), overscrollEffect = null) {
                                if (apps.error.isNotBlank()) item("error") {
                                    Text(apps.error, Modifier.padding(14.dp), color = colorScheme.error)
                                }
                                if (apps.requiresPolicyReload) item("reload") {
                                    TextButton(stringResource(R.string.routing_reload_draft), viewModel::discardPolicyAndReload)
                                }
                                items(allApps, key = AppInfoModel::id, contentType = { "app_item" }) { app ->
                                    AppItem(app, apps.appShowPackageName, app.id in apps.proxiedApps, 10.dp,
                                        enabled = apps.appProxyEnabled) { viewModel.toggle(app.id) }
                                }
                            }
                        }
                    }
                }
            }
            val searchResultTopPadding = 12.dp
            GlassSearchOverlay(
                expanded = searching,
                onDismissRequest = { searching = false },
                scrollMaterialThreshold = GlassSearchOverlayDefaults.VerticalPadding + searchResultTopPadding,
                label = stringResource(R.string.search),
                horizontalInsetsModifier = Modifier.windowInsetsPadding(
                    WindowInsets.displayCutout.only(WindowInsetsSides.Horizontal)),
                searchIcon = MiuixIcons.Glass.Search,
                closeIcon = MiuixIcons.Glass.Close,
                scrollOffset = { if (searchListState.firstVisibleItemIndex > 0) Float.MAX_VALUE
                    else searchListState.firstVisibleItemScrollOffset.toFloat() },
            ) { query ->
                LaunchedEffect(query) { viewModel.updateSearch(query); searchListState.scrollToItem(0) }
                val top = WindowInsets.statusBars.asPaddingValues().calculateTopPadding() +
                    GlassSearchOverlayDefaults.BarHeight + GlassSearchOverlayDefaults.VerticalPadding * 2 +
                    searchResultTopPadding
                LazyColumn(state = searchListState, modifier = Modifier.fillMaxSize(),
                    contentPadding = PaddingValues(top = top, bottom = bottomPadding + 12.dp),
                    overscrollEffect = null) {
                    if (apps.error.isNotBlank()) item("error") {
                        Text(apps.error, Modifier.padding(14.dp), color = colorScheme.error)
                    }
                    if (apps.requiresPolicyReload) item("reload") {
                        TextButton(stringResource(R.string.routing_reload_draft), viewModel::discardPolicyAndReload)
                    }
                    if (apps.appSearchQuery == query && !apps.isFilteringApps &&
                        query.isNotBlank() && results.isEmpty()) item("empty") {
                        Text(stringResource(R.string.search_no_results), Modifier.padding(24.dp))
                    }
                    items(if (apps.appSearchQuery == query && !apps.isFilteringApps) results else emptyList(),
                        key = AppInfoModel::id, contentType = { "app_item" }) { app ->
                        AppItem(app, apps.appShowPackageName, app.id in apps.proxiedApps, 10.dp,
                            enabled = apps.appProxyEnabled) {
                            if (apps.appSelectedFirst) searchListState.requestScrollToItem(
                                searchListState.firstVisibleItemIndex, searchListState.firstVisibleItemScrollOffset)
                            viewModel.toggle(app.id)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun AppProxyMode(apps: AppsUiState, onChange: (Boolean, String?) -> Unit) {
    AppContentCard(Modifier.padding(horizontal = 12.dp).padding(bottom = 12.dp).fillMaxWidth()) {
        OverlayDropdownPreference(
            title = stringResource(R.string.app_proxy_mode),
            items = listOf(
                stringResource(R.string.proxy_mode_off),
                stringResource(R.string.app_proxy_mode_blacklist),
                stringResource(R.string.app_proxy_mode_whitelist)
            ),
            selectedIndex = if (!apps.appProxyEnabled) 0 else if (apps.appProxyMode == "blacklist") 1 else 2,
            onSelectedIndexChange = { index ->
                onChange(index != 0, when (index) {
                    1 -> "blacklist"
                    2 -> "whitelist"
                    else -> null
                })
            }
        )
    }
}


@Composable
internal fun AppItem(
    app: AppInfoModel,
    showPackageName: Boolean,
    isProxied: Boolean,
    spacing: androidx.compose.ui.unit.Dp,
    enabled: Boolean,
    onClick: () -> Unit
) {
    AppContentCard(
        modifier = Modifier
            .padding(horizontal = 12.dp)
            .padding(bottom = spacing)
            .fillMaxWidth()
    ) {
        BasicComponent(
            enabled = enabled,
            onClick = onClick.takeIf { enabled },
            startAction = {
                Box(
                    modifier = Modifier
                        .requiredWidth(56.dp)
                        .padding(start = 12.dp)
                ) {
                    AppIcon(
                        app = app,
                        modifier = Modifier.padding(end = 12.dp)
                    )
                }
            },
            endActions = {
                if (app.userId != "0") {
                    StatusTag(
                        label = stringResource(
                            R.string.app_users_label,
                            app.userId
                        ),
                        backgroundColor = colorScheme.secondaryContainer.copy(alpha = 0.8f),
                        contentColor = colorScheme.onSecondaryContainer,
                        modifier = Modifier.padding(end = 12.dp)
                    )
                }
                top.yukonga.miuix.kmp.basic.Checkbox(
                    modifier = Modifier.padding(end = 12.dp),
                    state = androidx.compose.ui.state.ToggleableState(enabled && isProxied),
                    enabled = enabled,
                    onClick = onClick.takeIf { enabled }
                )
            }
        ) {
            Text(
                text = app.label,
                modifier = Modifier.basicMarquee(),
                fontSize = top.yukonga.miuix.kmp.theme.MiuixTheme.textStyles.headline1.fontSize,
                fontWeight = FontWeight(550),
                color = colorScheme.onBackground,
                maxLines = 1,
                softWrap = false
            )
            if (showPackageName) {
                Text(
                    text = app.packageName,
                    modifier = Modifier.basicMarquee(),
                    fontSize = 12.sp,
                    fontWeight = FontWeight(550),
                    color = colorScheme.onSurfaceVariantSummary,
                    maxLines = 1,
                    softWrap = false
                )
            }
        }
    }
}
