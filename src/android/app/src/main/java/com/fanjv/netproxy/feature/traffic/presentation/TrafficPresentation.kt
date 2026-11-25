package com.fanjv.netproxy.feature.traffic.presentation

import com.fanjv.netproxy.feature.traffic.data.CaptureRecord
import com.fanjv.netproxy.feature.traffic.data.CapturedHttp
import com.fanjv.netproxy.feature.traffic.data.RewriteMatch
import com.fanjv.netproxy.feature.traffic.data.RewriteRule
import java.security.MessageDigest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement

internal fun CaptureRecord.matchesSession(
    query: String, method: String?, statusClass: Int?, tls: String?,
    rewrittenOnly: Boolean = false, app: String? = null, outbound: String? = null,
): Boolean =
    (method == null || this.method == method) &&
        (!rewrittenOnly || rewritten) &&
        (app == null || appKey == app) &&
        (outbound == null || this.outbound == outbound || exit?.chain?.any { it.tag == outbound } == true) &&
        (statusClass == null || status / 100 == statusClass) &&
        (tls == null || tls == "decrypted" && this.tls == "decrypted" ||
            tls == "tunnel" && this.tls != "decrypted" && (this.tls != null || this.method == "TCP")) &&
        (query.isBlank() || "$host${target.orEmpty()}".contains(query.trim(), ignoreCase = true) ||
            "${if (this.tls == "decrypted") "https" else "http"}://$host${target.orEmpty()}".contains(query.trim(), ignoreCase = true))

private val prettyJson = Json { prettyPrint = true }

internal val com.fanjv.netproxy.feature.traffic.data.CaptureExit.appKey: String?
    get() = packageName ?: uid?.let { "UID $it" } ?: process

internal val CaptureRecord.appRef: String?
    get() = sourceApp?.let { source -> source.packageName?.let { "${source.userId}:$it" } }

internal val CaptureRecord.appKey: String?
    get() = appRef ?: sourceApp?.let { "UID ${it.uid}" } ?: exit?.appKey

internal enum class SessionGrouping { None, App, Domain, Outbound }

internal fun List<CaptureRecord>.groupSessions(grouping: SessionGrouping): Map<String?, List<CaptureRecord>> =
    groupBy { record ->
        when (grouping) {
            SessionGrouping.None -> null
            SessionGrouping.App -> record.appKey
            SessionGrouping.Domain -> record.host.sessionDomain()
            SessionGrouping.Outbound -> buildList {
                record.outbound.takeIf { it.isNotBlank() }?.let { add(it) }
                for (hop in record.exit?.chain.orEmpty()) {
                    if (hop.tag.isNotBlank() && lastOrNull() != hop.tag) add(hop.tag)
                }
            }.joinToString(" → ").ifBlank { null }
        }
    }

private fun String.sessionDomain(): String? {
    val value = trim()
    val host = when {
        value.startsWith('[') && ']' in value -> value.substringAfter('[').substringBefore(']')
        value.count { it == ':' } == 1 && value.substringAfter(':').toIntOrNull() != null -> value.substringBefore(':')
        else -> value
    }
    return host.trimEnd('.').lowercase().ifBlank { null }
}

internal fun List<RewriteRule>.indexOfCapture(match: RewriteMatch): Int? {
    fun RewriteRule.matches(): Boolean {
        val bytes = "$phase\u0000$pattern\u0000$script\u0000${domains.joinToString("\u0000")}".encodeToByteArray()
        val id = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }
        return id == match.id
    }
    if (getOrNull(match.ruleIndex)?.matches() == true) return match.ruleIndex
    return indices.filter { this[it].matches() }.singleOrNull()
}

internal fun isJsonBody(type: String?): Boolean {
    val mime = type?.substringBefore(';')?.trim()?.lowercase()
    return mime == "application/json" || mime?.endsWith("+json") == true
}

internal fun displayBody(data: String, type: String?): String {
    if (!isJsonBody(type)) return data
    return runCatching { prettyJson.encodeToString(JsonElement.serializer(), Json.parseToJsonElement(data)) }.getOrDefault(data)
}

internal fun bodyChunks(text: String): List<String> = buildList {
    var start = 0
    while (start < text.length) {
        val limit = minOf(start + 2048, text.length)
        var end = limit
        if (limit < text.length) {
            val newline = text.lastIndexOf('\n', limit - 1)
            if (newline >= start) end = newline + 1
            else if (text[limit].isLowSurrogate()) end--
        }
        add(text.substring(start, end))
        start = end
    }
}

internal fun asCurl(method: String, detail: CapturedHttp): String {
    fun quote(value: String) = "'" + value.replace("'", "'\"'\"'") + "'"
    val request = detail.request
    val body = request.body
    val args = buildList {
        add("curl --request ${quote(method)}")
        request.headers.forEach { (name, values) ->
            if (!name.equals("Content-Length", true) && !name.equals("Transfer-Encoding", true) &&
                !name.equals("Proxy-Authorization", true) && !(body.decoded && name.equals("Content-Encoding", true))) {
                values.forEach { add("--header ${quote("$name: $it")}") }
            }
        }
        if (body.data.isNotEmpty()) add(if (body.format == "base64") "--data-binary @-" else "--data-binary ${quote(body.data)}")
        add("-- ${quote(detail.url)}")
    }.joinToString(" ")
    val command = if (body.format == "base64" && body.data.isNotEmpty()) {
        "printf %s ${quote(body.data)} | base64 -d | $args"
    } else args
    return if (detail.urlTruncated || request.headersTruncated || body.truncated) "# Capture incomplete; replay may differ\n$command" else command
}
