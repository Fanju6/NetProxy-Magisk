package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import android.content.ClipData
import android.content.ClipboardManager
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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Edit
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
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.component.AppBarButton
import com.fanjv.netproxy.core.ui.component.AppBottomSheet as OverlayBottomSheet
import com.fanjv.netproxy.core.ui.component.AppContentCard
import com.fanjv.netproxy.core.ui.component.AppDialog as OverlayDialog
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.AppOverlayList
import com.fanjv.netproxy.core.ui.component.AppOverlayListDivider
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.AppTopAppBar as TopAppBar
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import com.fanjv.netproxy.core.ui.component.StatusTag
import com.fanjv.netproxy.core.ui.component.chromeIcon
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.resolve
import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import com.fanjv.netproxy.feature.catalog.model.CatalogNode
import com.fanjv.netproxy.feature.catalog.model.SubscriptionHistoryEntry
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Delete
import top.yukonga.miuix.kmp.icon.extended.Refresh
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

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

    LifecycleResumeEffect(id) {
        viewModel.load(id)
        onPauseOrDispose { }
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
                            AppBarButton(
                                enabled = state.operation.isEmpty(),
                                onClick = { viewModel.update(id) }
                            ) {
                                Icon(
                                    chromeIcon(MiuixIcons.Refresh),
                                    contentDescription = stringResource(R.string.common_update)
                                )
                            }
                            AppBarButton(onClick = { navigator.push(Route.SubscriptionEdit(id)) }) {
                                Icon(
                                    chromeIcon(Icons.Rounded.Edit),
                                    contentDescription = stringResource(R.string.common_edit)
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
                    SubscriptionSummaryCard(details.group)
                }
                item {
                    SectionLabel(stringResource(R.string.subscription_nodes_section, details.nodes.size))
                }
                items(details.nodes, key = { it.tag }) { node ->
                    AppContentCard(modifier = Modifier.fillMaxWidth()) {
                        BasicComponent(
                            title = node.tag,
                            summary = "${node.protocol.uppercase()} · ${node.server}:${node.port}",
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
                AppDialogButton(
                    text = stringResource(R.string.common_cancel),
                    onClick = { showDelete = false },
                    modifier = Modifier.weight(1f)
                )
                AppDialogButton(
                    text = stringResource(R.string.common_delete),
                    role = DialogActionRole.Destructive,
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
            AppOverlayList(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 8.dp)
            ) {
                BasicComponent(
                    title = stringResource(R.string.node_edit),
                    startAction = { OverlayActionIcon(Icons.Rounded.Edit) },
                    onClick = {
                        actionNode = null
                        navigator.push(Route.NodeEdit(
                            nodeRef = "$id/${selectedNode.tag}",
                        ))
                    }
                )
                AppOverlayListDivider()
                BasicComponent(
                    title = stringResource(R.string.node_export),
                    startAction = { OverlayActionIcon(Icons.Rounded.Share) },
                    onClick = {
                        viewModel.exportNode(id, selectedNode.tag)
                        actionNode = null
                    }
                )
                AppOverlayListDivider()
                BasicComponent(
                    title = stringResource(R.string.node_delete),
                    startAction = { OverlayActionIcon(MiuixIcons.Delete, destructive = true) },
                    onClick = {
                        viewModel.removeNode(id, selectedNode.tag)
                        actionNode = null
                    }
                )
            }
        }
    }

    SubscriptionUpdateSheet(state.operation)
}

@Composable
private fun SubscriptionSummaryCard(subscription: CatalogGroupSummary) {
    val isSubscription = subscription.type == "subscription"
    val status = subscriptionBadge(subscription)
    AppContentCard(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    text = subscription.name,
                    fontSize = 20.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f),
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
                if (status != null) {
                    val (background, foreground) = when (status) {
                        SubscriptionUpdateStatus.Updating -> colorScheme.primaryContainer to colorScheme.onPrimaryContainer
                        SubscriptionUpdateStatus.Failed, SubscriptionUpdateStatus.Expired ->
                            colorScheme.errorContainer to colorScheme.onErrorContainer
                        SubscriptionUpdateStatus.NeverUpdated -> colorScheme.secondaryContainer to colorScheme.onSecondaryContainer
                    }
                    StatusTag(stringResource(status.label), backgroundColor = background, contentColor = foreground)
                }
            }
            Text(
                text = when {
                    isSubscription && subscription.autoUpdate -> stringResource(
                        R.string.subscription_summary, subscription.nodeCount, formatDuration(subscription.updateInterval),
                    )
                    isSubscription -> stringResource(R.string.subscription_node_count, subscription.nodeCount)
                    else -> stringResource(R.string.local_config_summary, subscription.nodeCount)
                },
                color = colorScheme.onSurfaceVariantSummary,
                fontSize = 13.sp,
            )
            subscription.profileTitle.takeIf { it.isNotBlank() && it != subscription.name }?.let {
                Text(it, color = colorScheme.onSurfaceVariantSummary, fontSize = 13.sp)
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
    AppContentCard(modifier = Modifier.fillMaxWidth()) {
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
private fun formatDuration(seconds: Long): String = when {
    seconds % 86400L == 0L ->
        stringResource(R.string.subscription_duration_days, seconds / 86400L)

    seconds % 3600L == 0L ->
        stringResource(R.string.subscription_duration_hours, seconds / 3600L)

    else -> stringResource(R.string.subscription_duration_minutes, seconds / 60L)
}
