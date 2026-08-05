package com.fanjv.netproxy.feature.apps.data

import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.feature.apps.model.AppProxyConfig
import kotlinx.serialization.json.decodeFromJsonElement

/** 分应用代理策略的数据入口。 */
internal class AppPolicyRepository(
    private val client: NetProxyCtlClient
) {
    suspend fun config(): AppProxyConfig =
        client.json.decodeFromJsonElement(client.execute("app", "list").data)

    suspend fun setMode(mode: String) {
        client.execute("app", "mode", mode)
    }

    suspend fun add(id: String) {
        client.execute("app", "add", id)
    }

    suspend fun remove(id: String) {
        client.execute("app", "remove", id)
    }

    suspend fun setEnabled(enabled: Boolean) {
        client.execute("app", if (enabled) "enable" else "disable")
    }
}
