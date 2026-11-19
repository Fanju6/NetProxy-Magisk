package com.fanjv.netproxy.core.ui.component

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.widthIn
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.disabled
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.NavigationItem
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.LayerBackdrop
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.MoreCircle
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.textStyles

import android.os.Build
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import top.yukonga.miuix.kmp.layout.DialogDefaults
import top.yukonga.miuix.kmp.glass.*
import top.yukonga.miuix.kmp.utils.overScrollVertical


@Composable
internal fun AppDialogButton(
    text: String, onClick: () -> Unit, modifier: Modifier = Modifier,
    enabled: Boolean = true, role: DialogActionRole = DialogActionRole.Normal,
) {
    GlassDialogButton(
        onClick, modifier, enabled = enabled,
        role = when (role) {
            DialogActionRole.Normal -> GlassDialogButtonRole.Normal
            DialogActionRole.Primary -> GlassDialogButtonRole.Primary
            DialogActionRole.Destructive -> GlassDialogButtonRole.Destructive
        },
        insideMargin = PaddingValues(horizontal = 12.dp, vertical = 6.dp),
    ) {
        Text(text, style = textStyles.button, textAlign = TextAlign.Center)
    }
}

internal fun Modifier.appBarOverscroll(scroll: ScrollBehavior) =
    overScrollVertical(onOffsetChanged = { scroll.state.overscrollOffset = it })

@Composable
internal fun ManagerDropdown(
    title: String, items: List<String>, selectedIndex: Int, enabled: Boolean, modifier: Modifier,
    startAction: (@Composable () -> Unit)?, insideMargin: PaddingValues, onSelectedIndexChange: (Int) -> Unit,
) {
    GlassDropdownPreference(title = title, items = items, selectedIndex = selectedIndex,
        enabled = enabled, modifier = modifier, startAction = startAction, insideMargin = insideMargin,
        backdrop = glassBackdrop(LocalManagerBackdrop.current), onSelectedIndexChange = onSelectedIndexChange)
}

@Composable
internal fun AppBarButton(onClick: () -> Unit, enabled: Boolean = true, content: @Composable () -> Unit) {
    // Upstream has no enabled parameter; keep the operation guard without replacing its surface.
    GlassIconButton(
        onClick = { if (enabled) onClick() },
        backdrop = glassBackdrop(LocalBarBackdrop.current),
        modifier = if (enabled) Modifier else Modifier.semantics { disabled() },
        content = content,
    )
}

@Composable
internal fun ManagerNavigationBar(items: List<NavigationItem>, selected: Int, onSelect: (Int) -> Unit,
    backdrop: LayerBackdrop?, modifier: Modifier) {
    val scene = glassBackdrop(backdrop)
    BoxWithConstraints(
        modifier.fillMaxWidth().padding(top = 8.dp),
        contentAlignment = Alignment.Center,
    ) {
        GlassNavigationBar(
            items.map { GlassNavigationItem(chromeIcon(it.icon), it.label, it.label) }, selected, onSelect,
            backdrop = scene,
            modifier = Modifier.padding(horizontal = if (maxWidth < 360.dp) 16.dp else 24.dp)
                .widthIn(max = 410.dp),
        )
    }
}

@Composable
internal fun ManagerAppBar(
    title: String, backdrop: LayerBackdrop?, compact: Boolean,
    modifier: Modifier, color: Color, titleColor: Color, subtitle: String, subtitleColor: Color,
    scrollBehavior: ScrollBehavior?, navigationIcon: @Composable () -> Unit,
    actions: @Composable RowScope.() -> Unit, bottomContent: @Composable () -> Unit,
    contentOffset: (() -> Float)? = null,
) {
    val state = scrollBehavior?.state
    val scrolled by remember(state, contentOffset) {
        derivedStateOf {
            contentOffset?.invoke()?.let { it < 0f }
                ?: (state == null || minOf(state.contentOffset, state.heightOffset) < 0f)
        }
    }
    GlassTopAppBar(
        title = title, isContentScrolled = scrolled, modifier = modifier,
        expandable = !compact,
        backdrop = glassBackdrop(backdrop),
        largeTitle = if (compact) "" else title,
        subtitle = subtitle, scrollBehavior = scrollBehavior, titleAlpha = { titleColor.alpha },
        visualContentOffset = contentOffset ?: state?.let { state ->
            { minOf(state.contentOffset, state.heightOffset) + state.overscrollOffset }
        },
        progressiveBlur = backdrop != null && Build.VERSION.SDK_INT >= 33,
        navigationIcon = navigationIcon,
        actions = {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), content = actions)
        },
        bottomContent = bottomContent)
}

@Composable
internal fun ManagerDialog(show: Boolean, title: String?, summary: String?, onDismissRequest: (() -> Unit)?,
    onDismissFinished: (() -> Unit)?, insideMargin: DpSize?, content: @Composable () -> Unit) {
    GlassDialog(onDismissRequest = { onDismissRequest?.invoke() }, show = show,
        backdrop = glassBackdrop(LocalManagerBackdrop.current), title = title, summary = summary,
        onDismissFinished = onDismissFinished, insideMargin = insideMargin ?: DialogDefaults.insideMargin,
        content = content)
}

@Composable
internal fun ManagerSheet(show: Boolean, title: String?, onDismissRequest: () -> Unit, content: @Composable () -> Unit) {
    GlassBottomSheet(onDismissRequest = onDismissRequest, show = show,
        backdrop = glassBackdrop(LocalManagerBackdrop.current), title = title, content = content)
}

@Composable
internal fun ManagerMenu(entries: List<DropdownEntry>, description: String, page: @Composable (@Composable () -> Unit) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    val anchor = rememberGlassPopupAnchor()
    val scene = rememberBlurBackdrop()
    Box(Modifier.fillMaxSize()) {
        // Record the page and its anchor, never the popup that samples that recording.
        Box(Modifier.fillMaxSize().then(if (scene == null) Modifier else Modifier.layerBackdrop(scene))) {
            page {
                GlassIconButton(onClick = { expanded = !expanded },
                    modifier = Modifier.glassPopupAnchor(anchor, GlassTopAppBarDefaults.ButtonSize / 2, floating = true)) {
                    Icon(chromeIcon(MiuixIcons.MoreCircle), description, Modifier.glassPopupAnchorContent(anchor), tint = colorScheme.onSurface)
                }
            }
        }
        GlassIconDropdownMenu(show = expanded, onDismissRequest = { expanded = false }, anchor = anchor,
            anchorContent = { Icon(chromeIcon(MiuixIcons.MoreCircle), null, tint = colorScheme.onSurface) },
            entries = entries, backdrop = glassBackdrop(scene),
            collapseOnSelection = true)
    }
}


private fun glassBackdrop(backdrop: LayerBackdrop?): LayerBackdrop? = backdrop?.takeIf { Build.VERSION.SDK_INT >= 33 }
