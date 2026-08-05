package com.fanjv.netproxy.feature.apps.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
internal data class AppProxyConfig(
    val enabled: Boolean = true,
    val mode: String = "blacklist",
    @SerialName("android_users") val androidUsers: String = "",
    @SerialName("proxy_apps") val proxyApps: String = "",
    @SerialName("bypass_apps") val bypassApps: String = ""
)
