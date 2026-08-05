package com.fanjv.netproxy.feature.settings.presentation

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.DropdownImpl
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.ListPopupColumn
import top.yukonga.miuix.kmp.basic.ListPopupDefaults
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.PopupPositionProvider
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Refresh
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.overlay.OverlayListPopup
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

/** eBPF 透明代理设置页。 */
@Composable
internal fun ProxySettingsScreen(
    onBack: () -> Unit,
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
    viewModel: SettingsViewModel = netProxyViewModel()
) {
    val settingsUi by viewModel.state.collectAsStateWithLifecycle()
    val settings = settingsUi.proxySettings
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface

    val showTextEditDialog = remember { mutableStateOf(false) }
    var editingKey by remember { mutableStateOf("") }
    var editingLabel by remember { mutableStateOf("") }
    var editingValue by remember { mutableStateOf("") }
    var editingModuleValue by remember { mutableStateOf(false) }

    fun editValue(key: String, label: String, value: String, moduleValue: Boolean = false) {
        editingKey = key
        editingLabel = label
        editingValue = value
        editingModuleValue = moduleValue
        showTextEditDialog.value = true
    }

    LaunchedEffect(Unit) { viewModel.refresh() }

    Scaffold(
        topBar = {
            BlurredBar(backdrop) {
                AdaptiveTopAppBar(
                    color = barColor,
                    title = stringResource(R.string.proxy_settings),
                    scrollBehavior = scrollBehavior,
                    navigationIcon = {
                        IconButton(onClick = onBack) {
                            Icon(
                                imageVector = Icons.AutoMirrored.Rounded.ArrowBack,
                                contentDescription = null,
                                tint = colorScheme.onSurface
                            )
                        }
                    },
                    actions = {
                        val showRestartPopup = remember { mutableStateOf(false) }
                        OverlayListPopup(
                            show = showRestartPopup.value,
                            popupPositionProvider = ListPopupDefaults.ContextMenuPositionProvider,
                            alignment = PopupPositionProvider.Align.TopEnd,
                            onDismissRequest = { showRestartPopup.value = false }
                        ) {
                            ListPopupColumn {
                                DropdownImpl(
                                    text = stringResource(R.string.ebpf_restart_service),
                                    isSelected = false,
                                    optionSize = 1,
                                    index = 0,
                                    onSelectedIndexChange = {
                                        showRestartPopup.value = false
                                        viewModel.restartService()
                                    }
                                )
                            }
                        }
                        IconButton(
                            onClick = { showRestartPopup.value = true },
                            holdDownState = showRestartPopup.value
                        ) {
                            Icon(
                                imageVector = MiuixIcons.Refresh,
                                contentDescription = stringResource(R.string.ebpf_restart_service),
                                tint = colorScheme.onSurface
                            )
                        }
                    }
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.only(WindowInsetsSides.Horizontal)
    ) { innerPadding ->
        Box(modifier = if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier) {
            LazyColumn(
                modifier = Modifier
                    .fillMaxHeight()
                    .scrollEndHaptic()
                    .overScrollVertical()
                    .nestedScroll(scrollBehavior.nestedScrollConnection)
                    .padding(horizontal = 12.dp),
                contentPadding = innerPadding,
                overscrollEffect = null
            ) {
                groupedCardSection(
                    keyPrefix = "ebpf_core",
                    title = { stringResource(R.string.ebpf_core_settings) },
                    items = listOf(
                        CardItem("network") {
                            val labels = listOf(
                                stringResource(R.string.ebpf_network_all),
                                stringResource(R.string.ebpf_network_tcp),
                                stringResource(R.string.ebpf_network_udp)
                            )
                            val values = listOf("", "tcp", "udp")
                            OverlayDropdownPreference(
                                title = stringResource(R.string.ebpf_network),
                                items = labels,
                                selectedIndex = values.indexOf(settings.network).coerceAtLeast(0),
                                onSelectedIndexChange = {
                                    viewModel.updateProxySetting("EBPF_NETWORK", values[it])
                                }
                            )
                        },
                        CardItem("dns") {
                            SwitchPreference(
                                title = stringResource(R.string.ebpf_dns_hijack),
                                summary = stringResource(R.string.ebpf_dns_hijack_summary),
                                checked = settings.dnsMode == "hijack",
                                onCheckedChange = { viewModel.setDnsHijackEnabled(it) }
                            )
                        },
                        CardItem("ipv6") {
                            SwitchPreference(
                                title = stringResource(R.string.ebpf_ipv6),
                                summary = stringResource(R.string.ebpf_ipv6_summary),
                                checked = settings.ipv6Enabled,
                                onCheckedChange = {
                                    viewModel.updateProxySetting("EBPF_IPV6", if (it) "1" else "0")
                                }
                            )
                        },
                        CardItem("udp_timeout") {
                            val label = stringResource(R.string.ebpf_udp_timeout)
                            ArrowPreference(
                                title = label,
                                summary = settings.udpTimeout,
                                onClick = {
                                    editValue("EBPF_UDP_TIMEOUT", label, settings.udpTimeout)
                                }
                            )
                        },
                        CardItem("cgroup") {
                            val label = stringResource(R.string.ebpf_cgroup_path)
                            ArrowPreference(
                                title = label,
                                summary = settings.cgroupPath.ifBlank {
                                    stringResource(R.string.ebpf_cgroup_auto)
                                },
                                onClick = {
                                    editValue("EBPF_CGROUP_PATH", label, settings.cgroupPath)
                                }
                            )
                        }
                    )
                )

                groupedCardSection(
                    keyPrefix = "ebpf_bypass",
                    title = { stringResource(R.string.ebpf_bypass_settings) },
                    items = listOf(
                        CardItem("rule_sets") {
                            val label = stringResource(R.string.ebpf_bypass_rule_sets)
                            ArrowPreference(
                                title = label,
                                summary = settings.bypassRuleSets.ifBlank {
                                    stringResource(R.string.not_set)
                                },
                                onClick = {
                                    editValue("EBPF_BYPASS_RULE_SETS", label, settings.bypassRuleSets)
                                }
                            )
                        }
                    )
                )

                groupedCardSection(
                    keyPrefix = "ebpf_shared",
                    title = { stringResource(R.string.ebpf_shared_settings) },
                    items = listOf(
                        CardItem("enabled") {
                            SwitchPreference(
                                title = stringResource(R.string.ebpf_shared_enable),
                                summary = stringResource(R.string.ebpf_shared_enable_summary),
                                checked = settings.sharedNetworkEnabled,
                                onCheckedChange = {
                                    viewModel.updateProxySetting(
                                        "EBPF_SHARED_NETWORK",
                                        if (it) "1" else "0"
                                    )
                                }
                            )
                        },
                        CardItem("interfaces") {
                            val label = stringResource(R.string.ebpf_shared_interfaces)
                            ArrowPreference(
                                title = label,
                                summary = settings.sharedInterfaces,
                                onClick = {
                                    editValue(
                                        "EBPF_SHARED_INTERFACES",
                                        label,
                                        settings.sharedInterfaces
                                    )
                                }
                            )
                        }
                    )
                )

                groupedCardSection(
                    keyPrefix = "ebpf_maps",
                    title = { stringResource(R.string.ebpf_map_settings) },
                    items = listOf(
                        CardItem("tcp") {
                            val label = stringResource(R.string.ebpf_tcp_map)
                            ArrowPreference(
                                title = label,
                                summary = settings.tcpMapCapacity,
                                onClick = {
                                    editValue("EBPF_TCP_MAP_CAPACITY", label, settings.tcpMapCapacity)
                                }
                            )
                        },
                        CardItem("udp") {
                            val label = stringResource(R.string.ebpf_udp_map)
                            ArrowPreference(
                                title = label,
                                summary = settings.udpMapCapacity,
                                onClick = {
                                    editValue("EBPF_UDP_MAP_CAPACITY", label, settings.udpMapCapacity)
                                }
                            )
                        },
                        CardItem("socket") {
                            val label = stringResource(R.string.ebpf_socket_map)
                            ArrowPreference(
                                title = label,
                                summary = settings.socketMapCapacity,
                                onClick = {
                                    editValue(
                                        "EBPF_SOCKET_MAP_CAPACITY",
                                        label,
                                        settings.socketMapCapacity
                                    )
                                }
                            )
                        },
                        CardItem("shared") {
                            val label = stringResource(R.string.ebpf_shared_map)
                            ArrowPreference(
                                title = label,
                                summary = settings.sharedMapCapacity,
                                onClick = {
                                    editValue(
                                        "EBPF_SHARED_MAP_CAPACITY",
                                        label,
                                        settings.sharedMapCapacity
                                    )
                                }
                            )
                        }
                    )
                )

                groupedCardSection(
                    keyPrefix = "wifi_auto_switch",
                    title = { stringResource(R.string.wifi_auto_switch_title) },
                    items = listOf(
                        CardItem("enabled") {
                            SwitchPreference(
                                title = stringResource(R.string.wifi_auto_switch),
                                summary = stringResource(R.string.wifi_auto_switch_summary),
                                checked = settings.wifiAutoSwitch,
                                onCheckedChange = viewModel::setWifiAutoSwitch
                            )
                        },
                        CardItem("mode") {
                            val modes = listOf("blacklist", "whitelist")
                            OverlayDropdownPreference(
                                title = stringResource(R.string.wifi_ssid_mode),
                                items = listOf(
                                    stringResource(R.string.wifi_ssid_mode_blacklist),
                                    stringResource(R.string.wifi_ssid_mode_whitelist)
                                ),
                                selectedIndex = modes.indexOf(settings.wifiSsidMode).coerceAtLeast(0),
                                onSelectedIndexChange = { viewModel.setWifiSsidMode(modes[it]) }
                            )
                        },
                        CardItem("ssids") {
                            val label = stringResource(R.string.wifi_ssid_list)
                            ArrowPreference(
                                title = label,
                                summary = settings.wifiSsidList.ifBlank {
                                    stringResource(R.string.not_set)
                                },
                                onClick = {
                                    editValue(
                                        "WIFI_SSID_LIST",
                                        label,
                                        settings.wifiSsidList,
                                        moduleValue = true
                                    )
                                }
                            )
                        },
                        CardItem("cellular") {
                            SwitchPreference(
                                title = stringResource(R.string.proxy_on_cellular),
                                summary = stringResource(R.string.proxy_on_cellular_summary),
                                checked = settings.proxyOnCellular,
                                onCheckedChange = viewModel::setProxyOnCellular
                            )
                        }
                    )
                )

                item { Spacer(Modifier.height(80.dp + bottomPadding)) }
            }
        }
    }

    OverlayDialog(
        show = showTextEditDialog.value,
        insideMargin = DpSize(0.dp, 0.dp),
        onDismissRequest = { showTextEditDialog.value = false }
    ) {
        var value by remember(editingValue) { mutableStateOf(editingValue) }
        Column {
            Text(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 24.dp, bottom = 12.dp),
                text = stringResource(R.string.modify_label, editingLabel),
                fontSize = MiuixTheme.textStyles.title4.fontSize,
                fontWeight = FontWeight.Medium,
                textAlign = TextAlign.Center,
                color = colorScheme.onSurface
            )
            Column(
                modifier = Modifier.padding(horizontal = 24.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                TextField(
                    value = value,
                    onValueChange = { value = it },
                    label = stringResource(R.string.value_label),
                    modifier = Modifier.fillMaxWidth()
                )
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 12.dp, bottom = 24.dp),
                    horizontalArrangement = Arrangement.spacedBy(12.dp)
                ) {
                    TextButton(
                        text = stringResource(android.R.string.cancel),
                        onClick = { showTextEditDialog.value = false },
                        modifier = Modifier.weight(1f)
                    )
                    TextButton(
                        text = stringResource(R.string.save_text),
                        onClick = {
                            if (editingModuleValue && editingKey == "WIFI_SSID_LIST") {
                                viewModel.setWifiSsidList(value)
                            } else {
                                viewModel.updateProxySetting(editingKey, value)
                            }
                            showTextEditDialog.value = false
                        },
                        modifier = Modifier.weight(1f),
                        colors = ButtonDefaults.textButtonColorsPrimary()
                    )
                }
            }
        }
    }
}
