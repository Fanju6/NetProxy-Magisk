package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.catalog.data.SubscriptionRepository
import com.fanjv.netproxy.feature.catalog.model.SubscriptionDraft
import com.fanjv.netproxy.feature.catalog.model.SubscriptionEditorState
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.json.jsonPrimitive

internal data class SubscriptionEditorUiState(
    val id: String = "",
    val original: SubscriptionEditorState? = null,
    val draft: SubscriptionDraft = SubscriptionDraft(name = "", url = ""),
    val headersText: String = "",
    val loading: Boolean = false,
    val saving: Boolean = false,
    val saved: Boolean = false,
    val error: String = "",
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
            _state.update { it.copy(loading = true, saved = false, error = "") }
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
                            error = error.userMessage(),
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
                            error = error.userMessage(),
                            noticeId = it.noticeId + 1
                        )
                    }
                    return@launch
                }
            _state.update { it.copy(saving = true, error = "", saved = false) }
            runCatching {
                val original = snapshot.original
                if (original == null) repository.add(draft)
                else repository.edit(snapshot.id, original, draft)
            }.onSuccess {
                _state.update { it.copy(saving = false, draft = draft, saved = true) }
            }.onFailure { error ->
                _state.update {
                    it.copy(saving = false, error = error.userMessage(), noticeId = it.noticeId + 1)
                }
            }
        }
    }

    fun clearError() {
        _state.update { it.copy(error = "") }
    }

    private fun validate(
        draft: SubscriptionDraft,
        headersText: String,
        isNew: Boolean,
    ): SubscriptionDraft {
        // 新增订阅允许留空名称，由模块按 Profile-Title、文件名、主机名顺序自动取名；
        // 编辑既有订阅时清空名称属于误操作，仍需拒绝
        require(isNew || draft.name.isNotBlank()) { "订阅名称不能为空" }
        require(draft.url.isNotBlank()) { "订阅链接不能为空" }
        require(draft.updateIntervalSeconds >= 900) { "更新周期不能少于 15 分钟" }
        require(draft.timeoutSeconds > 0) { "下载超时必须大于 0 秒" }
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
            require(separator > 0) { "第 ${index + 1} 行请求头格式应为 名称: 值" }
            val name = line.substring(0, separator).trim()
            val value = line.substring(separator + 1).trim()
            require(name.isNotEmpty() && value.isNotEmpty()) {
                "第 ${index + 1} 行请求头不能为空"
            }
            put(name, value)
        }
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


