package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.toUiText
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.catalog.data.NodeRepository
import com.fanjv.netproxy.feature.catalog.data.SubscriptionRepository
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.feature.catalog.model.SubscriptionHistoryEntry
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
    val error: UiText = UiText.Empty,
    val notice: UiText = UiText.Empty,
    val noticeId: Long = 0,
    val latencies: Map<String, String> = emptyMap(),
    val editingNodeRef: String = "",
    val editingNodeLink: String = "",
    val exportedNodeLink: String = "",
    val exportedNodeLinkId: Long = 0
)

/** 管理单个订阅的节点摘要、历史和详情页操作。 */
internal class SubscriptionDetailsViewModel(
    private val repository: SubscriptionRepository,
    private val nodeRepository: NodeRepository
) : ViewModel() {
    private val _state = MutableStateFlow(SubscriptionDetailsUiState())
    val state: StateFlow<SubscriptionDetailsUiState> = _state.asStateFlow()

    fun load(id: String) {
        viewModelScope.launch {
            _state.update { it.copy(loading = true, error = UiText.Empty, details = null) }
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
                _state.update {
                    it.copy(
                        loading = false,
                        error = error.userMessage().toUiText(),
                        noticeId = it.noticeId + 1
                    )
                }
            }
        }
    }

    fun update(id: String) = runOperation("update", id) {
        repository.update(id)
        UiText.Resource(R.string.subscription_updated)
    }

    fun activate(id: String) = runOperation("activate", id) {
        repository.activate(id)
        UiText.Resource(R.string.subscription_activated)
    }

    fun remove(id: String, onRemoved: () -> Unit) = runOperation("remove", id) {
        repository.remove(id)
        onRemoved()
        UiText.Resource(R.string.subscription_deleted)
    }

    fun testNode(groupId: String, tag: String) {
        if (_state.value.operation.isNotEmpty()) return
        val nodeRef = "$groupId/$tag"
        viewModelScope.launch {
            _state.update {
                it.copy(
                    operation = "delay",
                    error = UiText.Empty,
                    latencies = it.latencies + (nodeRef to "testing")
                )
            }
            runCatching { nodeRepository.testDelay(nodeRef) }
                .onSuccess { result ->
                    val delay = result.groups.asSequence()
                        .flatMap { it.items.asSequence() }
                        .mapNotNull { it.urlTestDelay?.takeIf { value -> value > 0 } }
                        .firstOrNull()
                    _state.update {
                        it.copy(
                            operation = "",
                            latencies = it.latencies + (nodeRef to (delay?.toString()
                                ?: "timeout")),
                            notice = if (delay != null) {
                                UiText.Resource(R.string.node_delay, listOf(delay))
                            } else {
                                UiText.Resource(R.string.node_delay_timeout)
                            },
                            noticeId = it.noticeId + 1
                        )
                    }
                }
                .onFailure(::publishError)
        }
    }

    fun editNode(groupId: String, tag: String) {
        if (_state.value.operation.isNotEmpty()) return
        val nodeRef = "$groupId/$tag"
        viewModelScope.launch {
            _state.update { it.copy(operation = "export", error = UiText.Empty) }
            runCatching { nodeRepository.export(nodeRef) }
                .onSuccess { exported ->
                    _state.update {
                        it.copy(
                            operation = "",
                            editingNodeRef = nodeRef,
                            editingNodeLink = exported.link
                        )
                    }
                }
                .onFailure(::publishError)
        }
    }

    fun saveEditedNode(link: String) {
        val nodeRef = _state.value.editingNodeRef
        if (nodeRef.isBlank()) return
        val groupId = nodeRef.substringBefore('/')
        if (link.isBlank()) {
            publishError(UiText.Resource(R.string.node_link_empty))
            return
        }
        runNodeOperation("edit", groupId) {
            nodeRepository.edit(nodeRef, link.trim())
            _state.update { it.copy(editingNodeRef = "", editingNodeLink = "") }
            UiText.Resource(R.string.node_updated)
        }
    }

    fun dismissNodeEditor() {
        if (_state.value.operation == "edit") return
        _state.update { it.copy(editingNodeRef = "", editingNodeLink = "") }
    }

    fun exportNode(groupId: String, tag: String) {
        if (_state.value.operation.isNotEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(operation = "export", error = UiText.Empty) }
            runCatching { nodeRepository.export("$groupId/$tag") }
                .onSuccess { exported ->
                    _state.update {
                        it.copy(
                            operation = "",
                            exportedNodeLink = exported.link,
                            exportedNodeLinkId = it.exportedNodeLinkId + 1
                        )
                    }
                }
                .onFailure(::publishError)
        }
    }

    fun nodeLinkCopied() {
        _state.update {
            it.copy(
                exportedNodeLink = "",
                notice = UiText.Resource(R.string.node_link_copied),
                noticeId = it.noticeId + 1
            )
        }
    }

    fun removeNode(groupId: String, tag: String) = runNodeOperation("remove-node", groupId) {
        nodeRepository.remove("$groupId/$tag")
        UiText.Resource(R.string.node_deleted)
    }

    fun clearNotice() {
        _state.update { it.copy(notice = UiText.Empty, error = UiText.Empty) }
    }

    private fun publishError(error: Throwable) {
        publishError(error.userMessage().toUiText())
    }

    private fun publishError(message: UiText) {
        _state.update {
            it.copy(
                operation = "",
                error = message,
                noticeId = it.noticeId + 1
            )
        }
    }

    private fun runNodeOperation(
        operation: String,
        groupId: String,
        action: suspend () -> UiText
    ) {
        if (_state.value.operation.isNotEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(operation = operation, error = UiText.Empty) }
            runCatching { action() }
                .onSuccess { message ->
                    _state.update {
                        it.copy(operation = "", notice = message, noticeId = it.noticeId + 1)
                    }
                    load(groupId)
                }
                .onFailure(::publishError)
        }
    }

    private fun runOperation(
        operation: String,
        id: String,
        action: suspend () -> UiText
    ) {
        if (_state.value.operation.isNotEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(operation = operation, error = UiText.Empty) }
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
                            error = error.userMessage().toUiText(),
                            noticeId = it.noticeId + 1
                        )
                    }
                }
        }
    }
}
