package com.fanjv.netproxy.core.ui.component

import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.NavigationItem
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.blur.LayerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.MoreCircle
import top.yukonga.miuix.kmp.menu.WindowIconCascadingDropdownMenu
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme

import androidx.compose.foundation.layout.fillMaxWidth
import top.yukonga.miuix.kmp.basic.NavigationBar
import top.yukonga.miuix.kmp.basic.NavigationBarItem
import top.yukonga.miuix.kmp.basic.SmallTopAppBar
import top.yukonga.miuix.kmp.basic.TabRow
import top.yukonga.miuix.kmp.basic.TabRowDefaults
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.overlay.OverlayBottomSheet
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.utils.overScrollVertical

internal const val useGlassChrome = false

internal fun Modifier.appBarOverscroll(scroll: ScrollBehavior) = overScrollVertical()

@Composable
internal fun ManagerDropdown(
    title: String, items: List<String>, selectedIndex: Int, enabled: Boolean, modifier: Modifier,
    startAction: (@Composable () -> Unit)?, insideMargin: PaddingValues, onSelectedIndexChange: (Int) -> Unit,
) {
    OverlayDropdownPreference(title = title, items = items, selectedIndex = selectedIndex,
        enabled = enabled, modifier = modifier, startAction = startAction, insideMargin = insideMargin,
        onSelectedIndexChange = onSelectedIndexChange)
}

@Composable
internal fun AppBarButton(onClick: () -> Unit, enabled: Boolean = true, holdDownState: Boolean = false, content: @Composable () -> Unit) {
    IconButton(onClick = onClick, enabled = enabled, holdDownState = holdDownState, content = content)
}

@Composable
internal fun ManagerNavigationBar(items: List<NavigationItem>, selected: Int, onSelect: (Int) -> Unit, backdrop: LayerBackdrop?, modifier: Modifier) {
    BlurredBar(backdrop) {
        NavigationBar(modifier = modifier.fillMaxWidth(), color = if (backdrop == null) colorScheme.surface else Color.Transparent) {
            items.forEachIndexed { index, item ->
                NavigationBarItem(modifier = Modifier.weight(1f), icon = item.icon, label = item.label,
                    selected = selected == index, onClick = { onSelect(index) })
            }
        }
    }
}

@Composable
internal fun ManagerAppBar(
    title: String, backdrop: LayerBackdrop?, compact: Boolean,
    modifier: Modifier, color: Color, titleColor: Color, subtitle: String, subtitleColor: Color,
    scrollBehavior: ScrollBehavior?, navigationIcon: @Composable () -> Unit,
    actions: @Composable RowScope.() -> Unit, bottomContent: @Composable () -> Unit,
) {
    if (compact) SmallTopAppBar(title = title, modifier = modifier, color = color, titleColor = titleColor,
        subtitle = subtitle, subtitleColor = subtitleColor, scrollBehavior = scrollBehavior,
        navigationIcon = navigationIcon, actions = actions, bottomContent = bottomContent)
    else TopAppBar(title = title, modifier = modifier, color = color, titleColor = titleColor,
        subtitle = subtitle, subtitleColor = subtitleColor, scrollBehavior = scrollBehavior,
        navigationIcon = navigationIcon, actions = actions, bottomContent = bottomContent)
}

@Composable
internal fun ManagerDialog(show: Boolean, title: String?, summary: String?, onDismissRequest: (() -> Unit)?,
    onDismissFinished: (() -> Unit)?, insideMargin: DpSize?, content: @Composable () -> Unit) {
    OverlayDialog(show = show, title = title, summary = summary, onDismissRequest = onDismissRequest,
        onDismissFinished = onDismissFinished, insideMargin = insideMargin ?: top.yukonga.miuix.kmp.layout.DialogDefaults.insideMargin,
        content = content)
}

@Composable
internal fun ManagerSheet(show: Boolean, title: String?, onDismissRequest: () -> Unit, content: @Composable () -> Unit) {
    OverlayBottomSheet(show = show, title = title, onDismissRequest = onDismissRequest, content = content)
}

@Composable
internal fun ManagerMenu(entries: List<DropdownEntry>, description: String, page: @Composable (@Composable () -> Unit) -> Unit) {
    page {
        WindowIconCascadingDropdownMenu(entries = entries) {
            Icon(MiuixIcons.MoreCircle, description, tint = colorScheme.onSurface)
        }
    }
}

@Composable
internal fun ManagerTabs(tabs: List<String>, selected: Int, onSelect: (Int) -> Unit,
    minWidth: androidx.compose.ui.unit.Dp, maxWidth: androidx.compose.ui.unit.Dp, color: Color) {
    TabRow(tabs = tabs, selectedTabIndex = selected, onTabSelected = onSelect,
        minWidth = minWidth, maxWidth = maxWidth, height = 40.dp, colors = TabRowDefaults.tabRowColors(backgroundColor = color))
}
