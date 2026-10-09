package com.fanjv.netproxy.feature.kernel.presentation

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put

internal fun editorSchema(root: JsonObject, documentId: String): JsonObject {
    if (documentId != "inbound" && !documentId.startsWith("inbound/")) return root
    val definitions = root.getValue("\$defs").jsonObject
    val variants = definitions.getValue("Inbound").jsonObject.getValue("oneOf") as JsonArray
    fun nativeReference(type: String): JsonObject {
        val index = variants.indexOfFirst {
            it.jsonObject["properties"]?.jsonObject?.get("type")?.jsonObject?.get("const") == JsonPrimitive(type)
        }
        check(index >= 0) { "内置 Schema 缺少 $type 入站" }
        return buildJsonObject {
            put("allOf", JsonArray(listOf(
                buildJsonObject { put("\$ref", "#/\$defs/Inbound/oneOf/$index") },
                buildJsonObject {
                    put("type", "object")
                    put("properties", buildJsonObject {
                        put("tag", buildJsonObject { put("const", "netproxy-in") })
                        if (type == "tun") {
                            put("auto_route", buildJsonObject { put("const", true) })
                            put("auto_redirect", buildJsonObject { put("const", true) })
                        }
                    })
                    put("required", JsonArray((if (type == "tun") listOf("type", "tag", "auto_route", "auto_redirect", "address")
                        else listOf("type", "tag")).map(::JsonPrimitive)))
                }
            )))
        }
    }
    val properties = buildJsonObject {
        put("backend", buildJsonObject {
            put("type", "string")
            put("enum", JsonArray(listOf("ebpf", "tun").map(::JsonPrimitive)))
        })
        put("root_policy", buildJsonObject {
            put("type", "string")
            put("enum", JsonArray(listOf("default", "include", "exclude").map(::JsonPrimitive)))
        })
        put("app", buildJsonObject {
            put("type", "object")
            put("properties", buildJsonObject {
                put("enabled", buildJsonObject { put("type", "boolean") })
                put("mode", buildJsonObject {
                    put("type", "string")
                    put("enum", JsonArray(listOf("blacklist", "whitelist").map(::JsonPrimitive)))
                })
                for (field in listOf("proxy_apps", "bypass_apps")) put(field, buildJsonObject {
                    put("type", "array")
                    put("items", buildJsonObject {
                        put("type", "string")
                        put("pattern", "^[0-9]+:[A-Za-z0-9_]+(\\.[A-Za-z0-9_]+)*$")
                    })
                })
            })
            put("required", JsonArray(listOf("enabled", "mode", "proxy_apps", "bypass_apps").map(::JsonPrimitive)))
            put("additionalProperties", false)
        })
        put("ebpf", nativeReference("ebpf"))
        put("tun", nativeReference("tun"))
    }
    val fields = if (documentId == "inbound") properties.keys.toList() else listOf(documentId.substringAfter('/'))
    require(fields.all { it in properties })
    return buildJsonObject {
        put("\$defs", definitions)
        put("type", "object")
        put("properties", JsonObject(properties.filterKeys { it in fields }))
        put("required", JsonArray(fields.map(::JsonPrimitive)))
        put("additionalProperties", false)
    }
}
