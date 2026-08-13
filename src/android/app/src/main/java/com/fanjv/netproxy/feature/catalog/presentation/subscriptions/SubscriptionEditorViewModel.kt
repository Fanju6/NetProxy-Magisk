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
import com.fanjv.netproxy.feature.catalog.model.SubscriptionEditorState
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.jsonObject

internal data class SubscriptionEditorUiState(
    val id: String = "",
    val original: SubscriptionEditorState? = null,
    val draft: SubscriptionDraft = SubscriptionDraft(name = "", url = ""),
    val headersText: String = "",
    val loading: Boolean = false,
    val saving: Boolean = false,
    val saved: Boolean = false,
    val persisted: Boolean = false,
    val runtimeSyncState: String = "",
    val runtimeSyncPending: Boolean = false,
    val error: UiText = UiText.Empty,
    val noticeId: Long = 0
)

/** 管理订阅新增和编辑事务，避免编辑状态泄漏到列表或详情页面。 */
internal class SubscriptionEditorViewModel(
    private val repository: SubscriptionRepository
) : ViewModel() {
    private val _state = MutableStateFlow(SubscriptionEditorUiState())
    val state: StateFlow<SubscriptionEditorUiState> = _state.asStateFlow()

    fun load(id: String) {
        if (id.isBlank()) {
            _state.value = SubscriptionEditorUiState()
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(loading = true, saved = false, error = UiText.Empty) }
            runCatching { repository.readEditor(id) }
                .onSuccess { editor ->
                    _state.value = SubscriptionEditorUiState(
                        id = id,
                        original = editor,
                        draft = editor.toDraft(),
                        headersText = editor.customHeaders.entries.joinToString("\n") { (key, value) ->
                            "$key: ${value.jsonPrimitive.content}"
                        }
                    )
                }
                .onFailure { error ->
                    _state.update {
                        it.copy(
                            loading = false,
                            error = error.toUiText(),
                            noticeId = it.noticeId + 1
                        )
                    }
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
        if (_state.value.saving) return
        viewModelScope.launch {
            val snapshot = _state.value
            val draft = runCatching {
                validate(snapshot.draft, snapshot.headersText, isNew = snapshot.original == null)
            }
                .getOrElse { error ->
                    _state.update {
                        it.copy(
                            error = error.toUiText(),
                            noticeId = it.noticeId + 1
                        )
                    }
                    return@launch
                }
            _state.update { it.copy(saving = true, error = UiText.Empty, saved = false) }
            runCatching {
                val original = snapshot.original
                if (original == null) repository.add(draft)
                else repository.edit(snapshot.id, original, draft)
            }.onSuccess { data ->
                val runtime = data.runtimeSyncOutcome()
                _state.update {
                    it.copy(
                        saving = false,
                        draft = draft,
                        saved = true,
                        persisted = runtime.persisted,
                        runtimeSyncState = runtime.state,
                        runtimeSyncPending = runtime.pending
                    )
                }
            }.onFailure { error ->
                val runtime = (error as? NetProxyCtlException)?.data?.runtimeSyncOutcome()
                _state.update {
                    it.copy(
                        saving = false,
                        persisted = runtime?.persisted ?: it.persisted,
                        runtimeSyncState = runtime?.state ?: it.runtimeSyncState,
                        runtimeSyncPending = runtime?.pending ?: it.runtimeSyncPending,
                        error = error.toUiText(),
                        noticeId = it.noticeId + 1
                    )
                }
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

    private data class RuntimeSyncOutcome(
        val persisted: Boolean,
        val state: String,
        val pending: Boolean
    )

    private fun JsonElement.runtimeSyncOutcome(): RuntimeSyncOutcome {
        val objectValue = jsonObject
        return RuntimeSyncOutcome(
            persisted = objectValue["persisted"]?.jsonPrimitive?.booleanOrNull == true,
            state = objectValue["runtime_sync_state"]?.jsonPrimitive?.content.orEmpty(),
            pending = objectValue["runtime_sync_pending"]?.jsonPrimitive?.booleanOrNull == true
        )
    }

    private fun SubscriptionEditorState.toDraft() = SubscriptionDraft(
        name = name,
        url = url,
        userAgent = userAgent,
        hwid = hwid,
        customHeaders = customHeaders.mapValues { it.value.jsonPrimitive.content },
        autoUpdate = autoUpdate,
        updateIntervalSeconds = updateInterval,
        updateViaProxy = updateViaProxy,
        include = include,
        exclude = exclude,
        allowInsecure = allowInsecure,
        timeoutSeconds = timeout
    )
}
