package www.netproxy.web.ui

import android.R
import android.content.Context
import android.content.res.Resources.Theme
import android.util.TypedValue
import androidx.annotation.AttrRes
import androidx.core.content.ContextCompat
import java.util.concurrent.atomic.AtomicReference
import kotlin.collections.iterator
import com.google.android.material.R.attr as MaterialAttr

object MonetColorsProvider {
    private val colorsCss: AtomicReference<String?> = AtomicReference(null)

    fun getColorsCss(): String {
        return colorsCss.get() ?: ""
    }

    fun updateCss(context: Context) {
        val theme = context.theme
        val monetColors = mapOf(
            // App Base Colors
            "primary" to theme.getColorAttr(context, R.attr.colorPrimary),
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
            "surfaceTint" to theme.getColorAttr(context, R.attr.colorPrimary),
            "inverseSurface" to theme.getColorAttr(context, MaterialAttr.colorSurfaceInverse),
            "inverseOnSurface" to theme.getColorAttr(context, MaterialAttr.colorOnSurfaceInverse),
            "error" to theme.getColorAttr(context, MaterialAttr.colorOnErrorContainer),
            "onError" to theme.getColorAttr(context, MaterialAttr.colorOnError),
            "errorContainer" to theme.getColorAttr(context, MaterialAttr.colorErrorContainer),
            "onErrorContainer" to theme.getColorAttr(context, MaterialAttr.colorOnErrorContainer),
            "outline" to theme.getColorAttr(context, MaterialAttr.colorOutline),
            "outlineVariant" to theme.getColorAttr(context, MaterialAttr.colorOutlineVariant),
            "scrim" to theme.getColorAttr(context, R.attr.colorPrimaryDark),
            "surfaceBright" to theme.getColorAttr(context, MaterialAttr.colorSurfaceBright),
            "surfaceDim" to theme.getColorAttr(context, MaterialAttr.colorSurfaceDim),
            "surfaceContainer" to theme.getColorAttr(context, MaterialAttr.colorSurfaceContainer),
            "surfaceContainerHigh" to theme.getColorAttr(context, MaterialAttr.colorSurfaceContainerHigh),
            "surfaceContainerHighest" to theme.getColorAttr(context, MaterialAttr.colorSurfaceContainerHighest),
            "surfaceContainerLow" to theme.getColorAttr(context, MaterialAttr.colorSurfaceContainerLow),
            "surfaceContainerLowest" to theme.getColorAttr(context, MaterialAttr.colorSurfaceContainerLowest),
            "filledTonalButtonContentColor" to theme.getColorAttr(context, MaterialAttr.colorOnPrimaryContainer),
            "filledTonalButtonContainerColor" to theme.getColorAttr(context, MaterialAttr.colorSecondaryContainer),
            "filledTonalButtonDisabledContentColor" to theme.getColorAttr(context, MaterialAttr.colorOnSurfaceVariant),
            "filledTonalButtonDisabledContainerColor" to theme.getColorAttr(context, MaterialAttr.colorSurfaceVariant),
            "filledCardContentColor" to theme.getColorAttr(context, MaterialAttr.colorOnPrimaryContainer),
            "filledCardContainerColor" to theme.getColorAttr(context, MaterialAttr.colorPrimaryContainer),
            "filledCardDisabledContentColor" to theme.getColorAttr(context, MaterialAttr.colorOnSurfaceVariant),
            "filledCardDisabledContainerColor" to theme.getColorAttr(context, MaterialAttr.colorSurfaceVariant)
        )

        colorsCss.set(monetColors.toCssVars())
    }

    private fun Map<String, String>.toCssVars(): String {
        return buildString {
            append(":root {\n")
            for ((k, v) in this@toCssVars) {
                append("  --$k: $v;\n")
            }
            append("}\n")
        }
    }

    private fun Theme.getColorAttr(context: Context, @AttrRes attr: Int): String {
        val typedValue = TypedValue()
        resolveAttribute(attr, typedValue, true)
        val color = if (typedValue.type >= TypedValue.TYPE_FIRST_COLOR_INT && typedValue.type <= TypedValue.TYPE_LAST_COLOR_INT) {
            typedValue.data
        } else {
            ContextCompat.getColor(context, typedValue.resourceId)
        }
        return color.toCssValue()
    }

    private fun Int.toCssValue(): String {
        return String.format("#%06X%02X", this and 0xFFFFFF, (this ushr 24) and 0xFF)
    }
}
