package com.fanjv.netproxy.feature.theme.presentation

import androidx.compose.runtime.Immutable
import com.fanjv.netproxy.core.ui.theme.AppThemeDefaults

@Immutable
data class ThemeUiState(
    val colorMode: Int = AppThemeDefaults.colorMode.value,
    val miuixMonet: Boolean = AppThemeDefaults.miuixMonet,
    val keyColor: Int = AppThemeDefaults.keyColor,
    val colorStyle: String = AppThemeDefaults.paletteStyle.name,
    val colorSpec: String = AppThemeDefaults.colorSpec.name,
    val enableBlur: Boolean = true,
    val enablePredictiveBack: Boolean = false,
    val enableSmoothCorner: Boolean = true,
    val pageScale: Float = 1.0f
)
