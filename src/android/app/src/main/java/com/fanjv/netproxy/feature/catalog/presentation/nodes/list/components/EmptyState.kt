package com.fanjv.netproxy.feature.catalog.presentation.nodes.list.components

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import com.fanjv.netproxy.core.ui.component.EmptyCatalog

/** 节点列表的统一空状态。 */
@Composable
internal fun EmptyState(
    text: String,
    onRefresh: (() -> Unit)?,
    modifier: Modifier = Modifier
) {
    EmptyCatalog(text = text, onRefresh = onRefresh, modifier = modifier)
}


