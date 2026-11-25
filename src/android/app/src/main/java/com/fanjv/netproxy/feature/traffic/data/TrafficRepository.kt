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

    suspend fun certificate(): CaptureCertificate {
        val cert = api.json.decodeFromJsonElement<CaptureCertificate>(api.post("/traffic/ca-init").data)
        if (!cert.isPublicCertificate()) throw ModuleApiException("traffic.ca_invalid", "")
        return cert
    }
}

internal class TrafficChangedException : IllegalStateException()
