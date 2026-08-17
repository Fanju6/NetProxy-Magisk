package com.fanjv.netproxy.feature.dashboard.presentation

import androidx.compose.ui.geometry.Offset
import com.fanjv.netproxy.core.module.ServiceStatusSnapshot
import org.junit.Assert.assertEquals
import org.junit.Test

class TrafficTimelineReducerTest {
    @Test
    fun `first sample creates a visible baseline`() {
        val reducer = TrafficTimelineReducer()
        val state = reducer.reduce(service(1000, 500), 1000)

        assertEquals(1, state.samples.size)
        assertEquals(0, state.downloadBytesPerSecond)
        assertEquals(0, state.uploadBytesPerSecond)
    }

    @Test
    fun `second sample calculates rates from elapsed time`() {
        val reducer = TrafficTimelineReducer()
        reducer.reduce(service(1000, 500), 1000)
        val state = reducer.reduce(service(3000, 1500), 2000)

        assertEquals(2000, state.downloadBytesPerSecond)
        assertEquals(1000, state.uploadBytesPerSecond)
    }

    @Test
    fun `counter reset starts a new baseline`() {
        val reducer = TrafficTimelineReducer()
        reducer.reduce(service(1000, 500), 1000)
        reducer.reduce(service(3000, 1500), 2000)
        val state = reducer.reduce(service(10, 20), 3000)

        assertEquals(0, state.downloadBytesPerSecond)
        assertEquals(0, state.uploadBytesPerSecond)
    }

    @Test
    fun `timeline is bounded`() {
        val reducer = TrafficTimelineReducer(capacity = 3)
        repeat(5) { index ->
            reducer.reduce(service(index * 100L, 0), (index + 1) * 1000L)
        }

        assertEquals(3, reducer.reduce(service(600, 0), 6000).samples.size)
    }

    @Test
    fun `geometry stretches a short history across the chart`() {
        val geometry = trafficChartGeometry(
            listOf(TrafficSample(1, 0, 0), TrafficSample(2, 100, 50))
        )

        assertEquals(0f, geometry.download.first().x)
        assertEquals(1f, geometry.download.last().x)
    }

    @Test
    fun `geometry interpolation follows the target frame`() {
        val from = TrafficChartGeometry(
            download = listOf(Offset(0f, 0f), Offset(1f, 0.2f)),
            upload = listOf(Offset(0f, 0.4f), Offset(1f, 0.6f))
        )
        val to = TrafficChartGeometry(
            download = listOf(Offset(0f, 0.4f), Offset(0.5f, 0.8f), Offset(1f, 1f)),
            upload = listOf(Offset(0f, 0.2f), Offset(1f, 0.1f))
        )

        val halfway = interpolateTrafficChartGeometry(from, to, 0.5f)

        assertEquals(3, halfway.download.size)
        assertEquals(0.2f, halfway.download[0].y)
        assertEquals(to.download[2], halfway.download[2])
        assertEquals(to, interpolateTrafficChartGeometry(from, to, 1f))
    }

    private fun service(download: Long, upload: Long) =
        ServiceStatusSnapshot(
            state = "ready",
            downloadTotal = download,
            uploadTotal = upload
        )
}
