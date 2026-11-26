package com.fanjv.netproxy.feature.proxy.presentation

import android.content.Context
import android.widget.Toast
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
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
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.core.content.edit
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.AppContentCard
import com.fanjv.netproxy.core.ui.component.AppBottomSheet
import com.fanjv.netproxy.core.ui.component.AppOverlayList
import com.fanjv.netproxy.core.ui.component.AppOverlayListDivider
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.EmptyCatalog
import com.fanjv.netproxy.core.ui.component.ManagerMenu
import com.fanjv.netproxy.core.ui.component.appBarOverscroll
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import com.fanjv.netproxy.core.ui.component.deferredTopPadding
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.resolve
import com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components.NodeCard
import com.fanjv.netproxy.feature.proxy.data.PolicyGroup
import com.fanjv.netproxy.feature.proxy.data.PolicyMember
import com.fanjv.netproxy.feature.proxy.data.editablePolicyTypes
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.DropdownItem
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.PullToRefresh
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.basic.Text
import com.fanjv.netproxy.core.ui.component.AppTopAppBar as TopAppBar
import top.yukonga.miuix.kmp.basic.rememberPullToRefreshState
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.ExpandLess
import top.yukonga.miuix.kmp.icon.extended.ExpandMore
import top.yukonga.miuix.kmp.preference.CheckboxPreference
import com.fanjv.netproxy.core.ui.component.AppDialog as OverlayDialog
import com.fanjv.netproxy.core.ui.component.AppDropdown as OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

@Composable
internal fun PolicyScreen(
    bottomPadding: Dp,
    isActive: Boolean,
    viewModel: PolicyViewModel,
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
    var showDisplay by rememberSaveable { mutableStateOf(false) }
    var deleteTarget by remember { mutableStateOf<PolicyGroup?>(null) }
    var nodeActions by remember { mutableStateOf<PolicyMember?>(null) }
    var addNode by remember { mutableStateOf<PolicyMember?>(null) }

    val groups = state.snapshot?.groups.orEmpty()
    val selectedIndex = groups.indexOfFirst { it.tag == selectedTag }.coerceAtLeast(0)
    val current = groups.getOrNull(selectedIndex)
    val openTags = expanded ?: listOfNotNull(current?.tag)
    val tabbed = layout == 0
    val shown = if (tabbed) listOfNotNull(current) else groups
    val canTest = state.live && !state.operating
    val canManageCurrent = !state.stale && !state.operating && current?.type in editablePolicyTypes
    val subscriptionSuffix = stringResource(R.string.policy_subscription_group_suffix)

    SnackbarNoticeEffect(state.noticeId, state.notice.resolve(), state.noticeIsError, snackbar, viewModel::clearNotice)
    LaunchedEffect(state.membershipNotice) {
        state.membershipNotice?.let {
            Toast.makeText(context, it, Toast.LENGTH_SHORT).show()
            viewModel.clearMembershipNotice()
        }
    }

    nodeActions?.let { member ->
        AppBottomSheet(show = true, title = member.displayName, onDismissRequest = { nodeActions = null }) {
            AppOverlayList(Modifier.navigationBarsPadding()) {
                BasicComponent(
                    title = stringResource(R.string.node_test_delay),
                    onClick = if (state.live && member.runtimeAvailable && !state.operating) ({
                        nodeActions = null
                        viewModel.testNode(member)
                    }) else null,
                )
                AppOverlayListDivider()
                BasicComponent(
                    title = stringResource(R.string.policy_node_add_to_groups),
                    onClick = {
                        nodeActions = null
                        addNode = member
                    },
                )
            }
        }
    }
    addNode?.let { member ->
        val targets = groups.filter { it.type in editablePolicyTypes && it.tag != member.tag }.map { it.tag }
        NodePolicyGroupPicker(targets, { addNode = null }) { tags ->
            addNode = null
            viewModel.addNodeToGroups(member, tags)
        }
    }

    ManagerMenu(
        entries = listOf(DropdownEntry(items = listOf(
                                DropdownItem(
                                    text = stringResource(R.string.node_test_current_group),
                                    enabled = canTest && current?.items?.any { it.runtimeAvailable } == true,
                                    onClick = { current?.let { viewModel.delay(it.tag) } },
                                ),
                                DropdownItem(
                                    text = stringResource(R.string.node_display_settings),
                                    onClick = { showDisplay = true },
                                ),
                                DropdownItem(
                                    text = stringResource(R.string.policy_manage_groups),
                                    children = listOf(
                                        DropdownItem(
                                            text = stringResource(R.string.policy_add_group),
                                            enabled = !state.operating,
                                            onClick = { navigator.push(Route.PolicyGroupEdit()) },
                                        ),
                                        DropdownItem(
                                            text = stringResource(R.string.policy_create_subscription_groups),
                                            enabled = !state.stale && !state.operating,
                                            onClick = { viewModel.createSubscriptionGroups(subscriptionSuffix) },
                                        ),
                                        DropdownItem(
                                            text = stringResource(R.string.policy_edit_current_group),
                                            enabled = canManageCurrent,
                                            onClick = { current?.let { navigator.push(Route.PolicyGroupEdit(it.tag)) } },
                                        ),
                                        DropdownItem(
                                            text = stringResource(R.string.policy_delete_current_group),
                                            enabled = canManageCurrent,
                                            onClick = { deleteTarget = current },
                                        ),
                                    ),
                                ),
        ))),
        description = stringResource(R.string.more_actions),
    ) { moreButton ->
    Scaffold(
        snackbarHost = { AppSnackbarHost(snackbar, Modifier.padding(bottom = bottomPadding)) },
        topBar = {
            BlurredBar(backdrop) {
                TopAppBar(
                    title = stringResource(R.string.proxies),
                    color = barColor,
                    scrollBehavior = scroll,
                    actions = { moreButton() },
                    bottomContent = {
                        if (tabbed && groups.isNotEmpty()) {
                            PolicyGroupTabs(
                                labels = groups.map { it.tag },
                                selectedIndex = selectedIndex,
                                onSelect = { index -> groups.getOrNull(index)?.let { selectedTag = it.tag } },
                                backdrop = backdrop,
                                modifier = Modifier.padding(horizontal = 12.dp).padding(bottom = 6.dp)
                                    .deferredTopPadding { 12.dp * (1f - scroll.state.collapsedFraction) },
                            )
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
                    modifier = Modifier.fillMaxSize().scrollEndHaptic().appBarOverscroll(scroll)
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
                    if (groups.isNotEmpty() && (state.stale || state.runtimeUnavailable)) {
                        item(key = "status", span = { GridItemSpan(maxLineSpan) }) {
                            SmallTitle(stringResource(
                                if (state.stale) R.string.nodes_read_failed else R.string.policy_runtime_unavailable
                            ))
                        }
                    }
                    shown.forEach { group ->
                        val open = tabbed || group.tag in openTags
                        if (!tabbed) {
                            item(key = "group:${group.tag}", span = { GridItemSpan(maxLineSpan) }) {
                                PolicyGroupHeader(group.tag, group.items.size, open) {
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
                                    title = member.displayName,
                                    protocol = member.type.uppercase(),
                                    latency = state.nodeLatency[member.tag].takeIf {
                                        state.live && state.nodeLatencyIdentity == state.snapshot?.coreIdentity
                                    } ?: member.delay.takeIf { state.live && it > 0 }?.toString(),
                                    selected = state.selectedMember(group) == member.tag,
                                    enabled = state.canSelect(group, member),
                                    itemSize = cardSize,
                                    onClick = { viewModel.select(group.tag, member.tag) },
                                    onLongClick = if (!state.stale && !state.operating && member.type.isNotBlank() &&
                                        member.type !in editablePolicyTypes && member.type !in setOf("block", "dns")
                                    ) ({ nodeActions = member }) else null,
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    }

    val target = deleteTarget
    OverlayDialog(
        show = target != null,
        title = stringResource(R.string.policy_delete_current_group),
        summary = target?.let { stringResource(R.string.policy_delete_group_summary, it.tag) },
        onDismissRequest = { deleteTarget = null },
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            AppDialogButton(
                text = stringResource(R.string.common_cancel),
                onClick = { deleteTarget = null },
                modifier = Modifier.weight(1f),
            )
            AppDialogButton(
                text = stringResource(R.string.common_delete),
                onClick = {
                    deleteTarget = null
                    target?.let { viewModel.deleteGroup(it.tag, it.type) }
                },
                enabled = target != null && !state.stale && !state.operating,
                modifier = Modifier.weight(1f),
                role = DialogActionRole.Destructive,
            )
        }
    }

    OverlayDialog(
        show = showDisplay,
        title = stringResource(R.string.node_display_settings),
        onDismissRequest = { showDisplay = false },
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
            AppOverlayList(Modifier.fillMaxWidth()) {
                OverlayDropdownPreference(
                    title = stringResource(R.string.node_layout_style),
                    items = listOf(stringResource(R.string.node_layout_tabs), stringResource(R.string.node_layout_list)),
                    selectedIndex = layout,
                    onSelectedIndexChange = { layout = it; preferences.edit { putInt("layout_style", it) } },
                )
                AppOverlayListDivider()
                OverlayDropdownPreference(
                    title = stringResource(R.string.node_sort),
                    items = listOf(R.string.node_sort_default, R.string.node_sort_name, R.string.node_sort_protocol,
                        R.string.node_sort_latency).map { stringResource(it) },
                    selectedIndex = sort,
                    onSelectedIndexChange = { sort = it; preferences.edit { putInt("sort", it) } },
                )
                AppOverlayListDivider()
                OverlayDropdownPreference(
                    title = stringResource(R.string.node_density),
                    items = listOf(R.string.node_density_spacious, R.string.node_density_standard,
                        R.string.node_density_compact).map { stringResource(it) },
                    selectedIndex = density,
                    onSelectedIndexChange = { density = it; preferences.edit { putInt("density", it) } },
                )
                AppOverlayListDivider()
                OverlayDropdownPreference(
                    title = stringResource(R.string.node_card_size),
                    items = listOf(R.string.node_card_size_standard, R.string.node_card_size_compact,
                        R.string.node_card_size_minimal).map { stringResource(it) },
                    selectedIndex = cardSize,
                    onSelectedIndexChange = { cardSize = it; preferences.edit { putInt("item_size", it) } },
                )
            }
            AppDialogButton(
                text = stringResource(R.string.common_done),
                modifier = Modifier.fillMaxWidth(),
                role = DialogActionRole.Primary,
                onClick = { showDisplay = false },
            )
        }
    }
}

@Composable
private fun NodePolicyGroupPicker(groups: List<String>, dismiss: () -> Unit, save: (Set<String>) -> Unit) {
    var checked by remember { mutableStateOf(emptySet<String>()) }
    OverlayDialog(show = true, title = stringResource(R.string.policy_node_add_to_groups), onDismissRequest = dismiss) {
        if (groups.isEmpty()) Text(stringResource(R.string.policy_group_no_choices))
        LazyColumn(Modifier.heightIn(max = 320.dp)) {
            items(groups, key = { it }) { tag ->
                CheckboxPreference(
                    title = tag,
                    checked = tag in checked,
                    onCheckedChange = { value -> checked = if (value) checked + tag else checked - tag },
                )
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            AppDialogButton(stringResource(R.string.common_cancel), modifier = Modifier.weight(1f), onClick = dismiss)
            AppDialogButton(stringResource(R.string.common_done), modifier = Modifier.weight(1f), enabled = checked.isNotEmpty(),
                onClick = { save(checked) })
        }
    }
}

@Composable
private fun PolicyGroupHeader(name: String, count: Int, expanded: Boolean, onClick: () -> Unit) {
    AppContentCard(
        modifier = Modifier.fillMaxWidth(),
        cornerRadius = 16.dp,
        insideMargin = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
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
