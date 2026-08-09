package com.fanjv.netproxy.feature.settings.presentation

import androidx.compose.runtime.Immutable

@Immutable
data class ProxySettings(
    val network: String = "",
    val dnsMode: String = "hijack",
    val cgroupEnabled: Boolean = true,
    val cgroupIpv6Mode: String = "always",
    val bypassPrivateAddress: Boolean = true,
    val bypassRuleSets: String = "direct ChinaIP",
    val sharedNetworkEnabled: Boolean = false,
    val sharedInterfaces: String = "wlan2",
    val sharedIncludeSourceCidrs: String = "",
    val sharedExcludeSourceCidrs: String = "",
    val sharedIncludeMacAddresses: String = "",
    val sharedExcludeMacAddresses: String = "",
    val wifiAutoSwitch: Boolean = false,
    val wifiSsidMode: String = "blacklist",
    val wifiSsidList: String = "",
    val proxyOnCellular: Boolean = true
)

@Immutable
data class SettingsUiState(
    val autoStartEnabled: Boolean = false,
    val proxySettings: ProxySettings = ProxySettings(),
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val isDiagnosingEbpf: Boolean = false,
    val ebpfDiagnostic: String? = null,
    val error: String = ""
)
