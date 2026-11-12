package com.fanjv.netproxy.feature.proxy.presentation

import android.content.Context
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.add
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.core.content.edit
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.EmptyCatalog
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.core.ui.component.deferredTopPadding
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.resolve
import com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components.NodeCard
import com.fanjv.netproxy.feature.proxy.data.AUTO_POLICY_TAG
import com.fanjv.netproxy.feature.proxy.data.PolicyGroup
import com.fanjv.netproxy.feature.proxy.data.PolicyMember
import com.fanjv.netproxy.feature.proxy.data.editablePolicyTypes
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.PullToRefresh
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.basic.TabRow
import top.yukonga.miuix.kmp.basic.TabRowDefaults
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.basic.rememberPullToRefreshState
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.ExpandLess
import top.yukonga.miuix.kmp.icon.extended.ExpandMore
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.PressFeedbackType
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

@Composable
internal fun PolicyScreen(
    bottomPadding: Dp,
    isActive: Boolean,
    viewModel: PolicyViewModel = netProxyViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    val snackbar = rememberAppSnackbarHostState()
    val scroll = MiuixScrollBehavior()
    val pullToRefresh = rememberPullToRefreshState()
    val refreshTexts = listOf(
        stringResource(R.string.refresh_pulling),
        stringResource(R.string.refresh_release),
        stringResource(R.string.refresh_refresh),
        stringResource(R.string.refresh_complete),
    )
    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface
    val context = LocalContext.current
    val preferences = remember(context) { context.getSharedPreferences("node_display", Context.MODE_PRIVATE) }
    var layout by remember { mutableIntStateOf(preferences.getInt("layout_style", 0).coerceIn(0, 1)) }
    var density by remember { mutableIntStateOf(preferences.getInt("density", preferences.getInt("columns", 2) - 1).coerceIn(0, 2)) }
    var cardSize by remember { mutableIntStateOf(preferences.getInt("item_size", 0).coerceIn(0, 2)) }
    var sort by remember { mutableIntStateOf(preferences.getInt("sort", 0).coerceIn(0, 3)) }
    var selectedTag by rememberSaveable { mutableStateOf("") }
    var expanded by rememberSaveable { mutableStateOf<List<String>?>(null) }
    var showMenu by remember { mutableStateOf(false) }
    var showDisplay by rememberSaveable { mutableStateOf(false) }

    val groups = state.snapshot?.groups.orEmpty()
    val selectedIndex = groups.indexOfFirst { it.tag == selectedTag }.coerceAtLeast(0)
    val current = groups.getOrNull(selectedIndex)
    val openTags = expanded ?: listOfNotNull(current?.tag)
    val tabbed = layout == 0
    val shown = if (tabbed) listOfNotNull(current) else groups
    val canTest = state.live && state.pendingGroup == null

    LifecycleResumeEffect(isActive) {
        if (isActive) viewModel.setVisible(true)
        onPauseOrDispose { if (isActive) viewModel.setVisible(false) }
    }
    SnackbarNoticeEffect(state.noticeId, state.error.resolve(), true, snackbar, viewModel::clearNotice)

    Scaffold(
        snackbarHost = { AppSnackbarHost(snackbar, Modifier.padding(bottom = bottomPadding)) },
        topBar = {
            BlurredBar(backdrop) {
                TopAppBar(
                    title = stringResource(R.string.proxies),
                    color = barColor,
                    scrollBehavior = scroll,
                    actions = {
                        TopBarMoreMenu(
                            expanded = showMenu,
                            onExpandedChange = { showMenu = it },
                            actions = listOf(
                                TopBarMenuAction(
                                    text = stringResource(R.string.policy_add_group),
                                    enabled = state.pendingGroup == null,
                                    onClick = { navigator.push(Route.PolicyGroupEdit()) },
                                ),
                                TopBarMenuAction(
                                    text = stringResource(R.string.node_test_current_group),
                                    enabled = canTest && current?.items?.any { it.runtimeAvailable } == true,
                                    onClick = { current?.let { viewModel.delay(it.tag) } },
                                ),
                                TopBarMenuAction(stringResource(R.string.node_display_settings)) { showDisplay = true },
                                TopBarMenuAction(
                                    text = stringResource(R.string.policy_edit_group),
                                    enabled = current?.type in editablePolicyTypes && state.pendingGroup == null,
                                    onClick = { current?.let { navigator.push(Route.PolicyGroupEdit(it.tag)) } },
                                ),
                            ),
                            contentDescription = stringResource(R.string.more_actions)
                        )
                    },
                    bottomContent = {
                        if (tabbed && groups.isNotEmpty()) {
                            BoxWithConstraints(
                                modifier = Modifier.padding(horizontal = 12.dp).padding(bottom = 6.dp)
                                    .deferredTopPadding { 12.dp * (1f - scroll.state.collapsedFraction) },
                            ) {
                                val names = groups.map { policyLabel(it.tag, it.type) }
                                val measurer = rememberTextMeasurer()
                                val density = LocalDensity.current
                                val textStyle = MiuixTheme.textStyles.body1.copy(fontWeight = FontWeight.Bold)
                                val width = remember(names, measurer, density, textStyle, maxWidth) {
                                    val widest = names.maxOf { measurer.measure(it, style = textStyle, maxLines = 1).size.width }
                                    (with(density) { widest.toDp() } + 28.dp)
                                        .coerceIn(minOf(96.dp, maxWidth), maxWidth)
                                }
                                TabRow(
                                    tabs = names,
                                    selectedTabIndex = selectedIndex,
                                    onTabSelected = { index -> groups.getOrNull(index)?.let { selectedTag = it.tag } },
                                    minWidth = width,
                                    maxWidth = maxOf(width, minOf(116.dp, maxWidth)),
                                    height = 40.dp,
                                    colors = TabRowDefaults.tabRowColors(backgroundColor = barColor)
                                )
                            }
                        }
                    },
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.add(WindowInsets.displayCutout).only(WindowInsetsSides.Horizontal),
    ) { padding ->
        val direction = LocalLayoutDirection.current
        Box(Modifier.fillMaxSize().then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)) {
            PullToRefresh(
                isRefreshing = state.refreshing,
                onRefresh = { viewModel.refresh(showProgress = true) },
                pullToRefreshState = pullToRefresh,
                topAppBarScrollBehavior = scroll,
                refreshTexts = refreshTexts,
                contentPadding = PaddingValues(
                    top = padding.calculateTopPadding() + if (tabbed) 6.dp else 0.dp,
                    start = padding.calculateStartPadding(direction),
                    end = padding.calculateEndPadding(direction),
                ),
            ) {
                if (groups.isEmpty() && isActive) {
                    ContentStatus(
                        padding = PaddingValues(top = padding.calculateTopPadding(), bottom = bottomPadding),
                        text = stringResource(if (state.stale) R.string.nodes_read_failed else R.string.policy_empty),
                        loading = state.loading,
                    )
                }
                val spacing = if (density == 2) 8.dp else 10.dp
                val gridState = key(layout, if (tabbed) current?.tag else null) { rememberLazyGridState() }
                LazyVerticalGrid(
                    columns = GridCells.Fixed(density + 1),
                    state = gridState,
                    modifier = Modifier.fillMaxSize().scrollEndHaptic().overScrollVertical()
                        .nestedScroll(scroll.nestedScrollConnection),
                    contentPadding = PaddingValues(
                        start = padding.calculateStartPadding(direction) + 12.dp,
                        top = padding.calculateTopPadding() + if (tabbed) 6.dp else 12.dp,
                        end = padding.calculateEndPadding(direction) + 12.dp,
                        bottom = padding.calculateBottomPadding() + bottomPadding + 84.dp,
                    ),
                    horizontalArrangement = Arrangement.spacedBy(spacing),
                    verticalArrangement = Arrangement.spacedBy(spacing),
                    overscrollEffect = null
                ) {
                    if (groups.isNotEmpty() && (state.stale || state.runtime?.live == false)) {
                        item(key = "status", span = { GridItemSpan(maxLineSpan) }) {
                            SmallTitle(stringResource(if (state.stale) R.string.nodes_read_failed else R.string.policy_configured))
                        }
                    }
                    shown.forEach { group ->
                        val open = tabbed || group.tag in openTags
                        if (!tabbed) {
                            item(key = "group:${group.tag}", span = { GridItemSpan(maxLineSpan) }) {
                                PolicyGroupHeader(policyLabel(group.tag, group.type), group.items.size, open) {
                                    selectedTag = group.tag
                                    expanded = if (open) openTags - group.tag else openTags + group.tag
                                }
                            }
                        }
                        if (open) {
                            if (group.unavailableProviders.isNotEmpty()) {
                                item(key = "providers:${group.tag}", span = { GridItemSpan(maxLineSpan) }) {
                                    SmallTitle(stringResource(R.string.policy_providers_unavailable, group.unavailableProviders.joinToString()))
                                }
                            }
                            if (group.items.isEmpty()) {
                                item(key = "empty:${group.tag}", span = { GridItemSpan(maxLineSpan) }) {
                                    EmptyCatalog(
                                        text = stringResource(R.string.node_group_empty),
                                        onRefresh = null,
                                        modifier = Modifier.fillMaxWidth().padding(vertical = 32.dp),
                                    )
                                }
                            }
                            items(group.displayMembers(sort, state.live), key = { member ->
                                "member:${group.tag.length}:${group.tag}${member.tag}"
                            }) { member ->
                                NodeCard(
                                    title = when {
                                        member.tag == "direct" && member.type == "direct" -> stringResource(R.string.routing_direct)
                                        member.tag == "block" && member.type == "block" -> stringResource(R.string.routing_block)
                                        else -> policyLabel(member.tag, member.type, member.displayName)
                                    },
                                    protocol = member.type.uppercase(),
                                    latency = member.delay.takeIf { state.live && it > 0 }?.toString(),
                                    selected = state.live && group.selected == member.tag,
                                    enabled = group.selectable && member.runtimeAvailable && state.pendingGroup == null,
                                    itemSize = cardSize,
                                    onClick = { viewModel.select(group.tag, member.tag) },
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    OverlayDialog(
        show = showDisplay,
        title = stringResource(R.string.node_display_settings),
        onDismissRequest = { showDisplay = false },
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Card {
                OverlayDropdownPreference(
                    title = stringResource(R.string.node_layout_style),
                    items = listOf(stringResource(R.string.node_layout_tabs), stringResource(R.string.node_layout_list)),
                    selectedIndex = layout,
                    onSelectedIndexChange = { layout = it; preferences.edit { putInt("layout_style", it) } },
                )
                OverlayDropdownPreference(
                    title = stringResource(R.string.node_sort),
                    items = listOf(R.string.node_sort_default, R.string.node_sort_name, R.string.node_sort_protocol,
                        R.string.node_sort_latency).map { stringResource(it) },
                    selectedIndex = sort,
                    onSelectedIndexChange = { sort = it; preferences.edit { putInt("sort", it) } },
                )
                OverlayDropdownPreference(
                    title = stringResource(R.string.node_density),
                    items = listOf(R.string.node_density_spacious, R.string.node_density_standard,
                        R.string.node_density_compact).map { stringResource(it) },
                    selectedIndex = density,
                    onSelectedIndexChange = { density = it; preferences.edit { putInt("density", it) } },
                )
                OverlayDropdownPreference(
                    title = stringResource(R.string.node_card_size),
                    items = listOf(R.string.node_card_size_standard, R.string.node_card_size_compact,
                        R.string.node_card_size_minimal).map { stringResource(it) },
                    selectedIndex = cardSize,
                    onSelectedIndexChange = { cardSize = it; preferences.edit { putInt("item_size", it) } },
                )
            }
            TextButton(
                text = stringResource(R.string.common_done),
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.textButtonColorsPrimary(),
                onClick = { showDisplay = false },
            )
        }
    }
}

@Composable
private fun policyLabel(tag: String, type: String, fallback: String = tag): String =
    if (tag == AUTO_POLICY_TAG && type.equals("urltest", ignoreCase = true)) {
        stringResource(R.string.policy_auto_select)
    } else fallback

@Composable
private fun PolicyGroupHeader(name: String, count: Int, expanded: Boolean, onClick: () -> Unit) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        cornerRadius = 16.dp,
        insideMargin = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
        pressFeedbackType = PressFeedbackType.Sink,
        showIndication = true,
        onClick = onClick,
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(
                    text = name,
                    style = MiuixTheme.textStyles.body1.copy(fontWeight = FontWeight.Medium),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Text(
                    text = stringResource(R.string.node_count, count),
                    style = MiuixTheme.textStyles.body2,
                    color = colorScheme.onSurfaceVariantActions,
                )
            }
            Icon(
                imageVector = if (expanded) MiuixIcons.ExpandLess else MiuixIcons.ExpandMore,
                contentDescription = null,
                modifier = Modifier.size(20.dp),
                tint = colorScheme.onSurfaceVariantActions,
            )
        }
    }
}

internal val PolicyMember.displayName: String
    get() = name.ifEmpty { tag }

internal fun PolicyGroup.displayMembers(sort: Int, live: Boolean): List<PolicyMember> = when (sort) {
    1 -> items.sortedBy { it.displayName.lowercase() }
    2 -> items.sortedWith(compareBy<PolicyMember> { it.type }.thenBy { it.displayName.lowercase() })
    3 -> if (live) items.sortedBy { it.delay.takeIf { delay -> delay > 0 } ?: Int.MAX_VALUE } else items
    else -> items
}
