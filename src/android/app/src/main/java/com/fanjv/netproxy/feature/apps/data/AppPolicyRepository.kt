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

    suspend fun setMode(mode: String): AppProxyConfig =
        executeConfig("app", "mode", mode)

    suspend fun setUsers(userIds: Collection<String>): AppProxyConfig =
        if (userIds.isEmpty()) {
            executeConfig("app", "users", "all")
        } else {
            executeConfig("app", "users", *userIds.toTypedArray())
        }

    suspend fun add(id: String): AppProxyConfig =
        executeConfig("app", "add", id)

    suspend fun remove(id: String): AppProxyConfig =
        executeConfig("app", "remove", id)

    suspend fun setEnabled(enabled: Boolean): AppProxyConfig =
        executeConfig("app", if (enabled) "enable" else "disable")

    private suspend fun executeConfig(vararg args: String): AppProxyConfig =
        client.json.decodeFromJsonElement(client.execute(*args).data)
}
