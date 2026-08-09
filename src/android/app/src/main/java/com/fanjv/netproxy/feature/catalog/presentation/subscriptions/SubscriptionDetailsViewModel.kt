package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
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
    val error: String = "",
    val notice: String = "",
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

    fun testNode(groupId: String, tag: String) {
        if (_state.value.operation.isNotEmpty()) return
        val nodeRef = "$groupId/$tag"
        viewModelScope.launch {
            _state.update {
                it.copy(
                    operation = "delay",
                    error = "",
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
                            notice = if (delay != null) "节点延迟：${delay} ms" else "节点测速超时",
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
            _state.update { it.copy(operation = "export", error = "") }
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
        runNodeOperation("edit", groupId) {
            require(link.isNotBlank()) { "节点链接不能为空" }
            nodeRepository.edit(nodeRef, link.trim())
            _state.update { it.copy(editingNodeRef = "", editingNodeLink = "") }
            "节点已更新"
        }
    }

    fun dismissNodeEditor() {
        if (_state.value.operation == "edit") return
        _state.update { it.copy(editingNodeRef = "", editingNodeLink = "") }
    }

    fun exportNode(groupId: String, tag: String) {
        if (_state.value.operation.isNotEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(operation = "export", error = "") }
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
                notice = "节点链接已复制到剪贴板",
                noticeId = it.noticeId + 1
            )
        }
    }

    fun removeNode(groupId: String, tag: String) = runNodeOperation("remove-node", groupId) {
        nodeRepository.remove("$groupId/$tag")
        "节点已删除"
    }

    fun clearNotice() {
        _state.update { it.copy(notice = "", error = "") }
    }

    private fun publishError(error: Throwable) {
        _state.update {
            it.copy(
                operation = "",
                error = error.userMessage(),
                noticeId = it.noticeId + 1
            )
        }
    }

    private fun runNodeOperation(
        operation: String,
        groupId: String,
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
                    load(groupId)
                }
                .onFailure(::publishError)
        }
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


