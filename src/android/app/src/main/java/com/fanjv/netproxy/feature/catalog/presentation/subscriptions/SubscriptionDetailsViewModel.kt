package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.toUiText
import com.fanjv.netproxy.feature.catalog.data.NodeRepository
import com.fanjv.netproxy.feature.catalog.data.SubscriptionRepository
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.feature.catalog.model.SubscriptionHistoryEntry
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.Job
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
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

internal class SubscriptionDetailsViewModel(
    private val repository: SubscriptionRepository,
    private val nodeRepository: NodeRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
) : ViewModel(scope) {
    private val _state = MutableStateFlow(SubscriptionDetailsUiState())
    val state: StateFlow<SubscriptionDetailsUiState> = _state.asStateFlow()
    private var loadJob: Job? = null
    private var operationJob: Job? = null
    private var generation = 0L
    private var currentId: String? = null

    fun load(id: String) {
        if (_state.value.operation.isNotEmpty() && currentId == id) return
        if (currentId != id) operationJob?.cancel()
        loadJob?.cancel()
        val request = ++generation
        if (currentId != id) _state.value = SubscriptionDetailsUiState()
        currentId = id
        _state.update { it.copy(loading = true) }
        loadJob = viewModelScope.launch {
            try {
                val details = repository.details(id)
                val history = if (details.group.type == "subscription") repository.history(id) else emptyList()
                currentCoroutineContext().ensureActive()
                if (request == generation) _state.update {
                    it.copy(details = details, history = history, loading = false)
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                currentCoroutineContext().ensureActive()
                if (request == generation) _state.update {
                    it.copy(
                        loading = false,
                        error = if (it.error == UiText.Empty) error.toUiText() else it.error,
                        noticeId = it.noticeId + 1
                    )
                }
            } finally {
                if (request == generation) _state.update { it.copy(loading = false) }
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
        currentCoroutineContext().ensureActive()
        onRemoved()
        UiText.Resource(R.string.subscription_deleted)
    }

    fun testNode(groupId: String, tag: String) {
        val nodeRef = "$groupId/$tag"
        runOperation("delay", groupId, refresh = false) {
            _state.update { it.copy(latencies = it.latencies + (nodeRef to "testing")) }
            try {
                val result = nodeRepository.testDelay(nodeRef)
                currentCoroutineContext().ensureActive()
                val delay = result.groups.asSequence()
                    .flatMap { it.items.asSequence() }
                    .mapNotNull { it.urlTestDelay?.takeIf { value -> value > 0 } }
                    .firstOrNull()
                _state.update { it.copy(latencies = it.latencies + (nodeRef to (delay?.toString() ?: "timeout"))) }
                if (delay != null) UiText.Resource(R.string.node_delay, listOf(delay))
                else UiText.Resource(R.string.node_delay_timeout)
            } finally {
                _state.update {
                    if (it.latencies[nodeRef] == "testing") it.copy(latencies = it.latencies - nodeRef) else it
                }
            }
        }
    }

    fun editNode(groupId: String, tag: String) = runOperation("export", groupId, refresh = false) {
        val nodeRef = "$groupId/$tag"
        val exported = nodeRepository.export(nodeRef)
        currentCoroutineContext().ensureActive()
        _state.update { it.copy(editingNodeRef = nodeRef, editingNodeLink = exported.link) }
        UiText.Empty
    }

    fun saveEditedNode(link: String) {
        val nodeRef = _state.value.editingNodeRef
        if (nodeRef.isBlank()) return
        if (link.isBlank()) {
            publishError(UiText.Resource(R.string.node_link_empty))
            return
        }
        runOperation("edit", nodeRef.substringBefore('/')) {
            nodeRepository.edit(nodeRef, link.trim())
            currentCoroutineContext().ensureActive()
            _state.update { it.copy(editingNodeRef = "", editingNodeLink = "") }
            UiText.Resource(R.string.node_updated)
        }
    }

    fun dismissNodeEditor() {
        if (_state.value.operation == "edit") return
        _state.update { it.copy(editingNodeRef = "", editingNodeLink = "") }
    }

    fun exportNode(groupId: String, tag: String) = runOperation("export", groupId, refresh = false) {
        val exported = nodeRepository.export("$groupId/$tag")
        currentCoroutineContext().ensureActive()
        _state.update { it.copy(exportedNodeLink = exported.link, exportedNodeLinkId = it.exportedNodeLinkId + 1) }
        UiText.Empty
    }

    fun nodeLinkCopied() {
        _state.update {
            it.copy(exportedNodeLink = "", notice = UiText.Resource(R.string.node_link_copied), noticeId = it.noticeId + 1)
        }
    }

    fun removeNode(groupId: String, tag: String) = runOperation("remove-node", groupId) {
        nodeRepository.remove("$groupId/$tag")
        UiText.Resource(R.string.node_deleted)
    }

    fun clearNotice() {
        _state.update { it.copy(notice = UiText.Empty, error = UiText.Empty) }
    }

    private fun publishError(message: UiText) {
        _state.update { it.copy(error = message, noticeId = it.noticeId + 1) }
    }

    private fun runOperation(
        operation: String,
        id: String,
        refresh: Boolean = true,
        action: suspend () -> UiText
    ) {
        if (_state.value.operation.isNotEmpty() || (currentId != null && currentId != id)) return
        loadJob?.cancel()
        currentId = id
        val request = ++generation
        _state.update { it.copy(operation = operation, loading = false, error = UiText.Empty) }
        operationJob = viewModelScope.launch {
            var shouldRefresh = false
            try {
                val message = action()
                currentCoroutineContext().ensureActive()
                if (request == generation) {
                    _state.update { it.copy(notice = message, noticeId = it.noticeId + if (message != UiText.Empty) 1 else 0) }
                    shouldRefresh = refresh && operation != "remove"
                    if (operation == "remove") _state.update { it.copy(details = null, history = emptyList()) }
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                currentCoroutineContext().ensureActive()
                if (request == generation) {
                    publishError(error.toUiText())
                    shouldRefresh = error.subscriptionPersisted()
                    if (shouldRefresh && operation == "remove") {
                        _state.update { it.copy(details = null, history = emptyList()) }
                        shouldRefresh = false
                    }
                }
            } finally {
                if (request == generation) _state.update { it.copy(operation = "") }
            }
            if (shouldRefresh && request == generation) load(id)
        }
    }
}
