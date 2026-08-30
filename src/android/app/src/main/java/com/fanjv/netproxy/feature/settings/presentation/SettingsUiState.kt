package com.fanjv.netproxy.feature.settings.presentation

import androidx.compose.runtime.Immutable

@Immutable
data class ProxySettings(
    val mode: String = "local",
    val network: String = "",
    val localDnsMode: String = "hijack",
    val sharedDnsMode: String = "hijack",
    val localIpv6: Boolean = true,
    val sharedIpv6: Boolean = true,
    val localBypassPrivateAddress: Boolean = true,
    val sharedBypassPrivateAddress: Boolean = true,
    val bypassRuleSet: String = "direct,cn-ip",
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
