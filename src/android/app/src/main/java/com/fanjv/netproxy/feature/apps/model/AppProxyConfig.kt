package com.fanjv.netproxy.feature.apps.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.Transient

@Serializable
internal data class AppProxyConfig(
    val enabled: Boolean = true,
    val mode: String = "blacklist",
    @SerialName("proxy_apps") val proxyApps: List<String> = emptyList(),
    @SerialName("bypass_apps") val bypassApps: List<String> = emptyList(),
    @Transient val revision: String = "",
)
