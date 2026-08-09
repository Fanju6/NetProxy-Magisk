package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.R
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference

/** 节点协议选择器。 */
@Composable
internal fun ProtocolSelector(
    selectedProtocol: String,
    availableProtocols: List<String>,
    onProtocolSelected: (String) -> Unit,
    modifier: Modifier = Modifier
) {
    Card(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp)
            .padding(bottom = 12.dp)
    ) {
        OverlayDropdownPreference(
            title = androidx.compose.ui.res.stringResource(R.string.protocol_type),
            items = availableProtocols.map(String::uppercase),
            selectedIndex = availableProtocols.indexOf(selectedProtocol).coerceAtLeast(0),
            onSelectedIndexChange = { index ->
                availableProtocols.getOrNull(index)?.let(onProtocolSelected)
            }
        )
    }
}


