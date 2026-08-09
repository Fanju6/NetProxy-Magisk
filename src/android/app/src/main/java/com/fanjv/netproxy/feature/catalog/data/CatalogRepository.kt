package com.fanjv.netproxy.feature.catalog.data

import com.fanjv.netproxy.core.command.CommandFileStore
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.command.NetProxyCtlResponse
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.decodeFromJsonElement
import java.io.File

/** Catalog 数据层共享的命令执行、JSON 解码与短期文件能力。 */
internal class CatalogRepository(
    private val client: NetProxyCtlClient,
    private val commandFiles: CommandFileStore
) {
    internal val json: Json
        get() = client.json

    internal suspend fun execute(vararg args: String): NetProxyCtlResponse =
        client.execute(*args)

    internal suspend inline fun <reified T> decode(vararg args: String): T =
        client.json.decodeFromJsonElement(execute(*args).data)

    internal suspend fun <T> withTextFile(
        prefix: String,
        suffix: String,
        content: String,
        block: suspend (File) -> T
    ): T = commandFiles.withTextFile(prefix, suffix, content, block)
}
