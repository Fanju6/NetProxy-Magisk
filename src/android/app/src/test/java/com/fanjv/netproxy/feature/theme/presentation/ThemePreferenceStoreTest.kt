package com.fanjv.netproxy.feature.theme.presentation

import android.content.SharedPreferences
import com.fanjv.netproxy.core.ui.theme.AppThemeDefaults
import com.fanjv.netproxy.core.ui.theme.keyColorOptions
import java.lang.reflect.Proxy
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import top.yukonga.miuix.kmp.theme.ThemeColorSpec
import top.yukonga.miuix.kmp.theme.ThemePaletteStyle

class ThemePreferenceStoreTest {
    @Test
    fun emptyPreferencesEnableCyanMonetWithSystemAppearance() {
        val settings = ThemePreferenceStore(preferences()).read()

        assertTrue(settings.miuixMonet)
        assertEquals(0xFF00BCD4.toInt(), settings.keyColor)
        assertTrue(settings.keyColor in keyColorOptions)
        assertEquals(ThemePaletteStyle.TonalSpot.name, settings.colorStyle)
        assertEquals(ThemeColorSpec.Spec2025.name, settings.colorSpec)
    }

    @Test
    fun initialUiStateUsesTheSameDefaultsAsStoredPreferences() {
        val settings = ThemePreferenceStore(preferences()).read()
        val state = ThemeUiState()

        assertEquals(settings.miuixMonet, state.miuixMonet)
        assertEquals(settings.keyColor, state.keyColor)
        assertEquals(settings.colorStyle, state.colorStyle)
        assertEquals(settings.colorSpec, state.colorSpec)
    }

    @Test
    fun savedColorsAndScaleRemainUnchanged() {
        val settings = ThemePreferenceStore(
            preferences(
                "color_mode" to 2,
                "miuix_monet" to false,
                "key_color" to 0,
                "color_style" to ThemePaletteStyle.Neutral.name,
                "color_spec" to ThemeColorSpec.Spec2021.name,
                "page_scale" to 0.9f,
            )
        ).read()

        assertFalse(settings.miuixMonet)
        assertEquals(0, settings.keyColor)
        assertEquals(ThemePaletteStyle.Neutral.name, settings.colorStyle)
        assertEquals(ThemeColorSpec.Spec2021.name, settings.colorSpec)
        assertEquals(0.9f, settings.pageScale)
    }

    @Test
    fun disabledMonetDoesNotReenableWhenOtherPreferencesAreMissing() {
        val settings = ThemeManager(
            preferences("color_mode" to 1, "miuix_monet" to false)
        ).state.value

        assertFalse(settings.miuixMonet)
        assertEquals(AppThemeDefaults.keyColor, settings.keyColor)
    }

    @Test
    fun legacyColorModeOnlySuppliesTheDynamicColorPreference() {
        assertTrue(ThemePreferenceStore(preferences("color_mode" to 5)).read().miuixMonet)
        assertFalse(ThemePreferenceStore(preferences("color_mode" to 2)).read().miuixMonet)
    }

    private fun preferences(vararg values: Pair<String, Any>): SharedPreferences {
        val saved = values.toMap()
        return Proxy.newProxyInstance(
            SharedPreferences::class.java.classLoader,
            arrayOf(SharedPreferences::class.java),
        ) { _, method, arguments ->
            when (method.name) {
                "contains" -> saved.containsKey(arguments!![0])
                "getInt", "getBoolean", "getString", "getFloat" ->
                    saved[arguments!![0]] ?: arguments[1]
                else -> error("Unexpected SharedPreferences call: ${method.name}")
            }
        } as SharedPreferences
    }
}
