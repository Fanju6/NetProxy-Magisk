package com.fanjv.netproxy.feature.settings.model

import androidx.compose.runtime.Immutable
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.Transient

@Immutable
@Serializable
data class WifiPolicySettings(
    val enabled: Boolean = false,
    val mode: String = "blacklist",
    val blacklist: List<String> = emptyList(),
    val whitelist: List<String> = emptyList(),
    @SerialName("proxy_on_non_wifi") val proxyOnNonWifi: Boolean = true
) {
    init { require(mode in setOf("blacklist", "whitelist")) }
    val selection get() = if (enabled) mode else "off"
    val ssids get() = if (mode == "whitelist") whitelist else blacklist
    fun withSsids(values: List<String>) = if (mode == "whitelist") copy(whitelist = values) else copy(blacklist = values)
}

@Serializable
internal data class ModuleWifiConfig(
    val wifi: WifiPolicySettings = WifiPolicySettings(),
    @Transient val revision: String = ""
)

@Serializable
internal data class ModuleAutoStartConfig(
    @SerialName("auto_start") val enabled: Boolean = false,
    @Transient val revision: String = ""
)
