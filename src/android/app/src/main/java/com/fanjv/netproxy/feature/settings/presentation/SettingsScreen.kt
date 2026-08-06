package com.fanjv.netproxy.feature.settings.presentation

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.AppRegistration
import androidx.compose.material.icons.rounded.BugReport
import androidx.compose.material.icons.rounded.ContactPage
import androidx.compose.material.icons.rounded.Memory
import androidx.compose.material.icons.rounded.Palette
import androidx.compose.material.icons.rounded.PowerSettingsNew
import androidx.compose.material.icons.rounded.Router
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.groupedCardItems
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

/** 设置页：模块开关与各功能入口。 */
@Composable
internal fun SettingsScreen(
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
    isActive: Boolean = true,
    viewModel: SettingsViewModel = netProxyViewModel()
) {
    val settings by viewModel.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    val context = LocalContext.current
    val resources = LocalResources.current

    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val blurActive = backdrop != null
    val barColor = if (blurActive) Color.Transparent else colorScheme.surface

    DisposableEffect(isActive) {
        viewModel.setVisible(isActive)
        onDispose { if (isActive) viewModel.setVisible(false) }
    }


    Scaffold(
        topBar = {
            BlurredBar(backdrop) {
                TopAppBar(
                    color = barColor,
                    title = stringResource(R.string.settings),
                    scrollBehavior = scrollBehavior
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.only(WindowInsetsSides.Horizontal)
    ) { innerPadding ->
        Box(modifier = if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier) {
            LazyColumn(
                modifier = Modifier
                    .fillMaxHeight()
                    .scrollEndHaptic()
                    .overScrollVertical()
                    .nestedScroll(scrollBehavior.nestedScrollConnection)
                    .padding(horizontal = 12.dp),
                contentPadding = innerPadding,
                overscrollEffect = null,
            ) {
                groupedCardItems(
                    keyPrefix = "settings_entries",
                    outerTopPadding = 12.dp,
                    items = listOf(
                        CardItem("proxy") {
                            ArrowPreference(
                                title = stringResource(R.string.proxy_settings),
                                summary = stringResource(R.string.proxy_settings_summary),
                                startAction = {
                                    Icon(
                                        imageVector = Icons.Rounded.Router,
                                        contentDescription = null,
                                        modifier = Modifier.padding(end = 6.dp),
                                        tint = colorScheme.onBackground
                                    )
                                },
                                onClick = { navigator.push(Route.ProxySettings) }
                            )
                        },
                        CardItem("kernel") {
                            ArrowPreference(
                                title = stringResource(R.string.kernel_settings),
                                summary = stringResource(R.string.kernel_settings_summary),
                                startAction = {
                                    Icon(
                                        imageVector = Icons.Rounded.Memory,
                                        contentDescription = null,
                                        modifier = Modifier.padding(end = 6.dp),
                                        tint = colorScheme.onBackground
                                    )
                                },
                                onClick = { navigator.push(Route.KernelSettings) }
                            )
                        },
                        CardItem("apps") {
                            ArrowPreference(
                                title = stringResource(R.string.proxy_apps),
                                summary = stringResource(R.string.proxy_mode_summary),
                                startAction = {
                                    Icon(
                                        imageVector = Icons.Rounded.AppRegistration,
                                        contentDescription = null,
                                        modifier = Modifier.padding(end = 6.dp),
                                        tint = colorScheme.onBackground
                                    )
                                },
                                onClick = { navigator.push(Route.Apps) }
                            )
                        },
                    ),
                )

                groupedCardItems(
                    keyPrefix = "settings_module",
                    outerTopPadding = 12.dp,
                    items = listOf(
                        CardItem("autoStart") {
                            SwitchPreference(
                                title = stringResource(R.string.auto_start),
                                summary = stringResource(R.string.auto_start_summary),
                                startAction = {
                                    Icon(
                                        imageVector = Icons.Rounded.PowerSettingsNew,
                                        contentDescription = null,
                                        modifier = Modifier.padding(end = 6.dp),
                                        tint = colorScheme.onBackground
                                    )
                                },
                                checked = settings.autoStartEnabled,
                                onCheckedChange = { viewModel.setAutoStartEnabled(it) }
                            )
                        },
                    ),
                )

                groupedCardItems(
                    keyPrefix = "settings_theme",
                    outerTopPadding = 12.dp,
                    items = listOf(
                        CardItem("theme") {
                            ArrowPreference(
                                title = stringResource(R.string.settings_theme),
                                summary = stringResource(R.string.settings_theme_summary),
                                startAction = {
                                    Icon(
                                        Icons.Rounded.Palette,
                                        modifier = Modifier.padding(end = 6.dp),
                                        contentDescription = null,
                                        tint = colorScheme.onBackground
                                    )
                                },
                                onClick = { navigator.push(Route.ThemeSettings) }
                            )
                        },
                    ),
                )

                groupedCardItems(
                    keyPrefix = "settings_support",
                    outerTopPadding = 12.dp,
                    outerBottomPadding = 12.dp,
                    items = listOf(
                        CardItem("logs") {
                            ArrowPreference(
                                title = stringResource(R.string.logs),
                                startAction = {
                                    Icon(
                                        imageVector = Icons.Rounded.BugReport,
                                        contentDescription = null,
                                        modifier = Modifier.padding(end = 6.dp),
                                        tint = colorScheme.onBackground
                                    )
                                },
                                onClick = { navigator.push(Route.Logs) }
                            )
                        },
                        CardItem("about") {
                            ArrowPreference(
                                title = stringResource(R.string.about),
                                startAction = {
                                    Icon(
                                        imageVector = Icons.Rounded.ContactPage,
                                        contentDescription = null,
                                        modifier = Modifier.padding(end = 6.dp),
                                        tint = colorScheme.onBackground
                                    )
                                },
                                onClick = { navigator.push(Route.About) }
                            )
                        },
                    ),
                )
                item {
                    Spacer(Modifier.height(80.dp + bottomPadding))
                }
            }
        }
    }
}
