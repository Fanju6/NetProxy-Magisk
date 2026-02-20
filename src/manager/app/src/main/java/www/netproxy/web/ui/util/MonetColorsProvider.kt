package www.netproxy.web.ui.util

import android.content.Context
import android.content.res.Resources.Theme
import android.util.TypedValue
import androidx.annotation.AttrRes
import androidx.core.content.ContextCompat
import com.google.android.material.R.attr as MaterialAttr
import java.util.concurrent.atomic.AtomicReference

/**
 * 提供 Monet 动态颜色 CSS 变量
 * 
 * 从 Material Design 主题中提取颜色并生成 CSS 变量，
 * 供 WebUI 使用以实现动态主题色适配
 */
object MonetColorsProvider {
    private val colorsCss: AtomicReference<String?> = AtomicReference(null)

    /**
     * 获取当前缓存的 CSS 变量字符串
     * 
     * @return CSS 变量字符串，如果未初始化则返回空字符串
     */
    fun getColorsCss(): String {
        return colorsCss.get() ?: ""
    }

    /**
     * 兼容旧方法名
     * 
     * @return CSS 变量字符串
     */
    fun getCss(): String = getColorsCss()

    /**
     * 更新 CSS 变量缓存
     * 
     * 应在 Activity onCreate 和 onConfigurationChanged 时调用，
     * 以确保颜色与当前主题同步
     * 
     * @param context 用于获取主题和资源的上下文
     */
    fun updateCss(context: Context) {
        val theme = context.theme
        val monetColors = mapOf(
            "primary" to theme.getColorAttr(context, android.R.attr.colorPrimary),
            "onPrimary" to theme.getColorAttr(context, MaterialAttr.colorOnPrimary),
            "primaryContainer" to theme.getColorAttr(context, MaterialAttr.colorPrimaryContainer),
            "onPrimaryContainer" to theme.getColorAttr(context, MaterialAttr.colorOnPrimaryContainer),
            "inversePrimary" to theme.getColorAttr(context, MaterialAttr.colorPrimaryInverse),
            "secondary" to theme.getColorAttr(context, MaterialAttr.colorSecondary),
            "onSecondary" to theme.getColorAttr(context, MaterialAttr.colorOnSecondary),
            "secondaryContainer" to theme.getColorAttr(context, MaterialAttr.colorSecondaryContainer),
            "onSecondaryContainer" to theme.getColorAttr(context, MaterialAttr.colorOnSecondaryContainer),
            "tertiary" to theme.getColorAttr(context, MaterialAttr.colorTertiary),
            "onTertiary" to theme.getColorAttr(context, MaterialAttr.colorOnTertiary),
            "tertiaryContainer" to theme.getColorAttr(context, MaterialAttr.colorTertiaryContainer),
            "onTertiaryContainer" to theme.getColorAttr(context, MaterialAttr.colorOnTertiaryContainer),
            "background" to theme.getColorAttr(context, MaterialAttr.colorSurface),
            "onBackground" to theme.getColorAttr(context, MaterialAttr.colorOnBackground),
            "surface" to theme.getColorAttr(context, MaterialAttr.colorSurface),
            "tonalSurface" to theme.getColorAttr(context, MaterialAttr.colorSurfaceContainer),
            "onSurface" to theme.getColorAttr(context, MaterialAttr.colorOnSurface),
            "surfaceVariant" to theme.getColorAttr(context, MaterialAttr.colorSurfaceVariant),
            "onSurfaceVariant" to theme.getColorAttr(context, MaterialAttr.colorOnSurfaceVariant),
            "surfaceTint" to theme.getColorAttr(context, android.R.attr.colorPrimary),
            "inverseSurface" to theme.getColorAttr(context, MaterialAttr.colorSurfaceInverse),
            "inverseOnSurface" to theme.getColorAttr(context, MaterialAttr.colorOnSurfaceInverse),
            "error" to theme.getColorAttr(context, MaterialAttr.colorOnErrorContainer),
            "onError" to theme.getColorAttr(context, MaterialAttr.colorOnError),
            "errorContainer" to theme.getColorAttr(context, MaterialAttr.colorErrorContainer),
            "onErrorContainer" to theme.getColorAttr(context, MaterialAttr.colorOnErrorContainer),
            "outline" to theme.getColorAttr(context, MaterialAttr.colorOutline),
            "outlineVariant" to theme.getColorAttr(context, MaterialAttr.colorOutlineVariant),
            "scrim" to theme.getColorAttr(context, android.R.attr.colorPrimaryDark),
            "surfaceBright" to theme.getColorAttr(context, MaterialAttr.colorSurfaceBright),
            "surfaceDim" to theme.getColorAttr(context, MaterialAttr.colorSurfaceDim),
            "surfaceContainer" to theme.getColorAttr(context, MaterialAttr.colorSurfaceContainer),
            "surfaceContainerHigh" to theme.getColorAttr(context, MaterialAttr.colorSurfaceContainerHigh),
            "surfaceContainerHighest" to theme.getColorAttr(context, MaterialAttr.colorSurfaceContainerHighest),
            "surfaceContainerLow" to theme.getColorAttr(context, MaterialAttr.colorSurfaceContainerLow),
            "surfaceContainerLowest" to theme.getColorAttr(context, MaterialAttr.colorSurfaceContainerLowest)
        )

        colorsCss.set(monetColors.toCssVars())
    }

    /**
     * 将颜色 Map 转换为 CSS 变量字符串
     * 
     * @return 格式化的 CSS :root 变量声明
     */
    private fun Map<String, String>.toCssVars(): String {
        return buildString {
            append(":root {\n")
            for ((k, v) in this@toCssVars) {
                append("  --$k: $v;\n")
            }
            append("}\n")
        }
    }

    /**
     * 从主题中获取颜色属性值
     * 
     * @param context 用于解析资源引用的上下文
     * @param attr 主题属性 ID
     * @return CSS 格式的颜色值（带 Alpha 通道）
     */
    private fun Theme.getColorAttr(context: Context, @AttrRes attr: Int): String {
        val typedValue = TypedValue()
        resolveAttribute(attr, typedValue, true)
        val color = if (typedValue.type >= TypedValue.TYPE_FIRST_COLOR_INT && 
                        typedValue.type <= TypedValue.TYPE_LAST_COLOR_INT) {
            typedValue.data
        } else {
            ContextCompat.getColor(context, typedValue.resourceId)
        }
        return color.toCssValue()
    }

    /**
     * 将颜色值转换为 CSS 格式（带 Alpha 通道）
     * 
     * @return CSS 格式的颜色值，格式为 #RRGGBBAA
     */
    private fun Int.toCssValue(): String {
        return String.format("#%06X%02X", this and 0xFFFFFF, (this ushr 24) and 0xFF)
    }
}
