package com.fanjv.netproxy.feature.traffic.data

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
internal data class CaptureBreakpointRule(
    val domains: List<String> = emptyList(),
    val pattern: String = "",
    val phase: String = "request",
    val enabled: Boolean = true,
    val name: String = "",
)

@Serializable
internal data class CaptureBreakpointConfig(
    @SerialName("timeout_seconds") val timeoutSeconds: Int = 60,
    val rules: List<CaptureBreakpointRule>,
)

@Serializable
internal data class CapturePause(
    val id: String,
    val phase: String,
    val url: String,
    val method: String,
    val status: Int = 0,
    val headers: Map<String, List<String>> = emptyMap(),
    val body: String? = null,
    val deadline: String = "",
)

@Serializable
internal data class CaptureBreakpointEdit(
    val method: String? = null,
    val url: String? = null,
    val status: Int? = null,
    val headers: Map<String, List<String>>? = null,
    val body: String? = null,
)

@Serializable
internal data class CaptureBreakpointAudit(
    val rule: String = "",
    val phase: String,
    val outcome: String,
    val actor: String = "",
    val changes: List<String> = emptyList(),
    @SerialName("origin_changed") val originChanged: Boolean = false,
    @SerialName("original_exit") val originalExit: CaptureExit? = null,
    @SerialName("actual_exit") val actualExit: CaptureExit? = null,
    @SerialName("route_changed") val routeChanged: Boolean = false,
    @SerialName("unattributed") val unattributed: Boolean = false,
    @SerialName("route_failure") val routeFailure: String = "",
)
