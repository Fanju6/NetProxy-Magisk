package com.fanjv.netproxy.core.ui.theme

import android.content.Context
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalContext
import top.yukonga.miuix.kmp.squircle.LocalSquircleEnabled
import top.yukonga.miuix.kmp.theme.ColorSchemeMode
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.ThemeColorSpec
import top.yukonga.miuix.kmp.theme.ThemePaletteStyle
import top.yukonga.miuix.kmp.theme.ThemeController as MiuixThemeController

/** 主题配色模式：跟随系统、浅色、深色，以及对应的 Monet 动态色与纯黑变体。 */
enum class ColorMode(val value: Int) {
    SYSTEM(0),
    LIGHT(1),
    DARK(2),
    MONET_SYSTEM(3),
    MONET_LIGHT(4),
    MONET_DARK(5),
    DARK_AMOLED(6);

    companion object {
        fun fromValue(value: Int) = entries.find { it.value == value } ?: SYSTEM
    }

    val isSystem: Boolean get() = value == SYSTEM.value || value == MONET_SYSTEM.value
    val isDark: Boolean get() = value == DARK.value || value == MONET_DARK.value || value == DARK_AMOLED.value
    val isMonet: Boolean get() = value in MONET_SYSTEM.value..MONET_DARK.value

    fun toNonMonetMode(): Int = when (this) {
        MONET_SYSTEM -> SYSTEM.value
        MONET_LIGHT -> LIGHT.value
        MONET_DARK, DARK_AMOLED -> DARK.value
        else -> value
    }

    fun toMonetMode(): Int = when (this) {
        SYSTEM -> MONET_SYSTEM.value
        LIGHT -> MONET_LIGHT.value
        DARK -> MONET_DARK.value
        else -> value
    }
}

/** 应用主题设置快照（配色、动态色、关键色、圆角、模糊、预测式返回、浮动底栏等）。 */
data class AppThemeSettings(
    val colorMode: ColorMode,
    val miuixMonet: Boolean,
    val keyColor: Int,
    val paletteStyle: ThemePaletteStyle,
    val colorSpec: ThemeColorSpec,
    val enableSmoothCorner: Boolean,
    val enableBlur: Boolean,
    val enablePredictiveBack: Boolean,
    val enableFloatingBottomBar: Boolean,
    val enableFloatingBottomBarBlur: Boolean,
)

/** 从 SharedPreferences 读取并组装 [AppThemeSettings]。 */
object AppThemeController {
    fun getAppThemeSettings(context: Context): AppThemeSettings {
        val prefs = context.getSharedPreferences("settings", Context.MODE_PRIVATE)
        var colorModeValue = prefs.getInt("color_mode", ColorMode.SYSTEM.value)
        val miuixMonet = if (prefs.contains("miuix_monet")) {
            prefs.getBoolean("miuix_monet", false)
        } else {
            colorModeValue in ColorMode.MONET_SYSTEM.value..ColorMode.MONET_DARK.value
        }
        val colorMode = ColorMode.fromValue(colorModeValue)
        colorModeValue = when {
            miuixMonet && !colorMode.isMonet -> colorMode.toMonetMode()
            !miuixMonet && colorMode.isMonet -> colorMode.toNonMonetMode()
            else -> colorModeValue
        }

        val paletteStyle = runCatching {
            ThemePaletteStyle.valueOf(
                prefs.getString("color_style", ThemePaletteStyle.TonalSpot.name)
                    ?: ThemePaletteStyle.TonalSpot.name
            )
        }.getOrDefault(ThemePaletteStyle.TonalSpot)
        val colorSpec = runCatching {
            ThemeColorSpec.valueOf(
                prefs.getString("color_spec", ThemeColorSpec.Spec2021.name)
                    ?: ThemeColorSpec.Spec2021.name
            )
        }.getOrDefault(ThemeColorSpec.Spec2021)

        return AppThemeSettings(
            colorMode = ColorMode.fromValue(colorModeValue),
            miuixMonet = miuixMonet,
            keyColor = prefs.getInt("key_color", 0),
            paletteStyle = paletteStyle,
            colorSpec = colorSpec,
            enableSmoothCorner = prefs.getBoolean("enable_smooth_corner", true),
            enableBlur = prefs.getBoolean("enable_blur", true),
            enablePredictiveBack = prefs.getBoolean("enable_predictive_back", false),
            enableFloatingBottomBar = prefs.getBoolean("enable_floating_bottom_bar", true),
            enableFloatingBottomBarBlur = prefs.getBoolean("enable_floating_bottom_bar_blur", true),
        )
    }
}

/** 应用根主题：按 [AppThemeSettings] 配置 Miuix 主题，并通过 CompositionLocal 下发各开关。 */
@Composable
fun NetProxyTheme(
    appThemeSettings: AppThemeSettings? = null,
    content: @Composable () -> Unit
) {
    val context = LocalContext.current
    val currentSettings = appThemeSettings ?: AppThemeController.getAppThemeSettings(context)
    val systemDarkTheme = isSystemInDarkTheme()
    val darkTheme =
        currentSettings.colorMode.isDark || (currentSettings.colorMode.isSystem && systemDarkTheme)

    val controller = MiuixThemeController(
        when (currentSettings.colorMode) {
            ColorMode.SYSTEM -> ColorSchemeMode.System
            ColorMode.LIGHT -> ColorSchemeMode.Light
            ColorMode.DARK -> ColorSchemeMode.Dark
            ColorMode.MONET_SYSTEM -> ColorSchemeMode.MonetSystem
            ColorMode.MONET_LIGHT -> ColorSchemeMode.MonetLight
            ColorMode.MONET_DARK, ColorMode.DARK_AMOLED -> ColorSchemeMode.MonetDark
        },
        keyColor = if (currentSettings.keyColor == 0) null else Color(currentSettings.keyColor),
        isDark = darkTheme,
        paletteStyle = currentSettings.paletteStyle,
        colorSpec = currentSettings.colorSpec,
    )

    MiuixTheme(
        colors = controller.currentColors(),
    ) {
        CompositionLocalProvider(
            LocalColorMode provides currentSettings.colorMode.value,
            LocalEnableBlur provides currentSettings.enableBlur,
            LocalEnablePredictiveBack provides currentSettings.enablePredictiveBack,
            LocalEnableFloatingBottomBar provides currentSettings.enableFloatingBottomBar,
            LocalEnableFloatingBottomBarBlur provides currentSettings.enableFloatingBottomBarBlur,
            LocalSquircleEnabled provides currentSettings.enableSmoothCorner,
            content = content,
        )
    }
}

/** 当前是否处于深色主题（供 Composable 只读读取）。 */
@Composable
@ReadOnlyComposable
fun isInDarkTheme(): Boolean {
    return when (LocalColorMode.current) {
        ColorMode.LIGHT.value, ColorMode.MONET_LIGHT.value -> false
        ColorMode.DARK.value, ColorMode.MONET_DARK.value, ColorMode.DARK_AMOLED.value -> true
        else -> isSystemInDarkTheme()
    }
}

val LocalColorMode = staticCompositionLocalOf { ColorMode.SYSTEM.value }

val LocalEnableBlur = staticCompositionLocalOf { true }

val LocalEnablePredictiveBack = staticCompositionLocalOf { false }

val LocalEnableFloatingBottomBar = staticCompositionLocalOf { true }

val LocalEnableFloatingBottomBarBlur = staticCompositionLocalOf { true }

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
