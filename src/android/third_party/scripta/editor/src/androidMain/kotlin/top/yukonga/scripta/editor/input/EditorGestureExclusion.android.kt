package top.yukonga.scripta.editor.input

import androidx.compose.foundation.systemGestureExclusion
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Rect

actual fun Modifier.editorRightEdgeGestureExclusion(widthPx: Float): Modifier =
    systemGestureExclusion { coords ->
        Rect(coords.size.width - widthPx, 0f, coords.size.width.toFloat(), coords.size.height.toFloat())
    }
