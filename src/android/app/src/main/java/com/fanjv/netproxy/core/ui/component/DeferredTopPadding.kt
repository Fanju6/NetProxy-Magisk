package com.fanjv.netproxy.core.ui.component

import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.layout
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.Dp

internal fun Modifier.deferredTopPadding(top: () -> Dp): Modifier =
    layout { measurable, constraints ->
        val topPx = top().roundToPx().coerceAtLeast(0)
        val placeable = measurable.measure(
            constraints.copy(
                minHeight = (constraints.minHeight - topPx).coerceAtLeast(0),
                maxHeight = if (constraints.maxHeight == Constraints.Infinity) Constraints.Infinity else (constraints.maxHeight - topPx).coerceAtLeast(
                    0
                )
            )
        )
        val width = placeable.width.coerceIn(constraints.minWidth, constraints.maxWidth)
        val height =
            (placeable.height + topPx).coerceIn(constraints.minHeight, constraints.maxHeight)
        layout(width, height) {
            placeable.place(0, topPx)
        }
    }
