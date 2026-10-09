package com.fanjv.netproxy.feature.settings.presentation

import androidx.compose.runtime.Immutable

@Immutable
data class WifiPolicySettings(
    val enabled: Boolean = false,
    val mode: String = "blacklist",
    val blacklist: List<String> = emptyList(),
    val whitelist: List<String> = emptyList(),
    val proxyOnNonWifi: Boolean = true
) {
    val selection get() = if (enabled) mode else "off"
    val ssids get() = if (mode == "whitelist") whitelist else blacklist
    fun withSsids(values: List<String>) = if (mode == "whitelist") copy(whitelist = values) else copy(blacklist = values)
}

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
