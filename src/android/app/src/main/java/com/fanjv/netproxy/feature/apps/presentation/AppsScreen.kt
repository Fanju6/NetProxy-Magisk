package com.fanjv.netproxy.feature.apps.presentation

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.basicMarquee
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Box
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
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInWindow
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.SearchBarFake
import com.fanjv.netproxy.core.ui.component.SearchBox
import com.fanjv.netproxy.core.ui.component.SearchPager
import com.fanjv.netproxy.core.ui.component.SearchStatus
import com.fanjv.netproxy.core.ui.component.StatusTag
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.component.rememberCommitOnLeave
import com.fanjv.netproxy.feature.apps.data.AppIconCache
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.DropdownImpl
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.ListPopupColumn
import top.yukonga.miuix.kmp.basic.ListPopupDefaults
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.PopupPositionProvider
import top.yukonga.miuix.kmp.basic.PullToRefresh
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.basic.rememberPullToRefreshState
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.MoreCircle
import top.yukonga.miuix.kmp.overlay.OverlayListPopup
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.MiuixPopupUtils.Companion.MiuixPopupHost
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic


@Composable
fun AppIcon(
    packageName: String,
    userId: String,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    val iconSizePx = with(LocalDensity.current) { 40.dp.roundToPx() }
    val iconRevision = AppIconCache.revision
    var icon by remember(packageName, userId, iconSizePx, iconRevision) {
        mutableStateOf<ImageBitmap?>(null)
    }

    LaunchedEffect(packageName, userId, iconSizePx, iconRevision) {
        icon = AppIconCache.loadIcon(context, packageName, userId, iconSizePx)
    }

    Box(
        modifier = modifier
            .requiredSize(40.dp)
            .clip(RoundedCornerShape(10.dp)),
        contentAlignment = androidx.compose.ui.Alignment.Center
    ) {
        if (icon != null) {
            androidx.compose.foundation.Image(
                bitmap = icon!!,
                contentDescription = null,
                modifier = Modifier.requiredSize(40.dp),
                contentScale = ContentScale.Fit
            )
        } else {
            Icon(
                imageVector = Icons.Rounded.Android,
                contentDescription = null,
                modifier = Modifier.requiredSize(24.dp),
                tint = colorScheme.onBackground.copy(alpha = 0.5f)
            )
        }
    }
}

/** 应用页：分应用代理的开关与应用选择。 */
@Composable
internal fun AppsScreen(
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
    viewModel: AppsViewModel = netProxyViewModel(),
    onBack: (() -> Unit)? = null
) {
    val apps by viewModel.state.collectAsStateWithLifecycle()
    val spacing = 10.dp
    val showAppControls = apps.hasLoadedApps && apps.appProxyEnabled

    val searchStatus = rememberSaveable(saver = SearchStatus.Saver) { SearchStatus("") }
    val listState = rememberLazyListState()
    val searchListState = rememberLazyListState()

    LaunchedEffect(Unit) { viewModel.load() }
    val commitOnLeave = rememberCommitOnLeave(viewModel::requestPolicyFlush)
    val leave: () -> Unit = { commitOnLeave { onBack?.invoke() } }
    BackHandler(enabled = onBack != null && searchStatus.isCollapsed() &&
        (apps.hasPendingPolicy || apps.requiresPolicyReload)) { leave() }

    val scrollBehavior = MiuixScrollBehavior()
    val dynamicTopPadding =
        remember(scrollBehavior) { { 12.dp * (1f - scrollBehavior.state.collapsedFraction) } }
    val density = LocalDensity.current
    val backdrop = rememberBlurBackdrop()
    val blurActive = backdrop != null
    val barColor = if (blurActive) Color.Transparent else colorScheme.surface

    LaunchedEffect(searchStatus.searchText) {
        viewModel.updateSearch(searchStatus.searchText)
    }
    LaunchedEffect(apps.appProxyEnabled) {
        if (!apps.appProxyEnabled) {
            searchStatus.searchText = ""
            searchStatus.current = SearchStatus.Status.COLLAPSED
        }
    }

    Box(modifier = Modifier.fillMaxSize()) {
        Scaffold(
            topBar = {
                BlurredBar(backdrop) {
                    searchStatus.TopAppBarAnim(backgroundColor = barColor) {
                        TopAppBar(
                            color = barColor,
                            title = stringResource(R.string.proxy_apps),
                            scrollBehavior = scrollBehavior,
                            navigationIcon = {
                                onBack?.let { BackIconButton(onClick = leave) }
                            },
                            actions = {
                                if (showAppControls) {
                                    val showTopPopup = remember { mutableStateOf(false) }
                                    OverlayListPopup(
                                        show = showTopPopup.value,
                                        popupPositionProvider = ListPopupDefaults.ContextMenuPositionProvider,
                                        alignment = PopupPositionProvider.Align.TopEnd,
                                        onDismissRequest = {
                                            showTopPopup.value = false
                                        }
                                    ) {
                                        ListPopupColumn {
                                            DropdownImpl(
                                                text = stringResource(R.string.show_system_apps),
                                                isSelected = apps.showSystemApps,
                                                onSelectedIndexChange = {
                                                    viewModel.setShowSystemApps(!apps.showSystemApps)
                                                    showTopPopup.value = false
                                                },
                                                optionSize = 4,
                                                index = 0
                                            )
                                            DropdownImpl(
                                                text = stringResource(R.string.app_selected_first),
                                                isSelected = apps.appSelectedFirst,
                                                onSelectedIndexChange = {
                                                    viewModel.setSelectedFirst(!apps.appSelectedFirst)
                                                    showTopPopup.value = false
                                                },
                                                optionSize = 4,
                                                index = 1
                                            )
                                            DropdownImpl(
                                                text = stringResource(R.string.app_reverse_sort),
                                                isSelected = apps.appReverseSort,
                                                onSelectedIndexChange = {
                                                    viewModel.setReverseSort(!apps.appReverseSort)
                                                    showTopPopup.value = false
                                                },
                                                optionSize = 4,
                                                index = 2
                                            )
                                            DropdownImpl(
                                                text = stringResource(R.string.app_show_package_name),
                                                isSelected = apps.appShowPackageName,
                                                onSelectedIndexChange = {
                                                    viewModel.setShowPackageName(!apps.appShowPackageName)
                                                    showTopPopup.value = false
                                                },
                                                optionSize = 4,
                                                index = 3
                                            )
                                        }
                                    }
                                    IconButton(
                                        onClick = { showTopPopup.value = true },
                                        holdDownState = showTopPopup.value
                                    ) {
                                        Icon(
                                            imageVector = MiuixIcons.MoreCircle,
                                            contentDescription = null,
                                            tint = colorScheme.onSurface
                                        )
                                    }
                                }
                            },
                            bottomContent = {
                                if (showAppControls) {
                                    Box(
                                        modifier = Modifier
                                            .alpha(if (searchStatus.isCollapsed()) 1f else 0f)
                                            .onGloballyPositioned { coordinates ->
                                                with(density) {
                                                    searchStatus.offsetY =
                                                        coordinates.positionInWindow().y.toDp()
                                                }
                                            }
                                            .then(
                                                if (searchStatus.isCollapsed()) {
                                                    Modifier.pointerInput(Unit) {
                                                        detectTapGestures {
                                                            searchStatus.current =
                                                                SearchStatus.Status.EXPANDING
                                                        }
                                                    }
                                                } else {
                                                    Modifier
                                                }
                                            )
                                    ) {
                                        SearchBarFake(searchStatus.label, dynamicTopPadding)
                                    }
                                }
                            }
                        )
                    }
                }
            },
            popupHost = {
                if (showAppControls) {
                    val searchResults = remember(
                        apps.searchResults, apps.proxiedApps, apps.appSelectedFirst, apps.appReverseSort
                    ) { apps.orderedApps(apps.searchResults) }
                    searchStatus.SearchPager(
                        empty = apps.hasLoadedApps && !apps.isFilteringApps &&
                            apps.appSearchQuery.isNotBlank() && apps.searchResults.isEmpty(),
                        listState = searchListState,
                        searchBarTopPadding = dynamicTopPadding,
                    ) {
                        if (apps.error.isNotBlank() || apps.requiresPolicyReload) item("error") {
                            if (apps.error.isNotBlank()) Text(apps.error, Modifier.padding(14.dp), color = colorScheme.error)
                            if (apps.requiresPolicyReload) TextButton(stringResource(R.string.routing_reload_draft),
                                onClick = viewModel::discardPolicyAndReload)
                        }
                        items(
                            items = searchResults,
                            key = AppInfoModel::id,
                            contentType = { "app_item" },
                        ) { app ->
                            AppItem(app, apps.appShowPackageName, app.id in apps.proxiedApps, spacing) {
                                if (apps.appSelectedFirst) {
                                    // 重排时保持视口位置，不跟随已选条目的 key 上移。
                                    searchListState.requestScrollToItem(
                                        searchListState.firstVisibleItemIndex,
                                        searchListState.firstVisibleItemScrollOffset
                                    )
                                }
                                viewModel.toggle(app.id)
                            }
                        }
                    }
                }
                MiuixPopupHost()
            },
            contentWindowInsets = WindowInsets.systemBars.add(WindowInsets.displayCutout)
                .only(WindowInsetsSides.Horizontal)
        ) { innerPadding ->
            val layoutDirection = LocalLayoutDirection.current
            searchStatus.SearchBox {
                if (!apps.hasLoadedApps || (apps.isLoadingApps && apps.allApps.isEmpty())) {
                    Box(
                        modifier = Modifier
                            .fillMaxSize()
                            .padding(
                                top = innerPadding.calculateTopPadding(),
                                start = innerPadding.calculateStartPadding(layoutDirection),
                                end = innerPadding.calculateEndPadding(layoutDirection),
                                bottom = bottomPadding
                            )
                            .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier),
                        contentAlignment = androidx.compose.ui.Alignment.Center
                    ) {
                        InfiniteProgressIndicator()
                    }
                } else {
                    val pullToRefreshState = rememberPullToRefreshState()

                    val refreshTexts = listOf(
                        stringResource(R.string.refresh_pulling),
                        stringResource(R.string.refresh_release),
                        stringResource(R.string.refresh_refresh),
                        stringResource(R.string.refresh_complete),
                    )
                    val allApps = remember(
                        apps.allApps, apps.proxiedApps, apps.appSelectedFirst, apps.appReverseSort
                    ) { apps.orderedApps() }

                    PullToRefresh(
                        isRefreshing = apps.isLoadingApps,
                        onRefresh = { viewModel.load(force = true) },
                        pullToRefreshState = pullToRefreshState,
                        refreshTexts = refreshTexts,
                        contentPadding = PaddingValues(
                            top = innerPadding.calculateTopPadding(),
                            start = innerPadding.calculateStartPadding(layoutDirection),
                            end = innerPadding.calculateEndPadding(layoutDirection)
                        )
                    ) {
                        Box(modifier = if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier) {
                            LazyColumn(
                                state = listState,
                                modifier = Modifier
                                    .fillMaxHeight()
                                    .scrollEndHaptic()
                                    .overScrollVertical()
                                    .nestedScroll(scrollBehavior.nestedScrollConnection),
                                contentPadding = PaddingValues(
                                    top = innerPadding.calculateTopPadding(),
                                    start = innerPadding.calculateStartPadding(layoutDirection),
                                    end = innerPadding.calculateEndPadding(layoutDirection)
                                ),
                                overscrollEffect = null,
                            ) {
                                if (apps.error.isNotBlank()) item("error") {
                                    Text(apps.error, Modifier.padding(14.dp), color = colorScheme.error)
                                }
                                if (apps.requiresPolicyReload) item("reload") {
                                    TextButton(stringResource(R.string.routing_reload_draft),
                                        onClick = viewModel::discardPolicyAndReload)
                                }
                                item {
                                    Card(
                                        modifier = Modifier
                                            .padding(horizontal = 12.dp)
                                            .padding(bottom = spacing)
                                            .fillMaxWidth(),
                                    ) {
                                        val modes = listOf(
                                            stringResource(R.string.proxy_mode_off),
                                            stringResource(R.string.app_proxy_mode_blacklist),
                                            stringResource(R.string.app_proxy_mode_whitelist)
                                        )
                                        val selectedIndex = if (!apps.appProxyEnabled) 0
                                        else if (apps.appProxyMode == "blacklist") 1
                                        else 2

                                        OverlayDropdownPreference(
                                            title = stringResource(R.string.app_proxy_mode),
                                            items = modes,
                                            selectedIndex = selectedIndex,
                                            onSelectedIndexChange = { index ->
                                                if (index == 0) {
                                                    viewModel.setProxySettings(enabled = false)
                                                } else {
                                                    val mode =
                                                        if (index == 1) "blacklist" else "whitelist"
                                                    viewModel.setProxySettings(
                                                        enabled = true,
                                                        mode = mode
                                                    )
                                                }
                                            }
                                        )
                                    }
                                }

                                if (apps.appProxyEnabled) {
                                    items(
                                        items = allApps,
                                        key = AppInfoModel::id,
                                        contentType = { "app_item" },
                                    ) { app ->
                                        AppItem(
                                            app,
                                            apps.appShowPackageName,
                                            app.id in apps.proxiedApps,
                                            spacing,
                                        ) { viewModel.toggle(app.id) }
                                    }
                                }

                                item {
                                    Spacer(Modifier.height(bottomPadding + 80.dp))
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}


@Composable
private fun AppItem(
    app: AppInfoModel,
    showPackageName: Boolean,
    isProxied: Boolean,
    spacing: androidx.compose.ui.unit.Dp,
    onClick: () -> Unit
) {
    Card(
        modifier = Modifier
            .padding(horizontal = 12.dp)
            .padding(bottom = spacing)
            .fillMaxWidth()
    ) {
        BasicComponent(
            onClick = onClick,
            startAction = {
                Box(
                    modifier = Modifier
                        .requiredWidth(56.dp)
                        .padding(start = 12.dp)
                ) {
                    AppIcon(
                        packageName = app.packageName,
                        userId = app.userId,
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
                    state = androidx.compose.ui.state.ToggleableState(isProxied),
                    onClick = onClick
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
