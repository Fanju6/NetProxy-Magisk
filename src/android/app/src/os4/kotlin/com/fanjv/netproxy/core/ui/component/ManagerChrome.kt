package com.fanjv.netproxy.core.ui.component

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.disabled
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.NavigationItem
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.SmallTopAppBar
import top.yukonga.miuix.kmp.blur.LayerBackdrop
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.blur.rememberLayerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.MoreCircle
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme

import android.os.Build
import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import top.yukonga.miuix.kmp.layout.DialogDefaults
import top.yukonga.miuix.kmp.glass.*
import top.yukonga.miuix.kmp.utils.overScrollVertical

internal const val useGlassChrome = true

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
internal fun AppBarButton(onClick: () -> Unit, enabled: Boolean = true, holdDownState: Boolean = false, content: @Composable () -> Unit) {
    if (enabled) {
        GlassIconButton(onClick = onClick, content = content)
    } else {
        // Upstream GlassIconButton does not yet expose an enabled flag.
        IconButton(onClick = onClick, enabled = false,
            modifier = Modifier.size(GlassTopAppBarDefaults.ButtonSize).alpha(0.38f).semantics {
                role = Role.Button
                disabled()
            }, content = content)
    }
}

@Composable
internal fun ManagerNavigationBar(items: List<NavigationItem>, selected: Int, onSelect: (Int) -> Unit, backdrop: LayerBackdrop?, modifier: Modifier) {
    GlassNavigationBar(items.map { GlassNavigationItem(it.icon, it.label, it.label) }, selected, onSelect,
        backdrop = glassBackdrop(backdrop), modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp))
}

@Composable
internal fun ManagerAppBar(
    title: String, backdrop: LayerBackdrop?, compact: Boolean,
    modifier: Modifier, color: Color, titleColor: Color, subtitle: String, subtitleColor: Color,
    scrollBehavior: ScrollBehavior?, navigationIcon: @Composable () -> Unit,
    actions: @Composable RowScope.() -> Unit, bottomContent: @Composable () -> Unit,
) {
    if (compact) {
        // GlassTopAppBar only reveals its compact title through the collapse animation.
        ClassicBlurredBar(backdrop) {
            SmallTopAppBar(
                title = title,
                modifier = modifier,
                color = color,
                titleColor = titleColor,
                subtitle = subtitle,
                subtitleColor = subtitleColor,
                scrollBehavior = scrollBehavior,
                navigationIcon = navigationIcon,
                actions = { Row(horizontalArrangement = Arrangement.spacedBy(8.dp), content = actions) },
                bottomContent = bottomContent,
            )
        }
        return
    }
    val state = scrollBehavior?.state
    val scrolled by remember(state) {
        derivedStateOf { state == null || minOf(state.contentOffset, state.heightOffset) < 0f }
    }
    GlassTopAppBar(title = title, isContentScrolled = scrolled, modifier = modifier, backdrop = glassBackdrop(backdrop),
        largeTitle = title,
        subtitle = subtitle, scrollBehavior = scrollBehavior, titleAlpha = { titleColor.alpha },
        visualContentOffset = state?.let { state ->
            { minOf(state.contentOffset, state.heightOffset) + state.overscrollOffset }
        },
        progressiveBlur = backdrop != null && Build.VERSION.SDK_INT >= 33,
        navigationIcon = navigationIcon,
        actions = { Row(horizontalArrangement = Arrangement.spacedBy(8.dp), content = actions) },
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
                    Icon(MiuixIcons.MoreCircle, description, Modifier.glassPopupAnchorContent(anchor), tint = colorScheme.onSurface)
                }
            }
        }
        GlassIconDropdownMenu(show = expanded, onDismissRequest = { expanded = false }, anchor = anchor,
            anchorContent = { Icon(MiuixIcons.MoreCircle, null, tint = colorScheme.onSurface) },
            entries = entries, backdrop = glassBackdrop(scene),
            collapseOnSelection = true)
    }
}

@Composable
internal fun ManagerTabs(tabs: List<String>, selected: Int, onSelect: (Int) -> Unit,
    minWidth: androidx.compose.ui.unit.Dp, maxWidth: androidx.compose.ui.unit.Dp, color: Color) {
    if (tabs.isEmpty()) return
    val backdrop = glassBackdrop(LocalBarBackdrop.current)
    val source = backdrop ?: rememberLayerBackdrop()
    val shape = GlassSegmentedTabRowDefaults.shape()
    val track = if (backdrop == null) {
        Modifier.background(GlassSegmentedTabRowDefaults.restingTrackColor())
    } else {
        Modifier.glass(
            backdrop = backdrop,
            shape = shape,
            material = GlassSegmentedTabRowDefaults.material(),
            stroke = GlassSegmentedTabRowDefaults.stroke(),
            shading = false,
        )
    }
    BoxWithConstraints(Modifier.fillMaxWidth().clip(shape).then(track)) {
        val trackPadding = GlassSegmentedTabRowDefaults.TrackPadding
        val gap = GlassSegmentedTabRowDefaults.TabGap
        val viewport = this.maxWidth
        val slotMin = (minWidth + 4.dp).coerceAtMost((viewport - trackPadding * 2).coerceAtLeast(0.dp))
        val rowWidth = maxOf(viewport, (slotMin + gap) * tabs.size - gap + trackPadding * 2)
        val stride = (rowWidth - trackPadding * 2 + gap) / tabs.size
        val scroll = rememberScrollState()
        val density = LocalDensity.current
        LaunchedEffect(selected, stride, viewport) {
            val target = trackPadding + stride * selected + (stride - gap) / 2 - viewport / 2
            scroll.scrollTo(with(density) { target.roundToPx() }.coerceAtLeast(0))
        }
        // Only the segments and their indicator move; the track belongs to the viewport.
        Box(Modifier.horizontalScroll(scroll)) {
            GlassSegmentedTabRow(tabs = tabs, selectedIndex = selected, onSelect = onSelect,
                backdrop = source, surfaceAlpha = 0f, trackColor = Color.Transparent,
                shadow = null, stroke = null,
                modifier = Modifier.width(rowWidth))
        }
    }
}

private fun glassBackdrop(backdrop: LayerBackdrop?): LayerBackdrop? = backdrop?.takeIf { Build.VERSION.SDK_INT >= 33 }
