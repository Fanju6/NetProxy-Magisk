package com.fanjv.netproxy.core.ui

import kotlin.math.roundToInt

internal fun formatBytes(bytes: Long): String {
    if (bytes < 1024L) return "$bytes B"
    val units = arrayOf("KB", "MB", "GB", "TB")
    var value = bytes.toDouble()
    var index = -1
    do {
        value /= 1024.0
        index++
    } while (value >= 1024.0 && index < units.lastIndex)
    return if (value >= 100) "${value.roundToInt()} ${units[index]}"
    else "%.1f %s".format(value, units[index])
}
