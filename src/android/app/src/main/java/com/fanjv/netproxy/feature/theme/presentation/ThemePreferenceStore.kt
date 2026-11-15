package com.fanjv.netproxy.feature.theme.presentation

import android.content.SharedPreferences
import androidx.core.content.edit
import com.fanjv.netproxy.core.ui.theme.AppThemeDefaults

/** 主题偏好的 SharedPreferences 读写适配器。 */
internal class ThemePreferenceStore(
    private val prefs: SharedPreferences
) {
    fun read(): ThemePreferencesSnapshot {
        val miuixMonet = if (prefs.contains("miuix_monet")) {
            prefs.getBoolean("miuix_monet", AppThemeDefaults.miuixMonet)
        } else {
            prefs.getInt("color_mode", 3) in 3..5
        }
        return ThemePreferencesSnapshot(
            miuixMonet = miuixMonet,
            keyColor = prefs.getInt("key_color", AppThemeDefaults.keyColor),
            colorStyle = prefs.getString("color_style", AppThemeDefaults.paletteStyle.name)
                ?: AppThemeDefaults.paletteStyle.name,
            colorSpec = prefs.getString("color_spec", AppThemeDefaults.colorSpec.name)
                ?: AppThemeDefaults.colorSpec.name,
            pageScale = prefs.getFloat("page_scale", 1.0f)
        )
    }

    fun setMiuixMonet(enabled: Boolean) = prefs.edit { putBoolean("miuix_monet", enabled) }
    fun setKeyColor(color: Int) = prefs.edit { putInt("key_color", color) }
    fun setColorStyle(style: String) = prefs.edit { putString("color_style", style) }
    fun setColorSpec(spec: String) = prefs.edit { putString("color_spec", spec) }
    fun setPageScale(scale: Float) = prefs.edit { putFloat("page_scale", scale) }
}

internal data class ThemePreferencesSnapshot(
    val miuixMonet: Boolean,
    val keyColor: Int,
    val colorStyle: String,
    val colorSpec: String,
    val pageScale: Float,
)
