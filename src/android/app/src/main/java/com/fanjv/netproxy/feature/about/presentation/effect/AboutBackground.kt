package com.fanjv.netproxy.feature.about.presentation.effect

import android.annotation.SuppressLint
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.spring
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.ContentDrawScope
import androidx.compose.ui.node.DrawModifierNode
import androidx.compose.ui.node.ModifierNodeElement
import androidx.compose.ui.node.invalidateDraw
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalWindowInfo
import com.fanjv.netproxy.core.ui.theme.isInDarkTheme
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import top.yukonga.miuix.kmp.shader.isRuntimeShaderSupported
import top.yukonga.miuix.kmp.theme.MiuixTheme
import kotlin.math.floor

/** 关于页的动态背景容器。 */
@Composable
internal fun AboutBackground(
    active: Boolean,
    modifier: Modifier = Modifier,
    backdropModifier: Modifier = Modifier,
    alpha: () -> Float = { 1f },
    content: @Composable BoxScope.() -> Unit,
) {
    if (!isRuntimeShaderSupported()) {
        Box(modifier = modifier, content = content)
        return
    }

    Box(modifier = modifier) {
        val surface = MiuixTheme.colorScheme.surface
        val containerSize = LocalWindowInfo.current.containerSize
        val density = LocalDensity.current
        val widthDp = with(density) { containerSize.width.toDp().value }
        val heightDp = with(density) { containerSize.height.toDp().value }
        val ratio = if (widthDp > 0f) heightDp / widthDp else Float.POSITIVE_INFINITY
        val deviceType = if (
            widthDp >= 840f ||
            widthDp >= 600f && ratio < 1.2f
        ) {
            AboutDeviceType.Pad
        } else {
            AboutDeviceType.Phone
        }
        val darkTheme = isInDarkTheme()
        val painter = remember { AboutBackgroundPainter() }
        val preset = remember(deviceType, darkTheme) {
            AboutBackgroundConfig.get(deviceType, darkTheme)
        }
        val colorStage = remember { Animatable(0f) }

        LaunchedEffect(active, preset) {
            if (!active) return@LaunchedEffect
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
                .then(backdropModifier)
                .aboutBackgroundDraw(
                    painter = painter,
                    preset = preset,
                    deviceType = deviceType,
                    darkTheme = darkTheme,
                    surface = surface,
                    active = active,
                    colorStage = { colorStage.value },
                    alpha = alpha,
                ),
        )
        content()
    }
}

private fun Modifier.aboutBackgroundDraw(
    painter: AboutBackgroundPainter,
    preset: AboutBackgroundConfig.Preset,
    deviceType: AboutDeviceType,
    darkTheme: Boolean,
    surface: Color,
    active: Boolean,
    colorStage: () -> Float,
    alpha: () -> Float,
): Modifier = this then AboutBackgroundElement(
    painter = painter,
    preset = preset,
    deviceType = deviceType,
    darkTheme = darkTheme,
    surface = surface,
    active = active,
    colorStage = colorStage,
    alpha = alpha,
)

@SuppressLint("ModifierNodeInspectableProperties")
private data class AboutBackgroundElement(
    val painter: AboutBackgroundPainter,
    val preset: AboutBackgroundConfig.Preset,
    val deviceType: AboutDeviceType,
    val darkTheme: Boolean,
    val surface: Color,
    val active: Boolean,
    val colorStage: () -> Float,
    val alpha: () -> Float,
) : ModifierNodeElement<AboutBackgroundNode>() {
    override fun create(): AboutBackgroundNode = AboutBackgroundNode(
        painter = painter,
        preset = preset,
        deviceType = deviceType,
        darkTheme = darkTheme,
        surface = surface,
        active = active,
        colorStage = colorStage,
        alpha = alpha,
    )

    override fun update(node: AboutBackgroundNode) {
        node.update(
            painter = painter,
            preset = preset,
            deviceType = deviceType,
            darkTheme = darkTheme,
            surface = surface,
            active = active,
            colorStage = colorStage,
            alpha = alpha,
        )
    }
}

@SuppressLint("NewApi")
private class AboutBackgroundNode(
    private var painter: AboutBackgroundPainter,
    private var preset: AboutBackgroundConfig.Preset,
    private var deviceType: AboutDeviceType,
    private var darkTheme: Boolean,
    private var surface: Color,
    private var active: Boolean,
    private var colorStage: () -> Float,
    private var alpha: () -> Float,
) : Modifier.Node(), DrawModifierNode {
    private var animationJob: Job? = null
    private var animationTime = 0f
    private var startOffset = 0f

    override fun onAttach() {
        if (active) startAnimation()
    }

    override fun onDetach() {
        animationJob?.cancel()
        animationJob = null
    }

    fun update(
        painter: AboutBackgroundPainter,
        preset: AboutBackgroundConfig.Preset,
        deviceType: AboutDeviceType,
        darkTheme: Boolean,
        surface: Color,
        active: Boolean,
        colorStage: () -> Float,
        alpha: () -> Float,
    ) {
        this.painter = painter
        this.preset = preset
        this.deviceType = deviceType
        this.darkTheme = darkTheme
        this.surface = surface
        this.colorStage = colorStage
        this.alpha = alpha

        if (this.active != active) {
            this.active = active
            if (active) startAnimation() else stopAnimation()
        }
        invalidateDraw()
    }

    private fun startAnimation() {
        animationJob?.cancel()
        startOffset = animationTime
        animationJob = coroutineScope.launch {
            val minimumFrameNanos = 1_000_000_000L / 60L
            val origin = withFrameNanos { it }
            var lastFrame = origin
            while (isActive) {
                val now = withFrameNanos { it }
                if (now - lastFrame < minimumFrameNanos) continue
                lastFrame = now
                animationTime = startOffset + (now - origin) / 1_000_000_000f
                invalidateDraw()
            }
        }
    }

    private fun stopAnimation() {
        animationJob?.cancel()
        animationJob = null
    }

    override fun ContentDrawScope.draw() {
        drawRect(surface)
        if (active) {
            val currentAlpha = alpha()
            if (currentAlpha > 0f) {
                painter.updateResolution(size.width, size.height)
                painter.updateBoundIfNeeded(size.height * 0.8f, size.height, size.width)
                painter.updatePresetIfNeeded(deviceType, darkTheme)
                painter.updateColors(preset, colorStage())
                painter.updateAnimationTime(animationTime)
                painter.updatePoints(animationTime, preset)
                drawRect(painter.brush, alpha = currentAlpha)
            }
        }
        drawContent()
    }
}
