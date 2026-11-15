package com.fanjv.netproxy.core.ui.component

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.captionBar
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.union
import androidx.compose.foundation.layout.widthIn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
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
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.layout.onGloballyPositioned
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.NavigationItem
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.SmallTopAppBar
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TopAppBarState
import top.yukonga.miuix.kmp.blur.LayerBackdrop
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.MoreCircle
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.textStyles

import android.os.Build
import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.ui.Alignment
import top.yukonga.miuix.kmp.layout.DialogDefaults
import top.yukonga.miuix.kmp.glass.*
import top.yukonga.miuix.kmp.utils.overScrollVertical

private val LocalGlassBarScrolled = compositionLocalOf<Boolean?> { null }

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
internal fun AppBarButton(onClick: () -> Unit, enabled: Boolean = true, holdDownState: Boolean = false, content: @Composable () -> Unit) {
    val backdrop = glassBackdrop(LocalBarBackdrop.current)
    if (enabled) {
        GlassIconButton(onClick = onClick, backdrop = backdrop, content = content)
    } else {
        // Upstream has no disabled GlassIconButton; retain its material without a click target.
        val defaults = GlassTopAppBarDefaults
        val shape = GlassSegmentedTabRowDefaults.shape(defaults.ButtonSize)
        val scrolled = LocalGlassBarScrolled.current ?: true
        val surfaceAlpha by animateFloatAsState(
            if (scrolled) 1f else 0f, GlassMotion.topBarButtonFloat(),
        )
        val surface = if (backdrop == null) Modifier.background(defaults.buttonFill()) else Modifier.glass(
            backdrop = backdrop, shape = shape, material = defaults.buttonMaterial(),
            stroke = defaults.buttonStroke(), shading = false,
        )
        Box(
            modifier = Modifier.size(defaults.ButtonSize).semantics {
                role = Role.Button
                disabled()
            },
            contentAlignment = Alignment.Center,
        ) {
            Box(Modifier.matchParentSize().alpha(surfaceAlpha).clip(shape).then(surface))
            Box(Modifier.alpha(0.55f)) { content() }
        }
    }
}

@Composable
internal fun ManagerNavigationBar(items: List<NavigationItem>, selected: Int, onSelect: (Int) -> Unit,
    backdrop: LayerBackdrop?, modifier: Modifier) {
    val scene = glassBackdrop(backdrop)
    val systemBottom = WindowInsets.navigationBars.union(WindowInsets.captionBar).only(WindowInsetsSides.Bottom)
        .asPaddingValues().calculateBottomPadding()
    BoxWithConstraints(
        modifier.fillMaxWidth().padding(top = 8.dp, bottom = maxOf(24.dp, systemBottom + 8.dp)),
        contentAlignment = Alignment.Center,
    ) {
        GlassNavigationBar(
            items.map { GlassNavigationItem(chromeIcon(it.icon), it.label, it.label) }, selected, onSelect,
            backdrop = scene,
            defaultWindowInsetsPadding = false,
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
    if (compact && contentOffset == null) {
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
    // Keep the editor at the collapsed height; its scroll position only drives the glass.
    val pinnedState = if (compact) remember { TopAppBarState(-1f, -1f, 0f) } else null
    val behavior = if (pinnedState != null) MiuixScrollBehavior(state = pinnedState) else scrollBehavior
    val state = scrollBehavior?.state
    val scrolled by remember(state, contentOffset) {
        derivedStateOf {
            contentOffset?.invoke()?.let { it < 0f }
                ?: (state == null || minOf(state.contentOffset, state.heightOffset) < 0f)
        }
    }
    GlassTopAppBar(
        title = title, isContentScrolled = scrolled, modifier = modifier,
        contentModifier = if (pinnedState == null) Modifier else Modifier.onGloballyPositioned {
            pinnedState.heightOffset = pinnedState.heightOffsetLimit
        },
        backdrop = glassBackdrop(backdrop),
        largeTitle = if (compact) "" else title,
        subtitle = subtitle, scrollBehavior = behavior, titleAlpha = { titleColor.alpha },
        visualContentOffset = contentOffset ?: state?.let { state ->
            { minOf(state.contentOffset, state.heightOffset) + state.overscrollOffset }
        },
        progressiveBlur = backdrop != null && Build.VERSION.SDK_INT >= 33,
        navigationIcon = navigationIcon,
        actions = {
            CompositionLocalProvider(LocalGlassBarScrolled provides scrolled) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), content = actions)
            }
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

@Composable
internal fun ManagerTabs(tabs: List<String>, selected: Int, onSelect: (Int) -> Unit,
    minWidth: androidx.compose.ui.unit.Dp, maxWidth: androidx.compose.ui.unit.Dp, color: Color) {
    if (tabs.isEmpty()) return
    val backdrop = glassBackdrop(LocalBarBackdrop.current)
    val defaults = GlassSegmentedTabRowDefaults
    val shape = defaults.shape()
    val track = if (backdrop != null) {
        Modifier.glass(
            backdrop = backdrop,
            shape = shape,
            material = defaults.material(),
            stroke = defaults.stroke(),
            shading = false,
        )
    } else Modifier
    BoxWithConstraints(Modifier.fillMaxWidth().height(defaults.Height).clip(shape)
        .background(colorScheme.surfaceContainer).then(track)) {
        val trackPadding = defaults.TrackPadding
        val gap = defaults.TabGap
        val viewport = this.maxWidth
        val slotMin = (minWidth + 4.dp).coerceAtMost((viewport - trackPadding * 2).coerceAtLeast(0.dp))
        val rowWidth = maxOf(viewport, (slotMin + gap) * tabs.size - gap + trackPadding * 2)
        val stride = (rowWidth - trackPadding * 2 + gap) / tabs.size
        val scroll = rememberScrollState()
        val density = LocalDensity.current
        var positioned by remember { mutableStateOf(false) }
        LaunchedEffect(selected, stride, viewport) {
            val target = trackPadding + stride * selected + (stride - gap) / 2 - viewport / 2
            val offset = with(density) { target.roundToPx() }.coerceIn(0, scroll.maxValue)
            if (positioned) scroll.animateScrollTo(offset) else scroll.scrollTo(offset)
            positioned = true
        }
        // The selected background travels with its label, not on a separate animation.
        val tabShape = defaults.shape(defaults.Height - trackPadding * 2)
        Row(
            modifier = Modifier.fillMaxSize().horizontalScroll(scroll).width(rowWidth)
                .padding(trackPadding).selectableGroup(),
            horizontalArrangement = Arrangement.spacedBy(gap),
        ) {
            tabs.forEachIndexed { index, label ->
                val active = index == selected
                Box(
                    modifier = Modifier.weight(1f).fillMaxHeight().clip(tabShape)
                        .background(if (active) colorScheme.primaryContainer else Color.Transparent)
                        .selectable(selected = active, role = Role.Tab, onClick = { onSelect(index) })
                        .padding(horizontal = defaults.TabPaddingHorizontal),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        text = label,
                        style = if (active) textStyles.subtitle else textStyles.body2,
                        color = if (active) colorScheme.onPrimaryContainer else colorScheme.onSurface,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
        }
    }
}

private fun glassBackdrop(backdrop: LayerBackdrop?): LayerBackdrop? = backdrop?.takeIf { Build.VERSION.SDK_INT >= 33 }
