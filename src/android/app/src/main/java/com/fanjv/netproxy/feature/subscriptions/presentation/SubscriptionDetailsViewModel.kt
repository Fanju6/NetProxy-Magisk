package com.fanjv.netproxy.feature.subscriptions.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.feature.subscriptions.data.SubscriptionRepository
import com.fanjv.netproxy.feature.subscriptions.model.SubscriptionHistoryEntry
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

internal data class SubscriptionDetailsUiState(
    val details: CatalogNodeGroup? = null,
    val history: List<SubscriptionHistoryEntry> = emptyList(),
    val loading: Boolean = false,
    val operation: String = "",
    val error: String = "",
    val notice: String = "",
    val noticeId: Long = 0
)

/** 管理单个订阅的节点摘要、历史和详情页操作。 */
internal class SubscriptionDetailsViewModel(
    private val repository: SubscriptionRepository
) : ViewModel() {
    private val _state = MutableStateFlow(SubscriptionDetailsUiState())
    val state: StateFlow<SubscriptionDetailsUiState> = _state.asStateFlow()

    fun load(id: String) {
        viewModelScope.launch {
            _state.update { it.copy(loading = true, error = "", details = null) }
            runCatching {
                val details = repository.details(id)
                val history = if (details.group.type == "subscription") {
                    repository.history(id)
                } else {
                    emptyList()
                }
                details to history
            }.onSuccess { (details, history) ->
                _state.update {
                    it.copy(details = details, history = history, loading = false)
                }
            }.onFailure { error ->
                _state.update { it.copy(loading = false, error = error.userMessage()) }
            }
        }
    }

    fun update(id: String) = runOperation("update", id) {
        repository.update(id)
        "订阅更新完成"
    }

    fun activate(id: String) = runOperation("activate", id) {
        repository.activate(id)
        "已启用该订阅"
    }

    fun remove(id: String, onRemoved: () -> Unit) = runOperation("remove", id) {
        repository.remove(id)
        onRemoved()
        "订阅已删除"
    }

    fun clearNotice() {
        _state.update { it.copy(notice = "", error = "") }
    }

    private fun runOperation(
        operation: String,
        id: String,
        action: suspend () -> String
    ) {
        if (_state.value.operation.isNotEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(operation = operation, error = "") }
            runCatching { action() }
                .onSuccess { message ->
                    _state.update {
                        it.copy(operation = "", notice = message, noticeId = it.noticeId + 1)
                    }
                    if (operation != "remove") load(id)
                }
                .onFailure { error ->
                    _state.update {
                        it.copy(
                            operation = "",
                            error = error.userMessage(),
                            noticeId = it.noticeId + 1
                        )
                    }
                }
        }
    }
}
