package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull

internal data class NodeEditFields(
    val tag: String = "",
    val type: String = "vless",
    val server: String = "",
    val serverPort: String = "",
    val uuid: String = "",
    val flow: String = "none",
    val security: String = "auto",
    val alterId: String = "",
    val method: String = "aes-128-gcm",
    val password: String = "",
    val plugin: String = "",
    val pluginOpts: String = "",
    val upMbps: String = "",
    val downMbps: String = "",
    val obfsType: String = "none",
    val obfsPassword: String = "",
    val serverPorts: String = "",
    val hopInterval: String = "",
    val congestionControl: String = "cubic",
    val udpRelayMode: String = "quic",
    val udpOverStream: Boolean = false,
    val zeroRttHandshake: Boolean = false,
    val heartbeat: String = "",
    val transportType: String = "none",
    val path: String = "",
    val host: String = "",
    val serviceName: String = "",
    val tlsEnabled: Boolean = false,
    val serverName: String = "",
    val insecure: Boolean = false,
    val disableSni: Boolean = false,
    val alpn: String = "",
    val fingerprint: String = "none",
    val realityEnabled: Boolean = false,
    val realityPublicKey: String = "",
    val realityShortId: String = "",
    val echEnabled: Boolean = false,
    val echConfig: String = "",
    val echQueryServerName: String = ""
)

internal data class NodeEditDraft(
    val original: JsonObject,
    val initial: NodeEditFields,
    val fields: NodeEditFields = initial
) {
    fun toJson(): JsonObject {
        val outbound = original.outbound().toMutableMap()
        fun patch(before: Any, after: Any, value: JsonElement?, vararg path: String) {
            if (before != after) outbound.patch(path.toList(), value)
        }
        fun text(before: String, after: String, vararg path: String) =
            patch(before, after, after.takeIf { it.isNotEmpty() }?.let(::JsonPrimitive), *path)
        fun number(before: String, after: String, key: String) = patch(
            before, after,
            after.takeIf { it.isNotBlank() }?.let { value ->
                value.trim().toIntOrNull()?.let(::JsonPrimitive) ?: JsonPrimitive(value)
            }, key
        )
        fun flag(before: Boolean, after: Boolean, vararg path: String) =
            patch(before, after, JsonPrimitive(after), *path)
        fun list(before: String, after: String, separator: String, vararg path: String) = patch(
            before, after,
            after.takeIf { it.isNotBlank() }?.let { value ->
                JsonArray(value.split(separator).map(String::trim).filter(String::isNotEmpty).map(::JsonPrimitive))
            }, *path
        )

        text(initial.tag, fields.tag, "tag")
        text(initial.type, fields.type, "type")
        text(initial.server, fields.server, "server")
        number(initial.serverPort, fields.serverPort, "server_port")
        text(initial.uuid, fields.uuid, "uuid")
        patch(initial.flow, fields.flow, fields.flow.takeUnless { it == "none" || it.isEmpty() }?.let(::JsonPrimitive), "flow")
        text(initial.security, fields.security, "security")
        number(initial.alterId, fields.alterId, "alter_id")
        text(initial.method, fields.method, "method")
        text(initial.password, fields.password, "password")
        text(initial.plugin, fields.plugin, "plugin")
        text(initial.pluginOpts, fields.pluginOpts, "plugin_opts")
        number(initial.upMbps, fields.upMbps, "up_mbps")
        number(initial.downMbps, fields.downMbps, "down_mbps")
        if (initial.obfsType != fields.obfsType && fields.obfsType == "none") {
            outbound.remove("obfs")
        } else if (initial.obfsType != fields.obfsType) {
            val obfs = (outbound["obfs"] as? JsonObject)?.toMutableMap() ?: mutableMapOf()
            if (initial.obfsType == "gecko" && fields.obfsType != "gecko") {
                obfs.remove("min_packet_size")
                obfs.remove("max_packet_size")
            }
            obfs["type"] = JsonPrimitive(fields.obfsType)
            obfs["password"] = JsonPrimitive(fields.obfsPassword)
            outbound["obfs"] = JsonObject(obfs)
        } else {
            text(initial.obfsType, fields.obfsType, "obfs", "type")
            text(initial.obfsPassword, fields.obfsPassword, "obfs", "password")
        }
        list(initial.serverPorts, fields.serverPorts, ",", "server_ports")
        text(initial.hopInterval, fields.hopInterval, "hop_interval")
        text(initial.congestionControl, fields.congestionControl, "congestion_control")
        text(initial.udpRelayMode, fields.udpRelayMode, "udp_relay_mode")
        flag(initial.udpOverStream, fields.udpOverStream, "udp_over_stream")
        flag(initial.zeroRttHandshake, fields.zeroRttHandshake, "zero_rtt_handshake")
        text(initial.heartbeat, fields.heartbeat, "heartbeat")
        if (initial.transportType != fields.transportType && fields.transportType == "none") {
            outbound.remove("transport")
        } else if (initial.transportType != fields.transportType) {
            outbound["transport"] = switchTransport(outbound["transport"] as? JsonObject, initial.transportType, fields)
        } else {
            text(initial.transportType, fields.transportType, "transport", "type")
            text(initial.path, fields.path, "transport", "path")
            text(initial.serviceName, fields.serviceName, "transport", "service_name")
            if (fields.transportType == "ws") {
                val headers = (original.outbound()["transport"] as? JsonObject)?.get("headers") as? JsonObject
                val hostKey = headers?.keys?.firstOrNull { it.equals("host", ignoreCase = true) } ?: "Host"
                text(initial.host, fields.host, "transport", "headers", hostKey)
            } else if (fields.transportType == "httpupgrade") {
                text(initial.host, fields.host, "transport", "host")
            } else {
                list(initial.host, fields.host, ",", "transport", "host")
            }
        }
        flag(initial.tlsEnabled, fields.tlsEnabled, "tls", "enabled")
        text(initial.serverName, fields.serverName, "tls", "server_name")
        flag(initial.insecure, fields.insecure, "tls", "insecure")
        flag(initial.disableSni, fields.disableSni, "tls", "disable_sni")
        list(initial.alpn, fields.alpn, ",", "tls", "alpn")
        if (initial.fingerprint != fields.fingerprint) {
            outbound.patch(listOf("tls", "utls", "enabled"), JsonPrimitive(fields.fingerprint != "none"))
            if (fields.fingerprint != "none") {
                outbound.patch(listOf("tls", "utls", "fingerprint"), JsonPrimitive(fields.fingerprint))
            }
        }
        flag(initial.realityEnabled, fields.realityEnabled, "tls", "reality", "enabled")
        text(initial.realityPublicKey, fields.realityPublicKey, "tls", "reality", "public_key")
        text(initial.realityShortId, fields.realityShortId, "tls", "reality", "short_id")
        flag(initial.echEnabled, fields.echEnabled, "tls", "ech", "enabled")
        list(initial.echConfig, fields.echConfig, "\n", "tls", "ech", "config")
        text(initial.echQueryServerName, fields.echQueryServerName, "tls", "ech", "query_server_name")

        if (initial.type != fields.type) outbound.switchProtocol(initial.type, fields)

        val updated = JsonObject(outbound)
        val outbounds = original["outbounds"] as? JsonArray ?: return updated
        return JsonObject(original + ("outbounds" to JsonArray(listOf(updated) + outbounds.drop(1))))
    }

    companion object {
        fun parse(content: String): NodeEditDraft {
            val root = Json.parseToJsonElement(content) as JsonObject
            val outbound = root.outbound()
            val tls = outbound["tls"] as? JsonObject
            val transport = outbound["transport"] as? JsonObject
            val headers = transport?.get("headers") as? JsonObject
            val utls = tls?.get("utls") as? JsonObject
            val reality = tls?.get("reality") as? JsonObject
            val ech = tls?.get("ech") as? JsonObject
            val obfs = outbound["obfs"] as? JsonObject
            val fields = NodeEditFields(
                tag = outbound.text("tag"), type = outbound.text("type", "vless"),
                server = outbound.text("server"), serverPort = outbound.text("server_port"),
                uuid = outbound.text("uuid"), flow = outbound.text("flow", "none"),
                security = outbound.text("security", "auto"), alterId = outbound.text("alter_id"),
                method = outbound.text("method", "aes-128-gcm"), password = outbound.text("password"),
                plugin = outbound.text("plugin"), pluginOpts = outbound.text("plugin_opts"),
                upMbps = outbound.text("up_mbps"), downMbps = outbound.text("down_mbps"),
                obfsType = obfs.text("type", "none"), obfsPassword = obfs.text("password"),
                serverPorts = outbound["server_ports"].listableStrings().joinToString(","),
                hopInterval = outbound.text("hop_interval"),
                congestionControl = outbound.text("congestion_control", "cubic"),
                udpRelayMode = outbound.text("udp_relay_mode", "quic"),
                udpOverStream = outbound.flag("udp_over_stream"), zeroRttHandshake = outbound.flag("zero_rtt_handshake"),
                heartbeat = outbound.text("heartbeat"), transportType = transport.text("type", "none"),
                path = transport.text("path"),
                host = transport?.get("host").listableStrings().takeIf { it.isNotEmpty() }?.joinToString(",")
                    ?: headers?.entries?.firstOrNull { it.key.equals("host", ignoreCase = true) }?.value.listableStrings().joinToString(","),
                serviceName = transport.text("service_name"), tlsEnabled = tls.flag("enabled"),
                serverName = tls.text("server_name"), insecure = tls.flag("insecure"), disableSni = tls.flag("disable_sni"),
                alpn = tls?.get("alpn").listableStrings().joinToString(","),
                fingerprint = if (utls.flag("enabled")) utls.text("fingerprint", "chrome") else "none",
                realityEnabled = reality.flag("enabled"), realityPublicKey = reality.text("public_key"), realityShortId = reality.text("short_id"),
                echEnabled = ech.flag("enabled"), echConfig = ech?.get("config").listableStrings().joinToString("\n"),
                echQueryServerName = ech.text("query_server_name")
            )
            return NodeEditDraft(root, fields)
        }
    }
}

private val protocolFields = mapOf(
    "vless" to setOf("uuid", "flow", "network", "tls", "multiplex", "transport", "packet_encoding"),
    "vmess" to setOf("uuid", "security", "alter_id", "global_padding", "authenticated_length", "network", "tls", "packet_encoding", "multiplex", "transport"),
    "shadowsocks" to setOf("method", "password", "plugin", "plugin_opts", "network", "udp_over_tcp", "multiplex"),
    "trojan" to setOf("password", "network", "tls", "multiplex", "transport"),
    "anytls" to setOf("tls", "password", "idle_session_check_interval", "idle_session_timeout", "min_idle_session", "client_metadata", "disable_reuse"),
    "hysteria2" to setOf("server_ports", "hop_interval", "hop_interval_max", "up_mbps", "down_mbps", "obfs", "password", "network", "tls", "idle_timeout", "keep_alive_period", "stream_receive_window", "connection_receive_window", "max_concurrent_streams", "initial_packet_size", "disable_path_mtu_discovery", "bbr_profile", "brutal_debug", "disable_chrome_parrot", "realm"),
    "tuic" to setOf("uuid", "password", "congestion_control", "udp_relay_mode", "udp_over_stream", "zero_rtt_handshake", "heartbeat", "network", "tls", "idle_timeout", "keep_alive_period", "stream_receive_window", "connection_receive_window", "max_concurrent_streams", "initial_packet_size", "disable_path_mtu_discovery")
)

// 切换类型才移除原生旧类型专属键，共用键和未识别键不参与清理。
private fun MutableMap<String, JsonElement>.switchProtocol(before: String, fields: NodeEditFields) {
    val supported = protocolFields[fields.type].orEmpty()
    (protocolFields[before].orEmpty() - supported).forEach(::remove)
    fun required(key: String, value: String) { putIfAbsent(key, JsonPrimitive(value)) }
    fun optional(key: String, value: String) { if (value.isNotEmpty()) required(key, value) }
    fun number(key: String, value: String) {
        if (value.isNotBlank()) putIfAbsent(key, value.trim().toIntOrNull()?.let(::JsonPrimitive) ?: JsonPrimitive(value))
    }
    when (fields.type) {
        "vless", "vmess" -> {
            required("uuid", fields.uuid)
            if (fields.type == "vless") {
                if (fields.flow != "none") optional("flow", fields.flow)
            } else {
                required("security", fields.security)
                number("alter_id", fields.alterId)
            }
        }
        "shadowsocks" -> {
            required("method", fields.method)
            required("password", fields.password)
            optional("plugin", fields.plugin)
            optional("plugin_opts", fields.pluginOpts)
        }
        "trojan", "anytls" -> required("password", fields.password)
        "hysteria2" -> {
            required("password", fields.password)
            number("up_mbps", fields.upMbps)
            number("down_mbps", fields.downMbps)
            optional("hop_interval", fields.hopInterval)
            if (fields.serverPorts.isNotBlank()) putIfAbsent("server_ports", fields.serverPorts.stringList(","))
            if (fields.obfsType != "none") putIfAbsent("obfs", JsonObject(mapOf("type" to JsonPrimitive(fields.obfsType), "password" to JsonPrimitive(fields.obfsPassword))))
        }
        "tuic" -> {
            required("uuid", fields.uuid)
            required("password", fields.password)
            required("congestion_control", fields.congestionControl)
            required("udp_relay_mode", fields.udpRelayMode)
            putIfAbsent("udp_over_stream", JsonPrimitive(fields.udpOverStream))
            putIfAbsent("zero_rtt_handshake", JsonPrimitive(fields.zeroRttHandshake))
            optional("heartbeat", fields.heartbeat)
        }
    }
}

private val transportFields = mapOf(
    "http" to setOf("host", "path", "method", "headers", "idle_timeout", "ping_timeout"),
    "ws" to setOf("path", "headers", "max_early_data", "early_data_header_name"),
    "grpc" to setOf("service_name", "idle_timeout", "ping_timeout", "permit_without_stream"),
    "httpupgrade" to setOf("host", "path", "headers"),
    "quic" to emptySet()
)

private fun switchTransport(original: JsonObject?, before: String, fields: NodeEditFields): JsonObject {
    val transport = original?.toMutableMap() ?: mutableMapOf()
    (transportFields[before].orEmpty() - transportFields[fields.transportType].orEmpty()).forEach(transport::remove)
    transport["type"] = JsonPrimitive(fields.transportType)
    when (fields.transportType) {
        "ws", "http", "httpupgrade" -> {
            transport["path"] = JsonPrimitive(fields.path.ifBlank { "/" })
            when (fields.transportType) {
                "ws" -> {
                    val headers = (transport["headers"] as? JsonObject)?.toMutableMap() ?: mutableMapOf()
                    val key = headers.keys.firstOrNull { it.equals("host", ignoreCase = true) } ?: "Host"
                    if (fields.host.isNotEmpty()) headers[key] = JsonPrimitive(fields.host) else headers.remove(key)
                    if (headers.isEmpty()) transport.remove("headers") else transport["headers"] = JsonObject(headers)
                }
                "http" -> if (fields.host.isNotEmpty()) transport["host"] = fields.host.stringList(",") else transport.remove("host")
                "httpupgrade" -> if (fields.host.isNotEmpty()) transport["host"] = JsonPrimitive(fields.host) else transport.remove("host")
            }
        }
        "grpc" -> transport["service_name"] = JsonPrimitive(fields.serviceName)
    }
    return JsonObject(transport)
}

private fun String.stringList(separator: String): JsonArray =
    JsonArray(split(separator).map(String::trim).filter(String::isNotEmpty).map(::JsonPrimitive))

private fun JsonObject.outbound(): JsonObject = if (containsKey("outbounds")) {
    (get("outbounds") as JsonArray).first() as JsonObject
} else this

private fun JsonObject?.text(key: String, default: String = ""): String =
    (this?.get(key) as? JsonPrimitive)?.contentOrNull ?: default

private fun JsonObject?.flag(key: String): Boolean =
    (this?.get(key) as? JsonPrimitive)?.booleanOrNull == true

// 只沿实际修改的字段路径替换，未修改字段的缺省、Listable 形态和未知嵌套键均保留。
private fun MutableMap<String, JsonElement>.patch(path: List<String>, value: JsonElement?) {
    val key = path.first()
    if (path.size == 1) {
        if (value == null) remove(key) else put(key, value)
        return
    }
    val child = (get(key) as? JsonObject)?.toMutableMap() ?: mutableMapOf()
    child.patch(path.drop(1), value)
    if (child.isEmpty()) remove(key) else put(key, JsonObject(child))
}
