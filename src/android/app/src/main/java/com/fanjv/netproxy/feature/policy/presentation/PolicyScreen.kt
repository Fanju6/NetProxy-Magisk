package com.fanjv.netproxy.feature.policy.presentation

import android.content.Context
import android.widget.Toast
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Block
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Code
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.Movie
import androidx.compose.material.icons.rounded.MusicNote
import androidx.compose.material.icons.rounded.Notifications
import androidx.compose.material.icons.rounded.PlayCircle
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.Shuffle
import androidx.compose.material.icons.rounded.SmartToy
import androidx.compose.material.icons.rounded.Tune
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.edit
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.feature.catalog.model.CatalogNode
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.feature.catalog.model.NodeDelayResult
import com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components.NodeCard
import com.fanjv.netproxy.feature.policy.model.PolicyModel
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.CardDefaults
import top.yukonga.miuix.kmp.basic.DropdownImpl
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.PullToRefresh
import top.yukonga.miuix.kmp.basic.ListPopupColumn
import top.yukonga.miuix.kmp.overlay.OverlayListPopup
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.basic.rememberPullToRefreshState
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.ExpandMore
import top.yukonga.miuix.kmp.overlay.OverlayBottomSheet
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme

@Composable
internal fun PolicyScreen(
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
    isActive: Boolean = true,
    viewModel: PolicyViewModel = netProxyViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val context = androidx.compose.ui.platform.LocalContext.current
    val displayPreferences = remember(context) {
        context.getSharedPreferences("node_display", Context.MODE_PRIVATE)
    }
    var showDisplaySettings by remember { mutableStateOf(false) }
    var showMoreMenu by remember { mutableStateOf(false) }
    var policyEditorId by remember { mutableStateOf<String?>(null) }
    var deletePolicyId by remember { mutableStateOf<String?>(null) }
    var pickerPolicyId by remember { mutableStateOf<String?>(null) }
    var pickerGroupId by remember { mutableStateOf("") }
    val listState = rememberLazyListState()
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
    val scrollBehavior = MiuixScrollBehavior()
    val pullToRefreshState = rememberPullToRefreshState()
    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface
    val refreshTexts = listOf(
        stringResource(R.string.refresh_pulling),
        stringResource(R.string.refresh_release),
        stringResource(R.string.refresh_refresh),
        stringResource(R.string.refresh_complete),
    )

    LifecycleResumeEffect(isActive) {
        viewModel.setVisible(isActive)
        onPauseOrDispose { if (isActive) viewModel.setVisible(false) }
    }

    val savedMessage = stringResource(R.string.policy_custom_saved)
    LaunchedEffect(state.customSaveCompleted) {
        if (state.customSaveCompleted && policyEditorId != null) {
            policyEditorId = null
            Toast.makeText(context, state.error.ifBlank { savedMessage }, if (state.error.isBlank()) Toast.LENGTH_SHORT else Toast.LENGTH_LONG).show()
            val index = state.policies.indexOfFirst { it.id == state.savedCustomPolicyId }
            // 错误提示占用列表首项，保存后滚动时需计入。
            val leadingItems = (if (state.error.isNotBlank()) 1 else 0)
            if (index >= 0) listState.animateScrollToItem(index + leadingItems)
            else if (state.error.isNotBlank()) listState.animateScrollToItem(0)
        }
    }

    fun openCustomEditor(id: String) {
        if (state.busyId.isNotBlank()) return
        viewModel.beginCustomEdit()
        policyEditorId = id
    }

    Scaffold(topBar = {
        BlurredBar(backdrop) {
            TopAppBar(
                color = barColor,
                title = stringResource(R.string.policies),
                scrollBehavior = scrollBehavior,
                actions = {
                    RefreshAction(
                        enabled = !state.loading && state.busyId.isBlank(),
                        loading = state.loading,
                        onClick = { viewModel.refresh(force = true) }
                    )
                    TopBarMoreMenu(
                        expanded = showMoreMenu,
                        onExpandedChange = { showMoreMenu = it },
                        actions = listOf(
                            TopBarMenuAction(
                                text = stringResource(R.string.policy_add_custom),
                                onClick = { openCustomEditor("") }
                            ),
                            TopBarMenuAction(
                                text = stringResource(R.string.node_display_settings),
                                onClick = { showDisplaySettings = true }
                            )
                        ),
                        contentDescription = stringResource(R.string.more_actions)
                    )
                }
            )
        }
    }) { padding ->
        when {
            state.loading && state.policies.isEmpty() -> {
                Text(stringResource(R.string.policies_loading), modifier = Modifier.padding(padding).padding(20.dp))
            }
            state.error.isNotBlank() && state.policies.isEmpty() -> {
                Text(state.error, modifier = Modifier.padding(padding).padding(20.dp), color = colorScheme.error)
            }
            state.policies.isEmpty() -> {
                Text(stringResource(R.string.policy_empty), modifier = Modifier.padding(padding).padding(20.dp))
            }
            else -> {
                PullToRefresh(
                    isRefreshing = state.loading,
                    onRefresh = { viewModel.refresh(force = true) },
                    pullToRefreshState = pullToRefreshState,
                    topAppBarScrollBehavior = scrollBehavior,
                    refreshTexts = refreshTexts,
                    contentPadding = PaddingValues(top = padding.calculateTopPadding())
                ) {
                    LazyColumn(
                        modifier = Modifier.fillMaxSize(),
                        state = listState,
                        contentPadding = PaddingValues(
                            start = 12.dp,
                            // 首张卡片避开大标题区域。
                            top = padding.calculateTopPadding() + 56.dp,
                            end = 12.dp,
                            bottom = padding.calculateBottomPadding() + bottomPadding + 84.dp
                        ),
                        verticalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        if (state.error.isNotBlank()) item { Text(state.error, color = colorScheme.error) }
                        items(state.policies, key = PolicyModel::id) { policy ->
                            PolicyRow(
                                policy = policy,
                                busy = state.busyId == policy.id,
                                delay = state.delays[policy.id],
                                onOpen = {
                                    pickerPolicyId = policy.id
                                    pickerGroupId = policy.groupId
                                },
                                onToggleDisabled = { viewModel.setDisabled(policy, !policy.disabled) },
                                onEditCustom = { openCustomEditor(policy.id) },
                                onDeleteCustom = { deletePolicyId = policy.id },
                            )
                        }
                    }
                }
            }
        }
    }

    OverlayBottomSheet(
        show = showDisplaySettings,
        title = stringResource(R.string.node_display_settings),
        onDismissRequest = { showDisplaySettings = false }
    ) {
        Column(modifier = Modifier.padding(horizontal = 20.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            OverlayDropdownPreference(
                title = stringResource(R.string.node_sort_default),
                items = listOf(
                    stringResource(R.string.node_sort_default),
                    stringResource(R.string.node_sort_name),
                    stringResource(R.string.node_sort_protocol),
                    stringResource(R.string.node_sort_latency)
                ),
                selectedIndex = sortMode,
                onSelectedIndexChange = { index ->
                    sortMode = index
                    displayPreferences.edit { putInt("sort", index) }
                }
            )
            OverlayDropdownPreference(
                title = stringResource(R.string.node_density),
                items = listOf(
                    stringResource(R.string.node_density_spacious),
                    stringResource(R.string.node_density_standard),
                    stringResource(R.string.node_density_compact)
                ),
                selectedIndex = layoutDensity,
                onSelectedIndexChange = { index ->
                    layoutDensity = index
                    displayPreferences.edit { putInt("density", index) }
                }
            )
            OverlayDropdownPreference(
                title = stringResource(R.string.node_card_size),
                items = listOf(
                    stringResource(R.string.node_card_size_standard),
                    stringResource(R.string.node_card_size_compact),
                    stringResource(R.string.node_card_size_minimal)
                ),
                selectedIndex = itemSize,
                onSelectedIndexChange = { index ->
                    itemSize = index
                    displayPreferences.edit { putInt("item_size", index) }
                }
            )
            TextButton(
                text = stringResource(R.string.common_done),
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.textButtonColorsPrimary(),
                onClick = { showDisplaySettings = false }
            )
        }
    }

    val pickerPolicy = state.policies.firstOrNull { it.id == pickerPolicyId }
    OverlayBottomSheet(
        show = pickerPolicy != null,
        title = stringResource(R.string.policy_choose_node),
        onDismissRequest = { pickerPolicyId = null }
    ) {
        if (pickerPolicy != null) {
            PolicyNodePicker(
                policy = pickerPolicy,
                groupId = pickerGroupId,
                delay = state.delays[pickerPolicy.id],
                error = state.error,
                sortMode = sortMode,
                busy = state.busyId == pickerPolicy.id,
                onGroupSelected = { pickerGroupId = it },
                onSelect = { groupId, nodeTag, mode, region ->
                    viewModel.select(pickerPolicy, groupId, nodeTag, mode, region)
                    pickerPolicyId = null
                },
                onTest = { viewModel.test(pickerPolicy) }
            )
        }
    }

    val editorPolicy = state.policies.firstOrNull { it.id == policyEditorId }
    OverlayBottomSheet(
        show = policyEditorId != null,
        title = stringResource(if (editorPolicy == null) R.string.policy_add_custom else R.string.policy_edit_custom),
        onDismissRequest = { if (state.busyId != CUSTOM_BUSY_ID) policyEditorId = null }
    ) {
        if (policyEditorId != null) CustomPolicyEditor(
            policy = editorPolicy,
            busy = state.busyId == CUSTOM_BUSY_ID,
            error = state.customSaveError,
            onDismiss = { policyEditorId = null },
            onSave = { name, packages, domains, ports ->
                if (editorPolicy == null) viewModel.createCustom(name, packages, domains, ports)
                else viewModel.updateCustom(editorPolicy, name, packages, domains, ports)
            }
        )
    }

    val deletePolicy = state.policies.firstOrNull { it.id == deletePolicyId }
    OverlayDialog(
        show = deletePolicy != null,
        title = stringResource(R.string.policy_delete_custom),
        onDismissRequest = { deletePolicyId = null }
    ) {
        Column(modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.policy_custom_delete_confirm))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                TextButton(stringResource(android.R.string.cancel), modifier = Modifier.weight(1f), onClick = { deletePolicyId = null })
                TextButton(
                    stringResource(R.string.policy_delete_custom),
                    modifier = Modifier.weight(1f),
                    colors = ButtonDefaults.textButtonColorsPrimary(),
                    enabled = state.busyId.isBlank(),
                    onClick = {
                        deletePolicy?.let(viewModel::deleteCustom)
                        deletePolicyId = null
                    }
                )
            }
        }
    }
}

/** 顶栏刷新按钮：加载中旋转，给快速刷新一个可感知的动作反馈。 */
@Composable
private fun RefreshAction(enabled: Boolean, loading: Boolean, onClick: () -> Unit) {
    val spin = rememberInfiniteTransition(label = "refresh")
    val angle by spin.animateFloat(
        initialValue = 0f,
        targetValue = 360f,
        animationSpec = infiniteRepeatable(tween(900, easing = LinearEasing)),
        label = "angle"
    )
    IconButton(enabled = enabled, onClick = onClick) {
        Icon(
            Icons.Rounded.Refresh,
            stringResource(R.string.common_update),
            modifier = Modifier.rotate(if (loading) angle else 0f)
        )
    }
}

@Composable
private fun PolicyRow(
    policy: PolicyModel,
    busy: Boolean,
    delay: NodeDelayResult?,
    onOpen: () -> Unit,
    onToggleDisabled: () -> Unit,
    onEditCustom: () -> Unit,
    onDeleteCustom: () -> Unit,
) {
    val group = policy.groups.firstOrNull { it.group.id == policy.groupId }
    Card(modifier = Modifier.fillMaxWidth(), onClick = if (!busy) onOpen else null) {
        Row(
            modifier = Modifier.padding(horizontal = 14.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            PolicyIcon(policy)
            Column(
                modifier = Modifier.weight(1f).padding(horizontal = 12.dp),
                verticalArrangement = Arrangement.spacedBy(3.dp)
            ) {
                Text(
                    policy.name,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.Medium,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Box(
                        modifier = Modifier
                            .clip(RoundedCornerShape(8.dp))
                            .background(colorScheme.primary.copy(alpha = 0.12f))
                            .padding(horizontal = 6.dp, vertical = 2.dp)
                    ) {
                        Text(
                            policyTargetLabel(policy),
                            fontSize = 11.sp,
                            color = colorScheme.primary,
                            maxLines = 1
                        )
                    }
                    val selectedNode = if (policy.mode == "manual") {
                        group?.nodes?.firstOrNull { it.tag == policy.nodeTag }
                    } else null
                    val flag = selectedNode?.let { node ->
                        val tag = node.tag
                        if (tag.length >= 2 && tag[0].isSurrogate() && tag[1].isSurrogate()) tag.take(2) else null
                    }.orEmpty()
                    if (flag.isNotBlank()) {
                        Text(flag, fontSize = 13.sp, modifier = Modifier.padding(start = 8.dp))
                    }
                    if (policy.mode == "auto" || policy.mode == "manual") {
                        Text(
                            policySelectionSummary(policy, group),
                            modifier = Modifier.weight(1f).padding(start = 8.dp),
                            fontSize = 12.sp,
                            color = colorScheme.onSurfaceVariantActions,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    } else {
                        Spacer(Modifier.weight(1f))
                    }
                    val latency = selectedNode?.let { node -> nodeLatency(node, delay) }
                    if (latency != null) {
                        Text(
                            "$latency ms",
                            fontSize = 11.sp,
                            color = colorScheme.onSurfaceVariantSummary,
                            maxLines = 1
                        )
                    }
                }
            }
            if (!policy.isCustom) {
                IconButton(enabled = !busy, onClick = onToggleDisabled) {
                    Icon(
                        Icons.Rounded.Block,
                        contentDescription = null,
                        modifier = Modifier.size(18.dp),
                        tint = if (policy.disabled) colorScheme.onSurfaceVariantActions else colorScheme.primary
                    )
                }
            } else {
                IconButton(enabled = !busy, onClick = onEditCustom) {
                    Icon(
                        Icons.Rounded.Edit,
                        contentDescription = stringResource(R.string.policy_edit_custom),
                        modifier = Modifier.size(18.dp),
                        tint = colorScheme.onSurfaceVariantActions
                    )
                }
                IconButton(enabled = !busy, onClick = onDeleteCustom) {
                    Icon(
                        Icons.Rounded.Delete,
                        contentDescription = stringResource(R.string.policy_delete_custom),
                        modifier = Modifier.size(18.dp),
                        tint = colorScheme.error
                    )
                }
            }
            RuntimeDot(policy.runtimeSynced && !policy.disabled)
        }
    }
}

@Composable
private fun CustomPolicyEditor(
    policy: PolicyModel?,
    busy: Boolean,
    error: String,
    onDismiss: () -> Unit,
    onSave: (String, List<String>, List<String>, List<Int>) -> Unit,
) {
    var name by remember(policy?.id) { mutableStateOf(policy?.name.orEmpty()) }
    var packages by remember(policy?.id) { mutableStateOf(policy?.packageNames.orEmpty().joinToString("\n")) }
    var domains by remember(policy?.id) { mutableStateOf(policy?.domainSuffixes.orEmpty().joinToString("\n")) }
    var ports by remember(policy?.id) { mutableStateOf(policy?.ports.orEmpty().joinToString(", ")) }
    val packageItems = packages.lines().map(String::trim).filter(String::isNotBlank).distinct()
    val domainItems = domains.lines().map(String::trim).filter(String::isNotBlank).distinct()
    val portTokens = ports.split(',', '\n', ' ', '，').map(String::trim).filter(String::isNotBlank)
    val portItems = portTokens.mapNotNull(String::toIntOrNull).distinct()
    val hasInvalidPort = portTokens.any { it.toIntOrNull()?.let { port -> port in 1..65535 } != true }
    val hasMatch = packageItems.isNotEmpty() || domainItems.isNotEmpty() || portItems.isNotEmpty()

    Column(
        modifier = Modifier.padding(horizontal = 20.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        TextField(
            value = name,
            onValueChange = { name = it },
            label = stringResource(R.string.policy_custom_name),
            enabled = !busy,
            maxLines = 1,
            modifier = Modifier.fillMaxWidth()
        )
        TextField(
            value = packages,
            onValueChange = { packages = it },
            label = stringResource(R.string.policy_custom_packages),
            enabled = !busy,
            maxLines = 3,
            modifier = Modifier.fillMaxWidth()
        )
        TextField(
            value = domains,
            onValueChange = { domains = it },
            label = stringResource(R.string.policy_custom_domains),
            enabled = !busy,
            maxLines = 3,
            modifier = Modifier.fillMaxWidth()
        )
        TextField(
            value = ports,
            onValueChange = { ports = it },
            label = stringResource(R.string.policy_custom_ports),
            enabled = !busy,
            maxLines = 1,
            modifier = Modifier.fillMaxWidth()
        )
        if (hasInvalidPort) {
            Text(
                text = stringResource(R.string.policy_custom_ports_invalid),
                fontSize = 12.sp,
                color = colorScheme.error
            )
        }
        Text(
            text = stringResource(R.string.policy_custom_match_hint),
            fontSize = 12.sp,
            color = colorScheme.onSurfaceVariantSummary
        )
        if (policy == null) Text(
            text = stringResource(R.string.policy_custom_default_route),
            fontSize = 12.sp,
            color = colorScheme.onSurfaceVariantSummary
        )
        if (error.isNotBlank()) Text(text = error, color = colorScheme.error, fontSize = 13.sp)
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(16.dp)) {
            TextButton(stringResource(android.R.string.cancel), modifier = Modifier.weight(1f), enabled = !busy, onClick = onDismiss)
            TextButton(
                stringResource(if (busy) R.string.policy_saving else R.string.policy_save_custom),
                modifier = Modifier.weight(1f),
                enabled = !busy && name.isNotBlank() && hasMatch && !hasInvalidPort,
                colors = ButtonDefaults.textButtonColorsPrimary(),
                onClick = { onSave(name.trim(), packageItems, domainItems, portItems) }
            )
        }
    }
}

@Composable
private fun PolicyNodePicker(
    policy: PolicyModel,
    groupId: String,
    delay: NodeDelayResult?,
    error: String,
    sortMode: Int,
    busy: Boolean,
    onGroupSelected: (String) -> Unit,
    onSelect: (String, String, String, String) -> Unit,
    onTest: () -> Unit
) {
    val group = policy.groups.firstOrNull { it.group.id == groupId } ?: policy.groups.firstOrNull()
    Column(
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        PolicyModeSegmentedRow(
            mode = policy.mode,
            busy = busy,
            onMode = { mode ->
                onSelect(
                    group?.group?.id ?: policy.groupId,
                    "",
                    mode,
                    if (mode == "auto") policy.region else ""
                )
            }
        )
        if (policy.mode == "direct") {
            Text(
                text = stringResource(R.string.policy_mode_direct_hint),
                fontSize = 12.sp,
                color = colorScheme.onSurfaceVariantSummary
            )
        }
        if (policy.mode == "proxy") {
            Text(
                text = stringResource(R.string.policy_mode_proxy_hint),
                fontSize = 12.sp,
                color = colorScheme.onSurfaceVariantSummary
            )
        }
        NodeSelectionSection(
            policy = policy,
            groupId = groupId,
            delay = delay,
            error = error,
            sortMode = sortMode,
            busy = busy,
            onGroupSelected = onGroupSelected,
            onSelect = onSelect,
            onTest = onTest
        )
    }
}

@Composable
private fun NodeSelectionSection(
    policy: PolicyModel,
    groupId: String,
    delay: NodeDelayResult?,
    error: String,
    sortMode: Int,
    busy: Boolean,
    onGroupSelected: (String) -> Unit,
    onSelect: (String, String, String, String) -> Unit,
    onTest: () -> Unit
) {
    val group = policy.groups.firstOrNull { it.group.id == groupId } ?: policy.groups.firstOrNull()
    val selectedGroupIndex = policy.groups.indexOfFirst { it.group.id == group?.group?.id }.coerceAtLeast(0)
    var query by remember(policy.id, group?.group?.id) { mutableStateOf("") }
    // 节点选择器保持紧凑，不跟随页面卡片密度放大。
    val pickerItemSize = 2
    var selectionMode by remember(policy.id, group?.group?.id) {
        mutableStateOf(
            when {
                policy.mode == "manual" -> "manual"
                policy.region.isNotBlank() -> "region"
                else -> "global"
            }
        )
    }
    var showGroupMenu by remember(policy.id) { mutableStateOf(false) }
    Column(
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        if (policy.groups.size > 1) {
            OverlayListPopup(
                show = showGroupMenu,
                onDismissRequest = { showGroupMenu = false }
            ) {
                ListPopupColumn {
                    policy.groups.forEachIndexed { index, groupItem ->
                        DropdownImpl(
                            text = if (groupItem.group.id == "default") stringResource(R.string.node_local_config) else groupItem.group.name,
                            optionSize = policy.groups.size,
                            isSelected = index == selectedGroupIndex,
                            index = index,
                            onSelectedIndexChange = {
                                showGroupMenu = false
                                onGroupSelected(groupItem.group.id)
                            }
                        )
                    }
                }
            }
        }
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            if (policy.groups.size == 1) {
                Text(
                    text = group?.group?.name.orEmpty().ifBlank { stringResource(R.string.node_local_config) },
                    modifier = Modifier.weight(1f),
                    fontSize = 13.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    color = colorScheme.onSurfaceVariantActions
                )
            } else {
                Card(
                    modifier = Modifier.weight(1f).heightIn(min = 44.dp, max = 48.dp),
                    cornerRadius = 14.dp,
                    insideMargin = PaddingValues(horizontal = 12.dp, vertical = 0.dp),
                    colors = CardDefaults.defaultColors(color = colorScheme.surfaceContainer),
                    onClick = if (!busy) ({ showGroupMenu = true }) else null
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(1.dp)) {
                            Text(
                                text = stringResource(R.string.policy_group_picker),
                                fontSize = 11.sp,
                                color = colorScheme.onSurfaceVariantSummary,
                                maxLines = 1
                            )
                            Text(
                                text = group?.group?.name.orEmpty().ifBlank { stringResource(R.string.node_local_config) },
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                fontSize = 14.sp
                            )
                        }
                        val nodeCount = group?.nodes?.size ?: 0
                        Box(
                            modifier = Modifier
                                .clip(RoundedCornerShape(10.dp))
                                .background(colorScheme.primary.copy(alpha = 0.12f))
                                .padding(horizontal = 8.dp, vertical = 3.dp)
                        ) {
                            Text(
                                text = stringResource(R.string.node_count, nodeCount),
                                fontSize = 11.sp,
                                color = colorScheme.primary,
                                maxLines = 1
                            )
                        }
                        Icon(MiuixIcons.ExpandMore, contentDescription = null, modifier = Modifier.size(20.dp))
                    }
                }
            }
        }
        if (error.isNotBlank()) {
            Text(
                text = error,
                fontSize = 12.sp,
                color = colorScheme.error,
                maxLines = 3,
                overflow = TextOverflow.Ellipsis
            )
        }
        if (group == null || group.nodes.isEmpty()) {
            Text(stringResource(R.string.policy_no_nodes), color = colorScheme.onSurfaceVariantSummary)
        } else {
            val allNodes = policyNodes(group, delay, sortMode)
            val regions = groupNodesByRegion(allNodes)
            PickerModeTabs(
                mode = selectionMode,
                regionCount = regions.size,
                nodeCount = allNodes.size,
                enabled = !busy,
                onModeChange = { selectionMode = it }
            )
            when (selectionMode) {
                "global" -> {
                    SelectionSummaryCard(
                        title = stringResource(R.string.policy_global_auto),
                        summary = stringResource(R.string.policy_global_auto_hint),
                        selected = policy.mode == "auto" && policy.region.isBlank(),
                        enabled = !busy,
                        onClick = { onSelect(group.group.id, "", "auto", "") }
                    )
                    Spacer(Modifier.height(8.dp))
                    PickerTestFooter(
                        busy = busy,
                        delay = delay,
                        enabled = !busy && policy.mode != "proxy" && group.nodes.isNotEmpty(),
                        onTest = onTest
                    )
                }
                "region" -> {
                    LazyColumn(
                        modifier = Modifier.fillMaxWidth().heightIn(max = 520.dp),
                        contentPadding = PaddingValues(bottom = 12.dp),
                        verticalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        items(regions, key = { it.region }) { section ->
                            RegionSelectionRow(
                                section = section,
                                selected = policy.mode == "auto" && policy.region == section.region,
                                enabled = !busy,
                                onClick = { onSelect(group.group.id, "", "auto", section.region) }
                            )
                        }
                    }
                    Spacer(Modifier.height(8.dp))
                    PickerTestFooter(
                        busy = busy,
                        delay = delay,
                        enabled = !busy && policy.mode != "proxy" && group.nodes.isNotEmpty(),
                        onTest = onTest
                    )
                }
                else -> {
                    CompactSearchField(
                        value = query,
                        onValueChange = { query = it },
                        hint = stringResource(R.string.policy_search_nodes)
                    )
                    val filteredNodes = allNodes.filter { node ->
                        query.isBlank() ||
                            node.tag.contains(query, ignoreCase = true) ||
                            node.server.contains(query, ignoreCase = true)
                    }
                    if (filteredNodes.isEmpty()) {
                        Text(stringResource(R.string.policy_no_search_results), color = colorScheme.onSurfaceVariantSummary)
                    } else {
                        LazyColumn(
                            modifier = Modifier.fillMaxWidth().heightIn(max = 520.dp),
                            contentPadding = PaddingValues(bottom = 12.dp),
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            items(filteredNodes, key = { it.tag }) { node ->
                                NodeCard(
                                    title = node.tag,
                                    summary = nodeSummary(node, delay),
                                    protocol = node.protocol.uppercase().ifBlank { "NODE" },
                                    latency = nodeLatency(node, delay),
                                    selected = policy.mode == "manual" && policy.groupId == group.group.id && policy.nodeTag == node.tag,
                                    enabled = !busy,
                                    itemSize = pickerItemSize,
                                    outlined = true,
                                    onClick = { onSelect(group.group.id, node.tag, "manual", "") }
                                )
                            }
                        }
                    }
                    Spacer(Modifier.height(8.dp))
                    PickerTestFooter(
                        busy = busy,
                        delay = delay,
                        enabled = !busy && policy.mode != "proxy" && group.nodes.isNotEmpty(),
                        onTest = onTest
                    )
                }
            }
        }
    }
}

@Composable
private fun PickerTestFooter(
    busy: Boolean,
    delay: NodeDelayResult?,
    enabled: Boolean,
    onTest: () -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        TextButton(
            text = stringResource(if (busy) R.string.policy_testing else R.string.policy_test),
            modifier = Modifier.fillMaxWidth(),
            colors = ButtonDefaults.textButtonColorsPrimary(),
            enabled = enabled,
            onClick = onTest
        )
        if (busy) {
            PolicyTestingStatus()
        } else if (delay != null) {
            PolicyTestSummary(delay)
        }
    }
}

@Composable
private fun PickerModeTabs(
    mode: String,
    regionCount: Int,
    nodeCount: Int,
    enabled: Boolean,
    onModeChange: (String) -> Unit,
) {
    val tabs = listOf(
        Triple("global", stringResource(R.string.policy_tab_fastest), null as String?),
        Triple("region", stringResource(R.string.policy_region_auto), regionCount.toString()),
        Triple("manual", stringResource(R.string.policy_manual), nodeCount.toString()),
    )
    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        tabs.forEach { (value, label, badge) ->
            val selected = mode == value
            val background = if (selected) colorScheme.primary.copy(alpha = 0.16f)
                else colorScheme.surfaceContainer
            val borderColor = if (selected) colorScheme.primary
                else colorScheme.outline.copy(alpha = 0.35f)
            val contentColor = if (selected) colorScheme.primary
                else colorScheme.onSurfaceVariantActions
            Row(
                modifier = Modifier
                    .weight(1f)
                    .height(40.dp)
                    .clip(RoundedCornerShape(14.dp))
                    .background(background)
                    .border(1.dp, borderColor, RoundedCornerShape(14.dp))
                    .clickable(enabled = enabled) { onModeChange(value) },
                horizontalArrangement = Arrangement.Center,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = label,
                    fontSize = 12.sp,
                    color = contentColor,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                if (badge != null) {
                    Text(
                        text = " $badge",
                        fontSize = 11.sp,
                        color = if (selected) colorScheme.primary else colorScheme.onSurfaceVariantSummary,
                        maxLines = 1
                    )
                }
            }
        }
    }
}

@Composable
private fun SelectionSummaryCard(
    title: String,
    summary: String,
    selected: Boolean,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        cornerRadius = 14.dp,
        insideMargin = PaddingValues(horizontal = 12.dp, vertical = 10.dp),
        colors = CardDefaults.defaultColors(
            color = if (selected) colorScheme.primary.copy(alpha = 0.12f) else colorScheme.surfaceContainer
        ),
        onClick = if (enabled) onClick else null
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(title, fontSize = 14.sp, fontWeight = FontWeight.Medium, color = colorScheme.onSurface)
            Text(summary, fontSize = 12.sp, color = colorScheme.onSurfaceVariantSummary, maxLines = 2)
        }
    }
}

@Composable
private fun RegionSelectionRow(
    section: NodeRegionSection,
    selected: Boolean,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        cornerRadius = 14.dp,
        insideMargin = PaddingValues(horizontal = 12.dp, vertical = 8.dp),
        colors = CardDefaults.defaultColors(
            color = if (selected) colorScheme.primary.copy(alpha = 0.14f) else colorScheme.surfaceContainer
        ),
        onClick = { if (enabled) onClick() }
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(section.title, fontSize = 14.sp, fontWeight = FontWeight.Medium)
                Text(
                    text = stringResource(R.string.node_count, section.nodes.size),
                    fontSize = 12.sp,
                    color = colorScheme.onSurfaceVariantSummary
                )
            }
            if (selected) {
                Icon(
                    imageVector = Icons.Rounded.Check,
                    contentDescription = stringResource(R.string.policy_region_selected),
                    tint = colorScheme.primary,
                    modifier = Modifier.size(20.dp)
                )
            } else {
                Icon(
                    imageVector = MiuixIcons.ExpandMore,
                    contentDescription = null,
                    tint = colorScheme.onSurfaceVariantActions,
                    modifier = Modifier.size(20.dp)
                )
            }
        }
    }
}

@Composable
private fun CompactSearchField(value: String, onValueChange: (String) -> Unit, hint: String) {
    val shape = RoundedCornerShape(16.dp)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(48.dp)
            .clip(shape)
            .background(colorScheme.surfaceContainer)
            .border(1.dp, colorScheme.outline.copy(alpha = 0.35f), shape)
            .padding(horizontal = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Icon(Icons.Rounded.Search, contentDescription = null, modifier = Modifier.size(18.dp), tint = colorScheme.onSurfaceVariantSummary)
        BasicTextField(
            value = value,
            onValueChange = onValueChange,
            modifier = Modifier.weight(1f),
            singleLine = true,
            textStyle = TextStyle(color = colorScheme.onSurface, fontSize = 14.sp),
            decorationBox = { inner ->
                if (value.isBlank()) Text(hint, color = colorScheme.onSurfaceVariantSummary, fontSize = 14.sp)
                inner()
            }
        )
    }
}

@Composable
private fun policyTargetLabel(policy: PolicyModel): String = if (policy.disabled) {
    stringResource(R.string.policy_disabled)
} else when (policy.mode) {
    "direct" -> stringResource(R.string.policy_direct)
    "proxy" -> stringResource(R.string.policy_proxy)
    "auto" -> if (policy.region.isBlank()) stringResource(R.string.policy_auto) else stringResource(R.string.policy_region_auto)
    else -> stringResource(R.string.policy_manual)
}

@Composable
private fun policySelectionSummary(policy: PolicyModel, group: CatalogNodeGroup?): String = when (policy.mode) {
    "direct" -> stringResource(R.string.policy_direct)
    "proxy" -> stringResource(R.string.policy_proxy)
    "auto" -> if (policy.region.isBlank()) stringResource(
        R.string.policy_auto_in_group,
        group?.group?.name.orEmpty().ifBlank { stringResource(R.string.node_local_config) }
    ) else stringResource(R.string.policy_region_auto_summary, regionTitle(policy.region))
    else -> if (policy.nodeTag.isBlank()) {
        stringResource(
            R.string.policy_auto_in_group,
            group?.group?.name.orEmpty().ifBlank { stringResource(R.string.node_local_config) }
        )
    } else {
        stringResource(
            R.string.policy_manual_in_group,
            group?.group?.name.orEmpty().ifBlank { stringResource(R.string.node_local_config) },
            policy.nodeTag
        )
    }
}

@Composable
private fun PolicyIcon(policy: PolicyModel) {
    val icon = when (policy.id) {
        "ai" -> Icons.Rounded.SmartToy
        "youtube" -> Icons.Rounded.PlayCircle
        "tiktok" -> Icons.Rounded.MusicNote
        "netflix" -> Icons.Rounded.Movie
        "github" -> Icons.Rounded.Code
        "google" -> Icons.Rounded.Search
        "fcm" -> Icons.Rounded.Notifications
        else -> Icons.Rounded.Tune
    }
    Box(
        modifier = Modifier
            .size(40.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(colorScheme.primary.copy(alpha = 0.12f)),
        contentAlignment = Alignment.Center
    ) {
        Icon(icon, contentDescription = policy.name, tint = colorScheme.primary, modifier = Modifier.size(22.dp))
    }
}

@Composable
private fun PolicyModeSegmentedRow(
    mode: String,
    busy: Boolean,
    onMode: (String) -> Unit,
) {
    val items = listOf(
        Triple("direct", Icons.Rounded.Block, stringResource(R.string.policy_direct)),
        Triple("proxy", Icons.Rounded.Shuffle, stringResource(R.string.policy_proxy)),
    )
    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        items.forEach { (value, icon, label) ->
            val selected = mode == value
            val enabled = !busy
            val background = when {
                selected -> colorScheme.primary.copy(alpha = 0.16f)
                else -> colorScheme.surfaceContainer
            }
            val borderColor = when {
                selected -> colorScheme.primary
                else -> colorScheme.outline.copy(alpha = 0.35f)
            }
            val contentColor = when {
                selected -> colorScheme.primary
                enabled -> colorScheme.onSurfaceVariantActions
                else -> colorScheme.onSurfaceVariantSummary
            }
            Column(
                modifier = Modifier
                    .weight(1f)
                    .height(64.dp)
                    .clip(RoundedCornerShape(16.dp))
                    .background(background)
                    .border(1.dp, borderColor, RoundedCornerShape(16.dp))
                    .clickable(enabled = enabled) { onMode(value) },
                verticalArrangement = Arrangement.Center,
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Icon(
                    imageVector = icon,
                    contentDescription = null,
                    tint = contentColor,
                    modifier = Modifier.size(22.dp)
                )
                Text(
                    text = label,
                    fontSize = 12.sp,
                    color = contentColor,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 4.dp, start = 4.dp, end = 4.dp)
                )
            }
        }
    }
}

@Composable
private fun RuntimeDot(synced: Boolean) {
    val description = stringResource(if (synced) R.string.policy_runtime_active else R.string.policy_runtime_inactive)
    Box(
        modifier = Modifier
            .size(9.dp)
            .semantics { contentDescription = description }
            .clip(CircleShape)
            .background(if (synced) Color(0xFF32A852) else colorScheme.onSurfaceVariantSummary)
    )
}

private fun policyNodes(
    group: CatalogNodeGroup,
    delay: NodeDelayResult?,
    sortMode: Int
): List<CatalogNode> = when (sortMode) {
        1 -> group.nodes.sortedBy { it.tag.lowercase() }
        2 -> group.nodes.sortedBy { it.protocol.lowercase() }
        3 -> group.nodes.sortedBy { nodeLatencyValue(it, delay) }
        else -> group.nodes
    }

private data class NodeRegionSection(val title: String, val region: String, val nodes: List<CatalogNode>)

private data class NodeRegionRule(val title: String, val tokens: List<String>)

private val nodeRegionRules = listOf(
    NodeRegionRule("hk", listOf("🇭🇰", "香港", "hong kong", "hongkong")),
    NodeRegionRule("mo", listOf("🇲🇴", "澳门", "macau")),
    NodeRegionRule("tw", listOf("🇹🇼", "台湾", "taiwan")),
    NodeRegionRule("jp", listOf("🇯🇵", "日本", "japan")),
    NodeRegionRule("kr", listOf("🇰🇷", "韩国", "korea")),
    NodeRegionRule("sg", listOf("🇸🇬", "新加坡", "singapore")),
    NodeRegionRule("us", listOf("🇺🇸", "美国", "united states", "usa", "us-")),
    NodeRegionRule("ca", listOf("🇨🇦", "加拿大", "canada")),
    NodeRegionRule("gb", listOf("🇬🇧", "英国", "united kingdom", "uk-")),
    NodeRegionRule("de", listOf("🇩🇪", "德国", "germany")),
    NodeRegionRule("fr", listOf("🇫🇷", "法国", "france")),
    NodeRegionRule("nl", listOf("🇳🇱", "荷兰", "netherlands")),
    NodeRegionRule("au", listOf("🇦🇺", "澳大利亚", "australia")),
    NodeRegionRule("in", listOf("🇮🇳", "印度", "india")),
    NodeRegionRule("tr", listOf("🇹🇷", "土耳其", "turkey")),
    NodeRegionRule("ru", listOf("🇷🇺", "俄罗斯", "russia")),
)

private fun groupNodesByRegion(nodes: List<CatalogNode>): List<NodeRegionSection> {
    val sections = linkedMapOf<String, MutableList<CatalogNode>>()
    nodes.forEach { node ->
        val region = nodeRegionKey(node)
        sections.getOrPut(region) { mutableListOf() }.add(node)
    }
    return sections.entries
        .sortedWith(compareBy { if (it.key == "other") 1 else 0 })
        .map { NodeRegionSection(regionTitle(it.key), it.key, it.value) }
}

private fun nodeRegionKey(node: CatalogNode): String {
    if (node.region.isNotBlank()) return node.region
    val normalized = node.tag.lowercase()
    return nodeRegionRules.firstOrNull { rule ->
        rule.tokens.any { token -> normalized.contains(token.lowercase()) }
    }?.title ?: "other"
}

private fun regionTitle(key: String): String = when (key) {
    "hk" -> "🇭🇰 香港"
    "mo" -> "🇲🇴 澳门"
    "tw" -> "🇹🇼 台湾"
    "jp" -> "🇯🇵 日本"
    "kr" -> "🇰🇷 韩国"
    "sg" -> "🇸🇬 新加坡"
    "us" -> "🇺🇸 美国"
    "ca" -> "🇨🇦 加拿大"
    "gb" -> "🇬🇧 英国"
    "de" -> "🇩🇪 德国"
    "fr" -> "🇫🇷 法国"
    "nl" -> "🇳🇱 荷兰"
    "au" -> "🇦🇺 澳大利亚"
    "in" -> "🇮🇳 印度"
    "tr" -> "🇹🇷 土耳其"
    "ru" -> "🇷🇺 俄罗斯"
    else -> "other"
}

@Composable
private fun PolicyTestingStatus() {
    Text(
        text = stringResource(R.string.policy_testing_status),
        fontSize = 12.sp,
        color = colorScheme.primary
    )
}

@Composable
private fun PolicyTestSummary(delay: NodeDelayResult) {
    val results = delay.groups.flatMap { it.items }
    val tested = results.count { it.urlTestTime != null || it.urlTestDelay != null }
    val fastest = results.mapNotNull { it.urlTestDelay }.minOrNull()
    Text(
        text = when {
            tested == 0 && results.isEmpty() -> stringResource(R.string.policy_test_empty)
            fastest != null -> stringResource(R.string.policy_test_result, tested, fastest)
            else -> stringResource(R.string.policy_test_result_no_latency, tested)
        },
        fontSize = 12.sp,
        color = if (fastest != null) colorScheme.primary else colorScheme.onSurfaceVariantSummary
    )
}

private fun nodeResult(node: CatalogNode, delay: NodeDelayResult?) =
    delay?.groups?.flatMap { it.items }?.firstOrNull { result ->
        result.tag == node.tag ||
            result.tag.endsWith("/${node.tag}") ||
            node.tag.endsWith("/${result.tag}")
    }

private fun nodeLatency(node: CatalogNode, delay: NodeDelayResult?): String? {
    val result = nodeResult(node, delay) ?: return null
    return result.urlTestDelay?.toString() ?: if (result.urlTestTime != null) "failed" else null
}

private fun nodeLatencyValue(node: CatalogNode, delay: NodeDelayResult?): Int =
    nodeResult(node, delay)?.urlTestDelay ?: Int.MAX_VALUE

@Composable
private fun nodeSummary(node: CatalogNode, delay: NodeDelayResult?): String =
    if (nodeLatency(node, delay) == null) stringResource(R.string.policy_node_not_tested) else ""
