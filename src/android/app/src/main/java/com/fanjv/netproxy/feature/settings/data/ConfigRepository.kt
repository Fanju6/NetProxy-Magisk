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

internal data class ConfigValueUpdate(
    val key: String,
    val value: String,
    val forceQuotes: Boolean = false
)

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
    ) = updateValues(target, listOf(ConfigValueUpdate(key, value, forceQuotes)))

    suspend fun updateValues(target: String, updates: List<ConfigValueUpdate>) =
        updateMutex.withLock {
            val content = updates.fold(read(target)) { current, update ->
                ShellConfigFile.updateValue(
                    current,
                    update.key,
                    update.value,
                    update.forceQuotes
                )
            }
            apply(target, content)
        }

    suspend fun apply(target: String, content: String) =
        commandFiles.withTextFile("netproxy-config-", ".conf", content) { source ->
            client.execute("config", "apply", target, source.absolutePath)
        }

    suspend fun check() {
        client.execute("config", "check")
    }

    suspend fun ebpfStatus(mode: String = "configured"): String =
        client.execute("ebpf", "status", mode).data.jsonObject["content"]
            ?.jsonPrimitive?.content
            ?: error("模块没有返回 eBPF 诊断结果")
}
