package com.fanjv.netproxy.feature.settings.data

import com.fanjv.netproxy.core.command.CommandFileStore
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.command.ShellConfigFile
import com.fanjv.netproxy.feature.settings.model.ManagedConfigDocument
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** 模块与 sing-box 配置的事务读取、校验和写入入口。 */
internal class ConfigRepository(
    private val client: NetProxyCtlClient,
    private val commandFiles: CommandFileStore
) {
    private val updateMutex = Mutex()

    suspend fun listDocuments(): List<ManagedConfigDocument> =
        client.json.decodeFromJsonElement(client.execute("config", "list").data)

    suspend fun read(target: String): String =
        client.execute("config", "read", target).data.jsonObject["content"]
            ?.jsonPrimitive?.content
            ?: error("模块没有返回配置内容")

    suspend fun updateValue(
        target: String,
        key: String,
        value: String,
        forceQuotes: Boolean = false
    ) = updateMutex.withLock {
        apply(target, ShellConfigFile.updateValue(read(target), key, value, forceQuotes))
    }

    suspend fun apply(target: String, content: String) =
        commandFiles.withTextFile("netproxy-config-", ".conf", content) { source ->
            client.execute("config", "apply", target, source.absolutePath)
        }

    suspend fun check() {
        client.execute("config", "check")
    }
}
