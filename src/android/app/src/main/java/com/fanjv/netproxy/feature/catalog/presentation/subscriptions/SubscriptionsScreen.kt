package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.FileOpen
import androidx.compose.material.icons.rounded.Link
import androidx.compose.material.icons.rounded.RssFeed
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
import androidx.compose.ui.platform.LocalLayoutDirection
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
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.ContentStatus
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import com.fanjv.netproxy.core.ui.component.appBarOverscroll
import com.fanjv.netproxy.core.ui.component.chromeIcon
import com.fanjv.netproxy.core.ui.component.dialogTextFieldColors
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.formatBytes
import com.fanjv.netproxy.core.ui.resolve
import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import com.fanjv.netproxy.feature.catalog.presentation.nodes.CatalogNodesViewModel
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.HorizontalDivider
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.glass.GlassTextField
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Add
import top.yukonga.miuix.kmp.icon.extended.Delete
import top.yukonga.miuix.kmp.icon.extended.MoreCircle
import top.yukonga.miuix.kmp.icon.extended.Refresh
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

/** 资源页：本地节点与 URL 订阅。 */
@Composable
internal fun SubscriptionsScreen(
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
    isActive: Boolean = true,
    nodes: CatalogNodesViewModel,
    viewModel: SubscriptionsViewModel = netProxyViewModel()
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val nodeState by nodes.state.collectAsStateWithLifecycle()
    val navigator = LocalNavigator.current
    val snackbarHostState = rememberAppSnackbarHostState()
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface
    var showAddActions by remember { mutableStateOf(false) }
    var addNode by remember { mutableStateOf(false) }
    var link by remember { mutableStateOf("") }
    val filePicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let(nodes::importFile)
    }

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
    val nodeNotice = if (nodeState.error !is UiText.Empty) nodeState.error else nodeState.notice
    SnackbarNoticeEffect(
        eventId = nodeState.noticeId,
        message = nodeNotice.resolve(),
        isError = nodeState.error !is UiText.Empty,
        hostState = snackbarHostState,
        onConsumed = nodes::clearNotice
    )
    LaunchedEffect(nodeState.noticeId) {
        if (nodeState.noticeId > 0) viewModel.refresh(silent = true)
    }

    Scaffold(
        snackbarHost = {
            AppSnackbarHost(snackbarHostState, Modifier.padding(bottom = bottomPadding))
        },
        topBar = {
            BlurredBar(backdrop) {
                TopAppBar(
                    color = barColor,
                    title = stringResource(R.string.resources),
                    scrollBehavior = scrollBehavior,
                    actions = {
                        if (state.groups.any { it.type == "subscription" }) {
                            AppBarButton(
                                enabled = state.operation.isEmpty(),
                                onClick = viewModel::updateAll
                            ) {
                                Icon(
                                    imageVector = chromeIcon(MiuixIcons.Refresh),
                                    contentDescription = stringResource(R.string.subscription_update_all),
                                    tint = colorScheme.onSurface
                                )
                            }
                        }
                        AppBarButton(onClick = { showAddActions = true }) {
                            Icon(chromeIcon(MiuixIcons.Add), stringResource(R.string.common_add))
                        }
                    }
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.add(WindowInsets.displayCutout).only(WindowInsetsSides.Horizontal)
    ) { innerPadding ->
        val direction = LocalLayoutDirection.current
        Box(
            modifier = Modifier
                .fillMaxSize()
                .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)
        ) {
            when {
                state.groups.isEmpty() -> ContentStatus(
                    padding = PaddingValues(top = innerPadding.calculateTopPadding(),
                        bottom = innerPadding.calculateBottomPadding() + bottomPadding),
                    text = stringResource(if (state.loadFailed) R.string.subscriptions_read_failed else R.string.subscription_empty),
                    loading = state.loading,
                )

                else -> LazyColumn(
                    modifier = Modifier
                        .fillMaxSize()
                        .scrollEndHaptic()
                        .appBarOverscroll(scrollBehavior)
                        .nestedScroll(scrollBehavior.nestedScrollConnection),
                    contentPadding = PaddingValues(
                        start = innerPadding.calculateStartPadding(direction) + 12.dp,
                        top = innerPadding.calculateTopPadding() + 12.dp,
                        end = innerPadding.calculateEndPadding(direction) + 12.dp,
                        bottom = innerPadding.calculateBottomPadding() + bottomPadding + 84.dp
                    ),
                    overscrollEffect = null
                ) {
                    items(state.groups, key = CatalogGroupSummary::id) { group ->
                        CatalogGroupCard(
                            group = group,
                            busy = state.operation.isNotEmpty(),
                            onUpdate = { viewModel.updateSubscription(group.id) },
                            onDelete = { viewModel.remove(group.id) },
                            onEdit = { navigator.push(Route.SubscriptionDetails(group.id)) },
                        )
                    }
                }
            }
        }
    }

    SubscriptionUpdateSheet(state.operation)
    OverlayBottomSheet(
        show = showAddActions,
        title = stringResource(R.string.common_add),
        onDismissRequest = { showAddActions = false },
    ) {
        AppOverlayList(modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp)) {
            BasicComponent(
                title = stringResource(R.string.subscription_add_url),
                startAction = { OverlayActionIcon(Icons.Rounded.RssFeed) },
                onClick = {
                    showAddActions = false
                    navigator.push(Route.SubscriptionEdit(""))
                },
            )
            AppOverlayListDivider()
            BasicComponent(
                title = stringResource(R.string.node_single_link),
                summary = stringResource(R.string.node_single_link_summary),
                startAction = { OverlayActionIcon(Icons.Rounded.Link) },
                enabled = nodeState.operation.isEmpty(),
                onClick = { showAddActions = false; addNode = true },
            )
            AppOverlayListDivider()
            BasicComponent(
                title = stringResource(R.string.node_local_file),
                summary = stringResource(R.string.node_local_file_summary),
                startAction = { OverlayActionIcon(Icons.Rounded.FileOpen) },
                enabled = nodeState.operation.isEmpty(),
                onClick = { showAddActions = false; filePicker.launch(arrayOf("*/*")) },
            )
        }
    }
    OverlayDialog(
        show = addNode,
        title = stringResource(R.string.node_add_single),
        onDismissRequest = { addNode = false; link = "" },
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            GlassTextField(
                colors = dialogTextFieldColors(),
                value = link,
                onValueChange = { link = it },
                label = stringResource(R.string.node_link_label),
                modifier = Modifier.fillMaxWidth(),
                minLines = 3,
                maxLines = 6,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                AppDialogButton(
                    text = stringResource(R.string.common_cancel),
                    onClick = { addNode = false; link = "" },
                    modifier = Modifier.weight(1f),
                )
                AppDialogButton(
                    text = stringResource(R.string.common_add),
                    enabled = link.isNotBlank() && nodeState.operation.isEmpty(),
                    role = DialogActionRole.Primary,
                    onClick = { nodes.addNode(link); addNode = false; link = "" },
                    modifier = Modifier.weight(1f),
                )
            }
        }
    }
}

@Composable
private fun CatalogGroupCard(
    group: CatalogGroupSummary,
    busy: Boolean,
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

    AppContentCard(
        modifier = Modifier
            .fillMaxWidth()
            .padding(bottom = 12.dp),
        insideMargin = PaddingValues(16.dp),
        onClick = onEdit
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

        }

        HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))

        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
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

            Spacer(Modifier.width(8.dp))

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
                AppDialogButton(
                    text = stringResource(R.string.common_cancel),
                    modifier = Modifier.weight(1f),
                    onClick = { showDeleteDialog = false }
                )
                AppDialogButton(
                    text = stringResource(R.string.common_confirm),
                    modifier = Modifier.weight(1f),
                    role = DialogActionRole.Destructive,
                    onClick = {
                        showDeleteDialog = false
                        onDelete()
                    }
                )
            }
        }
    }
}
