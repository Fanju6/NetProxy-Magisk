package com.fanjv.netproxy.feature.logs.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.logs.data.LogRepository
import com.fanjv.netproxy.feature.logs.data.LogType
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.File

/** 通过 netproxyctl 读取、清理并导出脱敏日志。 */
internal class LogsViewModel(
    private val repository: LogRepository
) : ViewModel() {
    private val _state = MutableStateFlow(LogsUiState())
    val state: StateFlow<LogsUiState> = _state.asStateFlow()

    fun refresh(type: LogType) {
        viewModelScope.launch {
            runCatching {
                repository.read(type)
            }.onSuccess { logs ->
                _state.update {
                    when (type) {
                        LogType.SERVICE -> it.copy(serviceLogs = logs, error = "")
                        LogType.KERNEL -> it.copy(kernelLogs = logs, error = "")
                    }
                }
            }.onFailure { error ->
                _state.update { it.copy(error = error.userMessage()) }
            }
        }
    }

    fun clear(type: LogType, onResult: (Boolean) -> Unit = {}) {
        viewModelScope.launch {
            runCatching { repository.clear(type) }
                .onSuccess {
                    _state.update { state ->
                        when (type) {
                            LogType.SERVICE -> state.copy(serviceLogs = emptyList(), error = "")
                            LogType.KERNEL -> state.copy(kernelLogs = emptyList(), error = "")
                        }
                    }
                    onResult(true)
                }
                .onFailure { error ->
                    _state.update { it.copy(error = error.userMessage()) }
                    onResult(false)
                }
        }
    }

    suspend fun createReport(): File {
        return repository.createReport()
    }

}
