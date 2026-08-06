package com.fanjv.netproxy.core.module

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
internal data class ServiceStatusSnapshot(
    val state: String = "stopped",
    val pid: Int? = null,
    @SerialName("started_at") val startedAt: Long = 0,
    @SerialName("ready_at") val readyAt: Long = 0,
    @SerialName("uptime_seconds") val uptimeSeconds: Long = 0,
    val error: String = "",
    @SerialName("outbound_mode") val outboundMode: String = "rule",
    @SerialName("selector_mode") val selectorMode: String = "urltest",
    @SerialName("active_group_id") val activeGroupId: String = "",
    @SerialName("active_group_name") val activeGroupName: String = "",
    @SerialName("active_group_node_count") val activeGroupNodeCount: Int = 0,
    @SerialName("selected_node_ref") val selectedNodeRef: String = "",
    @SerialName("runtime_selected") val runtimeSelected: String = "",
    @SerialName("memory_bytes") val memoryBytes: Long = 0,
    @SerialName("process_cpu_ticks") val processCpuTicks: Long = 0,
    @SerialName("system_cpu_ticks") val systemCpuTicks: Long = 0,
    @SerialName("cpu_count") val cpuCount: Int = 1,
    @SerialName("connections_in") val connectionsIn: Int = 0,
    @SerialName("connections_out") val connectionsOut: Int = 0,
    @SerialName("upload_total") val uploadTotal: Long = 0,
    @SerialName("download_total") val downloadTotal: Long = 0,
    @SerialName("subscription_worker") val subscriptionWorker: String = "stopped",
    @SerialName("subscription_worker_pid") val subscriptionWorkerPid: Int? = null
)
