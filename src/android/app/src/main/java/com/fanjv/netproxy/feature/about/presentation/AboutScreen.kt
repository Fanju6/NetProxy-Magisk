package com.fanjv.netproxy.feature.about.presentation

import android.annotation.SuppressLint
import android.graphics.drawable.Drawable
import android.os.Build
import android.util.Log
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.spring
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
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
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.drawscope.ContentDrawScope
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.node.DrawModifierNode
import androidx.compose.ui.node.ModifierNodeElement
import androidx.compose.ui.node.invalidateDraw
import androidx.compose.ui.platform.InspectorInfo
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.graphics.createBitmap
import androidx.lifecycle.compose.dropUnlessResumed
import com.fanjv.netproxy.BuildConfig
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.theme.LocalEnableBlur
import com.fanjv.netproxy.core.ui.theme.isInDarkTheme
import com.fanjv.netproxy.navigation.LocalNavigator
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.CardDefaults
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.BlendColorEntry
import top.yukonga.miuix.kmp.blur.BlurBlendMode
import top.yukonga.miuix.kmp.blur.BlurColors
import top.yukonga.miuix.kmp.blur.asBrush
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.blur.rememberLayerBackdrop
import top.yukonga.miuix.kmp.blur.textureBlur
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.shader.RuntimeShader
import top.yukonga.miuix.kmp.shader.isRuntimeShaderSupported
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic
import kotlin.math.cos
import kotlin.math.floor
import kotlin.math.sin
import android.graphics.Canvas as AndroidCanvas
import androidx.compose.ui.graphics.BlendMode as ComposeBlendMode

/** 关于页：应用信息与动态背景特效。 */
@Composable
fun AboutScreen() {
    val navigator = LocalNavigator.current
    val uriHandler = LocalUriHandler.current
    val htmlString = stringResource(
        id = R.string.about_source_code,
        "<b><a href=\"https://github.com/Fanju6/NetProxy-Magisk\">GitHub</a></b>",
        "<b><a href=\"https://t.me/NetProxy_Magisk\">Telegram</a></b>"
    )
    val state = AboutUiState(
        title = stringResource(R.string.about),
        appName = stringResource(R.string.app_name),
        versionName = BuildConfig.VERSION_NAME,
        links = extractLinks(htmlString),
    )
    val actions = AboutScreenActions(
        onBack = dropUnlessResumed { navigator.pop() },
        onOpenLink = uriHandler::openUri,
    )

    AboutScreenMiuix(state, actions)
}

@Immutable
private data class AboutUiState(
    val title: String,
    val appName: String,
    val versionName: String,
    val links: List<LinkInfo>,
)

@Immutable
private data class AboutScreenActions(
    val onBack: () -> Unit,
    val onOpenLink: (String) -> Unit,
)

@Immutable
private data class LinkInfo(
    val fullText: String,
    val url: String
)

private fun extractLinks(html: String): List<LinkInfo> {
    val regex = Regex(
        """([^<>\n\r]+?)\s*<b>\s*<a\b[^>]*\bhref\s*=\s*(['"]?)([^'"\s>]+)\2[^>]*>([^<]+)</a>\s*</b>\s*(.*?)\s*(?=<br|\n|$)""",
        RegexOption.MULTILINE
    )

    return regex.findAll(html).mapNotNull { match ->
        try {
            val before = match.groupValues[1].trim()
            val url = match.groupValues[3].trim()
            val title = match.groupValues[4].trim()
            val after = match.groupValues[5].trim()

            LinkInfo("$before $title $after", url)
        } catch (e: Exception) {
            Log.e("AboutScreen", "extractLinks failed: ${e.message}")
            null
        }
    }.toList()
}

@Composable
private fun AboutScreenMiuix(
    state: AboutUiState,
    actions: AboutScreenActions,
) {
    val topAppBarScrollBehavior = MiuixScrollBehavior()
    val lazyListState = rememberLazyListState()

    val scrollProgress = remember {
        {
            when {
                lazyListState.firstVisibleItemIndex > 0 -> 1f
                else -> {
                    val spacer =
                        lazyListState.layoutInfo.visibleItemsInfo.firstOrNull { it.key == "logoSpacer" }
                    if (spacer != null && spacer.size > 0) {
                        (lazyListState.firstVisibleItemScrollOffset.toFloat() / spacer.size).coerceIn(
                            0f,
                            1f
                        )
                    } else {
                        0f
                    }
                }
            }
        }
    }

    val enableBlur = LocalEnableBlur.current
    val barBlurBackdrop = rememberBlurBackdrop(enableBlur)
    val collapsed = remember {
        derivedStateOf { scrollProgress() == 1f }
    }
    val blurActive = remember(barBlurBackdrop) {
        derivedStateOf { barBlurBackdrop != null && scrollProgress() == 1f }
    }

    Scaffold(
        topBar = {
            val progress = scrollProgress()
            val isCollapsed = collapsed.value
            val isBlurActive = blurActive.value
            val barColor = if (isBlurActive) {
                Color.Transparent
            } else {
                if (isCollapsed) colorScheme.surface else Color.Transparent
            }
            BlurredBar(backdrop = if (isBlurActive) barBlurBackdrop else null) {
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
        Box(modifier = if (barBlurBackdrop != null) Modifier.layerBackdrop(barBlurBackdrop) else Modifier) {
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

private fun Drawable.toImageBitmap(size: Int): ImageBitmap {
    val width = intrinsicWidth.takeIf { it > 0 } ?: size
    val height = intrinsicHeight.takeIf { it > 0 } ?: size
    val bitmap = createBitmap(width, height)
    val canvas = AndroidCanvas(bitmap)
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

    val isInDark = isInDarkTheme()
    val enableBlur = LocalEnableBlur.current
    val effectBackground =
        remember(enableBlur) { isRuntimeShaderSupported() && enableBlur && Build.VERSION.SDK_INT >= Build.VERSION_CODES.VANILLA_ICE_CREAM }

    val blendColors = remember(isInDark) {
        if (isInDark) ColorBlendToken.Overlay_Thin_Light
        else ColorBlendToken.Pured_Regular_Light
    }
    val logoBlend = remember(isInDark) {
        if (isInDark) {
            listOf(
                BlendColorEntry(Color(0xe6a1a1a1), BlurBlendMode.ColorDodge),
                BlendColorEntry(Color(0x4de6e6e6), BlurBlendMode.LinearLight),
                BlendColorEntry(Color(0xff1af500), BlurBlendMode.Lab),
            )
        } else {
            listOf(
                BlendColorEntry(Color(0xcc4a4a4a), BlurBlendMode.ColorBurn),
                BlendColorEntry(Color(0xff4f4f4f), BlurBlendMode.LinearLight),
                BlendColorEntry(Color(0xff1af200), BlurBlendMode.Lab),
            )
        }
    }

    var logoHeightDp by remember { mutableStateOf(300.dp) }

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

    BgEffectBackground(
        dynamicBackground = effectBackground,
        modifier = Modifier.fillMaxSize(),
        bgModifier = Modifier.layerBackdrop(backdrop),
        effectBackground = effectBackground,
        alpha = { 1f - scrollProgress() },
    ) {
        // Logo 区域
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(
                    top = logoPadding.calculateTopPadding() + 52.dp,
                    start = logoPadding.calculateStartPadding(layoutDirection),
                    end = logoPadding.calculateEndPadding(layoutDirection),
                )
                .onSizeChanged { size ->
                    with(density) { logoHeightDp = size.height.toDp() }
                },
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Box(
                contentAlignment = Alignment.Center,
                modifier = Modifier
                    .size(88.dp)
                    .graphicsLayer {
                        val iconProgress = ((scrollProgress() - 0.35f) / 0.15f).coerceIn(0f, 1f)
                        clip = true
                        shape = RoundedCornerShape(24.dp)
                        alpha = 1 - iconProgress
                        scaleX = 1 - (iconProgress * 0.05f)
                        scaleY = 1 - (iconProgress * 0.05f)
                    }
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
                        val projectNameProgress =
                            ((scrollProgress() - 0.20f) / 0.15f).coerceIn(0f, 1f)
                        alpha = 1 - projectNameProgress
                        scaleX = 1 - (projectNameProgress * 0.05f)
                        scaleY = 1 - (projectNameProgress * 0.05f)
                    }
                    .then(
                        if (enableBlur) {
                            Modifier.textureBlur(
                                backdrop = backdrop,
                                shape = RoundedCornerShape(0.dp),
                                blurRadius = 150f,
                                colors = BlurColors(blendColors = logoBlend),
                                contentBlendMode = ComposeBlendMode.DstIn,
                                enabled = true,
                            )
                        } else Modifier
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
                        val versionCodeProgress =
                            ((scrollProgress() - 0.05f) / 0.15f).coerceIn(0f, 1f)
                        alpha = 1 - versionCodeProgress
                        scaleX = 1 - (versionCodeProgress * 0.05f)
                        scaleY = 1 - (versionCodeProgress * 0.05f)
                    },
                color = colorScheme.onSurfaceVariantSummary,
                text = state.versionName,
                fontSize = 14.sp,
                textAlign = TextAlign.Center,
            )
        }

        // 可滚动内容
        LazyColumn(
            state = lazyListState,
            modifier = Modifier
                .fillMaxSize()
                .scrollEndHaptic()
                .overScrollVertical()
                .nestedScroll(topAppBarScrollBehavior.nestedScrollConnection),
            contentPadding = PaddingValues(
                top = scrollPadding.calculateTopPadding(),
                start = scrollPadding.calculateStartPadding(layoutDirection),
                end = scrollPadding.calculateEndPadding(layoutDirection),
            ),
            overscrollEffect = null,
        ) {
            // 与 Logo 等高的透明占位
            item(key = "logoSpacer") {
                Box(
                    Modifier
                        .fillMaxWidth()
                        .height(
                            logoHeightDp + 52.dp + logoPadding.calculateTopPadding() - scrollPadding.calculateTopPadding() + 126.dp,
                        ),
                    contentAlignment = Alignment.TopCenter,
                    content = { },
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
                                        colors = BlurColors(blendColors = blendColors),
                                        enabled = true,
                                    )
                                } else Modifier
                            ),
                        colors = CardDefaults.defaultColors(
                            if (enableBlur) Color.Transparent else colorScheme.surfaceContainer,
                            Color.Transparent,
                        ),
                    ) {
                        state.links.forEach {
                            ArrowPreference(
                                title = it.fullText,
                                onClick = {
                                    actions.onOpenLink(it.url)
                                }
                            )
                        }
                    }
                    Spacer(
                        Modifier.height(
                            WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding() +
                                    WindowInsets.captionBar.asPaddingValues()
                                        .calculateBottomPadding()
                        )
                    )
                }
            }
        }
    }
}

@SuppressLint("ConfigurationScreenWidthHeight")
@Composable
private fun shouldShowSplitPane(): Boolean {
    val config = LocalConfiguration.current
    val widthDp = config.screenWidthDp
    val heightDp = config.screenHeightDp
    val ratio = heightDp.toFloat() / widthDp.toFloat()
    return widthDp >= 840 || (widthDp >= 600 && ratio < 1.2f)
}

@SuppressLint("NewApi")
@Composable
private fun BgEffectBackground(
    dynamicBackground: Boolean,
    modifier: Modifier = Modifier,
    bgModifier: Modifier = Modifier,
    isFullSize: Boolean = true,
    effectBackground: Boolean = true,
    isOs3Effect: Boolean = true,
    alpha: () -> Float = { 1f },
    content: @Composable (BoxScope.() -> Unit),
) {
    val shaderSupported = remember { isRuntimeShaderSupported() }
    if (!shaderSupported) {
        Box(modifier = modifier, content = content)
        return
    }
    Box(
        modifier = modifier,
    ) {
        val surface = colorScheme.surface
        val deviceType = if (shouldShowSplitPane()) DeviceType.PAD else DeviceType.PHONE
        val isDarkTheme = isInDarkTheme()
        val painter = remember(isOs3Effect) { BgEffectPainter(isOs3Effect) }

        val preset = remember(deviceType, isDarkTheme, isOs3Effect) {
            BgEffectConfig.get(deviceType, isDarkTheme, isOs3Effect)
        }

        val colorStage = remember { Animatable(0f) }

        LaunchedEffect(dynamicBackground, preset) {
            if (!dynamicBackground) return@LaunchedEffect
            val animatesColors =
                preset.colors1 !== preset.colors2 || preset.colors2 !== preset.colors3
            if (!animatesColors) return@LaunchedEffect

            var targetStage = floor(colorStage.value) + 1f
            while (isActive) {
                delay((preset.colorInterpPeriod * 500).toLong())
                colorStage.animateTo(
                    targetValue = targetStage,
                    animationSpec = spring(dampingRatio = 0.9f, stiffness = 35f),
                )
                targetStage += 1f
            }
        }

        Spacer(
            modifier = Modifier
                .fillMaxSize()
                .then(bgModifier)
                .bgEffectDraw(
                    painter = painter,
                    preset = preset,
                    deviceType = deviceType,
                    isDarkTheme = isDarkTheme,
                    surface = surface,
                    effectBackground = effectBackground,
                    isFullSize = isFullSize,
                    playing = dynamicBackground,
                    colorStage = { colorStage.value },
                    alpha = alpha,
                ),
        )
        content()
    }
}

@SuppressLint("NewApi")
private fun Modifier.bgEffectDraw(
    painter: BgEffectPainter,
    preset: BgEffectConfig.Config,
    deviceType: DeviceType,
    isDarkTheme: Boolean,
    surface: Color,
    effectBackground: Boolean,
    isFullSize: Boolean,
    playing: Boolean,
    colorStage: () -> Float,
    alpha: () -> Float,
): Modifier = this then BgEffectElement(
    painter = painter,
    preset = preset,
    deviceType = deviceType,
    isDarkTheme = isDarkTheme,
    surface = surface,
    effectBackground = effectBackground,
    isFullSize = isFullSize,
    playing = playing,
    colorStage = colorStage,
    alpha = alpha,
)

@SuppressLint("NewApi")
private data class BgEffectElement(
    val painter: BgEffectPainter,
    val preset: BgEffectConfig.Config,
    val deviceType: DeviceType,
    val isDarkTheme: Boolean,
    val surface: Color,
    val effectBackground: Boolean,
    val isFullSize: Boolean,
    val playing: Boolean,
    val colorStage: () -> Float,
    val alpha: () -> Float,
) : ModifierNodeElement<BgEffectNode>() {

    override fun InspectorInfo.inspectableProperties() {
        name = "bgEffectDraw"
        properties["painter"] = painter
        properties["preset"] = preset
        properties["deviceType"] = deviceType
        properties["isDarkTheme"] = isDarkTheme
        properties["surface"] = surface
        properties["effectBackground"] = effectBackground
        properties["isFullSize"] = isFullSize
        properties["playing"] = playing
        properties["colorStage"] = colorStage
        properties["alpha"] = alpha
    }

    override fun create(): BgEffectNode = BgEffectNode(
        painter = painter,
        preset = preset,
        deviceType = deviceType,
        isDarkTheme = isDarkTheme,
        surface = surface,
        effectBackground = effectBackground,
        isFullSize = isFullSize,
        playing = playing,
        colorStage = colorStage,
        alpha = alpha,
    )

    override fun update(node: BgEffectNode) {
        node.update(
            painter = painter,
            preset = preset,
            deviceType = deviceType,
            isDarkTheme = isDarkTheme,
            surface = surface,
            effectBackground = effectBackground,
            isFullSize = isFullSize,
            playing = playing,
            colorStage = colorStage,
            alpha = alpha,
        )
    }
}

@SuppressLint("NewApi")
private class BgEffectNode(
    private var painter: BgEffectPainter,
    private var preset: BgEffectConfig.Config,
    private var deviceType: DeviceType,
    private var isDarkTheme: Boolean,
    private var surface: Color,
    private var effectBackground: Boolean,
    private var isFullSize: Boolean,
    private var playing: Boolean,
    private var colorStage: () -> Float,
    private var alpha: () -> Float,
) : Modifier.Node(),
    DrawModifierNode {

    private var animationJob: Job? = null
    private var animTime: Float = 0f
    private var startOffset: Float = 0f

    override fun onAttach() {
        if (playing) startAnimation()
    }

    override fun onDetach() {
        animationJob?.cancel()
        animationJob = null
    }

    fun update(
        painter: BgEffectPainter,
        preset: BgEffectConfig.Config,
        deviceType: DeviceType,
        isDarkTheme: Boolean,
        surface: Color,
        effectBackground: Boolean,
        isFullSize: Boolean,
        playing: Boolean,
        colorStage: () -> Float,
        alpha: () -> Float,
    ) {
        this.painter = painter
        this.preset = preset
        this.deviceType = deviceType
        this.isDarkTheme = isDarkTheme
        this.surface = surface
        this.effectBackground = effectBackground
        this.isFullSize = isFullSize
        this.colorStage = colorStage
        this.alpha = alpha

        if (this.playing != playing) {
            this.playing = playing
            if (playing) {
                startAnimation()
            } else {
                animationJob?.cancel()
                animationJob = null
            }
        }
        invalidateDraw()
    }

    private fun startAnimation() {
        animationJob?.cancel()
        startOffset = animTime
        animationJob = coroutineScope.launch {
            val minDeltaNanos = 1_000_000_000L / 60L
            val origin = withFrameNanos { it }
            var lastEmit = origin
            while (isActive) {
                val now = withFrameNanos { it }
                if (now - lastEmit < minDeltaNanos) continue
                lastEmit = now
                animTime = startOffset + (now - origin) / 1_000_000_000f
                invalidateDraw()
            }
        }
    }

    override fun ContentDrawScope.draw() {
        drawRect(surface)
        if (effectBackground) {
            val alphaValue = alpha()
            if (alphaValue > 0f) {
                val drawHeight = if (isFullSize) size.height * 0.8f else size.height * 0.5f

                painter.updateResolution(size.width, size.height)
                painter.updateBoundIfNeeded(drawHeight, size.height, size.width)
                painter.updatePresetIfNeeded(deviceType, isDarkTheme)
                painter.updateColors(preset, colorStage())
                painter.updateAnimTime(animTime)
                painter.updatePointsAnim(animTime, preset)

                drawRect(painter.brush, alpha = alphaValue)
            }
        }
        drawContent()
    }
}

private object BgEffectConfig {

    class Config(
        val points: FloatArray,
        val colors1: FloatArray,
        val colors2: FloatArray,
        val colors3: FloatArray,
        val colorInterpPeriod: Float,
        val lightOffset: Float,
        val saturateOffset: Float,
        val pointOffset: Float,
    )

    // OS2 数据

    private val OS2_PHONE_LIGHT_COLORS = floatArrayOf(
        0.57f,
        0.76f,
        0.98f,
        1.0f,
        0.98f,
        0.85f,
        0.68f,
        1.0f,
        0.98f,
        0.75f,
        0.93f,
        1.0f,
        0.73f,
        0.70f,
        0.98f,
        1.0f,
    )
    private val OS2_PHONE_LIGHT = Config(
        points = floatArrayOf(
            0.67f,
            0.42f,
            1.0f,
            0.69f,
            0.75f,
            1.0f,
            0.14f,
            0.71f,
            0.95f,
            0.14f,
            0.27f,
            0.8f
        ),
        colors1 = OS2_PHONE_LIGHT_COLORS,
        colors2 = OS2_PHONE_LIGHT_COLORS,
        colors3 = OS2_PHONE_LIGHT_COLORS,
        colorInterpPeriod = 100f,
        lightOffset = 0.1f,
        saturateOffset = 0.2f,
        pointOffset = 0.1f,
    )

    private val OS2_PHONE_DARK_COLORS = floatArrayOf(
        0.0f,
        0.31f,
        0.58f,
        1.0f,
        0.53f,
        0.29f,
        0.15f,
        1.0f,
        0.46f,
        0.06f,
        0.27f,
        1.0f,
        0.16f,
        0.12f,
        0.45f,
        1.0f,
    )
    private val OS2_PHONE_DARK = Config(
        points = floatArrayOf(
            0.63f,
            0.50f,
            0.88f,
            0.69f,
            0.75f,
            0.80f,
            0.17f,
            0.66f,
            0.81f,
            0.14f,
            0.24f,
            0.72f
        ),
        colors1 = OS2_PHONE_DARK_COLORS,
        colors2 = OS2_PHONE_DARK_COLORS,
        colors3 = OS2_PHONE_DARK_COLORS,
        colorInterpPeriod = 100f,
        lightOffset = -0.1f,
        saturateOffset = 0.2f,
        pointOffset = 0.1f,
    )

    private val OS2_PAD_LIGHT_COLORS = floatArrayOf(
        0.57f,
        0.76f,
        0.98f,
        1.0f,
        0.98f,
        0.85f,
        0.68f,
        1.0f,
        0.98f,
        0.75f,
        0.93f,
        0.95f,
        0.73f,
        0.70f,
        0.98f,
        0.90f,
    )
    private val OS2_PAD_LIGHT = Config(
        points = floatArrayOf(
            0.67f,
            0.37f,
            0.88f,
            0.54f,
            0.66f,
            1.0f,
            0.37f,
            0.71f,
            0.68f,
            0.28f,
            0.26f,
            0.62f
        ),
        colors1 = OS2_PAD_LIGHT_COLORS,
        colors2 = OS2_PAD_LIGHT_COLORS,
        colors3 = OS2_PAD_LIGHT_COLORS,
        colorInterpPeriod = 100f,
        lightOffset = 0.1f,
        saturateOffset = 0f,
        pointOffset = 0.1f,
    )

    private val OS2_PAD_DARK_COLORS = floatArrayOf(
        0.0f,
        0.31f,
        0.58f,
        1.0f,
        0.53f,
        0.29f,
        0.15f,
        1.0f,
        0.46f,
        0.06f,
        0.27f,
        1.0f,
        0.16f,
        0.12f,
        0.45f,
        1.0f,
    )
    private val OS2_PAD_DARK = Config(
        points = floatArrayOf(
            0.55f,
            0.42f,
            1.0f,
            0.56f,
            0.75f,
            1.0f,
            0.40f,
            0.59f,
            0.71f,
            0.43f,
            0.09f,
            0.75f
        ),
        colors1 = OS2_PAD_DARK_COLORS,
        colors2 = OS2_PAD_DARK_COLORS,
        colors3 = OS2_PAD_DARK_COLORS,
        colorInterpPeriod = 100f,
        lightOffset = -0.1f,
        saturateOffset = 0.2f,
        pointOffset = 0.1f,
    )

    // OS3 数据

    private val OS3_PHONE_LIGHT = Config(
        points = floatArrayOf(
            0.8f,
            0.2f,
            1.0f,
            0.8f,
            0.9f,
            1.0f,
            0.2f,
            0.9f,
            1.0f,
            0.2f,
            0.2f,
            1.0f
        ),
        colors1 = floatArrayOf(
            1.0f,
            0.9f,
            0.94f,
            1.0f,
            1.0f,
            0.84f,
            0.89f,
            1.0f,
            0.97f,
            0.73f,
            0.82f,
            1.0f,
            0.64f,
            0.65f,
            0.98f,
            1.0f
        ),
        colors2 = floatArrayOf(
            0.58f,
            0.74f,
            1.0f,
            1.0f,
            1.0f,
            0.9f,
            0.93f,
            1.0f,
            0.74f,
            0.76f,
            1.0f,
            1.0f,
            0.97f,
            0.77f,
            0.84f,
            1.0f
        ),
        colors3 = floatArrayOf(
            0.98f,
            0.86f,
            0.9f,
            1.0f,
            0.6f,
            0.73f,
            0.98f,
            1.0f,
            0.92f,
            0.93f,
            1.0f,
            1.0f,
            0.56f,
            0.69f,
            1.0f,
            1.0f
        ),
        colorInterpPeriod = 5.0f,
        lightOffset = 0.1f,
        saturateOffset = 0.2f,
        pointOffset = 0.2f,
    )

    private val OS3_PHONE_DARK = Config(
        points = floatArrayOf(
            0.8f,
            0.2f,
            1.0f,
            0.8f,
            0.9f,
            1.0f,
            0.2f,
            0.9f,
            1.0f,
            0.2f,
            0.2f,
            1.0f
        ),
        colors1 = floatArrayOf(
            0.2f,
            0.06f,
            0.88f,
            0.4f,
            0.3f,
            0.14f,
            0.55f,
            0.5f,
            0.0f,
            0.64f,
            0.96f,
            0.5f,
            0.11f,
            0.16f,
            0.83f,
            0.4f
        ),
        colors2 = floatArrayOf(
            0.07f,
            0.15f,
            0.79f,
            0.5f,
            0.62f,
            0.21f,
            0.67f,
            0.5f,
            0.06f,
            0.25f,
            0.84f,
            0.5f,
            0.0f,
            0.2f,
            0.78f,
            0.5f
        ),
        colors3 = floatArrayOf(
            0.58f,
            0.3f,
            0.74f,
            0.4f,
            0.27f,
            0.18f,
            0.6f,
            0.5f,
            0.66f,
            0.26f,
            0.62f,
            0.5f,
            0.12f,
            0.16f,
            0.7f,
            0.6f
        ),
        colorInterpPeriod = 8.0f,
        lightOffset = 0.0f,
        saturateOffset = 0.17f,
        pointOffset = 0.4f,
    )

    private val OS3_PAD_LIGHT = Config(
        points = floatArrayOf(
            0.8f,
            0.2f,
            1.0f,
            0.8f,
            0.9f,
            1.0f,
            0.2f,
            0.9f,
            1.0f,
            0.2f,
            0.2f,
            1.0f
        ),
        colors1 = floatArrayOf(
            0.99f,
            0.77f,
            0.86f,
            1.0f,
            0.74f,
            0.76f,
            1.0f,
            1.0f,
            0.72f,
            0.74f,
            1.0f,
            1.0f,
            0.98f,
            0.76f,
            0.8f,
            1.0f
        ),
        colors2 = floatArrayOf(
            0.66f,
            0.75f,
            1.0f,
            1.0f,
            1.0f,
            0.86f,
            0.91f,
            1.0f,
            0.74f,
            0.76f,
            1.0f,
            1.0f,
            0.97f,
            0.77f,
            0.84f,
            1.0f
        ),
        colors3 = floatArrayOf(
            0.97f,
            0.79f,
            0.85f,
            1.0f,
            0.65f,
            0.68f,
            0.98f,
            1.0f,
            0.66f,
            0.77f,
            1.0f,
            1.0f,
            0.72f,
            0.73f,
            0.98f,
            1.0f
        ),
        colorInterpPeriod = 7.0f,
        lightOffset = 0.1f,
        saturateOffset = 0.2f,
        pointOffset = 0.2f,
    )

    private val OS3_PAD_DARK = Config(
        points = floatArrayOf(
            0.8f,
            0.2f,
            1.0f,
            0.8f,
            0.9f,
            1.0f,
            0.2f,
            0.9f,
            1.0f,
            0.2f,
            0.2f,
            1.0f
        ),
        colors1 = floatArrayOf(
            0.66f,
            0.26f,
            0.62f,
            0.4f,
            0.06f,
            0.25f,
            0.84f,
            0.5f,
            0.0f,
            0.64f,
            0.96f,
            0.5f,
            0.14f,
            0.18f,
            0.55f,
            0.5f
        ),
        colors2 = floatArrayOf(
            0.07f,
            0.15f,
            0.79f,
            0.5f,
            0.11f,
            0.16f,
            0.83f,
            0.5f,
            0.06f,
            0.25f,
            0.84f,
            0.5f,
            0.66f,
            0.26f,
            0.62f,
            0.5f
        ),
        colors3 = floatArrayOf(
            0.58f,
            0.3f,
            0.74f,
            0.5f,
            0.11f,
            0.16f,
            0.83f,
            0.5f,
            0.66f,
            0.26f,
            0.62f,
            0.5f,
            0.27f,
            0.18f,
            0.6f,
            0.6f
        ),
        colorInterpPeriod = 7.0f,
        lightOffset = 0.0f,
        saturateOffset = 0.0f,
        pointOffset = 0.2f,
    )

    fun get(
        deviceType: DeviceType,
        isDark: Boolean,
        isOs3: Boolean,
    ): Config = if (!isOs3) {
        when (deviceType) {
            DeviceType.PHONE -> if (!isDark) OS2_PHONE_LIGHT else OS2_PHONE_DARK
            DeviceType.PAD -> if (!isDark) OS2_PAD_LIGHT else OS2_PAD_DARK
        }
    } else {
        when (deviceType) {
            DeviceType.PHONE -> if (!isDark) OS3_PHONE_LIGHT else OS3_PHONE_DARK
            DeviceType.PAD -> if (!isDark) OS3_PAD_LIGHT else OS3_PAD_DARK
        }
    }
}

@SuppressLint("NewApi")
private class BgEffectPainter(
    private val isOs3: Boolean = true,
) {

    val runtimeShader by lazy {
        val shaderCode = if (isOs3) OS3_BG_FRAG else OS2_BG_FRAG
        RuntimeShader(shaderCode).also {
            initStaticUniforms(it)
        }
    }

    val brush get() = runtimeShader.asBrush()

    private val resolution = FloatArray(2)
    private val bound = FloatArray(4)
    private val colorsBuffer = FloatArray(16)
    private val pointsAnimBuffer = FloatArray(8)

    private var animTime = Float.NaN
    private var isDarkCached: Boolean? = null
    private var deviceTypeCached: DeviceType? = null

    private var presetApplied = false

    private var cachedLogoHeight = Float.NaN
    private var cachedTotalHeight = Float.NaN
    private var cachedTotalWidth = Float.NaN

    private var cachedColorStage = Float.NaN
    private var cachedColorsPreset: BgEffectConfig.Config? = null

    private var cachedPointsAnimTime = Float.NaN
    private var cachedPointsAnimPreset: BgEffectConfig.Config? = null

    companion object {
        private const val U_TRANSLATE_Y = 0f
        private const val U_ALPHA_MULTI = 1f
        private const val U_NOISE_SCALE = 1.5f
        private const val U_POINT_RADIUS_MULTI = 1f
    }

    private fun initStaticUniforms(shader: RuntimeShader) {
        shader.setFloatUniform("uTranslateY", U_TRANSLATE_Y)
        shader.setFloatUniform("uNoiseScale", U_NOISE_SCALE)
        shader.setFloatUniform("uPointRadiusMulti", U_POINT_RADIUS_MULTI)
        shader.setFloatUniform("uAlphaMulti", U_ALPHA_MULTI)
    }

    fun updateResolution(width: Float, height: Float) {
        if (resolution[0] == width && resolution[1] == height) return
        resolution[0] = width
        resolution[1] = height
        runtimeShader.setFloatUniform("uResolution", resolution)
    }

    fun updateAnimTime(time: Float) {
        if (animTime == time) return
        animTime = time
        runtimeShader.setFloatUniform("uAnimTime", animTime)
    }

    fun updatePointsAnim(time: Float, preset: BgEffectConfig.Config) {
        if (cachedPointsAnimTime == time && cachedPointsAnimPreset === preset) return

        val offset = preset.pointOffset
        var i = 0
        while (i < 4) {
            val srcX = preset.points[i * 3]
            val srcY = preset.points[i * 3 + 1]
            val animX = srcX + sin(time + srcY) * offset
            val animY = srcY + cos(time + animX) * offset
            pointsAnimBuffer[i * 2] = animX
            pointsAnimBuffer[i * 2 + 1] = animY
            i++
        }
        runtimeShader.setFloatUniform("uPointsAnim", pointsAnimBuffer)

        cachedPointsAnimTime = time
        cachedPointsAnimPreset = preset
    }

    fun updateColors(preset: BgEffectConfig.Config, stage: Float) {
        if (cachedColorsPreset === preset && cachedColorStage == stage) return

        val base = stage.toInt()
        val fraction = stage - base
        val start = colorsForCycleIndex(preset, base)
        val end = colorsForCycleIndex(preset, base + 1)
        for (i in 0 until 16) {
            colorsBuffer[i] = start[i] + (end[i] - start[i]) * fraction
        }
        runtimeShader.setFloatUniform("uColors", colorsBuffer)

        cachedColorsPreset = preset
        cachedColorStage = stage
    }

    private fun colorsForCycleIndex(preset: BgEffectConfig.Config, index: Int): FloatArray =
        when (index.mod(4)) {
            1 -> preset.colors1
            3 -> preset.colors3
            else -> preset.colors2
        }

    fun updateBoundIfNeeded(
        logoHeight: Float,
        totalHeight: Float,
        totalWidth: Float,
    ) {
        if (cachedLogoHeight == logoHeight &&
            cachedTotalHeight == totalHeight &&
            cachedTotalWidth == totalWidth
        ) {
            return
        }

        updateBound(logoHeight, totalHeight, totalWidth)
        runtimeShader.setFloatUniform("uBound", bound)

        cachedLogoHeight = logoHeight
        cachedTotalHeight = totalHeight
        cachedTotalWidth = totalWidth
    }

    fun updatePresetIfNeeded(deviceType: DeviceType, isDark: Boolean) {
        if (presetApplied && isDarkCached == isDark && deviceTypeCached == deviceType) return

        applyPreset(deviceType, isDark)

        isDarkCached = isDark
        deviceTypeCached = deviceType
        presetApplied = true
    }

    private fun applyPreset(deviceType: DeviceType, isDark: Boolean) {
        val preset = BgEffectConfig.get(deviceType, isDark, isOs3)

        runtimeShader.setFloatUniform("uPoints", preset.points)
        runtimeShader.setFloatUniform("uLightOffset", preset.lightOffset)
        runtimeShader.setFloatUniform("uSaturateOffset", preset.saturateOffset)
    }

    private fun updateBound(
        logoHeight: Float,
        totalHeight: Float,
        totalWidth: Float,
    ) {
        val heightRatio = logoHeight / totalHeight
        if (totalWidth <= totalHeight) {
            bound[0] = 0f
            bound[1] = 1f - heightRatio
            bound[2] = 1f
            bound[3] = heightRatio
        } else {
            val aspectRatio = totalWidth / totalHeight
            val contentCenterY = 1f - heightRatio / 2f
            bound[0] = 0f
            bound[1] = contentCenterY - aspectRatio / 2f
            bound[2] = 1f
            bound[3] = aspectRatio
        }
    }
}

private object ColorBlendToken {

    val Pured_Regular_Light = listOf(
        BlendColorEntry(Color(0x340034F9), BlurBlendMode.Overlay),
        BlendColorEntry(Color(0xB3FFFFFF), BlurBlendMode.HardLight),
    )

    val Overlay_Thin_Light = listOf(
        BlendColorEntry(Color(0x4DA9A9A9), BlurBlendMode.Luminosity),
        BlendColorEntry(Color(0x1A9C9C9C), BlurBlendMode.PlusDarker),
    )
}

private enum class DeviceType {
    PHONE,
    PAD,
}

private const val OS2_BG_FRAG = """
    uniform vec2 uResolution;
    uniform float uAnimTime;
    uniform vec4 uBound;
    uniform float uTranslateY;
    uniform vec3 uPoints[4];
    uniform vec2 uPointsAnim[4];
    uniform vec4 uColors[4];
    uniform float uAlphaMulti;
    uniform float uNoiseScale;
    uniform float uPointRadiusMulti;
    uniform float uSaturateOffset;
    uniform float uLightOffset;

    vec3 rgb2hsv(vec3 c) {
        vec4 K = vec4(0.0, -1.0 / 3.0, 2.0 / 3.0, -1.0);
        vec4 p = mix(vec4(c.bg, K.wz), vec4(c.gb, K.xy), step(c.b, c.g));
        vec4 q = mix(vec4(p.xyw, c.r), vec4(c.r, p.yzx), step(p.x, c.r));
        float d = q.x - min(q.w, q.y);
        float e = 1.0e-10;
        return vec3(abs(q.z + (q.w - q.y) / (6.0 * d + e)), d / (q.x + e), q.x);
    }

    vec3 hsv2rgb(vec3 c) {
        vec4 K = vec4(1.0, 2.0 / 3.0, 1.0 / 3.0, 3.0);
        vec3 p = abs(fract(c.xxx + K.xyz) * 6.0 - K.www);
        return c.z * mix(K.xxx, clamp(p - K.xxx, 0.0, 1.0), c.y);
    }

    float hash(vec2 p) {
        vec3 p3 = fract(vec3(p.xyx) * 0.13);
        p3 += dot(p3, p3.yzx + 3.333);
        return fract((p3.x + p3.y) * p3.z);
    }

    float perlin(vec2 x) {
        vec2 i = floor(x);
        vec2 f = fract(x);

        float a = hash(i);
        float b = hash(i + vec2(1.0, 0.0));
        float c = hash(i + vec2(0.0, 1.0));
        float d = hash(i + vec2(1.0, 1.0));

        vec2 u = f * f * (3.0 - 2.0 * f);
        return mix(a, b, u.x) + (c - a) * u.y * (1.0 - u.x) + (d - b) * u.x * u.y;
    }

    float gradientNoise(in vec2 uv) {
        return fract(52.9829189 * fract(dot(uv, vec2(0.06711056, 0.00583715))));
    }

    vec4 main(vec2 fragCoord){
        vec2 vUv = fragCoord/uResolution;
        vUv.y = 1.0-vUv.y;
        vec2 uv = vUv;
        uv -= vec2(0., uTranslateY);
        uv.xy -= uBound.xy;
        uv.xy /= uBound.zw;

        vec4 color = vec4(0.0);
        float noiseValue = perlin(vUv * uNoiseScale + vec2(-uAnimTime, -uAnimTime));

        for (int i = 0; i < 4; i++){
            vec4 pointColor = uColors[i];
            pointColor.rgb *= pointColor.a;
            vec2 point = uPointsAnim[i];
            float rad = uPoints[i].z * uPointRadiusMulti;

            float d = distance(uv, point);
            float pct = smoothstep(rad, 0., d);
            color.rgb = mix(color.rgb, pointColor.rgb, pct);
            color.a = mix(color.a, pointColor.a, pct);
        }

        float oppositeNoise = smoothstep(0., 1., noiseValue);
        color.rgb /= color.a;
        vec3 hsv = rgb2hsv(color.rgb);
        hsv.y = mix(hsv.y, 0.0, oppositeNoise * uSaturateOffset);
        color.rgb = hsv2rgb(hsv);
        color.rgb += oppositeNoise * uLightOffset;

        color.a = clamp(color.a, 0., 1.);
        color.a *= uAlphaMulti;

        color += (10.0 / 255.0) * gradientNoise(fragCoord.xy) - (5.0 / 255.0);
        return vec4(color.rgb * color.a, color.a);
    }
"""

private const val OS3_BG_FRAG = """
    uniform vec2 uResolution;
    uniform float uAnimTime;
    uniform vec4 uBound;
    uniform float uTranslateY;
    uniform vec3 uPoints[4];
    uniform vec2 uPointsAnim[4];
    uniform vec4 uColors[4];
    uniform float uAlphaMulti;
    uniform float uNoiseScale;
    uniform float uPointRadiusMulti;
    uniform float uSaturateOffset;
    uniform float uLightOffset;

    vec3 rgb2hsv(vec3 c) {
        vec4 K = vec4(0.0, -1.0 / 3.0, 2.0 / 3.0, -1.0);
        vec4 p = mix(vec4(c.bg, K.wz), vec4(c.gb, K.xy), step(c.b, c.g));
        vec4 q = mix(vec4(p.xyw, c.r), vec4(c.r, p.yzx), step(p.x, c.r));
        float d = q.x - min(q.w, q.y);
        float e = 1.0e-10;
        return vec3(abs(q.z + (q.w - q.y) / (6.0 * d + e)), d / (q.x + e), q.x);
    }

    vec3 hsv2rgb(vec3 c) {
        vec4 K = vec4(1.0, 2.0 / 3.0, 1.0 / 3.0, 3.0);
        vec3 p = abs(fract(c.xxx + K.xyz) * 6.0 - K.www);
        return c.z * mix(K.xxx, clamp(p - K.xxx, 0.0, 1.0), c.y);
    }

    float hash(vec2 p) {
        vec3 p3 = fract(vec3(p.xyx) * 0.13);
        p3 += dot(p3, p3.yzx + 3.333);
        return fract((p3.x + p3.y) * p3.z);
    }

    float perlin(vec2 x) {
        vec2 i = floor(x); vec2 f = fract(x);

        float a = hash(i); float b = hash(i + vec2(1.0, 0.0));
        float c = hash(i + vec2(0.0, 1.0)); float d = hash(i + vec2(1.0, 1.0));

        vec2 u = f * f * (3.0 - 2.0 * f);
        return mix(a, b, u.x) + (c - a) * u.y * (1.0 - u.x) + (d - b) * u.x * u.y;
    }

    float gradientNoise(in vec2 uv) {
        return fract(52.9829189 * fract(dot(uv, vec2(0.06711056, 0.00583715))));
    }

    vec4 main(vec2 fragCoord){
        vec2 vUv = fragCoord/uResolution;
        vUv.y = 1.0-vUv.y;
        vec2 uv = vUv;
        uv -= vec2(0., uTranslateY);
        uv.xy -= uBound.xy;
        uv.xy /= uBound.zw;

        vec4 color = vec4(0.0);
        float noiseValue = perlin(vUv * uNoiseScale + vec2(-uAnimTime, -uAnimTime));

        for (int i = 0; i < 4; i++){
            vec4 pointColor = uColors[i];
            pointColor.rgb *= pointColor.a;
            vec2 point = uPointsAnim[i];
            float rad = uPoints[i].z * uPointRadiusMulti;

            float d = distance(uv, point);
            float pct = smoothstep(rad, 0., d);
            color.rgb = mix(color.rgb, pointColor.rgb, pct);
            color.a = mix(color.a, pointColor.a, pct);
        }

        float oppositeNoise = smoothstep(0., 1., noiseValue);
        color.rgb /= color.a;
        vec3 hsv = rgb2hsv(color.rgb);
        hsv.y = mix(hsv.y, 0.0, oppositeNoise * uSaturateOffset);
        color.rgb = hsv2rgb(hsv);
        color.rgb += oppositeNoise * uLightOffset;

        color.a = clamp(color.a, 0., 1.);
        color.a *= uAlphaMulti;

        color += (10.0 / 255.0) * gradientNoise(fragCoord.xy) - (5.0 / 255.0);
        return vec4(color.rgb * color.a, color.a);
    }
"""
