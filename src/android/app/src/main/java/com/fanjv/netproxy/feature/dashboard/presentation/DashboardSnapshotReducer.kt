package com.fanjv.netproxy.feature.dashboard.presentation

import com.fanjv.netproxy.core.module.ServiceStatusSnapshot

/** 将服务快照归并为仪表盘状态。 */
internal class DashboardSnapshotReducer(
    private val totalMemoryBytes: Long
) {
    private var lastCpuSample: Pair<Long, Long>? = null

    fun reduce(
        current: CatalogDashboardUiState,
        service: ServiceStatusSnapshot,
        nowMillis: Long,
        localAddress: String
    ): CatalogDashboardUiState {
        val nowSeconds = nowMillis / 1000
        val displayedReadyAt = service.readyAt
        val displayedUptime = if (displayedReadyAt > 0 && service.state == "ready") {
            (nowSeconds - displayedReadyAt).coerceAtLeast(0)
        } else {
            service.uptimeSeconds
        }
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
        val traffic = service.downloadTotal to service.uploadTotal

        if (service.state == "ready") {
            lastCpuSample = cpuSample
        } else {
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
            downloadTotal = traffic.first,
            uploadTotal = traffic.second,
            cpuUsage = cpuUsage,
            memoryUsage = memoryUsage,
            internalIp = localAddress
        )
    }
}
