package com.fanjv.netproxy.feature.apps.presentation

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Android
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
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
import com.fanjv.netproxy.core.ui.component.EmptyCatalog
import com.fanjv.netproxy.core.ui.component.SearchBarFake
import com.fanjv.netproxy.core.ui.component.SearchBox
import com.fanjv.netproxy.core.ui.component.SearchPager
import com.fanjv.netproxy.core.ui.component.SearchStatus
import com.fanjv.netproxy.core.ui.component.StatusTag
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
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
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.basic.rememberPullToRefreshState
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.MoreCircle
import top.yukonga.miuix.kmp.overlay.OverlayListPopup
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic


@Composable
fun AppIcon(
    packageName: String,
    userId: String,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    var icon by remember(packageName, userId) { mutableStateOf<ImageBitmap?>(null) }

    LaunchedEffect(packageName, userId) {
        icon = AppIconCache.loadIcon(context, packageName, userId)
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

    val searchStatus by viewModel.searchStatus

    LaunchedEffect(Unit) { viewModel.load() }

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

    Box(modifier = Modifier.fillMaxSize()) {
        Scaffold(
            topBar = {
                BlurredBar(backdrop) {
                    searchStatus.TopAppBarAnim(backgroundColor = barColor) {
                        TopAppBar(
                            color = barColor,
                            title = stringResource(R.string.apps),
                            scrollBehavior = scrollBehavior,
                            navigationIcon = {
                                onBack?.let { BackIconButton(onClick = it) }
                            },
                            actions = {
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
                                            text = stringResource(R.string.app_all_android_users),
                                            isSelected = apps.appAndroidUsers.isEmpty(),
                                            onSelectedIndexChange = {
                                                viewModel.selectAllAndroidUsers()
                                                showTopPopup.value = false
                                            },
                                            optionSize = 5 + apps.users.size,
                                            index = 0
                                        )
                                        apps.users.forEachIndexed { index, user ->
                                            DropdownImpl(
                                                text = stringResource(
                                                    R.string.app_android_user_scope,
                                                    user.name,
                                                    user.id
                                                ),
                                                isSelected = user.id in apps.appAndroidUsers,
                                                onSelectedIndexChange = {
                                                    viewModel.toggleAndroidUser(user.id)
                                                    showTopPopup.value = false
                                                },
                                                optionSize = 5 + apps.users.size,
                                                index = index + 1
                                            )
                                        }
                                        DropdownImpl(
                                            text = stringResource(R.string.show_system_apps),
                                            isSelected = apps.showSystemApps,
                                            onSelectedIndexChange = {
                                                viewModel.setShowSystemApps(!apps.showSystemApps)
                                                showTopPopup.value = false
                                            },
                                            optionSize = 5 + apps.users.size,
                                            index = apps.users.size + 1
                                        )
                                        DropdownImpl(
                                            text = stringResource(R.string.app_selected_first),
                                            isSelected = apps.appSelectedFirst,
                                            onSelectedIndexChange = {
                                                viewModel.setSelectedFirst(!apps.appSelectedFirst)
                                                showTopPopup.value = false
                                            },
                                            optionSize = 5 + apps.users.size,
                                            index = apps.users.size + 2
                                        )
                                        DropdownImpl(
                                            text = stringResource(R.string.app_reverse_sort),
                                            isSelected = apps.appReverseSort,
                                            onSelectedIndexChange = {
                                                viewModel.setReverseSort(!apps.appReverseSort)
                                                showTopPopup.value = false
                                            },
                                            optionSize = 5 + apps.users.size,
                                            index = apps.users.size + 3
                                        )
                                        DropdownImpl(
                                            text = stringResource(R.string.app_show_package_name),
                                            isSelected = apps.appShowPackageName,
                                            onSelectedIndexChange = {
                                                viewModel.setShowPackageName(!apps.appShowPackageName)
                                                showTopPopup.value = false
                                            },
                                            optionSize = 5 + apps.users.size,
                                            index = apps.users.size + 4
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
                            },
                            bottomContent = {
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
                        )
                    }
                }
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
                    var isRefreshing by remember { mutableStateOf(false) }
                    val pullToRefreshState = rememberPullToRefreshState()

                    LaunchedEffect(isRefreshing) {
                        if (isRefreshing) {
                            viewModel.load(force = true)
                            isRefreshing = false
                        }
                    }

                    val refreshTexts = listOf(
                        stringResource(R.string.refresh_pulling),
                        stringResource(R.string.refresh_release),
                        stringResource(R.string.refresh_refresh),
                        stringResource(R.string.refresh_complete),
                    )
                    val allApps = apps.allApps

                    PullToRefresh(
                        isRefreshing = isRefreshing,
                        onRefresh = { isRefreshing = true },
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
                                            title = stringResource(R.string.proxy_apps),
                                            summary = stringResource(R.string.proxy_mode_summary),
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
                                    items(allApps, key = AppInfoModel::packageName) { app ->
                                        AppItem(
                                            app,
                                            apps.appShowPackageName,
                                            app.isProxied,
                                            spacing,
                                            viewModel
                                        )
                                    }
                                } else {
                                    item("app_proxy_disabled") {
                                        Box(
                                            modifier = Modifier
                                                .fillMaxWidth()
                                                .height(300.dp),
                                            contentAlignment = androidx.compose.ui.Alignment.Center,
                                        ) {
                                            EmptyCatalog(
                                                text = stringResource(R.string.app_proxy_disabled_hint),
                                                onRefresh = null,
                                            )
                                        }
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

        if (apps.appProxyEnabled) {
            searchStatus.SearchPager(
                defaultResult = { },
                searchBarTopPadding = dynamicTopPadding,
            ) {
                items(apps.searchResults, key = AppInfoModel::packageName) { app ->
                    AppItem(app, apps.appShowPackageName, app.isProxied, spacing, viewModel)
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
    viewModel: AppsViewModel
) {
    val animationState = remember { Animatable(0f) }

    LaunchedEffect(Unit) {
        animationState.animateTo(
            targetValue = 1f,
            animationSpec = tween(durationMillis = 300)
        )
    }

    Card(
        modifier = Modifier
            .padding(horizontal = 12.dp)
            .padding(bottom = spacing)
            .fillMaxWidth()
            .graphicsLayer {
                val progress = animationState.value
                this.alpha = progress
                this.translationY = 50f * (1f - progress)
            }
    ) {
        BasicComponent(
            onClick = {
                viewModel.toggle(app.packageName)
            },
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
                if (app.userIds.size > 1 || app.userIds.firstOrNull() != "0") {
                    StatusTag(
                        label = stringResource(
                            R.string.app_users_label,
                            app.userIds.joinToString(", ")
                        ),
                        backgroundColor = colorScheme.secondaryContainer.copy(alpha = 0.8f),
                        contentColor = colorScheme.onSecondaryContainer,
                        modifier = Modifier.padding(end = 12.dp)
                    )
                }
                top.yukonga.miuix.kmp.basic.Checkbox(
                    modifier = Modifier.padding(end = 12.dp),
                    state = androidx.compose.ui.state.ToggleableState(isProxied),
                    onClick = {
                        viewModel.toggle(app.packageName)
                    }
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
