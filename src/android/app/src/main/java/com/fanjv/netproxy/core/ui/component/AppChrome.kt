package com.fanjv.netproxy.core.ui.component

import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.runtime.Composable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.DpSize
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.BasicComponentDefaults
import top.yukonga.miuix.kmp.blur.LayerBackdrop
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme

internal val LocalBarBackdrop = staticCompositionLocalOf<LayerBackdrop?> { null }
internal val LocalManagerBackdrop = staticCompositionLocalOf<LayerBackdrop?> { null }

@Composable
internal fun AppDropdown(
    title: String, items: List<String>, selectedIndex: Int,
    enabled: Boolean = true, modifier: Modifier = Modifier, startAction: (@Composable () -> Unit)? = null,
    insideMargin: PaddingValues = BasicComponentDefaults.InsideMargin, onSelectedIndexChange: (Int) -> Unit,
) {
    ManagerDropdown(title, items, selectedIndex, enabled, modifier, startAction, insideMargin, onSelectedIndexChange)
}

@Composable
internal fun AppTopAppBar(
    title: String, modifier: Modifier = Modifier, color: Color = colorScheme.surface,
    titleColor: Color = colorScheme.onSurface, subtitle: String = "",
    subtitleColor: Color = colorScheme.onSurfaceVariantSummary, scrollBehavior: ScrollBehavior? = null,
    navigationIcon: @Composable () -> Unit = {}, actions: @Composable RowScope.() -> Unit = {},
    bottomContent: @Composable () -> Unit = {},
) {
    ManagerAppBar(title, LocalBarBackdrop.current, false, modifier, color, titleColor, subtitle,
        subtitleColor, scrollBehavior, navigationIcon, actions, bottomContent)
}

@Composable
internal fun AppSmallTopAppBar(
    title: String, modifier: Modifier = Modifier, color: Color = colorScheme.surface,
    titleColor: Color = colorScheme.onSurface, subtitle: String = "",
    subtitleColor: Color = colorScheme.onSurfaceVariantSummary, scrollBehavior: ScrollBehavior? = null,
    navigationIcon: @Composable () -> Unit = {}, actions: @Composable RowScope.() -> Unit = {},
    bottomContent: @Composable () -> Unit = {},
) {
    ManagerAppBar(title, LocalBarBackdrop.current, true, modifier, color, titleColor, subtitle,
        subtitleColor, scrollBehavior, navigationIcon, actions, bottomContent)
}

@Composable
internal fun AppDialog(
    show: Boolean, title: String? = null, summary: String? = null,
    onDismissRequest: (() -> Unit)? = null, onDismissFinished: (() -> Unit)? = null,
    insideMargin: DpSize? = null, content: @Composable () -> Unit,
) {
    ManagerDialog(show, title, summary, onDismissRequest, onDismissFinished, insideMargin, content)
}

@Composable
internal fun AppBottomSheet(
    show: Boolean, title: String? = null, onDismissRequest: () -> Unit,
    content: @Composable () -> Unit,
) {
    ManagerSheet(show, title, onDismissRequest, content)
}
