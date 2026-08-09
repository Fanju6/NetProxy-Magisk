package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.R
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference

@Composable
internal fun ServerConfigSection(
    tag: String,
    onTagChange: (String) -> Unit,
    server: String,
    onServerChange: (String) -> Unit,
    serverPort: String,
    onServerPortChange: (String) -> Unit,
    type: String,
    onTypeChange: (String) -> Unit,
    uuid: String,
    onUuidChange: (String) -> Unit,
    flow: String,
    onFlowChange: (String) -> Unit,
    security: String,
    onSecurityChange: (String) -> Unit,
    alterId: String,
    onAlterIdChange: (String) -> Unit,
    method: String,
    onMethodChange: (String) -> Unit,
    password: String,
    onPasswordChange: (String) -> Unit,
    plugin: String,
    onPluginChange: (String) -> Unit,
    pluginOpts: String,
    onPluginOptsChange: (String) -> Unit,
    upMbps: String,
    onUpMbpsChange: (String) -> Unit,
    downMbps: String,
    onDownMbpsChange: (String) -> Unit,
    obfsType: String,
    onObfsTypeChange: (String) -> Unit,
    obfsPassword: String,
    onObfsPasswordChange: (String) -> Unit,
    serverPorts: String,
    onServerPortsChange: (String) -> Unit,
    hopInterval: String,
    onHopIntervalChange: (String) -> Unit,
    congestionControl: String,
    onCongestionControlChange: (String) -> Unit,
    udpRelayMode: String,
    onUdpRelayModeChange: (String) -> Unit,
    udpOverStream: Boolean,
    onUdpOverStreamChange: (Boolean) -> Unit,
    zeroRttHandshake: Boolean,
    onZeroRttHandshakeChange: (Boolean) -> Unit,
    heartbeat: String,
    onHeartbeatChange: (String) -> Unit,
    focusManager: androidx.compose.ui.focus.FocusManager,
    alterIdLabel: String,
    udpRelayModeTitle: String
) {
    Column {
        // ==================== 1. 常规设置 ====================
        Column {
            SmallTitle(
                text = stringResource(R.string.basic_settings),
                insideMargin = PaddingValues(
                    start = 26.dp,
                    top = 12.dp,
                    bottom = 8.dp,
                    end = 26.dp
                )
            )
        }
        Column {
            TextField(
                value = tag,
                onValueChange = { onTagChange(it) },
                label = stringResource(R.string.node_tag),
                singleLine = true,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp)
                    .padding(bottom = 12.dp),
                keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
            )
        }
        Column {
            ProtocolSelector(
                selectedProtocol = type,
                availableProtocols = listOf(
                    "vless",
                    "vmess",
                    "shadowsocks",
                    "trojan",
                    "hysteria2",
                    "tuic",
                    "anytls"
                ),
                onProtocolSelected = { onTypeChange(it) }
            )
        }
        Column {
            TextField(
                value = server,
                onValueChange = { onServerChange(it) },
                label = stringResource(R.string.server_address_label),
                singleLine = true,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp)
                    .padding(bottom = 12.dp),
                keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
            )
        }
        Column {
            TextField(
                value = serverPort,
                onValueChange = { onServerPortChange(it) },
                label = stringResource(R.string.server_port_label),
                singleLine = true,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp)
                    .padding(bottom = 12.dp),
                keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                keyboardOptions = KeyboardOptions(
                    imeAction = ImeAction.Done,
                    keyboardType = KeyboardType.Number
                )
            )
        }

        // ==================== 2. 协议专属设置 ====================
        Column {
            SmallTitle(
                text = stringResource(R.string.protocol_specific_config, type.uppercase()),
                insideMargin = PaddingValues(
                    start = 26.dp,
                    top = 8.dp,
                    bottom = 8.dp,
                    end = 26.dp
                )
            )
        }
        when (type) {
            "vless" -> {
                Column {
                    TextField(
                        value = uuid,
                        onValueChange = { onUuidChange(it) },
                        label = stringResource(R.string.user_id),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
                    )
                }
                Column {
                    val flowOptions = listOf("none", "xtls-rprx-vision")
                    Card(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp)
                    ) {
                        OverlayDropdownPreference(
                            title = stringResource(R.string.flow_control),
                            items = flowOptions,
                            selectedIndex = flowOptions.indexOf(flow).coerceAtLeast(0),
                            onSelectedIndexChange = { index -> onFlowChange(flowOptions[index]) }
                        )
                    }
                }
            }

            "vmess" -> {
                Column {
                    TextField(
                        value = uuid,
                        onValueChange = { onUuidChange(it) },
                        label = stringResource(R.string.user_id),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
                    )
                }
                Column {
                    val vmessSecurities =
                        listOf("auto", "none", "zero", "aes-128-gcm", "chacha20-poly1305")
                    Card(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp)
                    ) {
                        OverlayDropdownPreference(
                            title = stringResource(R.string.security_label),
                            items = vmessSecurities,
                            selectedIndex = vmessSecurities.indexOf(security)
                                .coerceAtLeast(0),
                            onSelectedIndexChange = { index ->
                                onSecurityChange(vmessSecurities[index])
                            }
                        )
                    }
                }
                Column {
                    TextField(
                        value = alterId,
                        onValueChange = { onAlterIdChange(it) },
                        label = alterIdLabel,
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(
                            imeAction = ImeAction.Done,
                            keyboardType = KeyboardType.Number
                        )
                    )
                }
            }

            "shadowsocks" -> {
                Column {
                    val ssMethods = listOf(
                        "aes-128-gcm",
                        "aes-256-gcm",
                        "chacha20-ietf-poly1305",
                        "2022-blake3-aes-128-gcm",
                        "2022-blake3-aes-256-gcm",
                        "2022-blake3-chacha20-poly1305"
                    )
                    Card(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp)
                    ) {
                        OverlayDropdownPreference(
                            title = stringResource(R.string.method_label),
                            items = ssMethods,
                            selectedIndex = ssMethods.indexOf(method).coerceAtLeast(0),
                            onSelectedIndexChange = { index -> onMethodChange(ssMethods[index]) }
                        )
                    }
                }
                Column {
                    TextField(
                        value = password,
                        onValueChange = { onPasswordChange(it) },
                        label = stringResource(R.string.password_key_label),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
                    )
                }
                Column {
                    TextField(
                        value = plugin,
                        onValueChange = { onPluginChange(it) },
                        label = stringResource(R.string.plugin_label),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
                    )
                }
                Column {
                    TextField(
                        value = pluginOpts,
                        onValueChange = { onPluginOptsChange(it) },
                        label = stringResource(R.string.plugin_options),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
                    )
                }
            }

            "trojan", "anytls" -> {
                Column {
                    TextField(
                        value = password,
                        onValueChange = { onPasswordChange(it) },
                        label = stringResource(R.string.password_only),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
                    )
                }
            }

            "hysteria2" -> {
                Column {
                    TextField(
                        value = password,
                        onValueChange = { onPasswordChange(it) },
                        label = stringResource(R.string.password_label),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
                    )
                }
                Column {
                    TextField(
                        value = upMbps,
                        onValueChange = { onUpMbpsChange(it) },
                        label = stringResource(R.string.up_mbps),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(
                            imeAction = ImeAction.Done,
                            keyboardType = KeyboardType.Number
                        )
                    )
                }
                Column {
                    TextField(
                        value = downMbps,
                        onValueChange = { onDownMbpsChange(it) },
                        label = stringResource(R.string.down_mbps),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(
                            imeAction = ImeAction.Done,
                            keyboardType = KeyboardType.Number
                        )
                    )
                }
                Column {
                    val obfsTypeList = listOf("none", "salamander")
                    Card(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp)
                    ) {
                        OverlayDropdownPreference(
                            title = stringResource(R.string.obfs_type),
                            items = obfsTypeList,
                            selectedIndex = obfsTypeList.indexOf(obfsType).coerceAtLeast(0),
                            onSelectedIndexChange = { index ->
                                onObfsTypeChange(obfsTypeList[index])
                            }
                        )
                    }
                }
                if (obfsType != "none") {
                    Column {
                        TextField(
                            value = obfsPassword,
                            onValueChange = { onObfsPasswordChange(it) },
                            label = stringResource(R.string.obfs_password),
                            singleLine = true,
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 12.dp)
                                .padding(bottom = 12.dp),
                            keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
                        )
                    }
                }
                Column {
                    TextField(
                        value = serverPorts,
                        onValueChange = { onServerPortsChange(it) },
                        label = stringResource(R.string.server_ports_hop),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(
                            imeAction = ImeAction.Done,
                            keyboardType = KeyboardType.Number
                        )
                    )
                }
                Column {
                    TextField(
                        value = hopInterval,
                        onValueChange = { onHopIntervalChange(it) },
                        label = stringResource(R.string.hop_interval_seconds),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(
                            imeAction = ImeAction.Done,
                            keyboardType = KeyboardType.Number
                        )
                    )
                }
            }

            "tuic" -> {
                Column {
                    TextField(
                        value = uuid,
                        onValueChange = { onUuidChange(it) },
                        label = stringResource(R.string.user_id_uuid),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
                    )
                }
                Column {
                    TextField(
                        value = password,
                        onValueChange = { onPasswordChange(it) },
                        label = stringResource(R.string.password_label),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
                    )
                }
                Column {
                    val ccOptions = listOf("cubic", "bbr", "new_reno")
                    val urmOptions = listOf("quic", "native")
                    Card(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp)
                    ) {
                        OverlayDropdownPreference(
                            title = stringResource(R.string.congestion_control),
                            items = ccOptions,
                            selectedIndex = ccOptions.indexOf(congestionControl)
                                .coerceAtLeast(0),
                            onSelectedIndexChange = { index ->
                                onCongestionControlChange(ccOptions[index])
                            }
                        )
                        OverlayDropdownPreference(
                            title = udpRelayModeTitle,
                            items = urmOptions,
                            selectedIndex = urmOptions.indexOf(udpRelayMode)
                                .coerceAtLeast(0),
                            onSelectedIndexChange = { index ->
                                onUdpRelayModeChange(urmOptions[index])
                            }
                        )
                        SwitchPreference(
                            title = stringResource(R.string.udp_over_stream),
                            checked = udpOverStream,
                            onCheckedChange = { onUdpOverStreamChange(it) }
                        )
                        SwitchPreference(
                            title = stringResource(R.string.zero_rtt_handshake),
                            checked = zeroRttHandshake,
                            onCheckedChange = { onZeroRttHandshakeChange(it) }
                        )
                    }
                }
                Column {
                    TextField(
                        value = heartbeat,
                        onValueChange = { onHeartbeatChange(it) },
                        label = stringResource(R.string.heartbeat_interval_seconds),
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp)
                            .padding(bottom = 12.dp),
                        keyboardActions = KeyboardActions(onDone = { focusManager.clearFocus() }),
                        keyboardOptions = KeyboardOptions(
                            imeAction = ImeAction.Done,
                            keyboardType = KeyboardType.Number
                        )
                    )
                }
            }
        }


    }
}


