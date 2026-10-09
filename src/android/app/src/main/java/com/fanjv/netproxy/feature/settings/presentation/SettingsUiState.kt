package com.fanjv.netproxy.feature.settings.presentation

import androidx.compose.runtime.Immutable

@Immutable
data class WifiPolicySettings(
    val enabled: Boolean = false,
    val mode: String = "blacklist",
    val ssids: String = "",
    val proxyOnCellular: Boolean = true
)

@Immutable
data class SettingsUiState(
    val hasLoaded: Boolean = false,
    val autoStartEnabled: Boolean = false,
    val wifi: WifiPolicySettings = WifiPolicySettings(),
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val hasPendingWifi: Boolean = false,
    val requiresReload: Boolean = false,
    val error: String = ""
)
