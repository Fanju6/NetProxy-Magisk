package com.fanjv.netproxy.feature.dashboard.presentation

import com.fanjv.netproxy.core.module.ServiceStatusSnapshot
import org.junit.Assert.assertEquals
import org.junit.Test

class DashboardRegionSelectionTest {
    @Test fun regionalAutoKeepsRegionInRunningAndStoppedLabels() {
        val selected = ServiceStatusSnapshot(
            state = "ready", activeGroupNodeCount = 2,
            activeGroupName = "Source", activeGroupRuntimeTag = "Source",
            activeRegion = "jp", runtimeSelected = "Source/Tokyo",
        )
        assertEquals("Tokyo · JP · Auto", dashboardNodeName(selected))
        assertEquals("Source/JP · Auto", dashboardNodeName(selected.copy(state = "stopped")))
        assertEquals("Tokyo · Auto-Fastest", dashboardNodeName(selected.copy(activeRegion = "")))
    }
}
