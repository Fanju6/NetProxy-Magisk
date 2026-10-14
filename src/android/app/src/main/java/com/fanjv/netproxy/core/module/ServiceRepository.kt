package com.fanjv.netproxy.core.module

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.ModuleApiException
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.coroutines.flow.map
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** 代理服务生命周期、模式与运行状态的数据入口。 */
internal class ServiceRepository(
    private val client: ModuleApiClient
) {
    suspend fun status(): ServiceStatusSnapshot =
        client.read("/service")

    fun observe() = client.observe(setOf("status", "traffic")).map { result -> result.mapCatching { events ->
        val status = events.getValue("status").decode<ServiceStatusSnapshot>()
        val traffic = events.getValue("traffic").decode<ServiceTrafficSnapshot>()
        if (status.state != "ready" || status.coreStartedAtMillis <= 0 ||
            !traffic.available || traffic.startedAtMillis != status.coreStartedAtMillis) status
        else {
            val core = traffic.status
            if (core == null || traffic.processCpuTicks == null || traffic.systemCpuTicks == null || traffic.cpuCount == null) {
                throw ModuleApiException("api.invalid_response", "模块返回的数据格式无效")
            }
            status.copy(memoryBytes = core.memory,
                connectionsIn = core.connectionsIn,
                connectionsOut = core.connectionsOut,
                uploadTotal = core.uplinkTotal,
                downloadTotal = core.downlinkTotal,
                processCpuTicks = traffic.processCpuTicks,
                systemCpuTicks = traffic.systemCpuTicks,
                cpuCount = traffic.cpuCount)
        }
    } }

    suspend fun action(action: String) {
        require(action in setOf("start", "stop", "restart", "reload"))
        client.post("/service/$action")
    }

    suspend fun setMode(mode: String) {
        require(mode.isNotBlank())
        client.put("/mode", buildJsonObject { put("mode", mode) })
    }
}

@Serializable
private data class ServiceTrafficSnapshot(
    val available: Boolean = false,
    @SerialName("started_at_millis") val startedAtMillis: Long = 0,
    val status: ServiceTrafficStatus? = null,
    @SerialName("process_cpu_ticks") val processCpuTicks: Long? = null,
    @SerialName("system_cpu_ticks") val systemCpuTicks: Long? = null,
    @SerialName("cpu_count") val cpuCount: Int? = null,
)

@Serializable
private data class ServiceTrafficStatus(
    val memory: Long,
    @SerialName("connections_in") val connectionsIn: Int,
    @SerialName("connections_out") val connectionsOut: Int,
    @SerialName("uplink_total") val uplinkTotal: Long,
    @SerialName("downlink_total") val downlinkTotal: Long,
)
