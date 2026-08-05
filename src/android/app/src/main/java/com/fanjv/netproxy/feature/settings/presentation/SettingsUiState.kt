package com.fanjv.netproxy.feature.settings.presentation

import androidx.compose.runtime.Immutable

@Immutable
data class ProxySettings(
    val network: String = "",
    val udpTimeout: String = "5m",
    val dnsMode: String = "hijack",
    val cgroupEnabled: Boolean = true,
    val cgroupPath: String = "",
    val ipv6Mode: String = "auto",
    val bypassRuleSets: String = "direct ChinaIP",
    val sharedNetworkEnabled: Boolean = false,
    val sharedInterfaces: String = "wlan2",
    val sharedIncludeSourceCidrs: String = "",
    val sharedExcludeSourceCidrs: String = "",
    val sharedTcPriority: String = "1",
    val tcpMapCapacity: String = "65536",
    val udpMapCapacity: String = "65536",
    val socketMapCapacity: String = "65536",
    val sharedMapCapacity: String = "65536",
    val wifiAutoSwitch: Boolean = false,
    val wifiSsidMode: String = "blacklist",
    val wifiSsidList: String = "",
    val proxyOnCellular: Boolean = true
)

@Immutable
data class SettingsUiState(
    val autoStartEnabled: Boolean = false,
    val gmsFixEnabled: Boolean = false,
    val proxySettings: ProxySettings = ProxySettings(),
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val isDiagnosingEbpf: Boolean = false,
    val ebpfDiagnostic: String? = null,
    val error: String = ""
)
