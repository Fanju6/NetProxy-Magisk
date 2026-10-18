package com.fanjv.netproxy.bridge

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.*

/** Fixed business requests and a shared GET/POST/SSE public response projection. */
internal object PolicyGroups {
    @Serializable
    data class Select(val requestId: String, val groupTag: String, val memberTag: String)

    @Serializable
    data class Delay(val requestId: String, val groupTag: String)

    fun select(raw: String): Select {
        val value = request(raw, setOf("requestId", "groupTag", "memberTag"))
        return try { Json.decodeFromJsonElement<Select>(value) }
            catch (_: Exception) { badRequest() }
    }

    fun delay(raw: String): Delay {
        val value = request(raw, setOf("requestId", "groupTag"))
        return try { Json.decodeFromJsonElement<Delay>(value) }
            catch (_: Exception) { badRequest() }
    }

    private fun request(raw: String, keys: Set<String>): JsonObject {
        val bytes = try { raw.encodeToByteArray(throwOnInvalidSequence = true) }
            catch (_: Exception) { badRequest() }
        if (bytes.size > 4 * 1024 * 1024) throw BridgeFailure("api.body_too_large", 413)
        val value = try { Json.parseToJsonElement(raw) as? JsonObject ?: badRequest() }
            catch (_: Exception) { badRequest() }
        if (value.keys != keys) badRequest()
        for ((key, item) in value) {
            val text = (item as? JsonPrimitive)?.takeIf { it.isString }?.content ?: badRequest()
            if (key == "requestId") {
                if (!text.matches(Regex("[A-Za-z0-9_-]{1,96}"))) badRequest()
            } else if (!validTag(text)) badRequest()
        }
        return value
    }

    private fun validTag(text: String): Boolean {
        if (text.isBlank() || text.any { it.isISOControl() }) return false
        return try { text.encodeToByteArray(throwOnInvalidSequence = true); true }
            catch (_: Exception) { false }
    }

    fun snapshot(data: JsonElement): JsonObject {
        val value = obj(data)
        val groups = value["groups"] as? JsonArray ?: invalid()
        val live = boolean(value, "live")
        val startedAt = number(value, "core_started_at_millis", 0..Long.MAX_VALUE)
        if (live.booleanOrNull == true && startedAt.longOrNull == 0L) invalid()
        return buildJsonObject {
            put("groups", JsonArray(groups.map { entry ->
                val group = obj(entry)
                buildJsonObject {
                    put("tag", tag(group, "tag"))
                    put("type", text(group, "type"))
                    put("selectable", boolean(group, "selectable"))
                    put("expanded", boolean(group, "expanded"))
                    group["selected"]?.let { put("selected", text(group, "selected")) }
                    val items = group["items"] as? JsonArray ?: invalid()
                    put("items", JsonArray(items.map { item ->
                        val member = obj(item)
                        buildJsonObject {
                            put("tag", tag(member, "tag"))
                            put("type", text(member, "type"))
                            member["url_test_time"]?.let { put("url_test_time", number(member, "url_test_time", 0..Long.MAX_VALUE)) }
                            member["url_test_delay"]?.let { put("url_test_delay", number(member, "url_test_delay", Int.MIN_VALUE.toLong()..Int.MAX_VALUE.toLong())) }
                        }
                    }))
                }
            }))
            put("live", live)
            put("core_started_at_millis", startedAt)
            put("default_outbound", text(value, "default_outbound"))
        }
    }

    private fun obj(value: JsonElement) = value as? JsonObject ?: invalid()
    fun failure(data: JsonElement?): JsonObject {
        val value = data as? JsonObject ?: return JsonObject(emptyMap())
        return buildJsonObject {
            for (key in listOf("accepted", "persisted", "runtime_synced")) {
                (value[key] as? JsonPrimitive)?.takeIf { !it.isString && it.booleanOrNull != null }?.let { put(key, it) }
            }
            (value["core_started_at_millis"] as? JsonPrimitive)
                ?.takeIf { !it.isString && it.longOrNull?.let { n -> n >= 0 } == true }
                ?.let { put("core_started_at_millis", it) }
        }
    }

    private fun text(value: JsonObject, key: String): JsonPrimitive =
        (value[key] as? JsonPrimitive)?.takeIf { it.isString } ?: invalid()
    private fun tag(value: JsonObject, key: String) = text(value, key).also { if (!validTag(it.content)) invalid() }
    private fun boolean(value: JsonObject, key: String) =
        (value[key] as? JsonPrimitive)?.takeIf { !it.isString && it.booleanOrNull != null } ?: invalid()
    private fun number(value: JsonObject, key: String, range: LongRange) =
        (value[key] as? JsonPrimitive)?.takeIf { !it.isString && it.longOrNull in range } ?: invalid()
    private fun badRequest(): Nothing = throw BridgeFailure("api.invalid_request", 400)
    private fun invalid(): Nothing = throw BridgeFailure("api.invalid_response", 502)
}
