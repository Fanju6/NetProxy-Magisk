package com.fanjv.netproxy.bridge

import kotlinx.serialization.json.*

internal object PublicStatus {
    private val textKeys = setOf("state", "configured_backend", "outbound_mode", "configured_outbound_mode",
        "default_outbound", "runtime_selected", "worker_state")
    private val numberKeys = setOf("started_at", "core_started_at_millis", "ready_at", "uptime_seconds",
        "memory_bytes", "process_cpu_ticks", "system_cpu_ticks", "cpu_count", "connections_in",
        "connections_out", "upload_total", "download_total")

    fun status(data: JsonElement): JsonObject {
        val value = data as? JsonObject ?: invalid()
        return buildJsonObject {
            for ((key, item) in value) when (key) {
                in textKeys -> put(key, string(item))
                in numberKeys -> put(key, number(item))
                "pid", "worker_pid" -> put(key, if (item is JsonNull) item else number(item))
                "active_backend" -> put(key, if (item is JsonNull) item else string(item))
                "available_outbound_modes" -> {
                    val modes = item as? JsonArray ?: invalid()
                    put(key, JsonArray(modes.map(::string)))
                }
                "error" -> put(key, if (string(item).content.isEmpty()) "" else SAFE_FAILURE_MESSAGE)
            }
        }
    }

    fun action(data: JsonElement): JsonObject {
        val value = data as? JsonObject ?: invalid()
        return buildJsonObject {
            value["action"]?.let { put("action", string(it)) }
            value["status"]?.let { put("status", status(it)) }
        }
    }

    private fun string(value: JsonElement) = (value as? JsonPrimitive)?.takeIf { it.isString } ?: invalid()
    private fun number(value: JsonElement) =
        (value as? JsonPrimitive)?.takeIf { !it.isString && (it.longOrNull != null || it.content.toULongOrNull() != null) } ?: invalid()
    private fun invalid(): Nothing = throw BridgeFailure("api.invalid_response", 502)
}
