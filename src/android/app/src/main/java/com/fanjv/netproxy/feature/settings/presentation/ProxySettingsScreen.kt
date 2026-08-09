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
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
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
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.overlay.OverlayDialog
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

    LaunchedEffect(Unit) { viewModel.ensureLoaded() }

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
                        var showMoreMenu by remember { mutableStateOf(false) }
                        TopBarMoreMenu(
                            expanded = showMoreMenu,
                            onExpandedChange = { showMoreMenu = it },
                            actions = listOf(
                                TopBarMenuAction(
                                    text = stringResource(R.string.ebpf_restart_service),
                                    onClick = viewModel::restartService
                                ),
                                TopBarMenuAction(
                                    text = stringResource(R.string.ebpf_diagnostics),
                                    enabled = !settingsUi.isDiagnosingEbpf,
                                    onClick = viewModel::diagnoseEbpf
                                )
                            ),
                            contentDescription = stringResource(R.string.more_actions)
                        )
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
                                    viewModel.setNetwork(values[it])
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
                        CardItem("cgroup_enabled") {
                            SwitchPreference(
                                title = stringResource(R.string.ebpf_cgroup_enable),
                                summary = stringResource(R.string.ebpf_cgroup_enable_summary),
                                checked = settings.cgroupEnabled,
                                onCheckedChange = viewModel::setCgroupEnabled
                            )
                        },
                        CardItem("ipv6_mode") {
                            val values = listOf("always", "auto", "off")
                            OverlayDropdownPreference(
                                title = stringResource(R.string.ebpf_ipv6_mode),
                                items = listOf(
                                    stringResource(R.string.ebpf_ipv6_mode_always),
                                    stringResource(R.string.ebpf_ipv6_mode_auto),
                                    stringResource(R.string.ebpf_ipv6_mode_off)
                                ),
                                selectedIndex = values.indexOf(settings.cgroupIpv6Mode)
                                    .coerceAtLeast(0),
                                onSelectedIndexChange = {
                                    viewModel.updateProxySetting(
                                        "EBPF_CGROUP_IPV6_MODE",
                                        values[it]
                                    )
                                }
                            )
                        },
                    )
                )

                groupedCardSection(
                    keyPrefix = "ebpf_bypass",
                    title = { stringResource(R.string.ebpf_bypass_settings) },
                    items = listOf(
                        CardItem("private_address") {
                            SwitchPreference(
                                title = stringResource(R.string.ebpf_bypass_private_address),
                                summary = stringResource(R.string.ebpf_bypass_private_address_summary),
                                checked = settings.bypassPrivateAddress,
                                onCheckedChange = {
                                    viewModel.updateProxySetting(
                                        "EBPF_BYPASS_PRIVATE_ADDRESS",
                                        if (it) "1" else "0"
                                    )
                                }
                            )
                        },
                        CardItem("rule_sets") {
                            val label = stringResource(R.string.ebpf_bypass_rule_sets)
                            ArrowPreference(
                                title = label,
                                summary = settings.bypassRuleSets.ifBlank {
                                    stringResource(R.string.not_set)
                                },
                                onClick = {
                                    editValue(
                                        "EBPF_BYPASS_RULE_SETS",
                                        label,
                                        settings.bypassRuleSets
                                    )
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
                                onCheckedChange = viewModel::setSharedNetworkEnabled
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
                        },
                        CardItem("include_source_cidrs") {
                            val label = stringResource(R.string.ebpf_shared_include_source_cidrs)
                            ArrowPreference(
                                title = label,
                                summary = settings.sharedIncludeSourceCidrs.ifBlank {
                                    stringResource(R.string.not_set)
                                },
                                onClick = {
                                    editValue(
                                        "EBPF_SHARED_INCLUDE_SOURCE_CIDRS",
                                        label,
                                        settings.sharedIncludeSourceCidrs
                                    )
                                }
                            )
                        },
                        CardItem("exclude_source_cidrs") {
                            val label = stringResource(R.string.ebpf_shared_exclude_source_cidrs)
                            ArrowPreference(
                                title = label,
                                summary = settings.sharedExcludeSourceCidrs.ifBlank {
                                    stringResource(R.string.not_set)
                                },
                                onClick = {
                                    editValue(
                                        "EBPF_SHARED_EXCLUDE_SOURCE_CIDRS",
                                        label,
                                        settings.sharedExcludeSourceCidrs
                                    )
                                }
                            )
                        },
                        CardItem("include_mac_addresses") {
                            val label = stringResource(R.string.ebpf_shared_include_mac_addresses)
                            ArrowPreference(
                                title = label,
                                summary = settings.sharedIncludeMacAddresses.ifBlank {
                                    stringResource(R.string.not_set)
                                },
                                onClick = {
                                    editValue(
                                        "EBPF_SHARED_INCLUDE_MAC_ADDRESSES",
                                        label,
                                        settings.sharedIncludeMacAddresses
                                    )
                                }
                            )
                        },
                        CardItem("exclude_mac_addresses") {
                            val label = stringResource(R.string.ebpf_shared_exclude_mac_addresses)
                            ArrowPreference(
                                title = label,
                                summary = settings.sharedExcludeMacAddresses.ifBlank {
                                    stringResource(R.string.not_set)
                                },
                                onClick = {
                                    editValue(
                                        "EBPF_SHARED_EXCLUDE_MAC_ADDRESSES",
                                        label,
                                        settings.sharedExcludeMacAddresses
                                    )
                                }
                            )
                        },
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
                                selectedIndex = modes.indexOf(settings.wifiSsidMode)
                                    .coerceAtLeast(0),
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
        val usesListHint = editingKey in listOf(
            "EBPF_BYPASS_RULE_SETS",
            "EBPF_SHARED_INTERFACES",
            "EBPF_SHARED_INCLUDE_SOURCE_CIDRS",
            "EBPF_SHARED_EXCLUDE_SOURCE_CIDRS",
            "EBPF_SHARED_INCLUDE_MAC_ADDRESSES",
            "EBPF_SHARED_EXCLUDE_MAC_ADDRESSES",
            "WIFI_SSID_LIST"
        )
        val valueHint = when (editingKey) {
            "EBPF_BYPASS_RULE_SETS" -> stringResource(R.string.settings_hint_rule_sets)
            "EBPF_SHARED_INTERFACES" -> stringResource(R.string.settings_hint_interfaces)
            "EBPF_SHARED_INCLUDE_SOURCE_CIDRS",
            "EBPF_SHARED_EXCLUDE_SOURCE_CIDRS" -> stringResource(R.string.settings_hint_cidrs)

            "EBPF_SHARED_INCLUDE_MAC_ADDRESSES",
            "EBPF_SHARED_EXCLUDE_MAC_ADDRESSES" -> stringResource(R.string.settings_hint_macs)

            "WIFI_SSID_LIST" -> stringResource(R.string.settings_hint_ssids)
            else -> stringResource(R.string.value_label)
        }
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
                    label = valueHint,
                    useLabelAsPlaceholder = usesListHint,
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

    OverlayDialog(
        show = settingsUi.ebpfDiagnostic != null,
        insideMargin = DpSize(0.dp, 0.dp),
        onDismissRequest = viewModel::dismissEbpfDiagnostic
    ) {
        Column(modifier = Modifier.padding(24.dp)) {
            Text(
                text = stringResource(R.string.ebpf_diagnostics),
                modifier = Modifier.fillMaxWidth(),
                fontSize = MiuixTheme.textStyles.title4.fontSize,
                fontWeight = FontWeight.Medium,
                textAlign = TextAlign.Center,
                color = colorScheme.onSurface
            )
            Text(
                text = settingsUi.ebpfDiagnostic.orEmpty(),
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 480.dp)
                    .verticalScroll(rememberScrollState())
                    .padding(vertical = 20.dp),
                color = colorScheme.onSurfaceVariantSummary,
                fontFamily = FontFamily.Monospace,
                fontSize = 12.sp,
                lineHeight = 17.sp
            )
            TextButton(
                text = stringResource(android.R.string.ok),
                onClick = viewModel::dismissEbpfDiagnostic,
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.textButtonColorsPrimary()
            )
        }
    }
}
