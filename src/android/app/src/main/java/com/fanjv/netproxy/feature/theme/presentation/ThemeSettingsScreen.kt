package com.fanjv.netproxy.feature.theme.presentation

import android.annotation.SuppressLint
import android.os.Build
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.add
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.captionBar
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.MenuOpen
import androidx.compose.material.icons.rounded.AspectRatio
import androidx.compose.material.icons.rounded.BlurOn
import androidx.compose.material.icons.rounded.CallToAction
import androidx.compose.material.icons.rounded.Colorize
import androidx.compose.material.icons.rounded.Palette
import androidx.compose.material.icons.rounded.RoundedCorner
import androidx.compose.material.icons.rounded.Style
import androidx.compose.material.icons.rounded.Wallpaper
import androidx.compose.material.icons.rounded.WaterDrop
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.groupedCardItems
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.theme.keyColorOptions
import com.fanjv.netproxy.navigation.LocalNavigator
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Slider
import top.yukonga.miuix.kmp.basic.SliderDefaults
import top.yukonga.miuix.kmp.basic.TabRow
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.theme.ThemeColorSpec
import top.yukonga.miuix.kmp.theme.ThemePaletteStyle
import top.yukonga.miuix.kmp.utils.overScrollVertical

/** 主题设置页：配色、动态色与界面效果。 */
@Composable
internal fun ThemeSettingsScreen(
    viewModel: ThemeViewModel
) {
    val navigator = LocalNavigator.current
    val context = LocalContext.current
    val activity = androidx.activity.compose.LocalActivity.current
    val theme by viewModel.state.collectAsStateWithLifecycle()
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val blurActive = backdrop != null
    val barColor = if (blurActive) Color.Transparent else colorScheme.surface

    Scaffold(
        topBar = {
            BlurredBar(backdrop) {
                AdaptiveTopAppBar(
                    color = barColor,
                    title = stringResource(R.string.settings_theme),
                    navigationIcon = {
                        IconButton(onClick = { navigator.pop() }) {
                            val layoutDirection = LocalLayoutDirection.current
                            Icon(
                                modifier = Modifier.graphicsLayer {
                                    if (layoutDirection == LayoutDirection.Rtl) scaleX = -1f
                                },
                                imageVector = Icons.AutoMirrored.Rounded.ArrowBack,
                                contentDescription = null,
                                tint = colorScheme.onBackground
                            )
                        }
                    },
                    scrollBehavior = scrollBehavior
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.add(WindowInsets.displayCutout)
            .only(WindowInsetsSides.Horizontal)
    ) { innerPadding ->
        Box(modifier = if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier) {
            LazyColumn(
                modifier = Modifier
                    .fillMaxHeight()
                    .overScrollVertical()
                    .nestedScroll(scrollBehavior.nestedScrollConnection)
                    .padding(horizontal = 12.dp),
                contentPadding = innerPadding,
                overscrollEffect = null,
            ) {
                item(key = "theme_preview") {
                    Spacer(modifier = Modifier.height(32.dp))
                    ThemePreviewCard(
                        enableFloatingBottomBar = theme.enableFloatingBottomBar,
                        enableFloatingBottomBarBlur = theme.enableFloatingBottomBarBlur,
                    )
                    Spacer(modifier = Modifier.height(72.dp))

                    TabRow(
                        tabs = listOf(
                            stringResource(R.string.settings_theme_mode_system),
                            stringResource(R.string.settings_theme_mode_light),
                            stringResource(R.string.settings_theme_mode_dark),
                        ),
                        selectedTabIndex = (if (theme.colorMode >= 3) theme.colorMode - 3 else theme.colorMode).coerceIn(
                            0,
                            2
                        ),
                        onTabSelected = { viewModel.setThemeMode(it) },
                        height = 48.dp,
                    )
                }

                groupedCardItems(
                    keyPrefix = "theme_monet",
                    outerTopPadding = 12.dp,
                    items = buildList {
                        add(CardItem("enabled") {
                            SwitchPreference(
                                title = stringResource(R.string.settings_monet),
                                startAction = {
                                    Icon(
                                        Icons.Rounded.Wallpaper,
                                        modifier = Modifier.padding(end = 6.dp),
                                        contentDescription = null,
                                        tint = colorScheme.onBackground
                                    )
                                },
                                checked = theme.miuixMonet,
                                onCheckedChange = viewModel::setMiuixMonet
                            )
                        })
                        if (theme.miuixMonet) {
                            add(CardItem("keyColor") {
                                val colorItems = listOf(
                                    stringResource(R.string.settings_key_color_default),
                                    stringResource(R.string.color_red),
                                    stringResource(R.string.color_pink),
                                    stringResource(R.string.color_purple),
                                    stringResource(R.string.color_deep_purple),
                                    stringResource(R.string.color_indigo),
                                    stringResource(R.string.color_blue),
                                    stringResource(R.string.color_cyan),
                                    stringResource(R.string.color_teal),
                                    stringResource(R.string.color_green),
                                    stringResource(R.string.color_yellow),
                                    stringResource(R.string.color_amber),
                                    stringResource(R.string.color_orange),
                                    stringResource(R.string.color_brown),
                                    stringResource(R.string.color_blue_grey),
                                    stringResource(R.string.color_sakura),
                                )
                                val colorValues = listOf(0) + keyColorOptions
                                OverlayDropdownPreference(
                                    title = stringResource(R.string.settings_key_color),
                                    items = colorItems,
                                    startAction = {
                                        Icon(
                                            Icons.Rounded.Colorize,
                                            modifier = Modifier.padding(end = 6.dp),
                                            contentDescription = null,
                                            tint = colorScheme.onBackground
                                        )
                                    },
                                    selectedIndex = colorValues.indexOf(theme.keyColor)
                                        .takeIf { it >= 0 } ?: 0,
                                    onSelectedIndexChange = { index ->
                                        viewModel.setKeyColor(colorValues[index])
                                    }
                                )
                            })
                            if (theme.keyColor != 0) {
                                add(CardItem("colorStyle") {
                                    val styles = ThemePaletteStyle.entries
                                    OverlayDropdownPreference(
                                        title = stringResource(R.string.settings_color_style),
                                        items = styles.map { it.name },
                                        startAction = {
                                            Icon(
                                                Icons.Rounded.Style,
                                                modifier = Modifier.padding(end = 6.dp),
                                                contentDescription = null,
                                                tint = colorScheme.onBackground
                                            )
                                        },
                                        selectedIndex = styles.indexOfFirst { it.name == theme.colorStyle }
                                            .coerceAtLeast(0),
                                        onSelectedIndexChange = { index ->
                                            viewModel.setColorStyle(styles[index].name)
                                        }
                                    )
                                })
                                add(CardItem("colorSpec") {
                                    val specs = ThemeColorSpec.entries
                                    OverlayDropdownPreference(
                                        title = stringResource(R.string.settings_color_spec),
                                        items = specs.map { it.name },
                                        startAction = {
                                            Icon(
                                                Icons.Rounded.Palette,
                                                modifier = Modifier.padding(end = 6.dp),
                                                contentDescription = null,
                                                tint = colorScheme.onBackground
                                            )
                                        },
                                        selectedIndex = specs.indexOfFirst { it.name == theme.colorSpec }
                                            .coerceAtLeast(0),
                                        onSelectedIndexChange = { index ->
                                            viewModel.setColorSpec(specs[index].name)
                                        }
                                    )
                                })
                            }
                        }
                    },
                )

                groupedCardItems(
                    keyPrefix = "theme_appearance",
                    outerTopPadding = 12.dp,
                    items = buildList {
                        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                            add(CardItem("blur") {
                                SwitchPreference(
                                    title = stringResource(R.string.settings_enable_blur),
                                    summary = stringResource(R.string.settings_enable_blur_summary),
                                    startAction = {
                                        Icon(
                                            Icons.Rounded.BlurOn,
                                            modifier = Modifier.padding(end = 6.dp),
                                            contentDescription = null,
                                            tint = colorScheme.onBackground
                                        )
                                    },
                                    checked = theme.enableBlur,
                                    onCheckedChange = viewModel::setEnableBlur
                                )
                            })
                        }
                        add(CardItem("floatingBottomBar") {
                            SwitchPreference(
                                title = stringResource(R.string.settings_floating_bottom_bar),
                                summary = stringResource(R.string.settings_floating_bottom_bar_summary),
                                startAction = {
                                    Icon(
                                        Icons.Rounded.CallToAction,
                                        modifier = Modifier.padding(end = 6.dp),
                                        contentDescription = null,
                                        tint = colorScheme.onBackground
                                    )
                                },
                                checked = theme.enableFloatingBottomBar,
                                onCheckedChange = viewModel::setEnableFloatingBottomBar
                            )
                        })
                        if (theme.enableFloatingBottomBar &&
                            Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU
                        ) {
                            add(CardItem("glass") {
                                SwitchPreference(
                                    title = stringResource(R.string.settings_enable_glass),
                                    summary = stringResource(R.string.settings_enable_glass_summary),
                                    startAction = {
                                        Icon(
                                            Icons.Rounded.WaterDrop,
                                            modifier = Modifier.padding(end = 6.dp),
                                            contentDescription = null,
                                            tint = colorScheme.onBackground
                                        )
                                    },
                                    checked = theme.enableFloatingBottomBarBlur,
                                    onCheckedChange = viewModel::setEnableFloatingBottomBarBlur
                                )
                            })
                        }
                        add(CardItem("smoothCorner") {
                            SwitchPreference(
                                title = stringResource(R.string.settings_smooth_corner),
                                summary = stringResource(R.string.settings_smooth_corner_summary),
                                startAction = {
                                    Icon(
                                        Icons.Rounded.RoundedCorner,
                                        modifier = Modifier.padding(end = 6.dp),
                                        contentDescription = null,
                                        tint = colorScheme.onBackground
                                    )
                                },
                                checked = theme.enableSmoothCorner,
                                onCheckedChange = viewModel::setEnableSmoothCorner
                            )
                        })
                    },
                )

                groupedCardItems(
                    keyPrefix = "theme_navigation",
                    outerTopPadding = 12.dp,
                    items = buildList {
                        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
                            add(CardItem("predictiveBack") {
                                SwitchPreference(
                                    title = stringResource(R.string.settings_enable_predictive_back),
                                    summary = stringResource(R.string.settings_enable_predictive_back_summary),
                                    startAction = {
                                        Icon(
                                            Icons.AutoMirrored.Rounded.MenuOpen,
                                            modifier = Modifier.padding(end = 6.dp),
                                            contentDescription = null,
                                            tint = colorScheme.onBackground
                                        )
                                    },
                                    checked = theme.enablePredictiveBack,
                                    onCheckedChange = { enabled ->
                                        viewModel.setEnablePredictiveBack(enabled)
                                        com.fanjv.netproxy.NetProxyApplication.setEnableOnBackInvokedCallback(
                                            context.applicationInfo,
                                            enabled
                                        )
                                        activity?.recreate()
                                    }
                                )
                            })
                        }
                        add(CardItem("pageScale") {
                            var sliderValue by remember(theme.pageScale) { mutableFloatStateOf(theme.pageScale) }
                            val showPageScalePanel = rememberSaveable { mutableStateOf(false) }
                            ArrowPreference(
                                title = stringResource(R.string.settings_page_scale),
                                summary = stringResource(R.string.settings_page_scale_summary),
                                startAction = {
                                    Icon(
                                        Icons.Rounded.AspectRatio,
                                        modifier = Modifier.padding(end = 6.dp),
                                        contentDescription = null,
                                        tint = colorScheme.onBackground
                                    )
                                },
                                endActions = {
                                    Text(
                                        text = stringResource(
                                            R.string.percentage_value,
                                            (sliderValue * 100).toInt(),
                                        ),
                                        color = colorScheme.onSurfaceVariantActions,
                                    )
                                },
                                onClick = { showPageScalePanel.value = !showPageScalePanel.value },
                                holdDownState = showPageScalePanel.value,
                                bottomAction = {
                                    Slider(
                                        value = sliderValue,
                                        onValueChange = {
                                            sliderValue = it
                                        },
                                        onValueChangeFinished = {
                                            viewModel.setPageScale(sliderValue)
                                        },
                                        valueRange = 0.8f..1.1f,
                                        showKeyPoints = true,
                                        keyPoints = listOf(0.8f, 0.9f, 1f, 1.1f),
                                        magnetThreshold = 0.01f,
                                        hapticEffect = SliderDefaults.SliderHapticEffect.Step,
                                    )
                                },
                            )
                        })
                    },
                )
                item {
                    Spacer(
                        Modifier.height(
                            WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding() +
                                    WindowInsets.captionBar.asPaddingValues()
                                        .calculateBottomPadding() +
                                    12.dp
                        )
                    )
                }
            }
        }
    }
}

@SuppressLint("ConfigurationScreenWidthHeight")
@Composable
private fun ThemePreviewCard(
    enableFloatingBottomBar: Boolean,
    enableFloatingBottomBarBlur: Boolean,
) {
    val configuration = LocalConfiguration.current
    val bgColor = colorScheme.surface
    val textColor = colorScheme.onBackground
    val cardColor = colorScheme.surfaceVariant
    val navBarColor = colorScheme.surface
    val primaryColor = colorScheme.primary
    val navSelectedColor = colorScheme.onSurfaceContainer
    val navUnselectedColor = colorScheme.onSurfaceContainer.copy(alpha = 0.5f)

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 12.dp),
        contentAlignment = Alignment.TopCenter
    ) {
        Box(
            modifier = Modifier
                .fillMaxWidth(0.42f)
                .clip(RoundedCornerShape(20.dp))
                .background(bgColor)
                .border(1.dp, colorScheme.outline, RoundedCornerShape(20.dp))
                .height((configuration.screenHeightDp * 0.32f).dp)
        ) {
            Column(
                modifier = Modifier.fillMaxSize()
            ) {
                // 模拟的顶栏
                Row(
                    modifier = Modifier
                        .height(42.dp)
                        .fillMaxWidth()
                        .padding(start = 12.dp, top = 18.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text(
                        text = stringResource(R.string.dashboard),
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Bold,
                        color = textColor
                    )
                }

                Column(
                    modifier = Modifier
                        .weight(1f)
                        .padding(horizontal = 8.dp, vertical = 4.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    // 速率图表卡片
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(65.dp)
                            .clip(RoundedCornerShape(8.dp))
                            .background(cardColor)
                            .padding(6.dp)
                    ) {
                        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            // 开关区域
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Box(
                                    modifier = Modifier
                                        .width(30.dp)
                                        .height(4.dp)
                                        .clip(RoundedCornerShape(2.dp))
                                        .background(textColor.copy(alpha = 0.6f))
                                )
                                Box(
                                    modifier = Modifier
                                        .size(width = 16.dp, height = 8.dp)
                                        .clip(RoundedCornerShape(4.dp))
                                        .background(primaryColor)
                                )
                            }
                            // 速率文字区域
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                Box(
                                    modifier = Modifier
                                        .width(20.dp)
                                        .height(3.dp)
                                        .clip(RoundedCornerShape(1.5.dp))
                                        .background(Color(0xFF2196F3).copy(alpha = 0.8f))
                                )
                                Box(
                                    modifier = Modifier
                                        .width(20.dp)
                                        .height(3.dp)
                                        .clip(RoundedCornerShape(1.5.dp))
                                        .background(Color(0xFF4CAF50).copy(alpha = 0.8f))
                                )
                            }
                            // 迷你图表
                            Canvas(modifier = Modifier.fillMaxSize()) {
                                val path = Path()
                                path.moveTo(0f, size.height * 0.8f)
                                path.quadraticTo(
                                    size.width * 0.2f,
                                    size.height * 0.2f,
                                    size.width * 0.5f,
                                    size.height * 0.5f
                                )
                                path.quadraticTo(
                                    size.width * 0.8f,
                                    size.height * 0.8f,
                                    size.width,
                                    size.height * 0.1f
                                )
                                drawPath(
                                    path = path,
                                    color = primaryColor.copy(alpha = 0.4f),
                                    style = Stroke(width = 1.dp.toPx())
                                )
                            }
                        }
                    }

                    // 网络统计卡片
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(28.dp)
                            .clip(RoundedCornerShape(8.dp))
                            .background(cardColor)
                            .padding(6.dp)
                    ) {
                        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Box(
                                modifier = Modifier
                                    .width(40.dp)
                                    .height(3.dp)
                                    .clip(RoundedCornerShape(1.5.dp))
                                    .background(textColor.copy(alpha = 0.4f))
                            )
                            Box(
                                modifier = Modifier
                                    .width(60.dp)
                                    .height(3.dp)
                                    .clip(RoundedCornerShape(1.5.dp))
                                    .background(textColor.copy(alpha = 0.2f))
                            )
                        }
                    }

                    // 系统统计（并排）
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Box(
                            modifier = Modifier
                                .weight(1f)
                                .height(32.dp)
                                .clip(RoundedCornerShape(8.dp))
                                .background(cardColor)
                                .padding(6.dp)
                        ) {
                            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                                Box(
                                    modifier = Modifier
                                        .size(6.dp)
                                        .clip(RoundedCornerShape(1.dp))
                                        .background(primaryColor.copy(alpha = 0.6f))
                                )
                                Box(
                                    modifier = Modifier
                                        .width(24.dp)
                                        .height(4.dp)
                                        .clip(RoundedCornerShape(2.dp))
                                        .background(textColor.copy(alpha = 0.7f))
                                )
                            }
                        }
                        Box(
                            modifier = Modifier
                                .weight(1f)
                                .height(32.dp)
                                .clip(RoundedCornerShape(8.dp))
                                .background(cardColor)
                                .padding(6.dp)
                        ) {
                            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                                Box(
                                    modifier = Modifier
                                        .size(6.dp)
                                        .clip(RoundedCornerShape(1.dp))
                                        .background(primaryColor.copy(alpha = 0.6f))
                                )
                                Box(
                                    modifier = Modifier
                                        .width(24.dp)
                                        .height(4.dp)
                                        .clip(RoundedCornerShape(2.dp))
                                        .background(textColor.copy(alpha = 0.7f))
                                )
                            }
                        }
                    }

                    // 出站模式卡片
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(36.dp)
                            .clip(RoundedCornerShape(8.dp))
                            .background(cardColor)
                            .padding(6.dp)
                    ) {
                        Column(verticalArrangement = Arrangement.spacedBy(5.dp)) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                Box(
                                    modifier = Modifier
                                        .size(8.dp)
                                        .clip(RoundedCornerShape(2.dp))
                                        .background(primaryColor)
                                )
                                Box(
                                    modifier = Modifier
                                        .width(45.dp)
                                        .height(4.dp)
                                        .clip(RoundedCornerShape(2.dp))
                                        .background(textColor.copy(alpha = 0.8f))
                                )
                            }
                            Box(
                                modifier = Modifier
                                    .fillMaxWidth(0.7f)
                                    .height(3.dp)
                                    .clip(RoundedCornerShape(1.5.dp))
                                    .background(textColor.copy(alpha = 0.3f))
                            )
                        }
                    }
                }

                // 导航栏底部留白
                Spacer(modifier = Modifier.height(36.dp))
            }

            if (enableFloatingBottomBar) {
                Box(
                    modifier = Modifier
                        .align(Alignment.BottomCenter)
                        .padding(bottom = 8.dp),
                ) {
                    Row(
                        modifier = Modifier
                            .height(26.dp)
                            .clip(RoundedCornerShape(13.dp))
                            .background(
                                if (enableFloatingBottomBarBlur) navBarColor.copy(alpha = 0.5f) else navBarColor
                            )
                            .border(0.5.dp, textColor.copy(alpha = 0.1f), RoundedCornerShape(13.dp))
                            .padding(horizontal = 10.dp),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        repeat(4) { index ->
                            Box(
                                modifier = Modifier
                                    .size(11.dp)
                                    .clip(RoundedCornerShape(2.dp))
                                    .background(
                                        if (index == 0) primaryColor else textColor.copy(
                                            alpha = 0.5f
                                        )
                                    )
                            )
                        }
                    }
                }
            } else {
                Column(
                    modifier = Modifier
                        .align(Alignment.BottomCenter)
                        .fillMaxWidth()
                ) {
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(0.5.dp)
                            .background(textColor.copy(alpha = 0.1f))
                    )
                    Row(
                        modifier = Modifier
                            .height(34.dp)
                            .fillMaxWidth()
                            .background(navBarColor)
                            .padding(top = 2.dp, bottom = 6.dp),
                        horizontalArrangement = Arrangement.SpaceEvenly,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        repeat(4) { index ->
                            Box(
                                modifier = Modifier
                                    .size(13.dp)
                                    .clip(RoundedCornerShape(3.dp))
                                    .background(if (index == 0) navSelectedColor else navUnselectedColor)
                            )
                        }
                    }
                }
            }
        }
    }
}


