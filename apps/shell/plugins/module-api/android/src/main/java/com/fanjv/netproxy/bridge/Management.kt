package com.fanjv.netproxy.bridge

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.*

/** Fixed operation DTOs; neither HTTP parameters nor device paths are supplied by JS. */
internal object Management {
    @Serializable data class Request(val requestId: String, val operation: String, val input: JsonObject? = null)
    data class Route(val method: String, val path: String, val mutation: Boolean, val body: JsonObject? = null, val query: Map<String, String> = emptyMap())
    private val json = Json
    private val hash = Regex("[a-f0-9]{64}")
    private val token = Regex("[A-Za-z0-9_.-]{1,128}")
    private val sections = setOf("log", "dns", "ntp", "certificate", "certificate_providers", "http_clients", "network_namespaces", "endpoints", "inbounds", "outbounds", "providers", "route", "services", "experimental")
    private val empty = JsonObject(emptyMap())

    fun target(value: String, write: Boolean = false): Boolean {
        if (value in setOf("module", "module/wifi", "module/auto_start", "inbound", "inbound/backend", "inbound/root_policy", "inbound/app", "inbound/ebpf", "inbound/tun", "singbox/config.json")) return true
        if (value.removePrefix("singbox/") in sections && value.startsWith("singbox/")) return true
        if (!write && value in setOf("runtime/inbound.json", "runtime/outbounds.json", "runtime/providers.json")) return true
        if (!value.startsWith("singbox/rules/local/")) return false
        val name = value.removePrefix("singbox/rules/local/")
        return name.length <= 255 && !name.startsWith('.') && !name.contains("..") && name.matches(Regex("[A-Za-z0-9_-][A-Za-z0-9._-]*\\.json"))
    }

    fun parse(raw: String): Request {
        if (raw.toByteArray(Charsets.UTF_8).size > 4 * 1024 * 1024) throw BridgeFailure("api.body_too_large")
        val request = try {
            val value = json.parseToJsonElement(raw) as? JsonObject ?: invalid()
            if (value.values.any { it is JsonNull }) invalid()
            json.decodeFromJsonElement<Request>(value)
        } catch (_: Exception) { invalid() }
        if (!request.requestId.matches(Regex("[A-Za-z0-9_-]{1,96}"))) invalid()
        val (required, optional) = when (request.operation) {
            "config_list", "apps", "wifi", "diagnostics" -> emptySet<String>() to emptySet()
            "config_read" -> setOf("target") to emptySet()
            "config_apply", "config_validate" -> setOf("target", "revision", "content") to emptySet()
            "icon" -> setOf("icon_key") to emptySet()
            "logs" -> emptySet<String>() to setOf("kind", "lines")
            "logs_clear" -> emptySet<String>() to setOf("kind")
            "ebpf" -> emptySet<String>() to setOf("mode", "raw")
            "node_read", "node_export", "node_remove" -> setOf("group_id", "node_tag") to emptySet()
            "node_add" -> setOf("content") to setOf("group_id", "allow_insecure")
            "node_edit" -> setOf("group_id", "node_tag", "content") to setOf("allow_insecure")
            "node_import" -> setOf("content") to setOf("allow_insecure")
            else -> invalid()
        }
        val input = request.input ?: empty
        if (!input.keys.containsAll(required) || input.keys.any { it !in required && it !in optional }) invalid()
        for ((key, value) in input) {
            val primitive = value as? JsonPrimitive ?: invalid()
            val valid = when (key) {
                "raw", "allow_insecure" -> !primitive.isString && primitive.booleanOrNull != null
                "lines" -> !primitive.isString && primitive.longOrNull?.let { it in 1..2000 } == true
                else -> primitive.isString && when (key) {
                    "target" -> target(primitive.content, request.operation != "config_read")
                    "revision", "icon_key" -> primitive.content.matches(hash)
                    "group_id" -> group(primitive.content)
                    "node_tag" -> primitive.content.isNotBlank() && primitive.content.none { it.isISOControl() }
                    "kind" -> primitive.content in setOf("service", "core")
                    "mode" -> primitive.content in setOf("configured", "all", "local", "shared")
                    "content" -> request.operation.startsWith("config_") || primitive.content.isNotBlank()
                    else -> false
                }
            }
            if (!valid) invalid()
        }
        return request
    }

    fun route(request: Request): Route {
        val input = request.input ?: empty
        fun text(key: String) = input.getValue(key).jsonPrimitive.content
        fun query(vararg keys: String) = input.filterKeys { it in keys }.mapValues { it.value.jsonPrimitive.content }
        return when (request.operation) {
            "config_list" -> Route("GET", "/configs", false)
            "config_read" -> Route("GET", "/config", false, query = query("target"))
            "config_apply", "config_validate" -> Route(if (request.operation == "config_apply") "PUT" else "POST", if (request.operation == "config_apply") "/config" else "/config/validate", true, JsonObject(input - "target"), query("target"))
            "apps" -> Route("GET", "/android/apps", false)
            "wifi" -> Route("GET", "/network/wifi-list", false)
            "logs" -> Route("GET", "/logs", false, query = query("kind", "lines"))
            "logs_clear" -> Route("DELETE", "/logs", true, empty, query("kind"))
            "ebpf" -> Route("POST", "/diagnostics/ebpf", true, input)
            "node_read", "node_export", "node_remove" -> Route(if (request.operation == "node_remove") "DELETE" else "GET", when (request.operation) { "node_read" -> "/nodes/get"; "node_export" -> "/nodes/export"; else -> "/nodes" }, request.operation == "node_remove", if (request.operation == "node_remove") empty else null, query("group_id", "node_tag"))
            "node_add", "node_edit", "node_import" -> Route(if (request.operation == "node_edit") "PUT" else "POST", if (request.operation == "node_import") "/nodes/import" else "/nodes", true, input)
            else -> invalid()
        }
    }

    fun success(request: Request, data: JsonElement): JsonElement {
        val input = request.input ?: empty
        return when (request.operation) {
            "config_list" -> JsonArray(array(data).mapNotNull { entry ->
                val value = obj(entry)
                val id = text(value, "id")
                if (!target(id)) null else pick(value, setOf("id", "filename", "category", "editable", "section"))
            })
            "config_read", "config_apply", "config_validate" -> {
                val value = obj(data)
                if (value["target"] != input["target"] || !text(value, "revision").matches(hash)) responseInvalid()
                if (request.operation == "config_read") { text(value, "content"); pick(value, setOf("target", "revision", "content")) }
                else pick(value, setOf("target", "revision"))
            }
            // Explicit editor/export operations are the only channels for private node content.
            "node_read" -> obj(data)
            "node_export" -> pick(obj(data), setOf("tag", "protocol", "link"))
            "node_add", "node_edit", "node_remove", "node_import" -> facts(data)
            "apps" -> buildJsonObject { put("users", JsonArray(array(obj(data)["users"]).map { entry ->
                val value = obj(entry)
                JsonObject(pick(value, setOf("id", "name", "revision")) + ("apps" to JsonArray(array(value["apps"]).map { pick(obj(it), setOf("package_name", "uid", "label", "system", "version_code", "last_update_time", "icon_key")) })))
            })) }
            "wifi" -> buildJsonObject {
                val ssids = array(obj(data)["ssids"])
                if (ssids.any { it !is JsonPrimitive || !it.isString }) responseInvalid()
                put("ssids", ssids)
            }
            "logs" -> {
                val value = obj(data)
                JsonObject(pick(value, setOf("kind", "content")) + ("entries" to JsonArray(array(value["entries"]).map { pick(obj(it), setOf("timestamp", "level", "component", "event", "result", "error_code", "message")) })))
            }
            "logs_clear" -> pick(obj(data), setOf("kind"))
            "ebpf" -> probe(data)
            else -> responseInvalid()
        }
    }

    fun failure(request: Request, data: JsonElement?): JsonElement =
        if (request.operation == "ebpf" && (data as? JsonObject)?.get("report") is JsonObject) probe(data, failed = true) else facts(data)

    private fun facts(data: JsonElement?): JsonObject {
        val value = data as? JsonObject ?: return empty
        return buildJsonObject {
            for ((key, item) in value) {
                val p = item as? JsonPrimitive
                when (key) {
                    "persisted", "runtime_synced", "runtime_sync_pending", "structure_changed" -> if (p != null && !p.isString && p.booleanOrNull != null) put(key, p)
                    "revision", "expected_revision", "actual_revision" -> if (p != null && (p.isString && p.content.matches(hash) || !p.isString && p.longOrNull?.let { it >= 0 } == true)) put(key, p)
                    "node_count" -> if (p != null && !p.isString && p.longOrNull?.let { it >= 0 } == true) put(key, p)
                    "group_id", "id" -> if (p?.isString == true && group(p.content)) put(key, p)
                    "target" -> if (p?.isString == true && target(p.content)) put(key, p)
                    "runtime_sync_state" -> if (p?.isString == true && p.content in setOf("applied", "failed", "not_running")) put(key, p)
                    "diagnostics" -> if (item is JsonArray) put(key, JsonArray(item.mapNotNull { entry ->
                        val diagnostic = entry as? JsonObject ?: return@mapNotNull null
                        buildJsonObject {
                            for (name in listOf("code", "level")) (diagnostic[name] as? JsonPrimitive)?.takeIf { it.isString && it.content.matches(token) }?.let { put(name, it) }
                            (diagnostic["index"] as? JsonPrimitive)?.takeIf { !it.isString && it.longOrNull?.let { n -> n >= 0 } == true }?.let { put("index", it) }
                            put("message", "Configuration validation failed")
                        }
                    }))
                }
            }
        }
    }

    private fun probe(data: JsonElement?, failed: Boolean = false): JsonObject {
        val value = obj(data)
        val report = obj(value["report"])
        val mode = text(value, "mode")
        val raw = value["raw"] as? JsonPrimitive ?: responseInvalid()
        if (mode !in setOf("configured", "all", "local", "shared") || raw.isString || raw.booleanOrNull == null) responseInvalid()
        val network = array(report["network"])
        if (network.any { it !is JsonPrimitive || !it.isString || !it.content.matches(token) }) responseInvalid()
        val findings = JsonArray(array(report["findings"]).map { entry ->
            val item = obj(entry)
            val fields = pick(item, setOf("status", "scope", "importance"))
            if (fields.values.any { it !is JsonPrimitive || !it.isString || !it.content.matches(token) }) responseInvalid()
            val feature = text(item, "feature")
            if (feature.length > 256 || feature.any { it.isISOControl() }) responseInvalid()
            val reason = (item["reason"] as? JsonPrimitive)?.takeIf { it.isString && it.content.matches(token) }?.content.orEmpty()
            JsonObject(fields + mapOf("feature" to JsonPrimitive(safeText(feature)), "reason" to JsonPrimitive(reason), "detail" to JsonPrimitive("")))
        })
        val summary = pick(obj(report["summary"]), setOf("pass", "warn", "fail", "unknown", "required_failures", "required_unknowns", "required_issues"))
        if (summary.values.any { it !is JsonPrimitive || it.isString || it.longOrNull?.let { n -> n < 0 } != false }) responseInvalid()
        val properties = pick(report, setOf("platform", "kernel_release", "architecture", "mode", "local_data_plane", "shared_data_plane", "ipv6", "preflight", "exact_object_load", "result"))
        for ((key, item) in properties) {
            val primitive = item as JsonPrimitive
            if (key in setOf("ipv6", "preflight", "exact_object_load")) {
                if (primitive.isString || primitive.booleanOrNull == null) responseInvalid()
            } else if (!primitive.isString || !primitive.content.matches(Regex("[A-Za-z0-9_.+-]{1,128}"))) responseInvalid()
        }
        val safeReport = JsonObject(properties +
            mapOf("network" to network, "findings" to findings, "summary" to summary))
        val content = if (raw.booleanOrNull == true) safeReport.toString() else if (failed) {
            // Error text can contain raw process stderr. Preserve typed findings, never the exception.
            "eBPF preflight failed\n" + findings.joinToString("\n") { finding ->
                val fields = finding.jsonObject
                listOf("scope", "feature", "status").joinToString(" / ") { fields[it]?.jsonPrimitive?.content.orEmpty() }
            }
        } else safeText(text(value, "content"))
        return buildJsonObject {
            put("mode", mode); put("raw", raw); put("content", content); put("report", safeReport)
        }
    }
    private fun safeText(value: String) = value.replace(Regex("(?i)(?:https?|socks5?)://\\S+"), "[redacted]")
        .replace(Regex("(?im)^.*(?:authorization|cookie|password|secret|hwid|headers?).*$"), "[redacted]")
    private fun group(value: String) = value.matches(Regex("[A-Za-z0-9][A-Za-z0-9._-]*")) && value != "staging" && !value.contains("..")
    private fun pick(value: JsonObject, keys: Set<String>): JsonObject = JsonObject(value.filterKeys { it in keys }.also { if (it.values.any { item -> item !is JsonPrimitive || item is JsonNull }) responseInvalid() })
    private fun obj(value: JsonElement?) = value as? JsonObject ?: responseInvalid()
    private fun array(value: JsonElement?) = value as? JsonArray ?: responseInvalid()
    private fun text(value: JsonObject, key: String) = (value[key] as? JsonPrimitive)?.takeIf { it.isString }?.content ?: responseInvalid()
    private fun invalid(): Nothing = throw BridgeFailure("api.invalid_request", 400)
    private fun responseInvalid(): Nothing = throw BridgeFailure("api.invalid_response", 502)
}
