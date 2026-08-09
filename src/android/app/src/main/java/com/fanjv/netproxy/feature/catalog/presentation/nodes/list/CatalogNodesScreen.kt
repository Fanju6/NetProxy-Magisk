package com.fanjv.netproxy.feature.catalog.presentation.nodes.list

import android.content.ClipData
import android.content.ClipboardManager
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.core.content.edit
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.feature.catalog.model.CatalogNode
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.feature.catalog.presentation.nodes.CatalogNodesViewModel
import com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components.CatalogGroupList
import com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components.CatalogNodeGrid
import com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components.EmptyState
import com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components.FilterBar
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route.NodeEdit
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.PullToRefresh
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.basic.rememberPullToRefreshState
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Add
import top.yukonga.miuix.kmp.overlay.OverlayBottomSheet
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme

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
                    if (layoutStyle == 0) {
                        FilterBar(
                            groups = state.groups,
                            selectedIndex = selectedIndex,
                            onSelected = { index ->
                                state.groups.getOrNull(index)?.group?.id?.let(viewModel::selectGroup)
                            }
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

                state.error.isNotBlank() && state.groups.isEmpty() -> EmptyState(
                    text = state.error,
                    onRefresh = { viewModel.refresh() },
                    modifier = Modifier
                        .align(Alignment.Center)
                        .padding(horizontal = 24.dp)
                )

                selectedGroup == null -> EmptyState(
                    text = "还没有可用节点",
                    onRefresh = null,
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
                            activeGroupId = state.selection.activeGroupId,
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
                            activeGroupId = state.selection.activeGroupId,
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
        Card(
            modifier = Modifier
                .fillMaxWidth()
                .padding(bottom = 8.dp)
        ) {
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
            Card(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 8.dp)
            ) {
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
