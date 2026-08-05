package com.fanjv.netproxy.feature.dashboard.presentation

import com.fanjv.netproxy.core.module.ServiceStatusSnapshot
import org.junit.Assert.assertEquals
import org.junit.Test

class DashboardSnapshotReducerTest {
    @Test
    fun `computes rates and resource usage from consecutive samples`() {
        val reducer = DashboardSnapshotReducer(totalMemoryBytes = 2_000)
        val initial = CatalogDashboardUiState()
        reducer.reduce(initial, service(download = 1_000, upload = 500), 1_000, "10.0.0.1")

        val state = reducer.reduce(
            initial,
            service(
                download = 3_000,
                upload = 1_500,
                processTicks = 30,
                systemTicks = 200,
                memory = 500
            ),
            3_000,
            "10.0.0.2"
        )

        assertEquals(1_000, state.downloadBytesPerSecond)
        assertEquals(500, state.uploadBytesPerSecond)
        assertEquals(40f, state.cpuUsage)
        assertEquals(25f, state.memoryUsage)
        assertEquals("10.0.0.2", state.internalIp)
    }

    @Test
    fun `uses local ready time until the process changes`() {
        val reducer = DashboardSnapshotReducer(totalMemoryBytes = 1)
        reducer.markStarted(100)

        val first = reducer.reduce(
            CatalogDashboardUiState(),
            service(pid = 10, readyAt = 95),
            101_000,
            "--"
        )
        val restarted = reducer.reduce(
            first,
            service(pid = 11, readyAt = 105),
            106_000,
            "--"
        )

        assertEquals(100, first.readyAt)
        assertEquals(1, first.uptimeSeconds)
        assertEquals(105, restarted.readyAt)
        assertEquals(1, restarted.uptimeSeconds)
    }

    private fun service(
        pid: Int = 10,
        readyAt: Long = 1,
        download: Long = 0,
        upload: Long = 0,
        processTicks: Long = 10,
        systemTicks: Long = 100,
        memory: Long = 0
    ) = ServiceStatusSnapshot(
        state = "ready",
        pid = pid,
        readyAt = readyAt,
        downloadTotal = download,
        uploadTotal = upload,
        processCpuTicks = processTicks,
        systemCpuTicks = systemTicks,
        cpuCount = 2,
        memoryBytes = memory,
        activeGroupNodeCount = 1
    )
}
