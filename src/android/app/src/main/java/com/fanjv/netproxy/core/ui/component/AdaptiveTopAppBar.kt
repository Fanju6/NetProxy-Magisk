package com.fanjv.netproxy.core.ui.component

import androidx.compose.foundation.layout.RowScope
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalWindowInfo
import androidx.compose.ui.unit.dp
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.SmallTopAppBar
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.theme.MiuixTheme

@Composable
fun AdaptiveTopAppBar(
    title: String,
    modifier: Modifier = Modifier,
    color: Color = MiuixTheme.colorScheme.surface,
    titleColor: Color = MiuixTheme.colorScheme.onSurface,
    subtitle: String = "",
    subtitleColor: Color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
    scrollBehavior: ScrollBehavior? = null,
    navigationIcon: @Composable () -> Unit = {},
    actions: @Composable RowScope.() -> Unit = {},
    bottomContent: @Composable () -> Unit = {},
) {
    val containerSize = LocalWindowInfo.current.containerSize
    val density = LocalDensity.current
    val isWideScreen = with(density) { containerSize.width.toDp() >= 600.dp }

    if (isWideScreen) {
        SmallTopAppBar(
            title = title,
            modifier = modifier,
            color = color,
            titleColor = titleColor,
            subtitle = subtitle,
            subtitleColor = subtitleColor,
            scrollBehavior = scrollBehavior,
            navigationIcon = navigationIcon,
            actions = actions,
            bottomContent = bottomContent,
        )
    } else {
        TopAppBar(
            title = title,
            modifier = modifier,
            color = color,
            titleColor = titleColor,
            subtitle = subtitle,
            subtitleColor = subtitleColor,
            scrollBehavior = scrollBehavior,
            navigationIcon = navigationIcon,
            actions = actions,
            bottomContent = bottomContent,
        )
    }
}
