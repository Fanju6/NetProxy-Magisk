package com.fanjv.netproxy.feature.dashboard.presentation

import com.fanjv.netproxy.core.module.ServiceStatusSnapshot

internal data class TrafficSample(
    val timestampMillis: Long,
    val downloadBytesPerSecond: Long,
    val uploadBytesPerSecond: Long
)

internal data class TrafficTimelineState(
    val samples: List<TrafficSample> = emptyList(),
    val downloadBytesPerSecond: Long = 0,
    val uploadBytesPerSecond: Long = 0
)

/** 将累计流量转换为有界的速率时间序列，首个样本只建立基线。 */
internal class TrafficTimelineReducer(
    private val capacity: Int = 60,
    private val maxGapMillis: Long = 30_000
) {
    private var previousDownload: Long? = null
    private var previousUpload: Long? = null
    private var previousAt = 0L
    private val samples = ArrayDeque<TrafficSample>()

    fun reset() {
        previousDownload = null
        previousUpload = null
        previousAt = 0L
        samples.clear()
    }

    fun reduce(service: ServiceStatusSnapshot, fallbackNowMillis: Long): TrafficTimelineState {
        if (service.state != "ready") {
            reset()
            return TrafficTimelineState()
        }

        val sampledAt = fallbackNowMillis
        val elapsed = sampledAt - previousAt
        val currentDownload = service.downloadTotal.coerceAtLeast(0)
        val currentUpload = service.uploadTotal.coerceAtLeast(0)
        val hasBaseline = previousDownload != null && previousUpload != null
        val validDelta = hasBaseline &&
            elapsed in 1..maxGapMillis &&
            currentDownload >= previousDownload!! &&
            currentUpload >= previousUpload!!
        val downloadRate = if (validDelta) {
            (currentDownload - previousDownload!!) * 1000 / elapsed
        } else {
            0
        }
        val uploadRate = if (validDelta) {
            (currentUpload - previousUpload!!) * 1000 / elapsed
        } else {
            0
        }

        previousDownload = currentDownload
        previousUpload = currentUpload
        previousAt = sampledAt
        samples.addLast(
            TrafficSample(
                timestampMillis = sampledAt,
                downloadBytesPerSecond = downloadRate,
                uploadBytesPerSecond = uploadRate
            )
        )
        while (samples.size > capacity) samples.removeFirst()
        return TrafficTimelineState(
            samples = samples.toList(),
            downloadBytesPerSecond = downloadRate,
            uploadBytesPerSecond = uploadRate
        )
    }
}
