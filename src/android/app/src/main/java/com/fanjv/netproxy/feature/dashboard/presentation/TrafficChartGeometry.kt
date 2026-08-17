package com.fanjv.netproxy.feature.dashboard.presentation

import androidx.compose.ui.geometry.Offset

internal data class TrafficChartGeometry(
    val download: List<Offset>,
    val upload: List<Offset>
)

internal fun trafficChartGeometry(samples: List<TrafficSample>): TrafficChartGeometry {
    if (samples.isEmpty()) {
        val baseline = listOf(Offset(0f, 0f), Offset(1f, 0f))
        return TrafficChartGeometry(baseline, baseline)
    }
    val maxSpeed = maxOf(
        samples.maxOf { it.downloadBytesPerSecond },
        samples.maxOf { it.uploadBytesPerSecond },
        1L
    ).toFloat() * 1.15f
    return TrafficChartGeometry(
        download = normalizedSeries(samples.map { it.downloadBytesPerSecond }, maxSpeed),
        upload = normalizedSeries(samples.map { it.uploadBytesPerSecond }, maxSpeed)
    )
}

internal fun interpolateTrafficChartGeometry(
    from: TrafficChartGeometry,
    to: TrafficChartGeometry,
    progress: Float
): TrafficChartGeometry {
    val fraction = progress.coerceIn(0f, 1f)
    if (fraction >= 1f) return to
    return TrafficChartGeometry(
        download = interpolateSeries(from.download, to.download, fraction),
        upload = interpolateSeries(from.upload, to.upload, fraction)
    )
}

private fun interpolateSeries(
    from: List<Offset>,
    to: List<Offset>,
    progress: Float
): List<Offset> = to.mapIndexed { index, target ->
    val start = from.getOrNull(index) ?: return@mapIndexed target
    Offset(
        x = lerp(start.x, target.x, progress),
        y = lerp(start.y, target.y, progress)
    )
}

private fun lerp(start: Float, end: Float, progress: Float): Float =
    start + (end - start) * progress

private fun normalizedSeries(values: List<Long>, maxSpeed: Float): List<Offset> {
    if (values.isEmpty()) return listOf(Offset(0f, 0f), Offset(1f, 0f))
    if (values.size == 1) {
        val y = values.first().toFloat() / maxSpeed
        return listOf(Offset(0f, y), Offset(1f, y))
    }
    val divisor = (values.lastIndex).toFloat()
    return values.mapIndexed { index, value ->
        Offset(index / divisor, (value.toFloat() / maxSpeed).coerceIn(0f, 1f))
    }
}
