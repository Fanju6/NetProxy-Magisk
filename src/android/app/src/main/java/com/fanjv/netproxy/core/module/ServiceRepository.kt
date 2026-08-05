package com.fanjv.netproxy.core.module

import com.fanjv.netproxy.core.command.NetProxyCtlClient
import kotlinx.serialization.json.decodeFromJsonElement

/** 代理服务生命周期、模式与运行状态的数据入口。 */
internal class ServiceRepository(
    private val client: NetProxyCtlClient
) {
    suspend fun status(): ServiceStatusSnapshot =
        client.json.decodeFromJsonElement(client.execute("service", "status").data)

    suspend fun action(action: String) {
        require(action in setOf("start", "stop", "restart", "reload"))
        client.execute("service", action)
    }

    suspend fun setMode(mode: String) {
        require(mode in setOf("rule", "global", "direct", "AllowAds"))
        client.execute("mode", mode)
    }
}
