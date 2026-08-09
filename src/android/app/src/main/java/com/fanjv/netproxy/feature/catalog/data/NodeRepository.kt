package com.fanjv.netproxy.feature.catalog.data

import com.fanjv.netproxy.feature.catalog.model.CatalogNodesSnapshot
import com.fanjv.netproxy.feature.catalog.model.NodeDelayResult
import kotlinx.serialization.Serializable
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
    private val catalog: CatalogRepository
) {
    suspend fun snapshot(): CatalogNodesSnapshot =
        catalog.decode("node", "snapshot")

    suspend fun add(link: String) {
        catalog.execute("node", "add", link)
    }

    suspend fun import(filePath: String, groupName: String = ""): String {
        val args = mutableListOf("node", "import", filePath)
        groupName.takeIf(String::isNotBlank)?.let(args::add)
        return catalog.execute(*args.toTypedArray()).data
            .let { element -> element.jsonObject["group_id"]?.jsonPrimitive?.content.orEmpty() }
    }

    suspend fun edit(nodeRef: String, source: String) {
        catalog.execute("node", "edit", nodeRef, source)
    }

    suspend fun get(nodeRef: String): String =
        catalog.execute("node", "get", nodeRef).data.toString()

    suspend fun editJson(nodeRef: String, content: String) {
        catalog.withTextFile("netproxy-node-", ".json", content) { source ->
            catalog.execute("node", "edit", nodeRef, source.absolutePath)
        }
    }

    suspend fun export(nodeRef: String): ExportedNodeLink =
        catalog.decode("node", "export", nodeRef)

    suspend fun remove(nodeRef: String) {
        catalog.execute("node", "remove", nodeRef)
    }

    suspend fun select(nodeRef: String) {
        catalog.execute("node", "use", nodeRef)
    }

    suspend fun selectAuto(groupId: String) {
        catalog.execute("node", "use", "auto", groupId)
    }

    suspend fun testDelay(target: String = "", groupId: String = ""): NodeDelayResult {
        val args = mutableListOf("node", "delay")
        target.takeIf(String::isNotBlank)?.let(args::add)
        groupId.takeIf(String::isNotBlank)?.let(args::add)
        return catalog.decode(*args.toTypedArray())
    }
}
