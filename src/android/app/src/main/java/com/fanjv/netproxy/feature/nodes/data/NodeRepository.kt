package com.fanjv.netproxy.feature.nodes.data

import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.feature.catalog.model.CatalogNodesSnapshot
import com.fanjv.netproxy.feature.catalog.model.NodeDelayResult
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

@Serializable
internal data class ExportedNodeLink(
    val tag: String,
    val protocol: String,
    val link: String
)

/** 节点 Catalog、选择与测速的数据入口。 */
internal class NodeRepository(
    private val client: NetProxyCtlClient
) {
    suspend fun snapshot(): CatalogNodesSnapshot =
        client.json.decodeFromJsonElement(client.execute("node", "snapshot").data)

    suspend fun add(link: String) {
        client.execute("node", "add", link)
    }

    suspend fun import(filePath: String, groupName: String = ""): String {
        val args = mutableListOf("node", "import", filePath)
        groupName.takeIf(String::isNotBlank)?.let(args::add)
        return client.execute(*args.toTypedArray()).data
            .let { element -> element.jsonObject["group_id"]?.jsonPrimitive?.content.orEmpty() }
    }

    suspend fun edit(nodeRef: String, source: String) {
        client.execute("node", "edit", nodeRef, source)
    }

    suspend fun export(nodeRef: String): ExportedNodeLink =
        client.json.decodeFromJsonElement(client.execute("node", "export", nodeRef).data)

    suspend fun remove(nodeRef: String) {
        client.execute("node", "remove", nodeRef)
    }

    suspend fun select(nodeRef: String) {
        client.execute("node", "use", nodeRef)
    }

    suspend fun selectAuto(groupId: String) {
        client.execute("node", "use", "auto", groupId)
    }

    suspend fun testDelay(target: String = "", groupId: String = ""): NodeDelayResult {
        val args = mutableListOf("node", "delay")
        target.takeIf(String::isNotBlank)?.let(args::add)
        groupId.takeIf(String::isNotBlank)?.let(args::add)
        return client.json.decodeFromJsonElement(client.execute(*args.toTypedArray()).data)
    }
}
