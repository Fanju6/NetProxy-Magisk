package com.fanjv.netproxy.feature.inbound.data

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull

internal val inboundJson = Json { prettyPrint = true }
internal const val DEFAULT_TUN_IPV6 = "fdfe:dcba:9876::1/126"

internal fun JsonObject.objectAt(key: String): JsonObject = this[key] as? JsonObject ?: JsonObject(emptyMap())
internal fun JsonObject.textAt(key: String, default: String = ""): String =
    (this[key] as? JsonPrimitive)?.contentOrNull ?: default
internal fun JsonObject.flagAt(key: String, default: Boolean = false): Boolean =
    (this[key] as? JsonPrimitive)?.booleanOrNull ?: default
internal fun JsonObject.ebpfDnsMode(): String = textAt("dns_mode").ifBlank { "respect_policy" }
internal fun JsonObject.tunDnsMode(): String = textAt("dns_mode").ifEmpty { "hijack" }
internal fun JsonObject.listAt(key: String): List<String> = when (val value = this[key]) {
    is JsonArray -> value.map { (it as JsonPrimitive).content }
    is JsonPrimitive -> listOfNotNull(value.contentOrNull).filter(String::isNotBlank)
    else -> emptyList()
}
internal fun JsonObject.withFields(vararg fields: Pair<String, JsonElement?>): JsonObject =
    JsonObject(toMutableMap().apply {
        fields.forEach { (key, value) -> if (value == null) remove(key) else put(key, value) }
    })

internal fun JsonObject.withPath(path: List<String>, value: JsonElement?): JsonObject {
    require(path.isNotEmpty())
    return if (path.size == 1) withFields(path.first() to value)
    else withFields(path.first() to objectAt(path.first()).withPath(path.drop(1), value))
}

internal fun stringArray(values: List<String>): JsonArray = JsonArray(values.map(::JsonPrimitive))

internal fun JsonObject.withTunIpv6(enabled: Boolean): JsonObject {
    val addresses = listAt("address")
    return withFields("address" to stringArray(
        if (enabled) {
            if (addresses.any { ':' in it }) addresses else addresses + DEFAULT_TUN_IPV6
        } else addresses.filterNot { ':' in it }
    ))
}

internal fun JsonObject.withFilter(includeKey: String, excludeKey: String, mode: String, values: List<String>): JsonObject {
    require(mode in setOf("all", "include", "exclude"))
    return withFields(
        includeKey to if (mode == "include") stringArray(values) else null,
        excludeKey to if (mode == "exclude") stringArray(values) else null
    )
}

internal fun JsonObject.filterMode(includeKey: String, excludeKey: String): String = when {
    listAt(includeKey).isNotEmpty() -> "include"
    listAt(excludeKey).isNotEmpty() -> "exclude"
    else -> "all"
}
