package com.fanjv.netproxy

import android.os.Build
import android.os.Bundle
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.navigationevent.NavigationEventInfo
import androidx.navigationevent.compose.NavigationBackHandler
import androidx.navigationevent.compose.rememberNavigationEventState
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.module.ModuleAccessViewModel
import com.fanjv.netproxy.core.ui.component.LocalModuleAvailability
import com.fanjv.netproxy.core.ui.component.ModulePage
import com.fanjv.netproxy.core.ui.theme.AppThemeDefaults
import com.fanjv.netproxy.core.ui.theme.AppThemeSettings
import com.fanjv.netproxy.core.ui.theme.ColorMode
import com.fanjv.netproxy.core.ui.theme.NetProxyTheme
import com.fanjv.netproxy.feature.about.presentation.AboutScreen
import com.fanjv.netproxy.feature.apps.presentation.AppsScreen
import com.fanjv.netproxy.feature.catalog.presentation.nodes.CatalogNodesViewModel
import com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.SingBoxNodeEditScreen
import com.fanjv.netproxy.feature.catalog.presentation.nodes.list.CatalogNodesScreen
import com.fanjv.netproxy.feature.catalog.presentation.subscriptions.SubscriptionDetailsScreen
import com.fanjv.netproxy.feature.catalog.presentation.subscriptions.SubscriptionEditorScreen
import com.fanjv.netproxy.feature.catalog.presentation.subscriptions.SubscriptionsScreen
import com.fanjv.netproxy.feature.dashboard.presentation.CatalogDashboardScreen
import com.fanjv.netproxy.feature.kernel.presentation.SingBoxJsonEditScreen
import com.fanjv.netproxy.feature.kernel.presentation.SingBoxKernelSettingsScreen
import com.fanjv.netproxy.feature.logs.presentation.LogsScreen
import com.fanjv.netproxy.feature.inbound.presentation.InboundSettingsScreen
import com.fanjv.netproxy.feature.settings.presentation.NetworkMatchingScreen
import com.fanjv.netproxy.feature.routing.presentation.RoutingRulesScreen
import com.fanjv.netproxy.feature.settings.presentation.SettingsScreen
import com.fanjv.netproxy.feature.theme.presentation.ThemeSettingsScreen
import com.fanjv.netproxy.feature.theme.presentation.ThemeViewModel
import com.fanjv.netproxy.navigation.AppDestination
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.MainBottomBar
import com.fanjv.netproxy.navigation.MainPagerState
import com.fanjv.netproxy.navigation.Route.About
import com.fanjv.netproxy.navigation.Route.Apps
import com.fanjv.netproxy.navigation.Route.JsonEdit
import com.fanjv.netproxy.navigation.Route.KernelSettings
import com.fanjv.netproxy.navigation.Route.Logs
import com.fanjv.netproxy.navigation.Route.Main
import com.fanjv.netproxy.navigation.Route.NodeEdit
import com.fanjv.netproxy.navigation.Route.InboundSettings
import com.fanjv.netproxy.navigation.Route.NetworkMatching
import com.fanjv.netproxy.navigation.Route.RoutingRules
import com.fanjv.netproxy.navigation.Route.SubscriptionDetails
import com.fanjv.netproxy.navigation.Route.SubscriptionEdit
import com.fanjv.netproxy.navigation.Route.ThemeSettings
import com.fanjv.netproxy.navigation.rememberMainPagerState
import com.fanjv.netproxy.navigation.rememberNavigator
import com.fanjv.netproxy.navigation.mainDestinations
import com.fanjv.netproxy.navigation.availableIn
import top.yukonga.miuix.kmp.basic.NavigationItem
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.nav.core.NavDisplay
import top.yukonga.miuix.kmp.theme.ThemeColorSpec
import top.yukonga.miuix.kmp.theme.ThemePaletteStyle

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        setTheme(R.style.Theme_NetProxy)
        super.onCreate(savedInstanceState)

        WindowCompat.setDecorFitsSystemWindows(window, false)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            window.attributes = window.attributes.apply {
                layoutInDisplayCutoutMode = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                    WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_ALWAYS
                } else {
                    WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES
                }
            }
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            window.isNavigationBarContrastEnforced = false
        }

        setContent {
            val themeViewModel: ThemeViewModel = netProxyViewModel()

            val themeState by themeViewModel.state.collectAsStateWithLifecycle()
            val paletteStyle = runCatching { ThemePaletteStyle.valueOf(themeState.colorStyle) }
                .getOrDefault(AppThemeDefaults.paletteStyle)
            val colorSpec = runCatching { ThemeColorSpec.valueOf(themeState.colorSpec) }
                .getOrDefault(AppThemeDefaults.colorSpec)
            val systemDensity = LocalDensity.current
            val density = remember(systemDensity, themeState.pageScale) {
                Density(systemDensity.density * themeState.pageScale, systemDensity.fontScale)
            }
            val appThemeSettings = AppThemeSettings(
                colorMode = ColorMode.fromValue(themeState.colorMode),
                miuixMonet = themeState.miuixMonet,
                keyColor = themeState.keyColor,
                paletteStyle = paletteStyle,
                colorSpec = colorSpec,
                enableSmoothCorner = themeState.enableSmoothCorner,
                enableBlur = themeState.enableBlur,
                enablePredictiveBack = themeState.enablePredictiveBack,
            )
            val darkMode = appThemeSettings.colorMode.isDark ||
                    (appThemeSettings.colorMode.isSystem && androidx.compose.foundation.isSystemInDarkTheme())

            SideEffect {
                WindowInsetsControllerCompat(window, window.decorView).apply {
                    isAppearanceLightStatusBars = !darkMode
                    isAppearanceLightNavigationBars = !darkMode
                }
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    window.isNavigationBarContrastEnforced = false
                }
            }

            CompositionLocalProvider(LocalDensity provides density) {
                NetProxyTheme(appThemeSettings = appThemeSettings) {
                    NetProxyApp(themeViewModel)
                }
            }
        }
    }
}

@Composable
internal fun NetProxyApp(themeViewModel: ThemeViewModel) {
    val navigator = rememberNavigator(Main)
    val catalogNodesViewModel: CatalogNodesViewModel = netProxyViewModel()
    val moduleAccess: ModuleAccessViewModel = netProxyViewModel()
    val availability by moduleAccess.state.collectAsStateWithLifecycle()

    LifecycleResumeEffect(Unit) {
        moduleAccess.refresh()
        onPauseOrDispose { }
    }

    CompositionLocalProvider(
        LocalNavigator provides navigator,
        LocalModuleAvailability provides availability,
    ) {
        Scaffold {
            NavDisplay(
                backStack = navigator.backStack,
                onBack = { navigator.pop() },
            ) {
                entry<Main> { MainScreen(themeViewModel, catalogNodesViewModel) }
                entry<Apps> {
                    ModulePage(R.string.proxy_apps, R.string.apps_read_failed, { navigator.pop() }) {
                        AppsScreen(onBack = { navigator.pop() })
                    }
                }
                entry<SubscriptionDetails> {
                    ModulePage(R.string.subscriptions, R.string.subscriptions_read_failed, { navigator.pop() }) {
                        SubscriptionDetailsScreen(
                            id = it.id,
                            onBack = { navigator.pop() }
                        )
                    }
                }
                entry<SubscriptionEdit> {
                    ModulePage(R.string.subscriptions, R.string.subscriptions_read_failed, { navigator.pop() }) {
                        SubscriptionEditorScreen(
                            id = it.id,
                            onBack = { navigator.pop() }
                        )
                    }
                }
                entry<NodeEdit> {
                    ModulePage(R.string.node_edit, R.string.nodes_read_failed, { navigator.pop() }) {
                        SingBoxNodeEditScreen(
                            viewModel = catalogNodesViewModel,
                            nodeRef = it.nodeRef,
                            onBack = { navigator.pop() }
                        )
                    }
                }
                entry<InboundSettings> {
                    ModulePage(R.string.inbound_settings, R.string.inbound_read_failed, { navigator.pop() }) {
                        InboundSettingsScreen(
                            onBack = { navigator.pop() },
                            bottomPadding = 0.dp
                        )
                    }
                }
                entry<NetworkMatching> {
                    ModulePage(R.string.network_matching, R.string.network_read_failed, { navigator.pop() }) {
                        NetworkMatchingScreen(onBack = { navigator.pop() })
                    }
                }
                entry<RoutingRules> {
                    ModulePage(R.string.routing_rules, R.string.routing_read_failed, { navigator.pop() }) {
                        RoutingRulesScreen(onBack = { navigator.pop() })
                    }
                }
                entry<KernelSettings> {
                    ModulePage(R.string.kernel_settings, R.string.singbox_documents_load_failed, { navigator.pop() }) {
                        SingBoxKernelSettingsScreen(
                            onBack = { navigator.pop() }
                        )
                    }
                }
                entry<JsonEdit> {
                    ModulePage(R.string.kernel_settings, R.string.singbox_documents_load_failed, { navigator.pop() }) {
                        SingBoxJsonEditScreen(
                            documentId = it.documentId,
                            onBack = { navigator.pop() }
                        )
                    }
                }
                entry<ThemeSettings> { ThemeSettingsScreen(viewModel = themeViewModel) }
                entry<About> { AboutScreen() }
                entry<Logs> {
                    ModulePage(R.string.logs, R.string.logs_read_failed, { navigator.pop() }) {
                        LogsScreen(onBack = { navigator.pop() })
                    }
                }
            }
        }
    }
}

@Composable
internal fun MainScreen(
    themeViewModel: ThemeViewModel,
    catalogNodesViewModel: CatalogNodesViewModel
) {
    val availability = LocalModuleAvailability.current
    var destination by rememberSaveable { mutableStateOf(AppDestination.Dashboard) }
    if (availability == null) {
        CatalogDashboardScreen()
        return
    }
    val destinations = mainDestinations(availability.available)
    // 权限改变会改变页码；保存目的地身份，避免“设置”被映射为“节点”。
    key(destinations) {
        MainPager(themeViewModel, catalogNodesViewModel, destinations,
            initialDestination = destination.availableIn(destinations),
            onDestinationChanged = { destination = it })
    }
}

@Composable
private fun MainPager(
    themeViewModel: ThemeViewModel,
    catalogNodesViewModel: CatalogNodesViewModel,
    destinations: List<AppDestination>,
    initialDestination: AppDestination,
    onDestinationChanged: (AppDestination) -> Unit,
) {
    val themeState by themeViewModel.state.collectAsStateWithLifecycle()
    val pagerState = rememberPagerState(initialPage = destinations.indexOf(initialDestination),
        pageCount = { destinations.size })
    val mainPagerState = rememberMainPagerState(pagerState)

    // 非动画态（如滑动）时与 pager 同步；
    // 导航动画结束后也重跑一次，否则点击导航可能跳过目的地页的首次按需加载。
    LaunchedEffect(mainPagerState.pagerState.currentPage, mainPagerState.isNavigating) {
        mainPagerState.syncPage()
        onDestinationChanged(destinations[mainPagerState.selectedPage])
    }

    val navItems = destinations.map {
        NavigationItem(
            label = stringResource(it.labelRes),
            icon = it.icon
        )
    }

    val navigator = LocalNavigator.current

    MainScreenBackHandler(
        mainState = mainPagerState,
        navigator = navigator,
        enablePredictiveBack = themeState.enablePredictiveBack,
    )

    val blurBackdrop = com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop()

    Scaffold(
        bottomBar = {
            MainBottomBar(
                mainState = mainPagerState,
                blurBackdrop = blurBackdrop,
                items = navItems,
            )
        }
    ) { innerPadding ->
        val bottomPadding = innerPadding.calculateBottomPadding()

        Box(modifier = Modifier) {
            HorizontalPager(
                modifier = Modifier
                    .fillMaxSize()
                    .then(
                        if (blurBackdrop != null) {
                            Modifier.layerBackdrop(blurBackdrop)
                        } else {
                            Modifier
                        }
                    ),
                state = pagerState,
                key = { destinations[it] },
                beyondViewportPageCount = 1,
                userScrollEnabled = destinations.size > 1
            ) { pageIndex ->
                when (destinations[pageIndex]) {
                    AppDestination.Dashboard -> CatalogDashboardScreen(
                        bottomPadding = bottomPadding,
                        isActive = mainPagerState.selectedPage == pageIndex,
                        onNavigateToNodes = {
                            val nodesIndex = destinations.indexOf(AppDestination.Nodes)
                            if (nodesIndex != -1) {
                                mainPagerState.animateToPage(nodesIndex)
                            }
                        }
                    )

                    AppDestination.Nodes -> CatalogNodesScreen(
                        bottomPadding = bottomPadding,
                        isActive = mainPagerState.selectedPage == pageIndex,
                        viewModel = catalogNodesViewModel
                    )

                    AppDestination.Subscriptions -> SubscriptionsScreen(
                        bottomPadding = bottomPadding,
                        isActive = mainPagerState.selectedPage == pageIndex
                    )

                    AppDestination.Settings -> SettingsScreen(
                        bottomPadding = bottomPadding,
                        isActive = mainPagerState.selectedPage == pageIndex
                    )
                }
            }
        }
    }
}

@Composable
private fun MainScreenBackHandler(
    mainState: MainPagerState,
    navigator: com.fanjv.netproxy.navigation.Navigator,
    enablePredictiveBack: Boolean,
) {
    val isPagerBackHandlerEnabled by remember {
        derivedStateOf {
            navigator.backStack.lastOrNull() is Main &&
                    navigator.backStack.size == 1 &&
                    mainState.selectedPage != 0
        }
    }

    if (enablePredictiveBack) {
        val navEventState = rememberNavigationEventState(NavigationEventInfo.None)
        NavigationBackHandler(
            state = navEventState,
            isBackEnabled = isPagerBackHandlerEnabled,
            onBackCompleted = {
                mainState.animateToPage(0)
            }
        )
    } else {
        BackHandler(enabled = isPagerBackHandlerEnabled) {
            mainState.animateToPage(0)
        }
    }
}
