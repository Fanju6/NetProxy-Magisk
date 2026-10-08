package com.fanjv.netproxy.feature.logs.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.logs.data.LogRepository
import com.fanjv.netproxy.feature.logs.data.LogType
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.File

/** 通过 netproxyctl 读取、清理并导出脱敏日志。 */
internal class LogsViewModel(
    private val repository: LogRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
) : ViewModel(scope) {
    private val _state = MutableStateFlow(LogsUiState())
    val state: StateFlow<LogsUiState> = _state.asStateFlow()
    private val reads = mutableMapOf<LogType, Job>()
    private val revisions = mutableMapOf<LogType, Long>()

    private fun invalidate(type: LogType): Long {
        reads.remove(type)?.cancel()
        val revision = (revisions[type] ?: 0) + 1
        revisions[type] = revision
        return revision
    }

    fun refresh(type: LogType) {
        val revision = invalidate(type)
        reads[type] = viewModelScope.launch {
            runCatching {
                repository.read(type)
            }.onSuccess { logs ->
                currentCoroutineContext().ensureActive()
                if (revisions[type] != revision) return@onSuccess
                _state.update {
                    when (type) {
                        LogType.SERVICE -> it.copy(serviceLogs = logs, error = "")
                        LogType.KERNEL -> it.copy(kernelLogs = logs, error = "")
                    }
                }
            }.onFailure { error ->
                if (error is CancellationException) throw error
                if (revisions[type] != revision) return@onFailure
                _state.update { it.copy(error = error.userMessage()) }
            }
        }
    }

    fun clear(type: LogType, onResult: (Boolean) -> Unit = {}) {
        val revision = invalidate(type)
        viewModelScope.launch {
            runCatching { repository.clear(type) }
                .onSuccess {
                    currentCoroutineContext().ensureActive()
                    invalidate(type)
                    _state.update { state ->
                        when (type) {
                            LogType.SERVICE -> state.copy(serviceLogs = emptyList(), error = "")
                            LogType.KERNEL -> state.copy(kernelLogs = emptyList(), error = "")
                        }
                    }
                    onResult(true)
                }
                .onFailure { error ->
                    if (error is CancellationException) throw error
                    if (revisions[type] != revision) return@onFailure
                    _state.update { it.copy(error = error.userMessage()) }
                    onResult(false)
                }
        }
    }

    suspend fun createReport(): File {
        return repository.createReport()
    }

}
