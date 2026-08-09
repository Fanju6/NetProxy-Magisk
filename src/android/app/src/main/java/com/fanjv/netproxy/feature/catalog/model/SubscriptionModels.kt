package com.fanjv.netproxy.feature.catalog.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject

@Serializable
internal data class SubscriptionEditorState(
    val id: String,
    val name: String,
    val url: String,
    @SerialName("user_agent") val userAgent: String = "",
    val hwid: String = "",
    @SerialName("custom_headers") val customHeaders: JsonObject = JsonObject(emptyMap()),
    @SerialName("auto_update") val autoUpdate: Boolean = true,
    @SerialName("update_interval") val updateInterval: Long = 86400,
    @SerialName("interval_source") val intervalSource: String = "default",
    @SerialName("update_via_proxy") val updateViaProxy: String = "auto",
    val include: String = "",
    val exclude: String = "",
    @SerialName("allow_insecure") val allowInsecure: Boolean = false,
    val timeout: Int = 60
)

internal data class SubscriptionDraft(
    val name: String,
    val url: String,
    val userAgent: String = "",
    val hwid: String = "",
    val customHeaders: Map<String, String> = emptyMap(),
    val autoUpdate: Boolean = true,
    val updateIntervalSeconds: Long = 86400,
    val updateViaProxy: String = "auto",
    val include: String = "",
    val exclude: String = "",
    val allowInsecure: Boolean = false,
    val timeoutSeconds: Int = 60
)

@Serializable
internal data class SubscriptionHistoryEntry(
    @SerialName("at") val time: String = "",
    val ok: Boolean = true,
    val code: String = "",
    val message: String = "",
    @SerialName("node_count") val nodeCount: Int? = null,
    val revision: Long? = null
)

