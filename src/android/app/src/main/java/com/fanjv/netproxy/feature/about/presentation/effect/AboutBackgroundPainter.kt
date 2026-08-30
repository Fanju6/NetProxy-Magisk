package com.fanjv.netproxy.feature.about.presentation.effect

import android.annotation.SuppressLint
import top.yukonga.miuix.kmp.blur.asBrush
import top.yukonga.miuix.kmp.shader.RuntimeShader
import kotlin.math.cos
import kotlin.math.sin

@SuppressLint("NewApi")
internal class AboutBackgroundPainter {
    private val runtimeShader by lazy {
        RuntimeShader(ABOUT_BACKGROUND_SHADER).also(::initializeShader)
    }
    val brush get() = runtimeShader.asBrush()

    private val resolution = FloatArray(2)
    private val bound = FloatArray(4)
    private val colors = FloatArray(16)
    private val animatedPoints = FloatArray(8)

    private var animationTime = Float.NaN
    private var cachedDarkTheme: Boolean? = null
    private var cachedDeviceType: AboutDeviceType? = null
    private var cachedLogoHeight = Float.NaN
    private var cachedTotalHeight = Float.NaN
    private var cachedTotalWidth = Float.NaN
    private var cachedColorStage = Float.NaN
    private var cachedColorPreset: AboutBackgroundConfig.Preset? = null
    private var cachedPointsTime = Float.NaN
    private var cachedPointsPreset: AboutBackgroundConfig.Preset? = null

    private fun initializeShader(shader: RuntimeShader) {
        shader.setFloatUniform("uTranslateY", 0f)
        shader.setFloatUniform("uNoiseScale", 1.5f)
        shader.setFloatUniform("uPointRadiusMulti", 1f)
        shader.setFloatUniform("uAlphaMulti", 1f)
    }

    fun updateResolution(width: Float, height: Float) {
        if (resolution[0] == width && resolution[1] == height) return
        resolution[0] = width
        resolution[1] = height
        runtimeShader.setFloatUniform("uResolution", resolution)
    }

    fun updateAnimationTime(time: Float) {
        if (animationTime == time) return
        animationTime = time
        runtimeShader.setFloatUniform("uAnimTime", time)
    }

    fun updatePoints(time: Float, preset: AboutBackgroundConfig.Preset) {
        if (cachedPointsTime == time && cachedPointsPreset === preset) return
        repeat(4) { index ->
            val sourceX = preset.points[index * 3]
            val sourceY = preset.points[index * 3 + 1]
            val animatedX = sourceX + sin(time + sourceY) * preset.pointOffset
            animatedPoints[index * 2] = animatedX
            animatedPoints[index * 2 + 1] =
                sourceY + cos(time + animatedX) * preset.pointOffset
        }
        runtimeShader.setFloatUniform("uPointsAnim", animatedPoints)
        cachedPointsTime = time
        cachedPointsPreset = preset
    }

    fun updateColors(preset: AboutBackgroundConfig.Preset, stage: Float) {
        if (cachedColorPreset === preset && cachedColorStage == stage) return
        val base = stage.toInt()
        val fraction = stage - base
        val start = colorsForCycle(preset, base)
        val end = colorsForCycle(preset, base + 1)
        for (index in colors.indices) {
            colors[index] = start[index] + (end[index] - start[index]) * fraction
        }
        runtimeShader.setFloatUniform("uColors", colors)
        cachedColorPreset = preset
        cachedColorStage = stage
    }

    private fun colorsForCycle(preset: AboutBackgroundConfig.Preset, index: Int): FloatArray =
        when (index.mod(4)) {
            1 -> preset.colors1
            3 -> preset.colors3
            else -> preset.colors2
        }

    fun updateBoundIfNeeded(logoHeight: Float, totalHeight: Float, totalWidth: Float) {
        if (
            cachedLogoHeight == logoHeight &&
            cachedTotalHeight == totalHeight &&
            cachedTotalWidth == totalWidth
        ) {
            return
        }

        val heightRatio = logoHeight / totalHeight
        if (totalWidth <= totalHeight) {
            bound[0] = 0f
            bound[1] = 1f - heightRatio
            bound[2] = 1f
            bound[3] = heightRatio
        } else {
            val aspectRatio = totalWidth / totalHeight
            val centerY = 1f - heightRatio / 2f
            bound[0] = 0f
            bound[1] = centerY - aspectRatio / 2f
            bound[2] = 1f
            bound[3] = aspectRatio
        }
        runtimeShader.setFloatUniform("uBound", bound)
        cachedLogoHeight = logoHeight
        cachedTotalHeight = totalHeight
        cachedTotalWidth = totalWidth
    }

    fun updatePresetIfNeeded(deviceType: AboutDeviceType, darkTheme: Boolean) {
        if (cachedDeviceType == deviceType && cachedDarkTheme == darkTheme) return
        val preset = AboutBackgroundConfig.get(deviceType, darkTheme)
        runtimeShader.setFloatUniform("uPoints", preset.points)
        runtimeShader.setFloatUniform("uLightOffset", preset.lightOffset)
        runtimeShader.setFloatUniform("uSaturateOffset", preset.saturateOffset)
        cachedDeviceType = deviceType
        cachedDarkTheme = darkTheme
    }
}
