package com.fanjv.netproxy.feature.proxy.presentation

import android.os.Build
import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.relocation.BringIntoViewRequester
import androidx.compose.foundation.relocation.bringIntoViewRequester
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.LayerBackdrop
import top.yukonga.miuix.kmp.glass.GlassSegmentedTabRowDefaults
import top.yukonga.miuix.kmp.glass.glass
import top.yukonga.miuix.kmp.theme.MiuixTheme

@Composable
internal fun PolicyGroupTabs(
    labels: List<String>,
    selectedIndex: Int,
    onSelect: (Int) -> Unit,
    backdrop: LayerBackdrop?,
    modifier: Modifier = Modifier,
) {
    if (labels.isEmpty()) return

    val defaults = GlassSegmentedTabRowDefaults
    val shape = defaults.shape()
    val colors = MiuixTheme.colorScheme
    val material = if (backdrop != null && Build.VERSION.SDK_INT >= 33) {
        Modifier.glass(
            backdrop = backdrop,
            shape = shape,
            material = defaults.material(),
            stroke = defaults.stroke(),
            shading = false,
        )
    } else Modifier

    // The glass track stays put; the labels and their selection move together.
    BoxWithConstraints(
        modifier.fillMaxWidth().height(defaults.Height).clip(shape)
            .background(colors.surfaceContainer).then(material).padding(defaults.TrackPadding),
    ) {
        val viewportWidth = maxWidth
        val equalShare = (viewportWidth - defaults.TabGap * (labels.size - 1)) / labels.size
        val minTabWidth = maxOf(72.dp, equalShare).coerceAtMost(viewportWidth)
        val tabShape = defaults.shape(defaults.Height - defaults.TrackPadding * 2)
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).selectableGroup(),
            horizontalArrangement = Arrangement.spacedBy(defaults.TabGap, Alignment.CenterHorizontally),
        ) {
            labels.forEachIndexed { index, label ->
                val active = index == selectedIndex
                val requester = remember { BringIntoViewRequester() }
                LaunchedEffect(active, label) {
                    if (active) requester.bringIntoView()
                }
                Box(
                    modifier = Modifier
                        .widthIn(min = minTabWidth, max = viewportWidth)
                        .fillMaxHeight()
                        .bringIntoViewRequester(requester)
                        .clip(tabShape)
                        .then(if (active) Modifier.background(colors.primaryContainer) else Modifier)
                        .selectable(selected = active, role = Role.Tab, onClick = { onSelect(index) })
                        .padding(horizontal = defaults.TabPaddingHorizontal),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        text = label,
                        style = if (active) MiuixTheme.textStyles.subtitle else MiuixTheme.textStyles.body2,
                        color = if (active) colors.onPrimaryContainer else colors.onSurface,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
        }
    }
}
