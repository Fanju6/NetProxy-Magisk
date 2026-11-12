package com.fanjv.netproxy.feature.dashboard.presentation

import org.junit.Assert.assertEquals
import org.junit.Test

class CatalogDashboardStateTest {
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
    fun `refresh mode and service operations invalidate in-flight dashboard snapshots`() {
        assertEquals(false, shouldApplyDashboardSnapshot(1, 2, ""))
        assertEquals(false, shouldApplyDashboardSnapshot(2, 2, "stop"))
        assertEquals(false, shouldApplyDashboardSnapshot(2, 2, "start"))
        assertEquals(false, shouldApplyDashboardSnapshot(2, 2, "mode"))
        assertEquals(true, shouldApplyDashboardSnapshot(2, 2, ""))
    }

}
