package com.fanjv.netproxy.feature.nodes.presentation

import android.content.ClipData
import android.content.ClipboardManager
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.FileOpen
import androidx.compose.material.icons.rounded.Link
import androidx.compose.material.icons.rounded.NetworkPing
import androidx.compose.material.icons.rounded.Share
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.nestedscroll.NestedScrollConnection
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.edit
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.EmptyCatalog
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.feature.catalog.model.CatalogNode
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route.NodeEdit
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.CardDefaults
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.PullToRefresh
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.TabRow
import top.yukonga.miuix.kmp.basic.TabRowDefaults
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.basic.rememberPullToRefreshState
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Add
import top.yukonga.miuix.kmp.icon.extended.ExpandLess
import top.yukonga.miuix.kmp.icon.extended.ExpandMore
import top.yukonga.miuix.kmp.icon.extended.Refresh
import top.yukonga.miuix.kmp.overlay.OverlayBottomSheet
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.PressFeedbackType
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

/** Catalog 节点页：离线读取分组，运行时仅负责选择与测速。 */
@Composable
internal fun CatalogNodesScreen(
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
    isActive: Boolean = true,
    viewModel: CatalogNodesViewModel = netProxyViewModel()
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    val context = LocalContext.current
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface
    var showAddSheet by remember { mutableStateOf(false) }
    var showLinkDialog by remember { mutableStateOf(false) }
    var showDisplaySettings by remember { mutableStateOf(false) }
    var showMoreMenu by remember { mutableStateOf(false) }
    var nodeLink by remember { mutableStateOf("") }
    var actionNode by remember { mutableStateOf<Pair<CatalogNodeGroup, CatalogNode>?>(null) }
    val displayPreferences = remember(context) {
        context.getSharedPreferences("node_display", android.content.Context.MODE_PRIVATE)
    }
    var layoutStyle by remember {
        mutableIntStateOf(displayPreferences.getInt("layout_style", 0).coerceIn(0, 1))
    }
    var layoutDensity by remember {
        mutableIntStateOf(
            displayPreferences.getInt(
                "density",
                displayPreferences.getInt("columns", 2) - 1
            ).coerceIn(0, 2)
        )
    }
    var itemSize by remember {
        mutableIntStateOf(displayPreferences.getInt("item_size", 0).coerceIn(0, 2))
    }
    var sortMode by remember {
        mutableIntStateOf(displayPreferences.getInt("sort", 0).coerceIn(0, 3))
    }
    var expandedGroups by remember {
        mutableStateOf(
            displayPreferences.getStringSet("expanded_groups", emptySet()).orEmpty()
        )
    }
    val pullToRefreshState = rememberPullToRefreshState()
    val snackbarHostState = rememberAppSnackbarHostState()
    val refreshTexts = listOf(
        stringResource(R.string.refresh_pulling),
        stringResource(R.string.refresh_release),
        stringResource(R.string.refresh_refresh),
        stringResource(R.string.refresh_complete),
    )

    DisposableEffect(isActive) {
        viewModel.setVisible(isActive)
        onDispose { if (isActive) viewModel.setVisible(false) }
    }

    val fileLauncher =
        rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri: Uri? ->
            uri?.let { viewModel.importFile(it) }
        }

    val noticeText = state.error.ifBlank { state.notice }
    SnackbarNoticeEffect(
        eventId = state.noticeId,
        message = noticeText,
        isError = state.error.isNotBlank(),
        hostState = snackbarHostState,
        onConsumed = viewModel::clearNotice
    )

    LaunchedEffect(state.exportedNodeLinkId) {
        val link = state.exportedNodeLink
        if (link.isBlank()) return@LaunchedEffect
        val clipboard = context.getSystemService(ClipboardManager::class.java)
        clipboard?.setPrimaryClip(ClipData.newPlainText("NetProxy node", link))
        viewModel.nodeLinkCopied()
    }

    val selectedIndex = state.groups.indexOfFirst { it.group.id == state.selectedGroupId }
        .coerceAtLeast(0)
    val selectedGroup = state.groups.getOrNull(selectedIndex)
    Scaffold(
        snackbarHost = {
            AppSnackbarHost(snackbarHostState, Modifier.padding(bottom = bottomPadding))
        },
        topBar = {
            BlurredBar(backdrop) {
                Column {
                    TopAppBar(
                        color = barColor,
                        title = "节点",
                        scrollBehavior = scrollBehavior,
                        actions = {
                            IconButton(onClick = { showAddSheet = true }) {
                                Icon(
                                    imageVector = MiuixIcons.Add,
                                    contentDescription = "添加节点",
                                    tint = colorScheme.onSurface
                                )
                            }
                            TopBarMoreMenu(
                                expanded = showMoreMenu,
                                onExpandedChange = { showMoreMenu = it },
                                actions = listOf(
                                    TopBarMenuAction(
                                        text = "测试当前分组延迟",
                                        enabled = selectedGroup?.nodes?.isNotEmpty() == true &&
                                                state.operation.isEmpty(),
                                        onClick = {
                                            selectedGroup?.group?.id?.let(viewModel::testGroupDelay)
                                        }
                                    ),
                                    TopBarMenuAction(
                                        text = "节点显示设置",
                                        onClick = { showDisplaySettings = true }
                                    )
                                ),
                                contentDescription = stringResource(R.string.more_actions)
                            )
                        }
                    )
                    if (state.groups.isNotEmpty() && layoutStyle == 0) {
                        TabRow(
                            tabs = state.groups.map {
                                val name =
                                    if (it.group.id == "default") "本地配置" else it.group.name
                                "$name (${it.nodes.size})"
                            },
                            selectedTabIndex = selectedIndex,
                            onTabSelected = { index ->
                                state.groups.getOrNull(index)?.group?.id?.let(viewModel::selectGroup)
                            },
                            modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                            colors = TabRowDefaults.tabRowColors(
                                backgroundColor = Color.Transparent
                            )
                        )
                    }
                }
            }
        },
        contentWindowInsets = WindowInsets.systemBars.only(WindowInsetsSides.Horizontal)
    ) { innerPadding ->
        Box(
            modifier = Modifier
                .fillMaxSize()
                .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)
        ) {
            when {
                !isActive && state.groups.isEmpty() -> Unit

                state.loading && state.groups.isEmpty() -> InfiniteProgressIndicator(
                    modifier = Modifier.align(Alignment.Center)
                )

                state.error.isNotBlank() && state.groups.isEmpty() -> EmptyCatalog(
                    text = state.error,
                    onRefresh = { viewModel.refresh() },
                    modifier = Modifier
                        .align(Alignment.Center)
                        .padding(horizontal = 24.dp)
                )

                selectedGroup == null -> EmptyCatalog(
                    text = "还没有可用节点\n请添加单节点链接或导入配置文件",
                    onRefresh = { showAddSheet = true },
                    modifier = Modifier
                        .align(Alignment.Center)
                        .padding(horizontal = 24.dp)
                )

                else -> PullToRefresh(
                    isRefreshing = state.loading,
                    onRefresh = { viewModel.refresh() },
                    pullToRefreshState = pullToRefreshState,
                    topAppBarScrollBehavior = scrollBehavior,
                    refreshTexts = refreshTexts,
                    contentPadding = PaddingValues(top = innerPadding.calculateTopPadding())
                ) {
                    val contentPadding = PaddingValues(
                        start = 12.dp,
                        top = innerPadding.calculateTopPadding() + 12.dp,
                        end = 12.dp,
                        bottom = innerPadding.calculateBottomPadding() + bottomPadding + 84.dp
                    )
                    if (layoutStyle == 0) {
                        CatalogNodeGrid(
                            group = selectedGroup,
                            selectedRef = state.selection.selected,
                            selectorMode = state.selection.selectorMode,
                            latencies = state.latencies,
                            busy = state.operation.isNotEmpty(),
                            columns = layoutDensity + 1,
                            itemSize = itemSize,
                            sortMode = sortMode,
                            onAuto = { viewModel.useAuto(selectedGroup.group.id) },
                            onNode = { viewModel.useNode(selectedGroup.group.id, it.tag) },
                            onNodeAction = { actionNode = selectedGroup to it },
                            modifier = Modifier.fillMaxSize(),
                            contentPadding = contentPadding,
                            nestedScrollConnection = scrollBehavior.nestedScrollConnection
                        )
                    } else {
                        CatalogGroupList(
                            groups = state.groups,
                            selectedRef = state.selection.selected,
                            selectorMode = state.selection.selectorMode,
                            latencies = state.latencies,
                            busy = state.operation.isNotEmpty(),
                            columns = layoutDensity + 1,
                            itemSize = itemSize,
                            sortMode = sortMode,
                            expandedGroups = expandedGroups,
                            onToggleGroup = { groupId ->
                                expandedGroups = if (groupId in expandedGroups) {
                                    expandedGroups - groupId
                                } else {
                                    expandedGroups + groupId
                                }
                                displayPreferences.edit {
                                    putStringSet("expanded_groups", expandedGroups)
                                }
                            },
                            onAuto = viewModel::useAuto,
                            onNode = { group, node -> viewModel.useNode(group.group.id, node.tag) },
                            onNodeAction = { group, node -> actionNode = group to node },
                            modifier = Modifier.fillMaxSize(),
                            contentPadding = contentPadding,
                            nestedScrollConnection = scrollBehavior.nestedScrollConnection
                        )
                    }
                }
            }
        }
    }

    OverlayDialog(
        show = showDisplaySettings,
        title = "节点显示设置",
        onDismissRequest = { showDisplaySettings = false }
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Card {
                OverlayDropdownPreference(
                    title = "布局样式",
                    items = listOf("分组标签页", "分组列表"),
                    selectedIndex = layoutStyle,
                    onSelectedIndexChange = { index ->
                        layoutStyle = index
                        displayPreferences.edit { putInt("layout_style", index) }
                    }
                )
                OverlayDropdownPreference(
                    title = "排序",
                    items = listOf("默认", "名称", "协议", "延迟"),
                    selectedIndex = sortMode,
                    onSelectedIndexChange = { index ->
                        sortMode = index
                        displayPreferences.edit { putInt("sort", sortMode) }
                    }
                )
                OverlayDropdownPreference(
                    title = "疏密",
                    items = listOf("松散", "标准", "紧凑"),
                    selectedIndex = layoutDensity,
                    onSelectedIndexChange = { index ->
                        layoutDensity = index
                        displayPreferences.edit { putInt("density", index) }
                    }
                )
                OverlayDropdownPreference(
                    title = "卡片尺寸",
                    items = listOf("标准", "紧凑", "极简"),
                    selectedIndex = itemSize,
                    onSelectedIndexChange = { index ->
                        itemSize = index
                        displayPreferences.edit { putInt("item_size", index) }
                    }
                )
            }
            TextButton(
                text = "完成",
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.textButtonColorsPrimary(),
                onClick = { showDisplaySettings = false }
            )
        }
    }

    OverlayBottomSheet(
        show = showAddSheet,
        title = "添加节点",
        onDismissRequest = { showAddSheet = false }
    ) {
        Card(modifier = Modifier
            .fillMaxWidth()
            .padding(bottom = 8.dp)) {
            BasicComponent(
                title = "单节点链接",
                summary = "VLESS、VMess、SS、Trojan 等链接",
                startAction = { SheetIcon(Icons.Rounded.Link) },
                onClick = {
                    showAddSheet = false
                    showLinkDialog = true
                }
            )
            BasicComponent(
                title = "本地文件",
                summary = "导入 Clash YAML、节点文本或 sing-box JSON",
                startAction = { SheetIcon(Icons.Rounded.FileOpen) },
                onClick = {
                    showAddSheet = false
                    fileLauncher.launch(arrayOf("*/*"))
                }
            )
        }
    }

    OverlayDialog(
        show = showLinkDialog,
        title = "添加单节点",
        onDismissRequest = { showLinkDialog = false }
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            TextField(
                value = nodeLink,
                onValueChange = { nodeLink = it },
                label = "节点链接",
                modifier = Modifier.fillMaxWidth(),
                minLines = 3,
                maxLines = 6
            )
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                TextButton(
                    text = "取消",
                    modifier = Modifier.weight(1f),
                    onClick = { showLinkDialog = false }
                )
                TextButton(
                    text = "添加",
                    modifier = Modifier.weight(1f),
                    enabled = nodeLink.isNotBlank(),
                    colors = ButtonDefaults.textButtonColorsPrimary(),
                    onClick = {
                        viewModel.addNode(nodeLink)
                        nodeLink = ""
                        showLinkDialog = false
                    }
                )
            }
        }
    }

    val selectedAction = actionNode
    OverlayBottomSheet(
        show = selectedAction != null,
        title = selectedAction?.second?.tag.orEmpty(),
        onDismissRequest = { actionNode = null }
    ) {
        if (selectedAction != null) {
            val (group, node) = selectedAction
            Card(modifier = Modifier
                .fillMaxWidth()
                .padding(bottom = 8.dp)) {
                BasicComponent(
                    title = "测试延迟",
                    startAction = { SheetIcon(Icons.Rounded.NetworkPing) },
                    onClick = {
                        viewModel.testDelay("${group.group.id}/${node.tag}")
                        actionNode = null
                    }
                )
                    BasicComponent(
                        title = "编辑节点",
                        startAction = { SheetIcon(Icons.Rounded.Edit) },
                        onClick = {
                            navigator.push(NodeEdit("${group.group.id}/${node.tag}"))
                            actionNode = null
                        }
                    )
                BasicComponent(
                    title = "导出节点",
                    startAction = { SheetIcon(Icons.Rounded.Share) },
                    onClick = {
                        viewModel.exportNode(group.group.id, node.tag)
                        actionNode = null
                    }
                )
                BasicComponent(
                    title = "删除节点",
                    startAction = { SheetIcon(Icons.Rounded.Delete) },
                    onClick = {
                        viewModel.removeNode(group.group.id, node.tag)
                        actionNode = null
                    }
                )
            }
        }
    }
}

@Composable
private fun CatalogNodeGrid(
    group: CatalogNodeGroup,
    selectedRef: String,
    selectorMode: String,
    latencies: Map<String, String>,
    busy: Boolean,
    columns: Int,
    itemSize: Int,
    sortMode: Int,
    onAuto: () -> Unit,
    onNode: (CatalogNode) -> Unit,
    onNodeAction: (CatalogNode) -> Unit,
    modifier: Modifier,
    contentPadding: PaddingValues,
    nestedScrollConnection: NestedScrollConnection
) {
    val automaticSelected = selectorMode == "urltest" &&
            selectedRef == "Auto/${group.group.id}"
    val sortedNodes = remember(group.nodes, sortMode, latencies) {
        sortCatalogNodes(group.nodes, sortMode, group.group.id, latencies)
    }
    val spacing = if (columns == 3) 8.dp else 10.dp
    LazyVerticalGrid(
        modifier = modifier
            .scrollEndHaptic()
            .overScrollVertical()
            .nestedScroll(nestedScrollConnection),
        contentPadding = contentPadding,
        columns = GridCells.Fixed(columns),
        verticalArrangement = Arrangement.spacedBy(spacing),
        horizontalArrangement = Arrangement.spacedBy(spacing),
        overscrollEffect = null
    ) {
        if (group.nodes.isNotEmpty()) {
            item(key = "auto:${group.group.id}") {
                NodeCard(
                    title = "Auto-Fastest",
                    summary = "自动测速",
                    protocol = "AUTO",
                    latency = latencies["Auto/${group.group.id}"],
                    selected = automaticSelected,
                    enabled = !busy,
                    itemSize = itemSize,
                    icon = MiuixIcons.Refresh,
                    onClick = onAuto
                )
            }
        }
        items(sortedNodes, key = { it.tag }) { node ->
            NodeCard(
                title = node.tag,
                summary = buildString {
                    append(node.server.ifBlank { "服务器信息不可用" })
                    if (node.port > 0) append(':').append(node.port)
                },
                protocol = node.protocol.uppercase().ifBlank { "NODE" },
                latency = latencies["${group.group.id}/${node.tag}"],
                selected = selectorMode == "manual" &&
                        selectedRef == "${group.group.id}/${node.tag}",
                enabled = !busy,
                itemSize = itemSize,
                onClick = { onNode(node) },
                onLongClick = { onNodeAction(node) }
            )
        }
        if (group.nodes.isEmpty()) {
            item(span = { GridItemSpan(maxLineSpan) }) {
                EmptyCatalog(
                    text = "该分组暂时没有节点",
                    onRefresh = null,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(vertical = 64.dp)
                )
            }
        }
    }
}

@Composable
private fun CatalogGroupList(
    groups: List<CatalogNodeGroup>,
    selectedRef: String,
    selectorMode: String,
    latencies: Map<String, String>,
    busy: Boolean,
    columns: Int,
    itemSize: Int,
    sortMode: Int,
    expandedGroups: Set<String>,
    onToggleGroup: (String) -> Unit,
    onAuto: (String) -> Unit,
    onNode: (CatalogNodeGroup, CatalogNode) -> Unit,
    onNodeAction: (CatalogNodeGroup, CatalogNode) -> Unit,
    modifier: Modifier,
    contentPadding: PaddingValues,
    nestedScrollConnection: NestedScrollConnection
) {
    val spacing = if (columns == 3) 8.dp else 10.dp
    LazyColumn(
        modifier = modifier
            .scrollEndHaptic()
            .overScrollVertical()
            .nestedScroll(nestedScrollConnection),
        contentPadding = contentPadding,
        overscrollEffect = null
    ) {
        groups.forEachIndexed { groupIndex, group ->
            val groupId = group.group.id
            val expanded = groupId in expandedGroups
            item(key = "header:$groupId") {
                Column {
                    if (groupIndex > 0) Spacer(Modifier.height(12.dp))
                    CatalogGroupHeader(
                        name = if (groupId == "default") "本地配置" else group.group.name,
                        count = group.nodes.size,
                        expanded = expanded,
                        onClick = { onToggleGroup(groupId) }
                    )
                    if (expanded) Spacer(Modifier.height(spacing))
                }
            }
            if (expanded) {
                val entries = listOf<CatalogNode?>(null) + sortCatalogNodes(
                    group.nodes,
                    sortMode,
                    groupId,
                    latencies
                )
                val rows = entries.chunked(columns)
                rows.forEachIndexed { rowIndex, row ->
                    item(key = "row:$groupId:$rowIndex") {
                        Row(horizontalArrangement = Arrangement.spacedBy(spacing)) {
                            row.forEach { node ->
                                Box(modifier = Modifier.weight(1f)) {
                                    if (node == null) {
                                        NodeCard(
                                            title = "Auto-Fastest",
                                            summary = "自动测速",
                                            protocol = "AUTO",
                                            latency = latencies["Auto/$groupId"],
                                            selected = selectorMode == "urltest" &&
                                                    selectedRef == "Auto/$groupId",
                                            enabled = !busy && group.nodes.isNotEmpty(),
                                            itemSize = itemSize,
                                            icon = MiuixIcons.Refresh,
                                            onClick = { onAuto(groupId) }
                                        )
                                    } else {
                                        NodeCard(
                                            title = node.tag,
                                            summary = node.serverWithPort(),
                                            protocol = node.protocol.uppercase().ifBlank { "NODE" },
                                            latency = latencies["$groupId/${node.tag}"],
                                            selected = selectorMode == "manual" &&
                                                    selectedRef == "$groupId/${node.tag}",
                                            enabled = !busy,
                                            itemSize = itemSize,
                                            onClick = { onNode(group, node) },
                                            onLongClick = { onNodeAction(group, node) }
                                        )
                                    }
                                }
                            }
                            repeat(columns - row.size) { Box(modifier = Modifier.weight(1f)) }
                        }
                        if (rowIndex < rows.lastIndex) {
                            Spacer(Modifier.height(spacing))
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun CatalogGroupHeader(
    name: String,
    count: Int,
    expanded: Boolean,
    onClick: () -> Unit
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        cornerRadius = 16.dp,
        insideMargin = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
        pressFeedbackType = PressFeedbackType.Sink,
        showIndication = true,
        onClick = onClick
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = name,
                    style = MiuixTheme.textStyles.body1.copy(fontWeight = FontWeight.Medium),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                Text(
                    text = "$count 个节点",
                    style = MiuixTheme.textStyles.body2,
                    color = colorScheme.onSurfaceVariantActions
                )
            }
            Icon(
                imageVector = if (expanded) MiuixIcons.ExpandLess else MiuixIcons.ExpandMore,
                contentDescription = null,
                modifier = Modifier.size(20.dp),
                tint = colorScheme.onSurfaceVariantActions
            )
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun NodeCard(
    title: String,
    summary: String,
    protocol: String,
    latency: String? = null,
    selected: Boolean,
    enabled: Boolean,
    itemSize: Int,
    icon: androidx.compose.ui.graphics.vector.ImageVector? = null,
    onClick: () -> Unit,
    onLongClick: (() -> Unit)? = null
) {
    val cornerRadius = when (itemSize) {
        1 -> 12.dp
        2 -> 8.dp
        else -> 16.dp
    }
    val innerPadding = when (itemSize) {
        1 -> 12.dp
        2 -> 8.dp
        else -> 16.dp
    }
    val shape = RoundedCornerShape(cornerRadius)
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .graphicsLayer {
                this.shape = shape
                clip = true
            }
            .border(
                width = if (selected) 1.5.dp else 0.dp,
                color = if (selected) colorScheme.primary else Color.Transparent,
                shape = shape
            ),
        cornerRadius = cornerRadius,
        insideMargin = PaddingValues(0.dp),
        colors = CardDefaults.defaultColors(
            color = if (selected) colorScheme.primary.copy(alpha = 0.1f)
            else colorScheme.surfaceContainer
        ),
        onClick = if (enabled) onClick else null,
        onLongPress = if (enabled) onLongClick else null,
        pressFeedbackType = PressFeedbackType.Sink,
        showIndication = true
    ) {
        Column(modifier = Modifier
            .fillMaxWidth()
            .padding(innerPadding)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                if (icon != null) {
                    Icon(
                        imageVector = icon,
                        contentDescription = null,
                        modifier = Modifier
                            .padding(end = 8.dp)
                            .size(18.dp),
                        tint = colorScheme.primary
                    )
                }
                Text(
                    text = title,
                    modifier = Modifier.weight(1f),
                    color = if (selected) colorScheme.primary else colorScheme.onSurface,
                    style = MiuixTheme.textStyles.body1.copy(
                        fontSize = when (itemSize) {
                            1 -> 13.sp
                            2 -> 12.sp
                            else -> 14.sp
                        }
                    ),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
            Spacer(Modifier.height(if (itemSize == 0) 8.dp else if (itemSize == 1) 4.dp else 2.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = protocol,
                    modifier = Modifier.weight(1f),
                    color = if (selected) colorScheme.primary else colorScheme.onSurfaceVariantActions,
                    style = MiuixTheme.textStyles.body2.copy(
                        fontSize = when (itemSize) {
                            1 -> 11.sp
                            2 -> 10.sp
                            else -> 12.sp
                        }
                    ),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                if (latency != null) {
                    Text(
                        text = latencyLabel(latency),
                        modifier = Modifier.padding(start = 8.dp),
                        color = latencyColor(latency),
                        style = MiuixTheme.textStyles.body2.copy(
                            fontSize = when (itemSize) {
                                1 -> 10.sp
                                2 -> 9.sp
                                else -> 11.sp
                            },
                            fontWeight = FontWeight.Medium
                        )
                    )
                } else {
                    Text(
                        text = summary,
                        modifier = Modifier
                            .padding(start = 8.dp)
                            .weight(1f),
                        color = colorScheme.onSurfaceVariantSummary,
                        style = MiuixTheme.textStyles.body2.copy(
                            fontSize = when (itemSize) {
                                1 -> 10.sp
                                2 -> 9.sp
                                else -> 11.sp
                            }
                        ),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }
        }
    }
}

private fun sortCatalogNodes(
    nodes: List<CatalogNode>,
    sortMode: Int,
    groupId: String,
    latencies: Map<String, String>
): List<CatalogNode> =
    when (sortMode) {
        1 -> nodes.sortedBy { it.tag.lowercase() }
        2 -> nodes.sortedWith(compareBy<CatalogNode> { it.protocol }.thenBy { it.tag })
        3 -> nodes.sortedBy { node ->
            latencies["$groupId/${node.tag}"]?.toIntOrNull() ?: Int.MAX_VALUE
        }

        else -> nodes
    }

@Composable
private fun latencyLabel(value: String): String = when (value) {
    "testing..." -> stringResource(com.fanjv.netproxy.R.string.latency_testing)
    "failed" -> stringResource(com.fanjv.netproxy.R.string.latency_failed)
    "timeout" -> stringResource(com.fanjv.netproxy.R.string.latency_timeout)
    else -> "$value ms"
}

@Composable
private fun latencyColor(value: String): Color = when (value) {
    "testing..." -> colorScheme.primary
    "failed", "timeout" -> if (MiuixTheme.isDynamicColor) colorScheme.error else Color(0xFFF72727)
    else -> when (value.toIntOrNull() ?: Int.MAX_VALUE) {
        in 0..799 -> Color(0xFF32A852)
        in 800..1499 -> Color(0xFFE39A20)
        else -> if (MiuixTheme.isDynamicColor) colorScheme.error else Color(0xFFF05252)
    }
}

private fun CatalogNode.serverWithPort(): String = buildString {
    append(server.ifBlank { "--" })
    if (port > 0) append(':').append(port)
}

@Composable
private fun SheetIcon(vector: androidx.compose.ui.graphics.vector.ImageVector) {
    Icon(
        imageVector = vector,
        contentDescription = null,
        modifier = Modifier
            .padding(end = 12.dp)
            .size(24.dp),
        tint = colorScheme.onSurface
    )
}
