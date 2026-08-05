package com.fanjv.netproxy.feature.dashboard.presentation

import com.fanjv.netproxy.core.module.ServiceStatusSnapshot
import kotlin.math.max

/** 将连续服务采样归并为仪表盘状态，并持有速率计算所需的最小历史。 */
internal class DashboardSnapshotReducer(
    private val totalMemoryBytes: Long
) {
    private var lastTraffic: Pair<Long, Long>? = null
    private var lastTrafficAt = 0L
    private var lastCpuSample: Pair<Long, Long>? = null
    private var localReadyAt: Long? = null
    private var localReadyPid: Int? = null

    fun markStarted(readyAt: Long) {
        localReadyAt = readyAt
        localReadyPid = null
    }

    fun clearReadyOverride() {
        localReadyAt = null
        localReadyPid = null
    }

    fun reduce(
        current: CatalogDashboardUiState,
        service: ServiceStatusSnapshot,
        nowMillis: Long,
        localAddress: String
    ): CatalogDashboardUiState {
        val nowSeconds = nowMillis / 1000
        if (service.state != "ready") {
            clearReadyOverride()
        } else if (localReadyAt != null) {
            if (localReadyPid == null) {
                localReadyPid = service.pid
            } else if (localReadyPid != service.pid) {
                clearReadyOverride()
            }
        }

        val displayedReadyAt = localReadyAt ?: service.readyAt
        val displayedUptime = if (displayedReadyAt > 0 && service.state == "ready") {
            (nowSeconds - displayedReadyAt).coerceAtLeast(0)
        } else {
            service.uptimeSeconds
        }
        val traffic = service.downloadTotal to service.uploadTotal
        val elapsed = (nowMillis - lastTrafficAt).coerceAtLeast(1)
        val previous = lastTraffic
        val downRate = if (previous != null && lastTrafficAt > 0) {
            max(0, traffic.first - previous.first) * 1000 / elapsed
        } else 0
        val upRate = if (previous != null && lastTrafficAt > 0) {
            max(0, traffic.second - previous.second) * 1000 / elapsed
        } else 0
        val cpuSample = service.processCpuTicks to service.systemCpuTicks
        val previousCpu = lastCpuSample
        val cpuUsage = if (previousCpu != null) {
            val processDelta = (cpuSample.first - previousCpu.first).coerceAtLeast(0)
            val systemDelta = (cpuSample.second - previousCpu.second).coerceAtLeast(0)
            if (systemDelta > 0) {
                (processDelta.toDouble() / systemDelta * service.cpuCount.coerceAtLeast(1) * 100)
                    .toFloat()
            } else 0f
        } else 0f
        val memoryUsage = if (totalMemoryBytes > 0) {
            service.memoryBytes.toFloat() / totalMemoryBytes.toFloat() * 100f
        } else 0f

        if (service.state == "ready") {
            lastTraffic = traffic
            lastTrafficAt = nowMillis
            lastCpuSample = cpuSample
        } else {
            lastTraffic = null
            lastTrafficAt = 0
            lastCpuSample = null
        }

        return current.copy(
            loading = false,
            serviceState = service.state,
            serviceError = service.error,
            readyAt = displayedReadyAt,
            uptimeSeconds = displayedUptime,
            outboundMode = service.outboundMode,
            activeGroupId = service.activeGroupId,
            currentNode = dashboardNodeName(service),
            downloadBytesPerSecond = downRate,
            uploadBytesPerSecond = upRate,
            downloadTotal = traffic.first,
            uploadTotal = traffic.second,
            cpuUsage = cpuUsage,
            memoryUsage = memoryUsage,
            downloadHistory = appendHistory(current.downloadHistory, downRate),
            uploadHistory = appendHistory(current.uploadHistory, upRate),
            internalIp = localAddress
        )
    }

    private fun appendHistory(history: List<Float>, value: Long): List<Float> =
        (history + value.toFloat()).takeLast(40)
}
