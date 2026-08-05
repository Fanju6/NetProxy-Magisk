package com.fanjv.netproxy.feature.dashboard.presentation

import com.fanjv.netproxy.core.module.ServiceStatusSnapshot
import org.junit.Assert.assertEquals
import org.junit.Test

class CatalogDashboardNodeNameTest {
    @Test
    fun `start and stop operations expose distinct service transitions`() {
        val starting = CatalogDashboardUiState(serviceState = "stopped", operation = "start")
        val stopping = CatalogDashboardUiState(serviceState = "ready", operation = "stop")

        assertEquals(true, starting.isStarting)
        assertEquals(false, starting.isStopping)
        assertEquals(false, stopping.isStarting)
        assertEquals(true, stopping.isStopping)
        assertEquals(false, starting.isReady)
        assertEquals(false, stopping.isReady)
    }

    @Test
    fun `mode operation keeps ready service status`() {
        val state = CatalogDashboardUiState(serviceState = "ready", operation = "mode")

        assertEquals(false, state.isStarting)
        assertEquals(false, state.isStopping)
        assertEquals(true, state.isServiceControlBusy)
        assertEquals(true, state.isReady)
    }

    @Test
    fun `service transition invalidates in-flight dashboard snapshots`() {
        assertEquals(false, shouldApplyDashboardSnapshot(1, 2, ""))
        assertEquals(false, shouldApplyDashboardSnapshot(2, 2, "stop"))
        assertEquals(false, shouldApplyDashboardSnapshot(2, 2, "start"))
        assertEquals(true, shouldApplyDashboardSnapshot(2, 2, "mode"))
    }

    @Test
    fun `automatic group reference falls back to Auto-Fastest`() {
        val status = status(
            selectorMode = "urltest",
            runtimeSelected = "Auto/group-id"
        )

        assertEquals("Auto-Fastest", dashboardNodeName(status))
    }

    @Test
    fun `automatic mode displays runtime node tag`() {
        val status = status(
            selectorMode = "urltest",
            runtimeSelected = "香港 01"
        )

        assertEquals("香港 01", dashboardNodeName(status))
    }

    @Test
    fun `manual group reference falls back to configured node tag`() {
        val status = status(
            selectorMode = "manual",
            runtimeSelected = "Select/group-id",
            selectedNodeRef = "group-id/日本 02"
        )

        assertEquals("日本 02", dashboardNodeName(status))
    }

    private fun status(
        selectorMode: String,
        runtimeSelected: String,
        selectedNodeRef: String = ""
    ) = ServiceStatusSnapshot(
        state = "ready",
        selectorMode = selectorMode,
        activeGroupId = "group-id",
        activeGroupNodeCount = 2,
        selectedNodeRef = selectedNodeRef,
        runtimeSelected = runtimeSelected
    )
}
