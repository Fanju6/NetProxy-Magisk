package com.fanjv.netproxy.core.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toArgb
import top.yukonga.miuix.kmp.theme.ColorSchemeMode
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.ThemeColorSpec
import top.yukonga.miuix.kmp.theme.ThemePaletteStyle
import top.yukonga.miuix.kmp.theme.ThemeController as MiuixThemeController

object AppThemeDefaults {
    const val miuixMonet = true
    val keyColor = 0xFF00BCD4.toInt()
    val paletteStyle = ThemePaletteStyle.TonalSpot
    val colorSpec = ThemeColorSpec.Spec2025
}

@Composable
internal fun NetProxyTheme(
    miuixMonet: Boolean,
    keyColor: Int,
    paletteStyle: ThemePaletteStyle,
    colorSpec: ThemeColorSpec,
    content: @Composable () -> Unit,
) {
    val controller = MiuixThemeController(
        if (miuixMonet) ColorSchemeMode.MonetSystem else ColorSchemeMode.System,
        keyColor = if (keyColor == 0) null else Color(keyColor),
        isDark = isSystemInDarkTheme(),
        paletteStyle = paletteStyle,
        colorSpec = colorSpec,
    )

    MiuixTheme(controller = controller, content = content)
}

@Composable
@ReadOnlyComposable
fun isInDarkTheme(): Boolean = isSystemInDarkTheme()

/** 主题设置中可选的预设关键色。 */
val keyColorOptions = listOf(
    Color(0xFFF44336).toArgb(),
    Color(0xFFE91E63).toArgb(),
    Color(0xFF9C27B0).toArgb(),
    Color(0xFF673AB7).toArgb(),
    Color(0xFF3F51B5).toArgb(),
    Color(0xFF2196F3).toArgb(),
    Color(0xFF00BCD4).toArgb(),
    Color(0xFF009688).toArgb(),
    Color(0xFF4FAF50).toArgb(),
    Color(0xFFFFEB3B).toArgb(),
    Color(0xFFFFC107).toArgb(),
    Color(0xFFFF9800).toArgb(),
    Color(0xFF795548).toArgb(),
    Color(0xFF607D8F).toArgb(),
    Color(0xFFFF9CA8).toArgb(),
)
