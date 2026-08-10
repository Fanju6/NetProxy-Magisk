package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.catalog.data.SubscriptionRepository
import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

internal data class SubscriptionsUiState(
    val groups: List<CatalogGroupSummary> = emptyList(),
    val loading: Boolean = false,
    val operation: String = "",
    val operationGroupId: String = "",
    val error: String = "",
    val notice: String = "",
    val noticeId: Long = 0
)

/** 管理订阅列表及列表级更新、启用和删除操作。 */
internal class SubscriptionsViewModel(
    private val repository: SubscriptionRepository
) : ViewModel() {
    private val _state = MutableStateFlow(SubscriptionsUiState())
    val state: StateFlow<SubscriptionsUiState> = _state.asStateFlow()
    private var visible = false
    private var loaded = false
    private var refreshJob: Job? = null
    private var refreshPending = false

    fun setVisible(value: Boolean) {
        visible = value
        if (value) refresh(silent = loaded)
    }

    fun refresh(silent: Boolean = false) {
        if (refreshJob?.isActive == true) {
            refreshPending = true
            return
        }
        refreshJob = viewModelScope.launch {
            try {
                if (!silent) _state.update { it.copy(loading = true, error = "") }
                runCatching { repository.list() }
                    .onSuccess { groups ->
                        _state.update { it.copy(groups = groups, loading = false) }
                        loaded = true
                    }
                    .onFailure { error ->
                        _state.update {
                            it.copy(
                                loading = false,
                                error = error.userMessage(),
                                noticeId = it.noticeId + 1
                            )
                        }
                    }
            } finally {
                refreshJob = null
                if (refreshPending && isActive) {
                    refreshPending = false
                    refresh(silent = true)
                }
            }
        }
    }

    fun updateSubscription(id: String) = runOperation("update", id) {
        repository.update(id)
        "订阅更新完成"
    }

    fun updateAll() = runOperation("update-all", "*") {
        repository.updateAll()
        "全部订阅更新完成"
    }

    fun activate(id: String) = runOperation("activate", id) {
        repository.activate(id)
        "已启用该配置"
    }

    fun remove(id: String, replacement: String = "") = runOperation("remove", id) {
        repository.remove(id, replacement)
        "订阅已删除"
    }

    fun cancelUpdate(id: String) = runOperation("cancel", id) {
        repository.cancelUpdate(id)
        "已请求取消更新"
    }

    fun clearNotice() {
        _state.update { it.copy(notice = "", error = "") }
    }

    private fun runOperation(
        operation: String,
        groupId: String,
        action: suspend () -> String
    ) {
        if (_state.value.operation.isNotEmpty()) return
        viewModelScope.launch {
            _state.update {
                it.copy(operation = operation, operationGroupId = groupId, error = "")
            }
            runCatching { action() }
                .onSuccess { message ->
                    _state.update {
                        it.copy(
                            operation = "",
                            operationGroupId = "",
                            notice = message,
                            noticeId = it.noticeId + 1
                        )
                    }
                    if (visible) refresh(silent = true)
                }
                .onFailure { error ->
                    _state.update {
                        it.copy(
                            operation = "",
                            operationGroupId = "",
                            error = error.userMessage(),
                            noticeId = it.noticeId + 1
                        )
                    }
                    if (visible) refresh(silent = true)
                }
        }
    }
}
