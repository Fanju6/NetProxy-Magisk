package com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.NestedScrollConnection
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.core.ui.component.EmptyCatalog
import com.fanjv.netproxy.feature.catalog.model.CatalogNode
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.ExpandLess
import top.yukonga.miuix.kmp.icon.extended.ExpandMore
import top.yukonga.miuix.kmp.icon.extended.Refresh
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.PressFeedbackType
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

/** 网格布局中的当前分组。 */
@Composable
internal fun CatalogNodeGrid(
    group: CatalogNodeGroup,
    activeGroupId: String,
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
    val automaticSelected = selectorMode == "urltest" && activeGroupId == group.group.id
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
        item {
            NodeCard(
                title = "Auto-Fastest",
                summary = "自动测速",
                protocol = "AUTO",
                latency = latencies["Auto/${group.group.id}"],
                selected = automaticSelected,
                enabled = !busy && group.nodes.isNotEmpty(),
                itemSize = itemSize,
                icon = MiuixIcons.Refresh,
                onClick = onAuto
            )
        }
        items(sortedNodes, key = { it.tag }) { node ->
            NodeCard(
                title = node.tag,
                summary = node.serverWithPort(),
                protocol = node.protocol.uppercase().ifBlank { "NODE" },
                latency = latencies["${group.group.id}/${node.tag}"],
                selected = selectorMode == "manual" && selectedRef == "${group.group.id}/${node.tag}",
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

/** 列表布局中的可折叠分组。 */
@Composable
internal fun CatalogGroupList(
    groups: List<CatalogNodeGroup>,
    activeGroupId: String,
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
                    group.nodes, sortMode, groupId, latencies
                )
                val rows = entries.chunked(columns)
                rows.forEachIndexed { rowIndex, row ->
                    item(key = "row:$groupId:$rowIndex") {
                        Row(horizontalArrangement = Arrangement.spacedBy(spacing)) {
                            row.forEach { node ->
                                Box(Modifier.weight(1f)) {
                                    if (node == null) {
                                        NodeCard(
                                            title = "Auto-Fastest",
                                            summary = "自动测速",
                                            protocol = "AUTO",
                                            latency = latencies["Auto/$groupId"],
                                            selected = selectorMode == "urltest" && activeGroupId == groupId,
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
                                            selected = selectorMode == "manual" && selectedRef == "$groupId/${node.tag}",
                                            enabled = !busy,
                                            itemSize = itemSize,
                                            onClick = { onNode(group, node) },
                                            onLongClick = { onNodeAction(group, node) }
                                        )
                                    }
                                }
                            }
                            repeat(columns - row.size) { Box(Modifier.weight(1f)) }
                        }
                        if (rowIndex < rows.lastIndex) Spacer(Modifier.height(spacing))
                    }
                }
            }
        }
    }
}

@Composable
private fun CatalogGroupHeader(name: String, count: Int, expanded: Boolean, onClick: () -> Unit) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        cornerRadius = 16.dp,
        insideMargin = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
        pressFeedbackType = PressFeedbackType.Sink,
        showIndication = true,
        onClick = onClick
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
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

private fun sortCatalogNodes(
    nodes: List<CatalogNode>,
    sortMode: Int,
    groupId: String,
    latencies: Map<String, String>
): List<CatalogNode> = when (sortMode) {
    1 -> nodes.sortedBy { it.tag.lowercase() }
    2 -> nodes.sortedWith(compareBy<CatalogNode> { it.protocol }.thenBy { it.tag })
    3 -> nodes.sortedBy { latencies["$groupId/${it.tag}"]?.toIntOrNull() ?: Int.MAX_VALUE }
    else -> nodes
}

private fun CatalogNode.serverWithPort(): String = buildString {
    append(server.ifBlank { "--" })
    if (port > 0) append(':').append(port)
}
