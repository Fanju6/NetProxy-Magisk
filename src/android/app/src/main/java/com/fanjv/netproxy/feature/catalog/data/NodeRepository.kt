package com.fanjv.netproxy.feature.catalog.data

import com.fanjv.netproxy.core.api.ModuleApiClient
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.*

@Serializable
internal data class ExportedNodeLink(val tag: String, val protocol: String, val link: String)

internal class NodeRepository(private val client: ModuleApiClient) {
    suspend fun add(link: String) {
        client.post("/nodes", buildJsonObject { put("content", link) })
    }

    suspend fun import(content: String): String {
        return client.post("/nodes/import", buildJsonObject { put("content", content) }).data
            .jsonObject["group_id"]?.jsonPrimitive?.content.orEmpty()
    }

    suspend fun edit(nodeRef: String, source: String) {
        client.put("/nodes", buildJsonObject {
            nodeTarget(nodeRef).forEach { (key, value) -> put(key, value) }
            put("content", source)
        })
    }

    suspend fun editJson(nodeRef: String, content: String) = edit(nodeRef, content)
    suspend fun get(nodeRef: String): String = client.get("/nodes/get", nodeTarget(nodeRef)).data.toString()
    suspend fun export(nodeRef: String): ExportedNodeLink = client.read("/nodes/export", nodeTarget(nodeRef))

    suspend fun remove(nodeRef: String) { client.delete("/nodes", nodeTarget(nodeRef)) }
    private fun nodeTarget(reference: String): Map<String, String> {
        val separator = reference.indexOf('/')
        require(separator > 0 && separator < reference.lastIndex)
        return mapOf("group_id" to reference.substring(0, separator), "node_tag" to reference.substring(separator + 1))
    }
}
