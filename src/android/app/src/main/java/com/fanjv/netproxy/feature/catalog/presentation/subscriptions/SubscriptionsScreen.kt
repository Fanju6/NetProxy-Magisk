package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import android.content.ClipData
import android.content.ClipboardManager
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
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.NetworkPing
import androidx.compose.material.icons.rounded.Share
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.resolve
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.EmptyCatalog
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import com.fanjv.netproxy.core.ui.component.StatusTag
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import com.fanjv.netproxy.feature.catalog.model.CatalogNode
import com.fanjv.netproxy.feature.catalog.model.SubscriptionHistoryEntry
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.HorizontalDivider
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Add
import top.yukonga.miuix.kmp.icon.extended.Delete
import top.yukonga.miuix.kmp.icon.extended.MoreCircle
import top.yukonga.miuix.kmp.icon.extended.Refresh
import top.yukonga.miuix.kmp.overlay.OverlayBottomSheet
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.squircle.squircleBackground
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.PressFeedbackType
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import kotlin.math.roundToInt

/** 订阅页：URL 订阅管理、流量状态与更新计划。 */
@Composable
internal fun SubscriptionsScreen(
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
    isActive: Boolean = true,
    viewModel: SubscriptionsViewModel = netProxyViewModel()
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    val snackbarHostState = rememberAppSnackbarHostState()
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface

    LifecycleResumeEffect(isActive) {
        viewModel.setVisible(isActive)
        onPauseOrDispose { if (isActive) viewModel.setVisible(false) }
    }

    val noticeText = if (state.error !is UiText.Empty) state.error else state.notice
    SnackbarNoticeEffect(
        eventId = state.noticeId,
        message = noticeText.resolve(),
        isError = state.error !is UiText.Empty,
        hostState = snackbarHostState,
        onConsumed = viewModel::clearNotice
    )

    Scaffold(
        snackbarHost = {
            AppSnackbarHost(snackbarHostState, Modifier.padding(bottom = bottomPadding))
        },
        topBar = {
            BlurredBar(backdrop) {
                TopAppBar(
                    color = barColor,
                    title = stringResource(R.string.subscriptions),
                    scrollBehavior = scrollBehavior,
                    actions = {
                        if (state.groups.any { it.type == "subscription" }) {
                            IconButton(
                                enabled = state.operation.isEmpty(),
                                onClick = viewModel::updateAll
                            ) {
                                Icon(
                                    imageVector = MiuixIcons.Refresh,
                                    contentDescription = stringResource(R.string.subscription_update_all),
                                    tint = colorScheme.onSurface
                                )
                            }
                        }
                        IconButton(
                            onClick = { navigator.push(Route.SubscriptionEdit("")) }
                        ) {
                            Icon(
                                imageVector = MiuixIcons.Add,
                                contentDescription = stringResource(R.string.subscription_add_url),
                                tint = colorScheme.onSurface
                            )
                        }
                    }
                )
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
                state.loading && state.groups.isEmpty() -> InfiniteProgressIndicator(
                    modifier = Modifier.align(Alignment.Center)
                )

                state.groups.isEmpty() -> EmptyCatalog(
                    text = stringResource(R.string.subscription_empty),
                    onRefresh = null,
                    modifier = Modifier
                        .align(Alignment.Center)
                        .padding(horizontal = 24.dp)
                )

                else -> LazyColumn(
                    modifier = Modifier
                        .fillMaxSize()
                        .scrollEndHaptic()
                        .overScrollVertical()
                        .nestedScroll(scrollBehavior.nestedScrollConnection),
                    contentPadding = PaddingValues(
                        start = 12.dp,
                        top = innerPadding.calculateTopPadding() + 12.dp,
                        end = 12.dp,
                        bottom = innerPadding.calculateBottomPadding() + bottomPadding + 84.dp
                    ),
                    overscrollEffect = null
                ) {
                    items(state.groups, key = CatalogGroupSummary::id) { group ->
                        CatalogGroupCard(
                            group = group,
                            busy = state.operation.isNotEmpty(),
                            onSelect = { viewModel.activate(group.id) },
                            onUpdate = { viewModel.updateSubscription(group.id) },
                            onDelete = { viewModel.remove(group.id) },
                            onEdit = { navigator.push(Route.SubscriptionDetails(group.id)) }
                        )
                    }
                }
            }
        }
    }

    SubscriptionUpdateSheet(state.operation)
}

@Composable
private fun CatalogGroupCard(
    group: CatalogGroupSummary,
    busy: Boolean,
    onSelect: () -> Unit,
    onUpdate: () -> Unit,
    onDelete: () -> Unit,
    onEdit: () -> Unit
) {
    var showDeleteDialog by remember { mutableStateOf(false) }
    val isSubscription = group.type == "subscription"
    val usage = group.usage
    val used = (usage?.upload ?: 0L) + (usage?.download ?: 0L)
    val total = usage?.total ?: 0L
    val updatedAt = group.lastSuccessAt.ifBlank { group.updatedAt }

    Card(
        modifier = Modifier
            .fillMaxWidth()
            .padding(bottom = 12.dp),
        insideMargin = PaddingValues(16.dp),
        pressFeedbackType = PressFeedbackType.Sink,
        showIndication = true,
        onClick = onSelect
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Text(
                text = group.name,
                modifier = Modifier.weight(1f),
                fontSize = 17.sp,
                fontWeight = FontWeight.SemiBold,
                color = colorScheme.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            if (group.active) {
                val activeColor = Color(0xFF32A852)
                Text(
                    text = stringResource(R.string.subscription_active),
                    fontSize = 12.sp,
                    fontWeight = FontWeight(750),
                    color = activeColor,
                    modifier = Modifier
                        .squircleBackground(activeColor.copy(alpha = 0.15f), 6.dp)
                        .padding(horizontal = 6.dp, vertical = 2.dp)
                )
            }
        }

        HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))

        Row(verticalAlignment = Alignment.CenterVertically) {
            Column {
                Text(
                    text = if (!isSubscription) {
                        stringResource(R.string.subscription_node_count, group.nodeCount)
                    } else if (total > 0) {
                        stringResource(
                            R.string.subscription_usage,
                            formatBytes(used),
                            formatBytes(total)
                        )
                    } else {
                        stringResource(R.string.subscription_no_usage)
                    },
                    modifier = Modifier.padding(top = 2.dp),
                    fontSize = 12.sp,
                    color = colorScheme.onSurfaceVariantSummary
                )
                Spacer(Modifier.weight(1f))
                Text(
                    text = if (updatedAt.isNotBlank()) {
                        stringResource(
                            R.string.subscription_updated_at,
                            stringResource(
                                if (isSubscription) {
                                    R.string.subscription_updated_label
                                } else {
                                    R.string.local_config_modified_label
                                }
                            ),
                            formatDate(updatedAt)
                        )
                    } else {
                        stringResource(
                            if (isSubscription) {
                                R.string.subscription_not_updated
                            } else {
                                R.string.local_config_modified_at
                            }
                        )
                    },
                    modifier = Modifier.padding(top = 2.dp),
                    fontSize = 12.sp,
                    color = colorScheme.onSurfaceVariantSummary
                )
            }

            Spacer(Modifier.weight(1f))

            IconButton(
                onClick = onEdit,
                minHeight = 35.dp,
                minWidth = 35.dp,
                backgroundColor = colorScheme.secondaryContainer
            ) {
                Icon(
                    modifier = Modifier.size(20.dp),
                    imageVector = MiuixIcons.MoreCircle,
                    contentDescription = stringResource(
                        if (isSubscription) R.string.subscription_details
                        else R.string.local_config_details
                    ),
                    tint = colorScheme.onSurfaceVariantSummary
                )
            }
            if (isSubscription) {
                Spacer(Modifier.width(8.dp))
                IconButton(
                    onClick = onUpdate,
                    enabled = !busy,
                    minHeight = 35.dp,
                    minWidth = 35.dp,
                    backgroundColor = colorScheme.secondaryContainer
                ) {
                    Icon(
                        modifier = Modifier.size(20.dp),
                        imageVector = MiuixIcons.Refresh,
                        contentDescription = stringResource(R.string.common_update),
                        tint = colorScheme.onSurfaceVariantSummary
                    )
                }
                Spacer(Modifier.width(8.dp))
                IconButton(
                    onClick = { showDeleteDialog = true },
                    enabled = !busy,
                    minHeight = 35.dp,
                    minWidth = 35.dp,
                    backgroundColor = colorScheme.secondaryContainer
                ) {
                    Icon(
                        modifier = Modifier.size(20.dp),
                        imageVector = MiuixIcons.Delete,
                        contentDescription = stringResource(R.string.common_delete),
                        tint = colorScheme.onSurfaceVariantSummary
                    )
                }
            }
        }
    }

    OverlayDialog(
        show = isSubscription && showDeleteDialog,
        title = stringResource(R.string.subscription_delete_title),
        onDismissRequest = { showDeleteDialog = false }
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.subscription_delete_message, group.name))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                TextButton(
                    text = stringResource(R.string.common_cancel),
                    modifier = Modifier.weight(1f),
                    onClick = { showDeleteDialog = false }
                )
                TextButton(
                    text = stringResource(R.string.common_confirm),
                    modifier = Modifier.weight(1f),
                    colors = ButtonDefaults.textButtonColorsPrimary(),
                    onClick = {
                        showDeleteDialog = false
                        onDelete()
                    }
                )
            }
        }
    }
}

/** 订阅详情：展示节点摘要和最近更新历史。 */
@Composable
internal fun SubscriptionDetailsScreen(
    id: String,
    onBack: () -> Unit,
    viewModel: SubscriptionDetailsViewModel = netProxyViewModel()
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    val context = LocalContext.current
    val snackbarHostState = rememberAppSnackbarHostState()
    val backdrop = rememberBlurBackdrop()
    val scrollBehavior = MiuixScrollBehavior()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface
    var showDelete by remember { mutableStateOf(false) }
    var actionNode by remember { mutableStateOf<CatalogNode?>(null) }
    var editedNodeLink by remember { mutableStateOf("") }

    LaunchedEffect(id) { viewModel.load(id) }
    LaunchedEffect(state.editingNodeRef, state.editingNodeLink) {
        if (state.editingNodeRef.isNotBlank()) editedNodeLink = state.editingNodeLink
    }
    LaunchedEffect(state.exportedNodeLinkId) {
        val link = state.exportedNodeLink
        if (link.isBlank()) return@LaunchedEffect
        context.getSystemService(ClipboardManager::class.java)
            ?.setPrimaryClip(ClipData.newPlainText("NetProxy node", link))
        viewModel.nodeLinkCopied()
    }
    val noticeText = if (state.error !is UiText.Empty) state.error else state.notice
    SnackbarNoticeEffect(
        eventId = state.noticeId,
        message = noticeText.resolve(),
        isError = state.error !is UiText.Empty,
        hostState = snackbarHostState,
        onConsumed = viewModel::clearNotice
    )

    Scaffold(
        snackbarHost = { AppSnackbarHost(snackbarHostState) },
        topBar = {
            BlurredBar(backdrop) {
                TopAppBar(
                    color = barColor,
                    title = stringResource(
                        if (state.details?.group?.type == "local") {
                            R.string.local_config_details
                        } else {
                            R.string.subscription_details
                        }
                    ),
                    scrollBehavior = scrollBehavior,
                    navigationIcon = { BackIconButton(onClick = onBack) },
                    actions = {
                        if (state.details?.group?.type == "subscription") {
                            IconButton(onClick = { navigator.push(Route.SubscriptionEdit(id)) }) {
                                Icon(
                                    Icons.Rounded.Edit,
                                    contentDescription = stringResource(R.string.common_edit)
                                )
                            }
                            IconButton(
                                enabled = state.operation.isEmpty(),
                                onClick = { viewModel.update(id) }
                            ) {
                                Icon(
                                    MiuixIcons.Refresh,
                                    contentDescription = stringResource(R.string.common_update)
                                )
                            }
                        }
                    }
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.only(WindowInsetsSides.Horizontal)
    ) { innerPadding ->
        val details = state.details
        if (state.loading && details == null) {
            Box(
                Modifier
                    .fillMaxSize()
                    .padding(innerPadding), contentAlignment = Alignment.Center
            ) {
                InfiniteProgressIndicator()
            }
        } else if (details != null) {
            LazyColumn(
                modifier = Modifier
                    .fillMaxSize()
                    .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)
                    .nestedScroll(scrollBehavior.nestedScrollConnection)
                    .scrollEndHaptic()
                    .overScrollVertical()
                    .padding(horizontal = 12.dp),
                contentPadding = PaddingValues(
                    top = innerPadding.calculateTopPadding() + 12.dp,
                    bottom = innerPadding.calculateBottomPadding() + 24.dp
                ),
                verticalArrangement = Arrangement.spacedBy(12.dp),
                overscrollEffect = null
            ) {
                item {
                    SubscriptionSummaryCard(details.group) {
                        viewModel.activate(id)
                    }
                }
                item {
                    SectionLabel(stringResource(R.string.subscription_nodes_section, details.nodes.size))
                }
                items(details.nodes, key = { it.tag }) { node ->
                    Card(modifier = Modifier.fillMaxWidth()) {
                        BasicComponent(
                            title = node.tag,
                            summary = buildString {
                                append("${node.protocol.uppercase()} · ${node.server}:${node.port}")
                                state.latencies["${details.group.id}/${node.tag}"]?.let { latency ->
                                    append(" · ")
                                    append(if (latency.all(Char::isDigit)) "$latency ms" else latency)
                                }
                            },
                            onClick = { actionNode = node }
                        )
                    }
                }
                if (details.group.type == "subscription") {
                    item { SectionLabel(stringResource(R.string.subscription_history_section)) }
                    if (state.history.isEmpty()) {
                        item {
                            Text(
                                text = stringResource(R.string.subscription_history_empty),
                                color = colorScheme.onSurfaceVariantSummary,
                                modifier = Modifier.padding(12.dp)
                            )
                        }
                    } else {
                        items(state.history.takeLast(20).reversed()) { history ->
                            HistoryRow(history)
                        }
                    }
                    item {
                        TextButton(
                            text = stringResource(R.string.common_delete),
                            onClick = { showDelete = true },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }
                }
            }
        }
    }

    OverlayDialog(
        show = showDelete,
        title = stringResource(R.string.subscription_delete_title),
        onDismissRequest = { showDelete = false }
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.subscription_delete_details_message))
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                TextButton(
                    text = stringResource(R.string.common_cancel),
                    onClick = { showDelete = false },
                    modifier = Modifier.weight(1f)
                )
                TextButton(
                    text = stringResource(R.string.common_delete),
                    onClick = {
                        showDelete = false
                        viewModel.remove(id, onBack)
                    },
                    modifier = Modifier.weight(1f)
                )
            }
        }
    }

    val selectedNode = actionNode
    OverlayBottomSheet(
        show = selectedNode != null,
        title = selectedNode?.tag.orEmpty(),
        onDismissRequest = { actionNode = null }
    ) {
        if (selectedNode != null) {
            Card(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 8.dp)
            ) {
                BasicComponent(
                    title = stringResource(R.string.node_test_delay),
                    startAction = { DetailActionIcon(Icons.Rounded.NetworkPing) },
                    onClick = {
                        viewModel.testNode(id, selectedNode.tag)
                        actionNode = null
                    }
                )
                BasicComponent(
                    title = stringResource(R.string.node_edit),
                    startAction = { DetailActionIcon(Icons.Rounded.Edit) },
                    onClick = {
                        viewModel.editNode(id, selectedNode.tag)
                        actionNode = null
                    }
                )
                BasicComponent(
                    title = stringResource(R.string.node_export),
                    startAction = { DetailActionIcon(Icons.Rounded.Share) },
                    onClick = {
                        viewModel.exportNode(id, selectedNode.tag)
                        actionNode = null
                    }
                )
                BasicComponent(
                    title = stringResource(R.string.node_delete),
                    startAction = { DetailActionIcon(MiuixIcons.Delete) },
                    onClick = {
                        viewModel.removeNode(id, selectedNode.tag)
                        actionNode = null
                    }
                )
            }
        }
    }

    OverlayDialog(
        show = state.editingNodeRef.isNotBlank(),
        title = stringResource(R.string.node_edit),
        onDismissRequest = viewModel::dismissNodeEditor
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            TextField(
                value = editedNodeLink,
                onValueChange = { editedNodeLink = it },
                label = stringResource(R.string.node_link_label),
                modifier = Modifier.fillMaxWidth(),
                minLines = 3,
                maxLines = 8
            )
            Text(
                text = if (state.details?.group?.type == "subscription") {
                    stringResource(R.string.subscription_edit_remote_note)
                } else {
                    stringResource(R.string.subscription_edit_local_note)
                },
                color = colorScheme.onSurfaceVariantSummary,
                fontSize = 13.sp
            )
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                TextButton(
                    text = stringResource(R.string.common_cancel),
                    modifier = Modifier.weight(1f),
                    enabled = state.operation != "edit",
                    onClick = viewModel::dismissNodeEditor
                )
                TextButton(
                    text = stringResource(R.string.save_text),
                    modifier = Modifier.weight(1f),
                    enabled = editedNodeLink.isNotBlank() && state.operation != "edit",
                    colors = ButtonDefaults.textButtonColorsPrimary(),
                    onClick = { viewModel.saveEditedNode(editedNodeLink) }
                )
            }
        }
    }

    SubscriptionUpdateSheet(state.operation)
}

@Composable
private fun DetailActionIcon(imageVector: androidx.compose.ui.graphics.vector.ImageVector) {
    Icon(
        imageVector = imageVector,
        modifier = Modifier.size(22.dp),
        tint = colorScheme.onSurfaceVariantSummary,
        contentDescription = null
    )
}

/** 新增/编辑 URL 订阅。敏感字段只通过 root CLI 的私有契约读取。 */
@Composable
internal fun SubscriptionEditorScreen(
    id: String,
    onBack: () -> Unit,
    viewModel: SubscriptionEditorViewModel = netProxyViewModel()
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val draft = state.draft
    val snackbarHostState = rememberAppSnackbarHostState()
    val backdrop = rememberBlurBackdrop()
    val scrollBehavior = MiuixScrollBehavior()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface

    LaunchedEffect(id) { viewModel.load(id) }
    LaunchedEffect(state.saved) {
        if (state.saved) onBack()
    }
    SnackbarNoticeEffect(
        eventId = state.noticeId,
        message = state.error.resolve(),
        isError = state.error !is UiText.Empty,
        hostState = snackbarHostState,
        onConsumed = viewModel::clearError
    )

    Scaffold(
        snackbarHost = { AppSnackbarHost(snackbarHostState) },
        topBar = {
            BlurredBar(backdrop) {
                TopAppBar(
                    color = barColor,
                    title = stringResource(
                        if (id.isBlank()) R.string.subscription_editor_add_title
                        else R.string.subscription_editor_edit_title
                    ),
                    scrollBehavior = scrollBehavior,
                    navigationIcon = { BackIconButton(onClick = onBack) },
                    actions = {
                        IconButton(
                            enabled = !state.saving && !state.loading,
                            onClick = viewModel::save
                        ) {
                            if (state.saving) {
                                InfiniteProgressIndicator(modifier = Modifier.size(20.dp))
                            } else {
                                Icon(
                                    Icons.Rounded.Check,
                                    contentDescription = stringResource(R.string.save_text)
                                )
                            }
                        }
                    }
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.only(WindowInsetsSides.Horizontal)
    ) { innerPadding ->
        if (state.loading) {
            Box(
                Modifier
                    .fillMaxSize()
                    .padding(innerPadding), contentAlignment = Alignment.Center
            ) {
                InfiniteProgressIndicator()
            }
        } else {
            LazyColumn(
                modifier = Modifier
                    .fillMaxSize()
                    .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)
                    .nestedScroll(scrollBehavior.nestedScrollConnection)
                    .scrollEndHaptic()
                    .overScrollVertical()
                    .padding(horizontal = 12.dp),
                contentPadding = PaddingValues(
                    top = innerPadding.calculateTopPadding() + 12.dp,
                    bottom = innerPadding.calculateBottomPadding() + 24.dp
                ),
                verticalArrangement = Arrangement.spacedBy(12.dp),
                overscrollEffect = null
            ) {
                item { SectionLabel(stringResource(R.string.subscription_editor_basic_section)) }
                item {
                    TextField(
                        value = draft.name,
                        onValueChange = { value ->
                            viewModel.update { it.copy(name = value) }
                        },
                        label = stringResource(
                            if (id.isBlank()) R.string.subscription_editor_name_auto
                            else R.string.subscription_editor_name
                        ),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
                item {
                    TextField(
                        value = draft.url,
                        onValueChange = { value ->
                            viewModel.update { it.copy(url = value) }
                        },
                        label = stringResource(R.string.subscription_editor_url),
                        modifier = Modifier.fillMaxWidth(),
                        minLines = 2,
                        maxLines = 4
                    )
                }
                item {
                    Card(modifier = Modifier.fillMaxWidth()) {
                        val intervals =
                            listOf(900L, 3600L, 21600L, 43200L, 86400L, 259200L, 604800L)
                        val intervalLabels = listOf(
                            stringResource(R.string.subscription_duration_minutes, 15),
                            stringResource(R.string.subscription_duration_hours, 1),
                            stringResource(R.string.subscription_duration_hours, 6),
                            stringResource(R.string.subscription_duration_hours, 12),
                            stringResource(R.string.subscription_duration_hours, 24),
                            stringResource(R.string.subscription_duration_days, 3),
                            stringResource(R.string.subscription_duration_days, 7)
                        )
                        OverlayDropdownPreference(
                            title = stringResource(R.string.subscription_editor_update_interval),
                            items = intervalLabels,
                            // 未命中候选档位时回退到 24 小时；不可对索引取下限，
                            // 否则 24 小时以下的档位会被一并抬高到该档
                            selectedIndex = intervals.indexOf(draft.updateIntervalSeconds)
                                .takeIf { it >= 0 } ?: intervals.indexOf(86400L),
                            onSelectedIndexChange = { index ->
                                viewModel.update {
                                    it.copy(updateIntervalSeconds = intervals[index])
                                }
                            }
                        )
                        SwitchPreference(
                            title = stringResource(R.string.subscription_editor_auto_update),
                            summary = stringResource(R.string.subscription_editor_auto_update_summary),
                            checked = draft.autoUpdate,
                            onCheckedChange = { enabled ->
                                viewModel.update { it.copy(autoUpdate = enabled) }
                            }
                        )
                        val viaValues = listOf("auto", "always", "never")
                        OverlayDropdownPreference(
                            title = stringResource(R.string.subscription_editor_update_network),
                            items = listOf(
                                stringResource(R.string.subscription_editor_network_auto),
                                stringResource(R.string.subscription_editor_network_proxy),
                                stringResource(R.string.subscription_editor_network_direct)
                            ),
                            selectedIndex = viaValues.indexOf(draft.updateViaProxy)
                                .coerceAtLeast(0),
                            onSelectedIndexChange = { index ->
                                viewModel.update {
                                    it.copy(updateViaProxy = viaValues[index])
                                }
                            }
                        )
                    }
                }
                item { SectionLabel(stringResource(R.string.subscription_editor_filter_section)) }
                item {
                    TextField(
                        value = draft.include,
                        onValueChange = { value ->
                            viewModel.update { it.copy(include = value) }
                        },
                        label = stringResource(R.string.subscription_editor_include),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
                item {
                    TextField(
                        value = draft.exclude,
                        onValueChange = { value ->
                            viewModel.update { it.copy(exclude = value) }
                        },
                        label = stringResource(R.string.subscription_editor_exclude),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
                item {
                    Card(modifier = Modifier.fillMaxWidth()) {
                        SwitchPreference(
                            title = stringResource(R.string.subscription_editor_allow_insecure),
                            summary = stringResource(R.string.subscription_editor_allow_insecure_summary),
                            checked = draft.allowInsecure,
                            onCheckedChange = { enabled ->
                                viewModel.update { it.copy(allowInsecure = enabled) }
                            }
                        )
                    }
                }
                item { SectionLabel(stringResource(R.string.subscription_editor_advanced_section)) }
                item {
                    TextField(
                        value = draft.userAgent,
                        onValueChange = { value ->
                            viewModel.update { it.copy(userAgent = value) }
                        },
                        label = stringResource(R.string.subscription_editor_user_agent),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
                item {
                    TextField(
                        value = draft.hwid,
                        onValueChange = { value ->
                            viewModel.update { it.copy(hwid = value) }
                        },
                        label = stringResource(R.string.subscription_editor_hwid),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
                item {
                    TextField(
                        value = state.headersText,
                        onValueChange = viewModel::updateHeaders,
                        label = stringResource(R.string.subscription_editor_headers),
                        modifier = Modifier.fillMaxWidth(),
                        minLines = 3,
                        maxLines = 8
                    )
                }
                item {
                    TextField(
                        value = draft.timeoutSeconds.toString(),
                        onValueChange = { value ->
                            value.toIntOrNull()?.let { seconds ->
                                viewModel.update { it.copy(timeoutSeconds = seconds) }
                            }
                        },
                        label = stringResource(R.string.subscription_editor_timeout),
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true
                    )
                }
                item {
                    TextButton(
                        text = if (state.saving) {
                            stringResource(R.string.subscription_editor_validating_save)
                        } else {
                            stringResource(R.string.save_text)
                        },
                        enabled = !state.saving,
                        colors = ButtonDefaults.textButtonColorsPrimary(),
                        onClick = viewModel::save,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            }
        }
    }
}

@Composable
private fun SubscriptionSummaryCard(
    subscription: CatalogGroupSummary,
    onActivate: () -> Unit
) {
    val isSubscription = subscription.type == "subscription"
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = subscription.name,
                    fontSize = 20.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f)
                )
                StatusTag(
                    label = if (subscription.active) {
                        stringResource(R.string.subscription_active)
                    } else {
                        subscriptionStatus(subscription).label
                    },
                    backgroundColor = colorScheme.primaryContainer,
                    contentColor = colorScheme.onPrimaryContainer
                )
            }
            Text(
                text = if (isSubscription) {
                    stringResource(
                        R.string.subscription_summary,
                        subscription.nodeCount,
                        formatDuration(subscription.updateInterval)
                    )
                } else {
                    stringResource(R.string.local_config_summary, subscription.nodeCount)
                },
                color = colorScheme.onSurfaceVariantSummary,
                fontSize = 13.sp
            )
            subscription.profileTitle.takeIf(String::isNotBlank)?.let {
                Text(it, color = colorScheme.onSurfaceVariantSummary, fontSize = 13.sp)
            }
            if (!subscription.active) {
                TextButton(
                    text = stringResource(
                        if (isSubscription) R.string.subscription_set_active
                        else R.string.local_config_set_active
                    ),
                    onClick = onActivate,
                    colors = ButtonDefaults.textButtonColorsPrimary(),
                    modifier = Modifier.fillMaxWidth()
                )
            }
        }
    }
}

@Composable
private fun HistoryRow(entry: SubscriptionHistoryEntry) {
    val nodeCount = entry.nodeCount?.let {
        stringResource(R.string.subscription_node_count, it)
    }
    val revision = entry.revision?.let {
        stringResource(R.string.subscription_revision, it)
    }
    Card(modifier = Modifier.fillMaxWidth()) {
        BasicComponent(
            title = when {
                !entry.ok -> stringResource(R.string.subscription_update_failed)
                entry.code == "subscription.not_modified" ->
                    stringResource(R.string.subscription_not_modified)
                else -> stringResource(R.string.subscription_update_success)
            },
            summary = buildString {
                append(formatDate(entry.time))
                nodeCount?.let { append(" · ").append(it) }
                revision?.let { append(" · ").append(it) }
                if (!entry.ok) {
                    entry.message.takeIf(String::isNotBlank)?.let {
                        append(" · ").append(it)
                    }
                }
            }
        )
    }
}

@Composable
private fun SectionLabel(text: String) {
    SmallTitle(
        text = text,
        insideMargin = PaddingValues(start = 16.dp, top = 8.dp, end = 16.dp, bottom = 0.dp)
    )
}

@Composable
private fun SubscriptionUpdateSheet(operation: String) {
    val updateAll = operation == "update-all"
    OverlayBottomSheet(
        show = operation == "update" || updateAll,
        title = stringResource(
            if (updateAll) R.string.subscription_update_all else R.string.common_update
        ),
        onDismissRequest = {}
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(bottom = 12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            InfiniteProgressIndicator(modifier = Modifier.size(28.dp))
            Spacer(Modifier.width(16.dp))
            Text(
                text = if (updateAll) {
                    stringResource(R.string.subscription_update_sheet_all)
                } else {
                    stringResource(R.string.subscription_update_sheet_single)
                },
                color = colorScheme.onSurface
            )
        }
    }
}

private data class SubscriptionStatus(val label: String, val color: Color)

private val activeProgressStages = setOf("download", "convert", "validate", "apply")

@Composable
private fun subscriptionStatus(subscription: CatalogGroupSummary): SubscriptionStatus {
    val expire = subscription.usage?.expire ?: 0L
    return when {
        subscription.progress?.stage?.let(activeProgressStages::contains) == true ->
            SubscriptionStatus(
                stringResource(R.string.subscription_update_in_progress),
                colorScheme.primary
            )

        subscription.lastError.isNotBlank() -> SubscriptionStatus(
            stringResource(R.string.subscription_update_failed),
            colorScheme.error
        )
        expire > 0 && expire <= Instant.now().epochSecond -> SubscriptionStatus(
            stringResource(R.string.subscription_expired),
            colorScheme.error
        )

        subscription.lastSuccessAt.isBlank() -> SubscriptionStatus(
            stringResource(R.string.subscription_never_updated),
            colorScheme.onSurfaceVariantSummary
        )

        else -> SubscriptionStatus(stringResource(R.string.subscription_normal), colorScheme.primary)
    }
}

private fun formatDate(value: String): String = runCatching {
    DateTimeFormatter.ofPattern("MM-dd HH:mm")
        .withZone(ZoneId.systemDefault())
        .format(Instant.parse(value))
}.getOrDefault(value.ifBlank { "--" })

@Composable
private fun formatDuration(seconds: Long): String = when {
    seconds % 86400L == 0L ->
        stringResource(R.string.subscription_duration_days, seconds / 86400L)

    seconds % 3600L == 0L ->
        stringResource(R.string.subscription_duration_hours, seconds / 3600L)

    else -> stringResource(R.string.subscription_duration_minutes, seconds / 60L)
}

private fun formatBytes(bytes: Long): String {
    if (bytes < 1024L) return "$bytes B"
    val units = arrayOf("KB", "MB", "GB", "TB")
    var value = bytes.toDouble()
    var index = -1
    do {
        value /= 1024.0
        index++
    } while (value >= 1024.0 && index < units.lastIndex)
    return if (value >= 100) "${value.roundToInt()} ${units[index]}"
    else "%.1f %s".format(value, units[index])
}
