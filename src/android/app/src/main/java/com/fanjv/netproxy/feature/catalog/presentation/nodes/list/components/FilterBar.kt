package com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.core.ui.component.deferredTopPadding
import com.fanjv.netproxy.R
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import top.yukonga.miuix.kmp.basic.TabRow
import top.yukonga.miuix.kmp.basic.TabRowDefaults

/** 节点分组筛选栏，保持分组标签切换行为。 */
@Composable
internal fun FilterBar(
    groups: List<CatalogNodeGroup>,
    selectedIndex: Int,
    onSelected: (Int) -> Unit,
    modifier: Modifier = Modifier,
    topPadding: () -> Dp = { 0.dp },
    backgroundColor: Color = Color.Transparent
) {
    if (groups.isEmpty()) return
    Column(
        modifier = modifier
            .padding(horizontal = 12.dp)
            .padding(bottom = 6.dp)
            .deferredTopPadding(topPadding)
    ) {
        TabRow(
            tabs = groups.map {
                val name = if (it.group.id == "default") {
                    stringResource(R.string.node_local_config)
                } else {
                    it.group.name
                }
                "$name (${it.nodes.size})"
            },
            selectedTabIndex = selectedIndex,
            onTabSelected = onSelected,
            height = 40.dp,
            colors = TabRowDefaults.tabRowColors(backgroundColor = backgroundColor)
        )
    }
}
