package com.fanjv.netproxy.feature.traffic.data

import android.content.Context
import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.ModuleApiException
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import java.io.File

@Serializable
internal data class CaptureStatus(val running: Boolean, val count: Int)

@Serializable
internal data class CapturePreview(val running: Boolean, val count: Int, val entries: List<CaptureRecord>)

@Serializable
internal data class CaptureBatch(
    val id: String,
    @SerialName("started_at") val startedAt: String,
    val count: Int,
    val running: Boolean,
)

@Serializable
internal data class CapturePage(
    val entries: List<CaptureRecord>,
    val batch: String? = null,
    val before: String? = null,
    val since: String? = null,
    val oldest: String? = null,
)

@Serializable
internal data class CaptureDomain(val host: String, val count: Int)

@Serializable
internal data class PinnedBypass(
    val host: String,
    @SerialName("user_id") val userId: Int,
    @SerialName("package") val packageName: String,
)

@Serializable
internal data class AlpnBypass(
    val host: String,
    @SerialName("user_id") val userId: Int,
    @SerialName("package") val packageName: String,
    @SerialName("expires_at") val expiresAt: String,
)

@Serializable
internal data class CaptureRecord(
    val time: String,
    val method: String,
    val host: String,
    val outbound: String,
    val status: Int,
    val rewrite: String? = null,
    val tls: String? = null,
    @SerialName("passthrough_reason") val passthroughReason: String? = null,
    @SerialName("ca_stores") val caStores: CaptureCaStores? = null,
    @SerialName("duration_ms") val durationMs: Long,
    val id: String? = null,
    val target: String? = null,
    val rewritten: Boolean = false,
    val exit: CaptureExit? = null,
    @SerialName("source_app") val sourceApp: CaptureSourceApp? = null,
    val protocol: String? = null,
    val sequence: Long? = null,
    @SerialName("replay_of") val replayOf: String? = null,
    @SerialName("replay_original_exit") val replayOriginalExit: CaptureExit? = null,
    @SerialName("replay_route_changed") val replayRouteChanged: Boolean = false,
    @SerialName("replay_unattributed") val replayUnattributed: Boolean = false,
)

@Serializable
internal data class CaptureReplayEdit(
    val method: String,
    val url: String,
    val headers: Map<String, List<String>>,
    val body: String,
    @SerialName("body_base64") val bodyBase64: Boolean = false,
)

@Serializable
private data class CaptureReplayInput(val id: String, val edit: CaptureReplayEdit? = null)

@Serializable
private data class CaptureReplayResult(val id: String)

@Serializable
internal data class CaptureCaStores(val user: String, val system: String)

@Serializable
internal data class CaptureSourceApp(
    val uid: Int,
    @SerialName("user_id") val userId: Int,
    @SerialName("package") val packageName: String? = null,
)

@Serializable
internal data class CaptureApps(
    val apps: List<String> = emptyList(),
    val mode: String? = null,
    val blacklist: List<String> = emptyList(),
    val whitelist: List<String> = apps,
    @SerialName("disable_quic") val disableQuic: Boolean? = null,
) {
    val effectiveMode: String get() = mode ?: if (apps.isEmpty()) "all" else "whitelist"
    val selected: List<String> get() = when (effectiveMode) {
        "blacklist" -> blacklist
        "whitelist" -> whitelist
        else -> emptyList()
    }

    fun withSelection(selection: Set<String>): CaptureApps = when (effectiveMode) {
        "blacklist" -> copy(blacklist = selection.sorted())
        "whitelist" -> copy(whitelist = selection.sorted())
        else -> this
    }
}

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
    val websocket: WebSocketDetail? = null,
)

@Serializable
internal data class WebSocketDetail(
    val frames: List<WebSocketFrame>,
    val truncated: Boolean,
    val messages: List<WebSocketFrame> = emptyList(),
    @SerialName("messages_truncated") val messagesTruncated: Boolean = false,
)

@Serializable
internal data class WebSocketFrame(
    val direction: String,
    val opcode: Int,
    val size: Long,
    val text: String? = null,
    val truncated: Boolean,
    @SerialName("close_code") val closeCode: Int? = null,
    val compressed: Boolean = false,
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
    val trailers: Map<String, List<String>> = emptyMap(),
    @SerialName("trailers_truncated") val trailersTruncated: Boolean = false,
    val grpc: GrpcDetail? = null,
)

@Serializable
internal data class GrpcDetail(
    val messages: List<GrpcMessage>,
    val truncated: Boolean = false,
    val status: String? = null,
    val message: String? = null,
)

@Serializable
internal data class GrpcMessage(
    val size: Long,
    val compressed: Boolean = false,
    val truncated: Boolean = false,
    val fields: List<ProtoField> = emptyList(),
)

@Serializable
internal data class ProtoField(
    val number: Int,
    val wire: Int,
    val value: String? = null,
    val fields: List<ProtoField> = emptyList(),
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
    val decoded: Boolean = false,
    val blob: Boolean = false,
    @SerialName("content_range") val contentRange: String? = null,
    val filename: String? = null,
)

@Serializable
internal data class RewriteRule(
    val pattern: String,
    val phase: String,
    val script: String,
    val domains: List<String> = emptyList(),
    val enabled: Boolean = true,
    val name: String = "",
)

@Serializable
private data class RewriteRules(val rules: List<RewriteRule>)

@Serializable
internal data class MitmPolicy(
    val enabled: Boolean,
    val hosts: List<String>,
    @SerialName("system_trust") val systemTrust: Boolean,
    @SerialName("bypass_hosts") val bypassHosts: List<String> = emptyList(),
)

@Serializable
internal data class CaptureCertificate(val pem: String, val filename: String, val sha256: String) {
    fun isPublicCertificate(): Boolean =
        Regex("[a-f0-9]{8}\\.0").matches(filename) &&
            Regex("[a-fA-F0-9]{64}").matches(sha256) &&
            Regex("-----BEGIN CERTIFICATE-----\\r?\\n[A-Za-z0-9+/=\\r\\n]+-----END CERTIFICATE-----")
                .matches(pem.trim())
}

internal class TrafficRepository(private val api: ModuleApiClient, private val cache: CapturePageCache? = null) {
    constructor(api: ModuleApiClient, context: Context) :
        this(api, CapturePageCache(File(context.applicationContext.cacheDir, "traffic-pages"), api.json))

    suspend fun status(): CaptureStatus = api.read("/traffic/status")
    suspend fun preview(): CapturePreview = api.read("/traffic/preview")
    suspend fun records(batch: String? = null): List<CaptureRecord> =
        api.read("/traffic/list", if (batch == null) emptyMap() else mapOf("batch" to batch))
    suspend fun page(batch: String? = null, before: String? = null, since: String? = null, oldest: String? = null,
        host: String? = null): CapturePage {
        if (host == null && batch != null && before != null && oldest != null) {
            cache?.read(batch, oldest, before)?.let { return it }
        }
        val page = api.read<CapturePage>("/traffic/page", buildMap {
            batch?.let { put("batch", it) }
            before?.let { put("before", it) }
            since?.let { put("since", it) }
            host?.let { put("host", it) }
        })
        if (host == null && batch != null && before != null && page.batch == batch && page.oldest != null) {
            cache?.write(batch, page.oldest, before, page)
        }
        return page
    }
    suspend fun domains(batch: String? = null): List<CaptureDomain> =
        api.read("/traffic/domains", if (batch == null) emptyMap() else mapOf("batch" to batch))
    suspend fun batches(): List<CaptureBatch> = api.read<List<CaptureBatch>>("/traffic/batches").also { batches ->
        cache?.retain(batches.mapTo(mutableSetOf()) { it.id })
    }
    suspend fun deleteBatch(id: String) {
        api.delete("/traffic/batches", mapOf("id" to id))
        cache?.invalidate(id)
    }
    suspend fun detail(id: String): CaptureDetail = api.read("/traffic/detail", mapOf("id" to id))
    suspend fun replay(id: String, edit: CaptureReplayEdit?): CaptureRecord {
        val payload = if (edit == null) buildJsonObject {
            put("id", id)
        } else api.json.encodeToJsonElement(CaptureReplayInput(id, edit)).jsonObject
        val result = api.json.decodeFromJsonElement<CaptureReplayResult>(api.post("/traffic/replay", payload).data)
        return api.read("/traffic/detail", mapOf("id" to result.id))
    }
    suspend fun downloadBody(id: String, side: String, destination: File, size: Int) {
        api.download("/traffic/body", mapOf("id" to id, "side" to side), destination)
        require(destination.length() == size.toLong())
    }
    suspend fun rules(): List<RewriteRule> = api.read<RewriteRules>("/traffic/rules").rules
    suspend fun policy(): MitmPolicy = api.read("/traffic/mitm")
    suspend fun pinned(): List<PinnedBypass> = api.read("/traffic/pinned")
    suspend fun clearPinned(): List<PinnedBypass> = api.json.decodeFromJsonElement(api.delete("/traffic/pinned").data)
    suspend fun alpnBypasses(): List<AlpnBypass> = api.read("/traffic/alpn-bypasses")
    suspend fun clearAlpnBypasses(): List<AlpnBypass> = api.json.decodeFromJsonElement(api.delete("/traffic/alpn-bypasses").data)
    suspend fun apps(): CaptureApps = api.captureApps()
    suspend fun saveApps(policy: CaptureApps): CaptureApps = api.json.decodeFromJsonElement(
        api.saveCaptureApps(api.json.encodeToJsonElement(policy.copy(apps = emptyList(), mode = policy.effectiveMode)).jsonObject).data,
    )

    suspend fun setCapture(enabled: Boolean): CaptureStatus =
        api.json.decodeFromJsonElement(api.post(if (enabled) "/traffic/start" else "/traffic/stop").data)

    suspend fun clear(): CaptureStatus =
        api.json.decodeFromJsonElement<CaptureStatus>(api.delete("/traffic/list").data).also { cache?.invalidate() }

    suspend fun saveRules(expected: List<RewriteRule>, updated: List<RewriteRule>): List<RewriteRule> {
        if (rules() != expected) throw TrafficChangedException()
        api.put("/traffic/rules", api.json.encodeToJsonElement(RewriteRules(updated)).jsonObject)
        return rules()
    }

    suspend fun savePolicy(expected: MitmPolicy, updated: MitmPolicy): MitmPolicy {
        if (policy() != expected) throw TrafficChangedException()
        return api.json.decodeFromJsonElement(api.put("/traffic/mitm", api.json.encodeToJsonElement(updated).jsonObject).data)
    }

    fun persistedPolicy(error: ModuleApiException): MitmPolicy? {
        if (error.resultCode != "traffic.trust_remove_incomplete" && error.resultCode != "traffic.user_trust_incomplete") return null
        return runCatching { api.json.decodeFromJsonElement<MitmPolicy>(error.data) }.getOrNull()
    }

    suspend fun removeUserCertificate() {
        api.delete("/traffic/ca/trust")
    }

    suspend fun certificate(initialize: Boolean = true): CaptureCertificate {
        val cert = if (initialize) api.json.decodeFromJsonElement<CaptureCertificate>(api.post("/traffic/ca-init").data)
            else api.read<CaptureCertificate>("/traffic/ca")
        if (!cert.isPublicCertificate()) throw ModuleApiException("traffic.ca_invalid", "")
        return cert
    }
}

internal class TrafficChangedException : IllegalStateException()
