package com.fanjv.netproxy.feature.catalog.data

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.feature.catalog.model.*
import kotlinx.serialization.json.*
import kotlinx.coroutines.flow.map

internal class SubscriptionRepository(private val client: ModuleApiClient) {
    suspend fun list(): List<CatalogGroupSummary> = client.read("/subscriptions")
    fun observe() = client.observe(setOf("subscriptions")).map { result -> result.mapCatching { events ->
        events.getValue("subscriptions").decode<List<CatalogGroupSummary>>()
    } }
    suspend fun details(id: String): CatalogNodeGroup = client.read("/subscriptions/$id")
    suspend fun readEditor(id: String): SubscriptionEditorState = client.read("/subscriptions/$id", mapOf("private" to "true"))

    suspend fun add(draft: SubscriptionDraft): JsonElement = client.post("/subscriptions", draft.document()
        .let { if (draft.name.isBlank()) JsonObject(it - "name") else it }).data

    suspend fun edit(id: String, original: SubscriptionEditorState, updated: SubscriptionDraft): JsonElement {
        val changes = subscriptionEditBody(original, updated)
        return if (changes.isEmpty()) JsonObject(emptyMap()) else client.put("/subscriptions/$id", changes).data
    }

    suspend fun update(id: String) { client.post("/subscriptions/$id/update") }
    suspend fun updateAll() { client.post("/subscriptions/update-all") }
    suspend fun activate(id: String) {
        client.put("/selection", buildJsonObject { put("group_id", id); put("node_tag", "") })
    }
    suspend fun remove(id: String, replacementGroupId: String = "") {
        client.delete("/subscriptions/$id", if (replacementGroupId.isBlank()) emptyMap() else mapOf("replacement_group_id" to replacementGroupId))
    }
    suspend fun history(id: String): List<SubscriptionHistoryEntry> = client.read("/subscriptions/$id/history")
}

internal fun subscriptionEditBody(original: SubscriptionEditorState, updated: SubscriptionDraft): JsonObject {
    val previous = subscriptionSnapshots.encodeToJsonElement(original).jsonObject
    return JsonObject(updated.document().filter { (key, value) ->
        previous[if (key == "download_timeout") "timeout" else key] != value
    })
}

private val subscriptionSnapshots = Json { encodeDefaults = true }

private fun SubscriptionDraft.document(): JsonObject = buildJsonObject {
    put("name", name); put("url", url); put("user_agent", userAgent); put("hwid", hwid)
    put("custom_headers", JsonObject(customHeaders.mapValues { JsonPrimitive(it.value) }))
    put("auto_update", autoUpdate); put("update_interval", updateIntervalSeconds)
    put("update_via_proxy", updateViaProxy); put("include", include); put("exclude", exclude)
    put("allow_insecure", allowInsecure); put("download_timeout", timeoutSeconds)
}
