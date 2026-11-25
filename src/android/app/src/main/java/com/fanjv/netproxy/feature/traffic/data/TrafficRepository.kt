package com.fanjv.netproxy.feature.traffic.data

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.ModuleApiException
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonObject

@Serializable
internal data class CaptureStatus(val running: Boolean, val count: Int)

@Serializable
internal data class CaptureRecord(
    val time: String,
    val method: String,
    val host: String,
    val outbound: String,
    val status: Int,
    val rewrite: String? = null,
    val tls: String? = null,
    @SerialName("duration_ms") val durationMs: Long,
    val id: String? = null,
    val target: String? = null,
    val rewritten: Boolean = false,
    val exit: CaptureExit? = null,
)

@Serializable
internal data class CaptureExit(
    val inbound: String? = null,
    val chain: List<CaptureHop> = emptyList(),
    val remote: String? = null,
    @SerialName("package") val packageName: String? = null,
    val uid: Int? = null,
    val process: String? = null,
)

@Serializable
internal data class CaptureHop(val tag: String, val type: String)

@Serializable
internal data class CaptureDetail(val http: CapturedHttp? = null)

@Serializable
internal data class CapturedHttp(
    val url: String,
    @SerialName("url_truncated") val urlTruncated: Boolean,
    val request: CapturedMessage,
    val response: CapturedMessage,
    val rewrites: List<RewriteMatch> = emptyList(),
)

@Serializable
internal data class RewriteMatch(
    val id: String,
    @SerialName("rule_index") val ruleIndex: Int,
    val pattern: String,
    val phase: String,
    val state: String,
    val actions: List<String> = emptyList(),
    val redirect: String? = null,
    @SerialName("original_status") val originalStatus: Int? = null,
    val headers: List<HeaderChange> = emptyList(),
    val body: RewriteBodyChange? = null,
    val error: String? = null,
    @SerialName("duration_us") val durationUs: Long = 0,
)

@Serializable
internal data class HeaderChange(
    val name: String,
    val before: List<String> = emptyList(),
    val after: List<String> = emptyList(),
    val truncated: Boolean,
)

@Serializable
internal data class RewriteBodyChange(val before: Int, val after: Int)

@Serializable
internal data class CapturedMessage(
    val headers: Map<String, List<String>>,
    @SerialName("headers_truncated") val headersTruncated: Boolean,
    val body: CapturedBody,
)

@Serializable
internal data class CapturedBody(
    val data: String,
    val format: String,
    @SerialName("content_type") val contentType: String? = null,
    @SerialName("content_encoding") val contentEncoding: String? = null,
    val truncated: Boolean,
    val size: Long? = null,
    @SerialName("captured_size") val capturedSize: Int? = null,
)

@Serializable
internal data class RewriteRule(val pattern: String, val phase: String, val script: String)

@Serializable
private data class RewriteRules(val rules: List<RewriteRule>)

@Serializable
internal data class MitmPolicy(
    val enabled: Boolean,
    val hosts: List<String>,
    @SerialName("system_trust") val systemTrust: Boolean,
)

@Serializable
internal data class CaptureCertificate(val pem: String, val filename: String, val sha256: String) {
    fun isPublicCertificate(): Boolean =
        Regex("[a-f0-9]{8}\\.0").matches(filename) &&
            Regex("[a-fA-F0-9]{64}").matches(sha256) &&
            Regex("-----BEGIN CERTIFICATE-----\\r?\\n[A-Za-z0-9+/=\\r\\n]+-----END CERTIFICATE-----")
                .matches(pem.trim())
}

internal class TrafficRepository(private val api: ModuleApiClient) {
    suspend fun status(): CaptureStatus = api.read("/traffic/status")
    suspend fun records(): List<CaptureRecord> = api.read("/traffic/list")
    suspend fun detail(id: String): CaptureDetail = api.read("/traffic/detail", mapOf("id" to id))
    suspend fun rules(): List<RewriteRule> = api.read<RewriteRules>("/traffic/rules").rules
    suspend fun policy(): MitmPolicy = api.read("/traffic/mitm")

    suspend fun setCapture(enabled: Boolean): CaptureStatus =
        api.json.decodeFromJsonElement(api.post(if (enabled) "/traffic/start" else "/traffic/stop").data)

    suspend fun clear(): CaptureStatus = api.json.decodeFromJsonElement(api.delete("/traffic/list").data)

    suspend fun saveRules(expected: List<RewriteRule>, updated: List<RewriteRule>): List<RewriteRule> {
        if (rules() != expected) throw TrafficChangedException()
        api.put("/traffic/rules", api.json.encodeToJsonElement(RewriteRules(updated)).jsonObject)
        return updated
    }

    suspend fun savePolicy(expected: MitmPolicy, updated: MitmPolicy): MitmPolicy {
        if (policy() != expected) throw TrafficChangedException()
        return api.json.decodeFromJsonElement(api.put("/traffic/mitm", api.json.encodeToJsonElement(updated).jsonObject).data)
    }

    fun persistedPolicy(error: ModuleApiException): MitmPolicy? {
        if (error.resultCode != "traffic.trust_remove_incomplete") return null
        return runCatching { api.json.decodeFromJsonElement<MitmPolicy>(error.data) }.getOrNull()
    }

    suspend fun certificate(initialize: Boolean = true): CaptureCertificate {
        val cert = if (initialize) api.json.decodeFromJsonElement<CaptureCertificate>(api.post("/traffic/ca-init").data)
            else api.read<CaptureCertificate>("/traffic/ca")
        if (!cert.isPublicCertificate()) throw ModuleApiException("traffic.ca_invalid", "")
        return cert
    }
}

internal class TrafficChangedException : IllegalStateException()
