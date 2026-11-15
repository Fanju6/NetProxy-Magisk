package com.fanjv.netproxy.feature.theme.presentation

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.add
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.captionBar
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.AspectRatio
import androidx.compose.material.icons.rounded.Colorize
import androidx.compose.material.icons.rounded.Palette
import androidx.compose.material.icons.rounded.Style
import androidx.compose.material.icons.rounded.Wallpaper
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.translate
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalWindowInfo
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.chromeIcon
import com.fanjv.netproxy.core.ui.component.contentCardRim
import com.fanjv.netproxy.core.ui.component.groupedCardItems
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.theme.AppThemeDefaults
import com.fanjv.netproxy.core.ui.theme.keyColorOptions
import com.fanjv.netproxy.navigation.AppDestination
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.R
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Slider
import top.yukonga.miuix.kmp.basic.SliderDefaults
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.theme.ThemeColorSpec
import top.yukonga.miuix.kmp.theme.ThemePaletteStyle
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic
import com.fanjv.netproxy.core.ui.component.AppDropdown as OverlayDropdownPreference

/** 外观页：配色与界面缩放。 */
@Composable
internal fun ThemeSettingsScreen(
    viewModel: ThemeViewModel
) {
    val navigator = LocalNavigator.current
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
                    navigationIcon = { BackIconButton(onClick = { navigator.pop() }) },
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
                    .scrollEndHaptic()
                    .overScrollVertical()
                    .nestedScroll(scrollBehavior.nestedScrollConnection)
                    .padding(horizontal = 12.dp),
                contentPadding = innerPadding,
                overscrollEffect = null,
            ) {
                item(key = "theme_preview") {
                    ThemePreviewCard()
                }

                groupedCardItems(
                    keyPrefix = "theme_monet",
                    outerTopPadding = 0.dp,
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
                                    val style = ThemePaletteStyle.entries.firstOrNull { it.name == theme.colorStyle }
                                        ?: AppThemeDefaults.paletteStyle
                                    val specs = when (style) {
                                        ThemePaletteStyle.Cmf -> listOf("Spec2026")
                                        ThemePaletteStyle.TonalSpot, ThemePaletteStyle.Neutral,
                                        ThemePaletteStyle.Vibrant, ThemePaletteStyle.Expressive ->
                                            ThemeColorSpec.entries.map { it.name }
                                        else -> listOf(ThemeColorSpec.Spec2021.name)
                                    }
                                    OverlayDropdownPreference(
                                        title = stringResource(R.string.settings_color_spec),
                                        items = specs,
                                        enabled = specs.size > 1,
                                        startAction = {
                                            Icon(
                                                Icons.Rounded.Palette,
                                                modifier = Modifier.padding(end = 6.dp),
                                                contentDescription = null,
                                                tint = colorScheme.onBackground
                                            )
                                        },
                                        selectedIndex = specs.indexOf(theme.colorSpec).coerceAtLeast(0),
                                        onSelectedIndexChange = { index ->
                                            viewModel.setColorSpec(specs[index])
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
                    items = listOf(
                        CardItem("pageScale") {
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
                        },
                    ),
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

@Composable
private fun ThemePreviewCard() {
    val window = LocalWindowInfo.current.containerSize
    val ratio = if (window.width > 0 && window.height > 0) {
        window.width.toFloat() / window.height
    } else {
        9f / 20f
    }
    val frame = RoundedCornerShape(18.dp)
    val surface = colorScheme.surface
    val card = colorScheme.surfaceContainer
    val edge = contentCardRim()
    val accent = colorScheme.primary
    val switchThumb = colorScheme.onPrimary
    val label = stringResource(R.string.dashboard_preview)
    val dashboard = stringResource(R.string.dashboard)
    val serviceStatus = stringResource(R.string.service_status)
    val cpu = stringResource(R.string.cpu_usage)
    val memory = stringResource(R.string.mem_usage)
    val outbound = stringResource(R.string.dashboard_outbound_mode)
    val titleColor = colorScheme.onSurface
    val summaryColor = colorScheme.onSurfaceVariantSummary
    val textMeasurer = rememberTextMeasurer()
    val navIcons = AppDestination.entries.map { rememberVectorPainter(chromeIcon(it.icon)) }

    Box(
        modifier = Modifier.fillMaxWidth().padding(top = 40.dp, bottom = 60.dp),
        contentAlignment = Alignment.Center,
    ) {
        Canvas(
            modifier = Modifier.fillMaxWidth(0.43f).aspectRatio(ratio)
                .clip(frame).background(surface).border(0.5.dp, titleColor.copy(alpha = 0.14f), frame)
                .clearAndSetSemantics { contentDescription = label },
        ) {
            fun silhouette(
                left: Float, top: Float, width: Float, height: Float,
                fill: Color = card, radius: Float = 0.052f, stroke: Boolean = true,
            ) {
                val offset = Offset(size.width * left, size.height * top)
                val dimensions = Size(size.width * width, size.height * height)
                val corners = CornerRadius(size.width * radius)
                drawRoundRect(fill, offset, dimensions, corners)
                if (stroke) drawRoundRect(edge, offset, dimensions, corners, style = Stroke(0.5.dp.toPx()))
            }

            fun previewLabel(text: String, left: Float, centerY: Float, width: Float, height: Float, style: TextStyle) {
                val layout = textMeasurer.measure(
                    text = text,
                    style = style,
                    overflow = TextOverflow.Ellipsis,
                    maxLines = 1,
                    constraints = Constraints(
                        maxWidth = (size.width * width).toInt(),
                        maxHeight = (size.height * height).toInt(),
                    ),
                )
                drawText(layout, topLeft = Offset(size.width * left, size.height * centerY - layout.size.height / 2f))
            }

            silhouette(0.035f, 0.187f, 0.93f, 0.259f)
            silhouette(0.785f, 0.213f, 0.137f, 0.035f, accent, radius = 0.08f, stroke = false)
            drawCircle(
                color = switchThumb,
                radius = minOf(size.width * 0.032f, size.height * 0.012f),
                center = Offset(size.width * 0.884f, size.height * 0.2305f),
            )
            for (guide in listOf(0.343f, 0.409f)) {
                val y = size.height * guide
                drawLine(edge, Offset(size.width * 0.08f, y), Offset(size.width * 0.92f, y), 0.5.dp.toPx())
            }
            silhouette(0.035f, 0.462f, 0.45f, 0.098f)
            silhouette(0.515f, 0.462f, 0.45f, 0.098f)
            silhouette(0.035f, 0.575f, 0.93f, 0.13f)
            silhouette(0.035f, 0.72f, 0.93f, 0.071f)
            silhouette(0.07f, 0.902f, 0.86f, 0.068f, radius = 0.10f)
            silhouette(0.079f, 0.907f, 0.195f, 0.058f, accent.copy(alpha = 0.16f), radius = 0.09f, stroke = false)

            val detail = TextStyle(color = summaryColor, fontSize = 9.sp)
            previewLabel(
                dashboard, left = 0.075f, centerY = 0.142f, width = 0.85f, height = 0.088f,
                style = TextStyle(color = titleColor, fontSize = 12.sp, fontWeight = FontWeight.SemiBold),
            )
            previewLabel(serviceStatus, 0.08f, 0.23f, 0.65f, 0.064f, detail)
            previewLabel(cpu, 0.08f, 0.50f, 0.37f, 0.065f, detail)
            previewLabel(memory, 0.56f, 0.50f, 0.37f, 0.065f, detail)
            previewLabel(outbound, 0.08f, 0.755f, 0.84f, 0.065f, detail)

            val iconSize = minOf(size.width * 0.06f, size.height * 0.034f)
            navIcons.forEachIndexed { index, painter ->
                translate(
                    left = size.width * (0.1765f + index * 0.215f) - iconSize / 2,
                    top = size.height * 0.936f - iconSize / 2,
                ) {
                    with(painter) {
                        draw(Size(iconSize, iconSize), colorFilter = ColorFilter.tint(if (index == 0) accent else summaryColor))
                    }
                }
            }
        }
    }
}
