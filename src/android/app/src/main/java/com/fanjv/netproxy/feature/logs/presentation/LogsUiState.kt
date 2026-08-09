package com.fanjv.netproxy.feature.logs.presentation

import androidx.compose.runtime.Immutable
import com.fanjv.netproxy.feature.logs.data.LogItem

@Immutable
data class LogsUiState(
    val serviceLogs: List<LogItem> = emptyList(),
    val kernelLogs: List<LogItem> = emptyList(),
    val error: String = ""
)
