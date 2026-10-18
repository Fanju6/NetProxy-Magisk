package com.fanjv.netproxy.bridge

import kotlinx.serialization.json.*

/** 在原生边界投影公开 DTO，不让新增或意外返回的私有字段进入 WebView。 */
internal object PublicCatalog {
    private val groupKeys = setOf("id", "name", "provider_tag", "type", "node_count", "revision",
        "auto_update", "update_interval", "update_via_proxy", "profile_title", "last_attempt_at",
        "last_success_at", "next_update_at", "updated_at")
    private val nodeKeys = setOf("tag", "protocol", "server", "port")
    private val historyKeys = setOf("at", "ok", "code", "node_count", "revision", "duration_seconds")

    fun read(path: String, data: JsonElement): JsonElement = when {
        path == "/service" -> PublicStatus.status(data)
        path in setOf("/service/start", "/service/stop", "/service/restart", "/service/reload", "/service/check") -> PublicStatus.action(data)
        path == "/subscriptions" -> groups(data)
        path == "/catalog" || path == "/nodes/snapshot" -> snapshot(data)
        path in setOf("/policy-groups", "/policy-groups/select", "/policy-groups/delay") -> PolicyGroups.snapshot(data)
        path.startsWith("/subscriptions/") && path.endsWith("/history") -> history(data)
        path.startsWith("/subscriptions/") && path.endsWith("/progress") -> {
            val value = obj(data)
            val id = text(value, "id")
            if (path != "/subscriptions/$id/progress") invalid()
            buildJsonObject { put("id", id); put("progress", progress(value["progress"], id)) }
        }
        path.startsWith("/subscriptions/") -> {
            val value = obj(data)
            val group = group(value["group"] ?: invalid())
            if (path != "/subscriptions/${text(group, "id")}") invalid()
            buildJsonObject {
                put("group", group)
                put("nodes", JsonArray(array(value["nodes"]).map { pick(obj(it), nodeKeys) }))
            }
        }
        else -> data
    }

    fun event(name: String, data: JsonElement): JsonElement = when (name) {
        "status" -> PublicStatus.status(data)
        "subscriptions" -> groups(data)
        "groups" -> PolicyGroups.snapshot(data)
        else -> data
    }

    private fun groups(data: JsonElement) = JsonArray(array(data).map(::group))

    private fun snapshot(data: JsonElement): JsonObject {
        val value = obj(data)
        return buildJsonObject {
            put("groups", JsonArray(array(value["groups"]).map { entry ->
                val item = obj(entry)
                buildJsonObject {
                    put("group", group(item["group"] ?: invalid()))
                    put("nodes", JsonArray(array(item["nodes"]).map { pick(obj(it), nodeKeys) }))
                }
            }))
        }
    }

    private fun group(data: JsonElement): JsonObject {
        val value = obj(data)
        if (text(value, "type") !in setOf("local", "subscription") ||
            (value["provider_tag"] as? JsonPrimitive)?.isString != true) invalid()
        val id = text(value, "id")
        return JsonObject(pick(value, groupKeys) + mapOf(
            "usage" to (value["usage"]?.takeUnless { it is JsonNull }?.let { pick(obj(it), setOf("upload", "download", "total", "expire")) } ?: JsonNull),
            "progress" to progress(value["progress"], id),
            "profile_web_page_url" to JsonPrimitive(""),
            // 原始失败文本可能包含下载 URL；只传失败事实，具体结果由历史错误码展示。
            "last_error" to JsonPrimitive(if ((value["last_error"] as? JsonPrimitive)?.content.orEmpty().isBlank()) "" else "Subscription update failed"),
        ))
    }

    private fun progress(data: JsonElement?, id: String): JsonElement {
        if (data == null || data is JsonNull) return JsonNull
        val value = obj(data)
        val stage = text(value, "stage")
        if ((value["schema"] as? JsonPrimitive)?.intOrNull != 1 || text(value, "group_id") != id ||
            stage !in setOf("download", "convert", "validate", "apply")) invalid()
        return buildJsonObject {
            put("schema", 1); put("group_id", id); put("stage", stage)
            put("message", "Subscription update in progress")
            put("updated_at", text(value, "updated_at"))
        }
    }

    private fun history(data: JsonElement): JsonArray = JsonArray(array(data).map { entry ->
        val value = obj(entry)
        val ok = (value["ok"] as? JsonPrimitive)?.booleanOrNull ?: invalid()
        val code = text(value, "code")
        if (!code.matches(Regex("[a-z][a-z0-9_]*(\\.[a-z][a-z0-9_]*)+"))) invalid()
        JsonObject(pick(value, historyKeys) + ("message" to JsonPrimitive(if (ok) "Subscription update completed" else "Subscription update failed")))
    })

    private fun pick(value: JsonObject, keys: Set<String>): JsonObject {
        val result = value.filterKeys { it in keys }
        if (result.values.any { it !is JsonPrimitive || it is JsonNull }) invalid()
        return JsonObject(result)
    }
    private fun obj(value: JsonElement) = value as? JsonObject ?: invalid()
    private fun array(value: JsonElement?) = value as? JsonArray ?: invalid()
    private fun text(value: JsonObject, key: String): String =
        (value[key] as? JsonPrimitive)?.takeIf { it.isString }?.content ?: invalid()
    private fun invalid(): Nothing = throw BridgeFailure("api.invalid_response", 502)
}
