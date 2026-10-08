package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.UiTextException
import com.fanjv.netproxy.core.ui.toUiText
import com.fanjv.netproxy.feature.catalog.data.SubscriptionRepository
import com.fanjv.netproxy.feature.catalog.model.SubscriptionDraft
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
import kotlinx.serialization.json.JsonElement

internal class SubscriptionEditorViewModel(
    private val repository: SubscriptionRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
) : ViewModel(scope) {
    private val _state = MutableStateFlow(SubscriptionEditorUiState())
    val state: StateFlow<SubscriptionEditorUiState> = _state.asStateFlow()
    private var loadJob: Job? = null
    private var generation = 0L
    private var routeId: String? = null

    fun load(id: String) {
        if (_state.value.saving) return
        if (routeId == id && (id.isBlank() || _state.value.original != null || loadJob?.isActive == true)) return
        loadJob?.cancel()
        val request = ++generation
        if (routeId != id) _state.value = SubscriptionEditorUiState(id = id)
        routeId = id
        if (id.isBlank()) return
        val snapshot = _state.value
        _state.update { it.copy(loading = true, saved = false) }
        loadJob = viewModelScope.launch {
            try {
                val editor = repository.readEditor(id)
                currentCoroutineContext().ensureActive()
                if (request == generation) _state.update { it.rebase(snapshot, editor) }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                currentCoroutineContext().ensureActive()
                if (request == generation) _state.update {
                    it.copy(loading = false, error = error.toUiText(), noticeId = it.noticeId + 1)
                }
            } finally {
                if (request == generation) _state.update { it.copy(loading = false) }
            }
        }
    }

    fun update(transform: (SubscriptionDraft) -> SubscriptionDraft) {
        _state.update { it.copy(draft = transform(it.draft), saved = false) }
    }

    fun updateHeaders(value: String) {
        _state.update { it.copy(headersText = value, saved = false) }
    }

    fun save() {
        if (_state.value.saving || _state.value.loading) return
        loadJob?.cancel()
        val request = ++generation
        var snapshot = _state.value
        _state.update { it.copy(saving = true, saved = false) }
        viewModelScope.launch {
            var commandError: Exception? = null
            try {
                if (snapshot.baselineReadRequired || (snapshot.id.isNotBlank() && snapshot.original == null)) {
                    if (snapshot.id.isBlank()) throw UiTextException(UiText.Resource(R.string.subscription_update_failed))
                    val editor = repository.readEditor(snapshot.id)
                    currentCoroutineContext().ensureActive()
                    if (request != generation) return@launch
                    snapshot = snapshot.recoverBaseline(editor)
                    _state.update { it.recoverBaseline(editor) }
                }
                val draft = validate(snapshot.draft, snapshot.headersText, isNew = snapshot.id.isBlank())
                _state.update { it.copy(error = UiText.Empty) }
                val data: JsonElement = try {
                    if (snapshot.id.isBlank()) {
                        check(!snapshot.persisted)
                        repository.add(draft)
                    } else {
                        repository.edit(snapshot.id, requireNotNull(snapshot.original), draft)
                    }
                } catch (error: CancellationException) {
                    throw error
                } catch (error: Exception) {
                    if (!error.subscriptionPersisted()) throw error
                    commandError = error
                    (error as NetProxyCtlException).data
                }
                currentCoroutineContext().ensureActive()
                if (request != generation) return@launch
                val runtime = data.subscriptionOutcome()
                val committedId = runtime.groupId.ifBlank { snapshot.id }
                _state.update {
                    it.copy(
                        id = committedId,
                        baselineReadRequired = true,
                        persisted = runtime.persisted || it.persisted,
                        runtimeSyncState = runtime.state,
                        runtimeSyncPending = runtime.pending,
                        error = commandError?.toUiText() ?: UiText.Empty,
                        noticeId = it.noticeId + if (commandError != null) 1 else 0
                    )
                }
                if (committedId.isBlank()) throw UiTextException(UiText.Resource(R.string.subscription_update_failed))
                val editor = repository.readEditor(committedId)
                currentCoroutineContext().ensureActive()
                if (request == generation) _state.update {
                    val unchanged = it.sameInput(snapshot)
                    it.rebase(snapshot, editor).copy(saved = commandError == null && unchanged)
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                currentCoroutineContext().ensureActive()
                if (request == generation) _state.update {
                    it.copy(
                        error = commandError?.toUiText() ?: error.toUiText(),
                        noticeId = it.noticeId + 1
                    )
                }
            } finally {
                if (request == generation) _state.update { it.copy(saving = false) }
            }
        }
    }

    fun clearError() {
        _state.update { it.copy(error = UiText.Empty) }
    }

    private fun validate(
        draft: SubscriptionDraft,
        headersText: String,
        isNew: Boolean,
    ): SubscriptionDraft {
        // 新增订阅允许留空名称，由模块按 Profile-Title、文件名、主机名顺序自动取名；
        // 编辑既有订阅时清空名称属于误操作，仍需拒绝
        if (!isNew && draft.name.isBlank()) {
            throw UiTextException(UiText.Resource(R.string.subscription_name_empty))
        }
        if (draft.url.isBlank()) {
            throw UiTextException(UiText.Resource(R.string.subscription_url_empty))
        }
        if (draft.updateIntervalSeconds < 900) {
            throw UiTextException(UiText.Resource(R.string.subscription_interval_invalid))
        }
        if (draft.timeoutSeconds <= 0) {
            throw UiTextException(UiText.Resource(R.string.subscription_timeout_invalid))
        }
        return draft.copy(
            name = draft.name.trim(),
            url = draft.url.trim(),
            customHeaders = parseHeaders(headersText)
        )
    }

    private fun parseHeaders(text: String): Map<String, String> = buildMap {
        text.lineSequence().forEachIndexed { index, raw ->
            val line = raw.trim()
            if (line.isEmpty()) return@forEachIndexed
            val separator = line.indexOf(':')
            if (separator <= 0) {
                throw UiTextException(
                    UiText.Resource(R.string.subscription_header_invalid, listOf(index + 1))
                )
            }
            val name = line.substring(0, separator).trim()
            val value = line.substring(separator + 1).trim()
            if (name.isEmpty() || value.isEmpty()) {
                throw UiTextException(
                    UiText.Resource(R.string.subscription_header_empty, listOf(index + 1))
                )
            }
            put(name, value)
        }
    }
}
