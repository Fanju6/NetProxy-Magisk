package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.R
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference

/** 传输层配置区。 */
@Composable
internal fun TransportSection(
    transportType: String,
    path: String,
    host: String,
    serviceName: String,
    onTransportTypeChange: (String) -> Unit,
    onPathChange: (String) -> Unit,
    onHostChange: (String) -> Unit,
    onServiceNameChange: (String) -> Unit,
    onImeDone: () -> Unit
) {
    val types = listOf("none", "ws", "grpc", "http", "httpupgrade")
    Column(verticalArrangement = Arrangement.Top) {
        SmallTitle(
            text = stringResource(R.string.transport_settings),
            insideMargin = PaddingValues(start = 26.dp, top = 8.dp, bottom = 8.dp, end = 26.dp)
        )
        Card(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp)
                .padding(bottom = 12.dp)
        ) {
            OverlayDropdownPreference(
                title = stringResource(R.string.transport_protocol),
                items = types.map { if (it == "none") "DIRECT (TCP)" else it.uppercase() },
                selectedIndex = types.indexOf(transportType).coerceAtLeast(0),
                onSelectedIndexChange = { index -> onTransportTypeChange(types[index]) }
            )
        }
        if (transportType in setOf("ws", "httpupgrade", "http", "h2")) {
            EditorField(
                value = path,
                label = stringResource(R.string.path_default),
                onValueChange = onPathChange,
                onImeDone = onImeDone
            )
            EditorField(
                value = host,
                label = stringResource(R.string.domain_name_host),
                onValueChange = onHostChange,
                onImeDone = onImeDone
            )
        }
        if (transportType == "grpc") {
            EditorField(
                value = serviceName,
                label = stringResource(R.string.service_name),
                onValueChange = onServiceNameChange,
                onImeDone = onImeDone
            )
        }
    }
}


