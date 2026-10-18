package com.fanjv.netproxy.bridge

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.*
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

/** Fixed URL subscription operations, without caller-controlled HTTP paths or queries. */
internal object SubscriptionActions {
    private val json = Json
    private val groupPattern = Regex("[A-Za-z0-9][A-Za-z0-9._-]*")
    private val booleanKeys = setOf("persisted", "not_modified", "structure_changed", "runtime_synced", "runtime_sync_pending", "name_changed", "requires_update")
    private val inputKeys = setOf("name", "url", "user_agent", "hwid", "custom_headers", "auto_update", "update_interval", "update_via_proxy", "include", "exclude", "allow_insecure", "download_timeout")
    private val headerPattern = Regex("[!#$%&'*+.^_`|~0-9A-Za-z-]+")

    @Serializable
    data class Request(val requestId: String, val groupId: String, val action: String)

    @Serializable
    data class Write(val requestId: String, val action: String, val groupId: String? = null,
        val input: JsonObject? = null)

    fun parse(raw: String): Request {
        checkSize(raw)
        val request = try { json.decodeFromString<Request>(raw) }
            catch (_: Exception) { throw BridgeFailure("api.invalid_request", 400) }
        if (!request.requestId.matches(Regex("[A-Za-z0-9_-]{1,96}")) ||
            !request.groupId.matches(groupPattern) || request.groupId == "staging" || request.groupId.contains("..") ||
            request.action !in setOf("update", "cancel")) throw BridgeFailure("api.invalid_request", 400)
        return request
    }

    fun parseWrite(raw: String): Write {
        checkSize(raw)
        val value = try { json.parseToJsonElement(raw) as? JsonObject ?: badRequest() }
            catch (_: Exception) { badRequest() }
        val action = (value["action"] as? JsonPrimitive)?.takeIf { it.isString }?.content ?: badRequest()
        val required = setOf("requestId", "action") + when (action) {
            "add" -> setOf("input")
            "edit" -> setOf("groupId", "input")
            "remove" -> setOf("groupId")
            "update_all" -> emptySet()
            else -> badRequest()
        }
        if (value.keys != required || value.values.any { it is JsonNull }) badRequest()
        val request = try { json.decodeFromJsonElement<Write>(value) }
            catch (_: Exception) { badRequest() }
        if (!request.requestId.matches(Regex("[A-Za-z0-9_-]{1,96}")) ||
            request.groupId?.let { !validGroup(it) } == true) badRequest()
        request.input?.let { validateInput(it, action == "add") }
        return request
    }

    private fun validateInput(input: JsonObject, create: Boolean) {
        if (input.keys.any { it !in inputKeys } || input.values.any { it is JsonNull } || (create && "url" !in input)) badRequest()
        for ((key, value) in input) {
            val primitive = value as? JsonPrimitive
            when (key) {
                "custom_headers" -> {
                    val headers = value as? JsonObject ?: badRequest()
                    for ((name, item) in headers) {
                        val text = (item as? JsonPrimitive)?.takeIf { it.isString }?.content ?: badRequest()
                        if (!name.matches(headerPattern) || text.any { it != '\t' && it.isISOControl() }) badRequest()
                    }
                }
                "auto_update", "allow_insecure" -> {
                    if (primitive == null || primitive.isString || primitive.booleanOrNull == null) badRequest()
                }
                "update_interval", "download_timeout" -> {
                    if (primitive == null || primitive.isString) badRequest()
                    val number = primitive.longOrNull ?: badRequest()
                    if (number !in (if (key == "update_interval") 900L else 1L)..9_007_199_254_740_991L) badRequest()
                }
                else -> {
                    val text = primitive?.takeIf { it.isString }?.content ?: badRequest()
                    if (text.any { it.isISOControl() }) badRequest()
                    if (key == "name" && text.isBlank()) badRequest()
                    if (key == "update_via_proxy" && text !in setOf("auto", "always", "never")) badRequest()
                    if (key == "url" && (!text.startsWith("http://", true) && !text.startsWith("https://", true) ||
                            text.any { it.isWhitespace() } || text.toHttpUrlOrNull() == null)) badRequest()
                }
            }
        }
    }

    private fun validGroup(id: String) = id.matches(groupPattern) && id != "staging" && !id.contains("..")
    private fun checkSize(raw: String) {
        if (raw.toByteArray(Charsets.UTF_8).size > 4 * 1024 * 1024) throw BridgeFailure("api.body_too_large", 413)
    }
    private fun badRequest(): Nothing = throw BridgeFailure("api.invalid_request", 400)

    /** Applied to both success and failure data; raw causes and private metadata never cross IPC. */
    fun result(data: JsonElement?, groupId: String? = null, summary: Boolean = false): JsonObject {
        val value = data as? JsonObject ?: return JsonObject(emptyMap())
        return buildJsonObject {
            for ((key, element) in value) {
                when {
                    key == "group_id" || key == "id" -> {
                        val id = element as? JsonPrimitive ?: invalid()
                        if (!id.isString || !validGroup(id.content) || (groupId != null && id.content != groupId)) invalid()
                        put(key, id)
                    }
                    key == "node_count" || key == "revision" -> {
                        val number = element as? JsonPrimitive ?: invalid()
                        if (number.isString || (number.longOrNull ?: invalid()) < 0) invalid()
                        put(key, number)
                    }
                    key in booleanKeys -> {
                        val boolean = element as? JsonPrimitive ?: invalid()
                        if (boolean.isString || boolean.booleanOrNull == null) invalid()
                        put(key, boolean)
                    }
                    key == "runtime_sync_state" -> {
                        val state = element as? JsonPrimitive ?: invalid()
                        if (!state.isString || state.content !in setOf("applied", "not_running", "failed")) invalid()
                        put(key, state)
                    }
                    summary && key in setOf("updated", "failed") -> {
                        val ids = element as? JsonArray ?: invalid()
                        for (item in ids) {
                            val id = item as? JsonPrimitive ?: invalid()
                            if (!id.isString || !validGroup(id.content)) invalid()
                        }
                        put(key, ids)
                    }
                    summary && key == "first_error" -> put(key, result(element))
                }
            }
        }
    }

    private fun invalid(): Nothing = throw BridgeFailure("api.invalid_response", 502)
}
