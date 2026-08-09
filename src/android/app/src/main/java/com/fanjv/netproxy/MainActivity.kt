package com.fanjv.netproxy

import android.os.Bundle
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.navigation3.rememberViewModelStoreNavEntryDecorator
import androidx.navigation3.runtime.entryProvider
import androidx.navigation3.runtime.rememberSaveableStateHolderNavEntryDecorator
import androidx.navigation3.ui.NavDisplay
import androidx.navigationevent.NavigationEventInfo
import androidx.navigationevent.compose.NavigationBackHandler
import androidx.navigationevent.compose.rememberNavigationEventState
import com.fanjv.netproxy.core.di.netProxyViewModel
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
import com.fanjv.netproxy.feature.settings.presentation.ProxySettingsScreen
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
import com.fanjv.netproxy.navigation.Route.ProxySettings
import com.fanjv.netproxy.navigation.Route.SubscriptionDetails
import com.fanjv.netproxy.navigation.Route.SubscriptionEdit
import com.fanjv.netproxy.navigation.Route.ThemeSettings
import com.fanjv.netproxy.navigation.rememberMainPagerState
import com.fanjv.netproxy.navigation.rememberNavigator
import kotlinx.coroutines.delay
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.NavigationItem
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.theme.ThemeColorSpec
import top.yukonga.miuix.kmp.theme.ThemePaletteStyle

class MainActivity : ComponentActivity() {
    private val alphaExpired = mutableStateOf(AlphaExpiry.isExpired())

    override fun onCreate(savedInstanceState: Bundle?) {
        setTheme(R.style.Theme_NetProxy)
        super.onCreate(savedInstanceState)

        WindowCompat.setDecorFitsSystemWindows(window, false)
        window.attributes = window.attributes.apply {
            layoutInDisplayCutoutMode =
                WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_ALWAYS
        }
        window.isNavigationBarContrastEnforced = false

        setContent {
            val isExpired by alphaExpired
            val themeViewModel: ThemeViewModel = netProxyViewModel()

            LaunchedEffect(isExpired) {
                if (!isExpired) {
                    val remaining = BuildConfig.ALPHA_EXPIRES_AT_MILLIS -
                            System.currentTimeMillis()
                    if (remaining > 0) delay(remaining)
                    alphaExpired.value = true
                }
            }
            val themeState by themeViewModel.state.collectAsStateWithLifecycle()
            val paletteStyle = runCatching { ThemePaletteStyle.valueOf(themeState.colorStyle) }
                .getOrDefault(ThemePaletteStyle.TonalSpot)
            val colorSpec = runCatching { ThemeColorSpec.valueOf(themeState.colorSpec) }
                .getOrDefault(ThemeColorSpec.Spec2021)
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
                enableFloatingBottomBar = themeState.enableFloatingBottomBar,
                enableFloatingBottomBarBlur = themeState.enableFloatingBottomBarBlur,
            )
            val darkMode = appThemeSettings.colorMode.isDark ||
                    (appThemeSettings.colorMode.isSystem && androidx.compose.foundation.isSystemInDarkTheme())

            SideEffect {
                WindowInsetsControllerCompat(window, window.decorView).apply {
                    isAppearanceLightStatusBars = !darkMode
                    isAppearanceLightNavigationBars = !darkMode
                }
                window.isNavigationBarContrastEnforced = false
            }

            CompositionLocalProvider(LocalDensity provides density) {
                NetProxyTheme(appThemeSettings = appThemeSettings) {
                    if (isExpired) {
                        AlphaExpiredScreen(onExit = ::finishAndRemoveTask)
                    } else {
                        NetProxyApp(themeViewModel)
                    }
                }
            }
        }
    }

    override fun onResume() {
        super.onResume()
        alphaExpired.value = AlphaExpiry.isExpired()
    }
}

@Composable
private fun AlphaExpiredScreen(onExit: () -> Unit) {
    BackHandler(enabled = true) {}

    Scaffold {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 32.dp),
            contentAlignment = Alignment.Center,
        ) {
            Column(
                modifier = Modifier.fillMaxWidth(),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text(
                    text = stringResource(R.string.alpha_expired_title),
                    color = colorScheme.onBackground,
                    fontSize = 24.sp,
                    fontWeight = FontWeight.Bold,
                )
                Spacer(Modifier.height(12.dp))
                Text(
                    text = stringResource(R.string.alpha_expired_message),
                    color = colorScheme.onSurfaceVariantSummary,
                    fontSize = 15.sp,
                    textAlign = TextAlign.Center,
                )
                Spacer(Modifier.height(24.dp))
                TextButton(
                    text = stringResource(R.string.alpha_expired_exit),
                    onClick = onExit,
                    modifier = Modifier.fillMaxWidth(),
                    colors = ButtonDefaults.textButtonColorsPrimary(),
                )
            }
        }
    }
}

@Composable
internal fun NetProxyApp(themeViewModel: ThemeViewModel) {
    val navigator = rememberNavigator(Main)
    val catalogNodesViewModel: CatalogNodesViewModel = netProxyViewModel()

    CompositionLocalProvider(
        LocalNavigator provides navigator
    ) {
        Scaffold {
            NavDisplay(
                backStack = navigator.backStack,
                entryDecorators = listOf(
                    rememberSaveableStateHolderNavEntryDecorator(),
                    rememberViewModelStoreNavEntryDecorator()
                ),
                onBack = { navigator.pop() },
                entryProvider = entryProvider {
                    entry<Main> { MainScreen(themeViewModel, catalogNodesViewModel) }
                    entry<Apps> {
                        AppsScreen(
                            onBack = { navigator.pop() }
                        )
                    }
                    entry<SubscriptionDetails> {
                        SubscriptionDetailsScreen(
                            id = it.id,
                            onBack = { navigator.pop() }
                        )
                    }
                    entry<SubscriptionEdit> {
                        SubscriptionEditorScreen(
                            id = it.id,
                            onBack = { navigator.pop() }
                        )
                    }
                    entry<NodeEdit> {
                        SingBoxNodeEditScreen(
                            viewModel = catalogNodesViewModel,
                            nodeRef = it.nodeRef,
                            onBack = { navigator.pop() }
                        )
                    }
                    entry<ProxySettings> {
                        ProxySettingsScreen(
                            onBack = { navigator.pop() },
                            bottomPadding = 0.dp
                        )
                    }
                    entry<KernelSettings> {
                        SingBoxKernelSettingsScreen(
                            onBack = { navigator.pop() }
                        )
                    }
                    entry<JsonEdit> {
                        SingBoxJsonEditScreen(
                            documentId = it.documentId,
                            onBack = { navigator.pop() }
                        )
                    }
                    entry<ThemeSettings> { ThemeSettingsScreen(viewModel = themeViewModel) }
                    entry<About> { AboutScreen() }
                    entry<Logs> {
                        LogsScreen(
                            onBack = { navigator.pop() }
                        )
                    }
                }

            )
        }
    }
}

@Composable
internal fun MainScreen(
    themeViewModel: ThemeViewModel,
    catalogNodesViewModel: CatalogNodesViewModel
) {
    val themeState by themeViewModel.state.collectAsStateWithLifecycle()
    val destinations = AppDestination.entries
    val pagerState = rememberPagerState(initialPage = 0, pageCount = { destinations.size })
    val mainPagerState = rememberMainPagerState(pagerState)
    val enableFloatingBottomBar = themeState.enableFloatingBottomBar
    val enableFloatingBottomBarBlur = themeState.enableFloatingBottomBarBlur

    // 目的地列表变化时，确保 selectedPage 不越界
    LaunchedEffect(destinations) {
        if (mainPagerState.selectedPage >= destinations.size) {
            mainPagerState.animateToPage((destinations.size - 1).coerceAtLeast(0))
        }
    }

    // 非动画态（如滑动）时与 pager 同步；
    // 导航动画结束后也重跑一次，否则点击导航可能跳过目的地页的首次按需加载。
    LaunchedEffect(mainPagerState.pagerState.currentPage, mainPagerState.isNavigating) {
        mainPagerState.syncPage()
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
            Box(modifier = Modifier.fillMaxWidth()) {
                MainBottomBar(
                    mainState = mainPagerState,
                    blurBackdrop = blurBackdrop,
                    items = navItems,
                    enableFloatingBottomBar = enableFloatingBottomBar,
                    enableFloatingBottomBarBlur = enableFloatingBottomBarBlur,
                    modifier = Modifier.align(Alignment.BottomCenter),
                )
            }
        }
    ) { innerPadding ->
        val bottomPadding = innerPadding.calculateBottomPadding()

        Box(modifier = Modifier) {
            HorizontalPager(
                modifier = Modifier
                    .fillMaxSize()
                    .then(
                        if (blurBackdrop != null && (enableFloatingBottomBar && enableFloatingBottomBarBlur || !enableFloatingBottomBar)) {
                            Modifier.layerBackdrop(blurBackdrop)
                        } else {
                            Modifier
                        }
                    ),
                state = pagerState,
                beyondViewportPageCount = 3,
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
