package com.fanjv.netproxy.feature.theme.presentation

import androidx.compose.runtime.Immutable
import top.yukonga.miuix.kmp.theme.ThemeColorSpec
import top.yukonga.miuix.kmp.theme.ThemePaletteStyle

@Immutable
data class ThemeUiState(
    val colorMode: Int = 0,
    val miuixMonet: Boolean = false,
    val keyColor: Int = 0,
    val colorStyle: String = ThemePaletteStyle.TonalSpot.name,
    val colorSpec: String = ThemeColorSpec.Spec2021.name,
    val enableBlur: Boolean = true,
    val enablePredictiveBack: Boolean = false,
    val enableFloatingBottomBar: Boolean = true,
    val enableFloatingBottomBarBlur: Boolean = true,
    val enableSmoothCorner: Boolean = true,
    val pageScale: Float = 1.0f
)


