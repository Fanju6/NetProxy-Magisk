package com.fanjv.netproxy.feature.theme.presentation

import android.content.SharedPreferences
import androidx.core.content.edit
import com.fanjv.netproxy.core.ui.theme.ColorMode
import top.yukonga.miuix.kmp.theme.ThemeColorSpec
import top.yukonga.miuix.kmp.theme.ThemePaletteStyle

/** 主题偏好的 SharedPreferences 读写适配器。 */
internal class ThemePreferenceStore(
    private val prefs: SharedPreferences
) {
    fun read(): ThemePreferencesSnapshot {
        val colorMode = prefs.getInt("color_mode", ColorMode.SYSTEM.value)
        val miuixMonet = if (prefs.contains("miuix_monet")) {
            prefs.getBoolean("miuix_monet", false)
        } else {
            colorMode in ColorMode.MONET_SYSTEM.value..ColorMode.MONET_DARK.value
        }
        return ThemePreferencesSnapshot(
            colorMode = colorMode,
            miuixMonet = miuixMonet,
            keyColor = prefs.getInt("key_color", 0),
            colorStyle = prefs.getString("color_style", ThemePaletteStyle.TonalSpot.name)
                ?: ThemePaletteStyle.TonalSpot.name,
            colorSpec = prefs.getString("color_spec", ThemeColorSpec.Spec2021.name)
                ?: ThemeColorSpec.Spec2021.name,
            enableBlur = prefs.getBoolean("enable_blur", true),
            enablePredictiveBack = prefs.getBoolean("enable_predictive_back", false),
            enableFloatingBottomBar = prefs.getBoolean("enable_floating_bottom_bar", true),
            enableFloatingBottomBarBlur = prefs.getBoolean("enable_floating_bottom_bar_blur", true),
            enableSmoothCorner = prefs.getBoolean("enable_smooth_corner", true),
            pageScale = prefs.getFloat("page_scale", 1.0f)
        )
    }

    fun setThemeMode(mode: Int, miuixMonet: Boolean): Int {
        val effectiveMode = if (miuixMonet) mode + 3 else mode
        prefs.edit { putInt("color_mode", effectiveMode) }
        return effectiveMode
    }

    fun setMiuixMonet(enabled: Boolean, colorMode: Int): Int {
        val currentColorMode = ColorMode.fromValue(colorMode)
        val newThemeMode = if (enabled) {
            if (!currentColorMode.isMonet) currentColorMode.toMonetMode() else currentColorMode.value
        } else {
            if (currentColorMode.isMonet) currentColorMode.toNonMonetMode() else currentColorMode.value
        }
        prefs.edit {
            putBoolean("miuix_monet", enabled)
            putInt("color_mode", newThemeMode)
        }
        return newThemeMode
    }

    fun setKeyColor(color: Int) = prefs.edit { putInt("key_color", color) }
    fun setColorStyle(style: String) = prefs.edit { putString("color_style", style) }
    fun setColorSpec(spec: String) = prefs.edit { putString("color_spec", spec) }
    fun setEnableBlur(enabled: Boolean) = prefs.edit { putBoolean("enable_blur", enabled) }
    fun setEnablePredictiveBack(enabled: Boolean) =
        prefs.edit { putBoolean("enable_predictive_back", enabled) }

    fun setEnableFloatingBottomBar(enabled: Boolean) =
        prefs.edit { putBoolean("enable_floating_bottom_bar", enabled) }

    fun setEnableFloatingBottomBarBlur(enabled: Boolean) =
        prefs.edit { putBoolean("enable_floating_bottom_bar_blur", enabled) }

    fun setEnableSmoothCorner(enabled: Boolean) =
        prefs.edit { putBoolean("enable_smooth_corner", enabled) }

    fun setPageScale(scale: Float) = prefs.edit { putFloat("page_scale", scale) }
}

internal data class ThemePreferencesSnapshot(
    val colorMode: Int,
    val miuixMonet: Boolean,
    val keyColor: Int,
    val colorStyle: String,
    val colorSpec: String,
    val enableBlur: Boolean,
    val enablePredictiveBack: Boolean,
    val enableFloatingBottomBar: Boolean,
    val enableFloatingBottomBarBlur: Boolean,
    val enableSmoothCorner: Boolean,
    val pageScale: Float,
)


