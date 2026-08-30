package com.fanjv.netproxy.feature.about.presentation

import android.graphics.Canvas
import android.graphics.drawable.Drawable
import android.os.Build
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.add
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.foundation.layout.captionBar
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.graphics.createBitmap
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.theme.LocalEnableBlur
import com.fanjv.netproxy.core.ui.theme.isInDarkTheme
import com.fanjv.netproxy.feature.about.presentation.effect.AboutBackground
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.CardDefaults
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.BlendColorEntry
import top.yukonga.miuix.kmp.blur.BlurBlendMode
import top.yukonga.miuix.kmp.blur.BlurColors
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.blur.rememberLayerBackdrop
import top.yukonga.miuix.kmp.blur.textureBlur
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.shader.isRuntimeShaderSupported
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

@Composable
internal fun AboutScreenMiuix(
    state: AboutUiState,
    actions: AboutScreenActions,
) {
    val topAppBarScrollBehavior = MiuixScrollBehavior()
    val lazyListState = rememberLazyListState()
    val scrollProgress = remember(lazyListState) {
        {
            if (lazyListState.firstVisibleItemIndex > 0) {
                1f
            } else {
                val spacer = lazyListState.layoutInfo.visibleItemsInfo
                    .firstOrNull { it.key == "logoSpacer" }
                if (spacer == null || spacer.size <= 0) {
                    0f
                } else {
                    (lazyListState.firstVisibleItemScrollOffset.toFloat() / spacer.size)
                        .coerceIn(0f, 1f)
                }
            }
        }
    }

    val barBlurBackdrop = rememberBlurBackdrop()
    val collapsed by remember {
        derivedStateOf { scrollProgress() == 1f }
    }
    val blurActive by remember(barBlurBackdrop) {
        derivedStateOf { barBlurBackdrop != null && collapsed }
    }

    Scaffold(
        topBar = {
            val progress = scrollProgress()
            val barColor = when {
                blurActive -> Color.Transparent
                collapsed -> colorScheme.surface
                else -> Color.Transparent
            }
            BlurredBar(backdrop = barBlurBackdrop.takeIf { blurActive }) {
                AdaptiveTopAppBar(
                    title = state.title,
                    scrollBehavior = topAppBarScrollBehavior,
                    color = barColor,
                    titleColor = colorScheme.onSurface.copy(
                        alpha = ((progress - 0.35f) / 0.65f).coerceIn(0f, 1f),
                    ),
                    navigationIcon = {
                        BackIconButton(onClick = actions.onBack)
                    },
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.add(WindowInsets.displayCutout)
            .only(WindowInsetsSides.Horizontal),
    ) { innerPadding ->
        Box(
            modifier = if (barBlurBackdrop != null) {
                Modifier.layerBackdrop(barBlurBackdrop)
            } else {
                Modifier
            },
        ) {
            AboutContent(
                state = state,
                actions = actions,
                innerPadding = innerPadding,
                topAppBarScrollBehavior = topAppBarScrollBehavior,
                lazyListState = lazyListState,
                scrollProgress = scrollProgress,
            )
        }
    }
}

private fun Drawable.toImageBitmap(fallbackSize: Int): ImageBitmap {
    val width = intrinsicWidth.takeIf { it > 0 } ?: fallbackSize
    val height = intrinsicHeight.takeIf { it > 0 } ?: fallbackSize
    val bitmap = createBitmap(width, height)
    val canvas = Canvas(bitmap)
    setBounds(0, 0, canvas.width, canvas.height)
    draw(canvas)
    return bitmap.asImageBitmap()
}

@Composable
private fun AboutContent(
    state: AboutUiState,
    actions: AboutScreenActions,
    innerPadding: PaddingValues,
    topAppBarScrollBehavior: ScrollBehavior,
    lazyListState: LazyListState,
    scrollProgress: () -> Float,
) {
    val context = LocalContext.current
    val layoutDirection = LocalLayoutDirection.current
    val density = LocalDensity.current
    val appIcon = remember(context) {
        context.packageManager.getApplicationIcon(context.applicationInfo).toImageBitmap(256)
    }
    val backdrop = rememberLayerBackdrop()
    val darkTheme = isInDarkTheme()
    val enableBlur = LocalEnableBlur.current
    val backgroundActive = remember(enableBlur) {
        enableBlur && Build.VERSION.SDK_INT >= Build.VERSION_CODES.VANILLA_ICE_CREAM &&
                isRuntimeShaderSupported()
    }
    val cardBlendColors = remember(darkTheme) { aboutCardBlendColors(darkTheme) }
    val logoBlendColors = remember(darkTheme) { aboutLogoBlendColors(darkTheme) }
    var logoHeight by remember { mutableStateOf(300.dp) }

    val scrollPadding = PaddingValues(
        top = innerPadding.calculateTopPadding(),
        start = innerPadding.calculateStartPadding(layoutDirection),
        end = innerPadding.calculateEndPadding(layoutDirection),
    )
    val logoPadding = PaddingValues(
        top = innerPadding.calculateTopPadding() + 40.dp,
        start = innerPadding.calculateStartPadding(layoutDirection),
        end = innerPadding.calculateEndPadding(layoutDirection),
    )

    AboutBackground(
        active = backgroundActive,
        modifier = Modifier.fillMaxSize(),
        backdropModifier = Modifier.layerBackdrop(backdrop),
        alpha = { 1f - scrollProgress() },
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(
                    top = logoPadding.calculateTopPadding() + 52.dp,
                    start = logoPadding.calculateStartPadding(layoutDirection),
                    end = logoPadding.calculateEndPadding(layoutDirection),
                )
                .onSizeChanged { size ->
                    with(density) { logoHeight = size.height.toDp() }
                },
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Box(
                modifier = Modifier
                    .size(88.dp)
                    .graphicsLayer {
                        val progress = ((scrollProgress() - 0.35f) / 0.15f).coerceIn(0f, 1f)
                        clip = true
                        shape = RoundedCornerShape(24.dp)
                        alpha = 1f - progress
                        scaleX = 1f - progress * 0.05f
                        scaleY = 1f - progress * 0.05f
                    },
                contentAlignment = Alignment.Center,
            ) {
                Image(
                    modifier = Modifier.fillMaxSize(),
                    bitmap = appIcon,
                    contentDescription = null,
                )
            }
            Text(
                modifier = Modifier
                    .padding(top = 12.dp, bottom = 5.dp)
                    .graphicsLayer {
                        val progress = ((scrollProgress() - 0.20f) / 0.15f).coerceIn(0f, 1f)
                        alpha = 1f - progress
                        scaleX = 1f - progress * 0.05f
                        scaleY = 1f - progress * 0.05f
                    }
                    .then(
                        if (enableBlur) {
                            Modifier.textureBlur(
                                backdrop = backdrop,
                                shape = RoundedCornerShape(0.dp),
                                blurRadius = 150f,
                                colors = BlurColors(blendColors = logoBlendColors),
                                contentBlendMode = BlendMode.DstIn,
                                enabled = true,
                            )
                        } else {
                            Modifier
                        },
                    ),
                text = state.appName,
                color = colorScheme.onBackground,
                fontWeight = FontWeight.Bold,
                fontSize = 35.sp,
            )
            Text(
                modifier = Modifier
                    .fillMaxWidth()
                    .graphicsLayer {
                        val progress = ((scrollProgress() - 0.05f) / 0.15f).coerceIn(0f, 1f)
                        alpha = 1f - progress
                        scaleX = 1f - progress * 0.05f
                        scaleY = 1f - progress * 0.05f
                    },
                text = state.versionName,
                color = colorScheme.onSurfaceVariantSummary,
                fontSize = 14.sp,
                textAlign = TextAlign.Center,
            )
        }

        LazyColumn(
            state = lazyListState,
            modifier = Modifier
                .fillMaxSize()
                .scrollEndHaptic()
                .overScrollVertical()
                .nestedScroll(topAppBarScrollBehavior.nestedScrollConnection),
            contentPadding = scrollPadding,
            overscrollEffect = null,
        ) {
            item(key = "logoSpacer") {
                Spacer(
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(
                            logoHeight + 52.dp + logoPadding.calculateTopPadding() -
                                    scrollPadding.calculateTopPadding() + 126.dp,
                        ),
                )
            }
            item(key = "about") {
                Column(
                    modifier = Modifier
                        .fillParentMaxHeight()
                        .padding(bottom = innerPadding.calculateBottomPadding() + 12.dp),
                ) {
                    Card(
                        modifier = Modifier
                            .padding(horizontal = 12.dp)
                            .then(
                                if (enableBlur) {
                                    Modifier.textureBlur(
                                        backdrop = backdrop,
                                        shape = RoundedCornerShape(16.dp),
                                        blurRadius = 60f,
                                        colors = BlurColors(blendColors = cardBlendColors),
                                        enabled = true,
                                    )
                                } else {
                                    Modifier
                                },
                            ),
                        colors = CardDefaults.defaultColors(
                            if (enableBlur) Color.Transparent else colorScheme.surfaceContainer,
                            Color.Transparent,
                        ),
                    ) {
                        state.links.forEach { link ->
                            ArrowPreference(
                                title = link.label,
                                onClick = { actions.onOpenLink(link.url) },
                            )
                        }
                    }
                    Spacer(
                        modifier = Modifier.height(
                            WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding() +
                                    WindowInsets.captionBar.asPaddingValues()
                                        .calculateBottomPadding(),
                        ),
                    )
                }
            }
        }
    }
}

private fun aboutCardBlendColors(darkTheme: Boolean): List<BlendColorEntry> =
    if (darkTheme) {
        listOf(
            BlendColorEntry(Color(0x4DA9A9A9), BlurBlendMode.Luminosity),
            BlendColorEntry(Color(0x1A9C9C9C), BlurBlendMode.PlusDarker),
        )
    } else {
        listOf(
            BlendColorEntry(Color(0x340034F9), BlurBlendMode.Overlay),
            BlendColorEntry(Color(0xB3FFFFFF), BlurBlendMode.HardLight),
        )
    }

private fun aboutLogoBlendColors(darkTheme: Boolean): List<BlendColorEntry> =
    if (darkTheme) {
        listOf(
            BlendColorEntry(Color(0xE6A1A1A1), BlurBlendMode.ColorDodge),
            BlendColorEntry(Color(0x4DE6E6E6), BlurBlendMode.LinearLight),
            BlendColorEntry(Color(0xFF1AF500), BlurBlendMode.Lab),
        )
    } else {
        listOf(
            BlendColorEntry(Color(0xCC4A4A4A), BlurBlendMode.ColorBurn),
            BlendColorEntry(Color(0xFF4F4F4F), BlurBlendMode.LinearLight),
            BlendColorEntry(Color(0xFF1AF200), BlurBlendMode.Lab),
        )
    }
