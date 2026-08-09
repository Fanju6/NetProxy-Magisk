package com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components

import androidx.compose.foundation.layout.padding
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import top.yukonga.miuix.kmp.basic.TabRow
import top.yukonga.miuix.kmp.basic.TabRowDefaults

/** 节点分组筛选栏，保留原有的分组标签切换行为。 */
@androidx.compose.runtime.Composable
internal fun FilterBar(
    groups: List<CatalogNodeGroup>,
    selectedIndex: Int,
    onSelected: (Int) -> Unit,
    modifier: Modifier = Modifier
) {
    if (groups.isEmpty()) return
    TabRow(
        tabs = groups.map {
            val name = if (it.group.id == "default") "本地配置" else it.group.name
            "$name (${it.nodes.size})"
        },
        selectedTabIndex = selectedIndex,
        onTabSelected = onSelected,
        modifier = modifier.padding(horizontal = 12.dp, vertical = 8.dp),
        colors = TabRowDefaults.tabRowColors(backgroundColor = Color.Transparent)
    )
}


