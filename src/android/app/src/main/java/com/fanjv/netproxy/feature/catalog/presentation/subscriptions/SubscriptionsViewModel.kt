package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.toUiText
import com.fanjv.netproxy.feature.catalog.data.SubscriptionRepository
import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

internal data class SubscriptionsUiState(
    val groups: List<CatalogGroupSummary> = emptyList(),
    val loading: Boolean = false,
    val loadFailed: Boolean = false,
    val operation: String = "",
    val operationGroupId: String = "",
    val error: UiText = UiText.Empty,
    val notice: UiText = UiText.Empty,
    val noticeId: Long = 0
)

/** 管理订阅列表及列表级更新、启用和删除操作。 */
internal class SubscriptionsViewModel(
    private val repository: SubscriptionRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
) : ViewModel(scope) {
    private val _state = MutableStateFlow(SubscriptionsUiState())
    val state: StateFlow<SubscriptionsUiState> = _state.asStateFlow()
    private var visible = false
    private var loaded = false
    private var refreshJob: Job? = null
    private var refreshGeneration = 0L

    fun setVisible(value: Boolean) {
        visible = value
        if (value) refresh(silent = loaded)
        else {
            ++refreshGeneration
            refreshJob?.cancel()
            _state.update { it.copy(loading = false) }
        }
    }

    fun refresh(silent: Boolean = false) {
        if (_state.value.operation.isNotEmpty()) return
        refreshJob?.cancel()
        val request = ++refreshGeneration
        if (!silent) _state.update { it.copy(loading = true) }
        refreshJob = viewModelScope.launch {
            try {
                val groups = repository.list()
                currentCoroutineContext().ensureActive()
                if (request == refreshGeneration) {
                    _state.update { it.copy(groups = groups, loading = false, loadFailed = false) }
                    loaded = true
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                currentCoroutineContext().ensureActive()
                if (request == refreshGeneration) _state.update {
                    it.copy(
                        loading = false,
                        loadFailed = true,
                        error = if (it.error == UiText.Empty) error.toUiText() else it.error,
                        noticeId = it.noticeId + 1
                    )
                }
            } finally {
                if (request == refreshGeneration) _state.update { it.copy(loading = false) }
            }
        }
    }

    fun updateSubscription(id: String) = runOperation("update", id) {
        repository.update(id)
        UiText.Resource(R.string.subscription_updated)
    }

    fun updateAll() = runOperation("update-all", "*") {
        repository.updateAll()
        UiText.Resource(R.string.subscription_update_all_success)
    }

    fun activate(id: String) = runOperation("activate", id) {
        repository.activate(id)
        UiText.Resource(R.string.subscription_activated)
    }

    fun remove(id: String, replacement: String = "") = runOperation("remove", id) {
        repository.remove(id, replacement)
        UiText.Resource(R.string.subscription_deleted)
    }

    fun clearNotice() {
        _state.update { it.copy(notice = UiText.Empty, error = UiText.Empty) }
    }

    private fun runOperation(
        operation: String,
        groupId: String,
        action: suspend () -> UiText
    ) {
        if (_state.value.operation.isNotEmpty()) return
        ++refreshGeneration
        refreshJob?.cancel()
        _state.update {
            it.copy(operation = operation, operationGroupId = groupId, loading = false, error = UiText.Empty)
        }
        viewModelScope.launch {
            try {
                val message = action()
                currentCoroutineContext().ensureActive()
                _state.update { it.copy(notice = message, noticeId = it.noticeId + 1) }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                currentCoroutineContext().ensureActive()
                _state.update { it.copy(error = error.toUiText(), noticeId = it.noticeId + 1) }
            } finally {
                _state.update { it.copy(operation = "", operationGroupId = "") }
            }
            if (visible) refresh(silent = true)
        }
    }
}
