package com.fanjv.netproxy.feature.catalog.presentation.nodes

import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.toUiText
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.catalog.data.NodeImportStore
import com.fanjv.netproxy.feature.catalog.data.NodeRepository
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.feature.catalog.model.CurrentNodeSelection
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

internal data class CatalogNodesUiState(
    val groups: List<CatalogNodeGroup> = emptyList(),
    val selection: CurrentNodeSelection = CurrentNodeSelection(),
    val selectedGroupId: String = "",
    val focusGroupId: String = "",
    val focusGroupRevision: Long = 0,
    val latencies: Map<String, String> = emptyMap(),
    val loading: Boolean = false,
    val operation: String = "",
    val error: UiText = UiText.Empty,
    val notice: UiText = UiText.Empty,
    val noticeId: Long = 0,
    val exportedNodeLink: String = "",
    val exportedNodeLinkId: Long = 0
)

internal class CatalogNodesViewModel(
    private val repository: NodeRepository,
    private val importStore: NodeImportStore
) : ViewModel() {
    private val _state = MutableStateFlow(CatalogNodesUiState())
    val state: StateFlow<CatalogNodesUiState> = _state.asStateFlow()
    private var refreshJob: Job? = null
    private var refreshPending = false
    private var loaded = false

    fun setVisible(visible: Boolean) {
        if (visible) refresh(silent = loaded)
    }

    fun refresh(silent: Boolean = false) {
        if (refreshJob?.isActive == true) {
            refreshPending = true
            return
        }
        refreshJob = viewModelScope.launch {
            try {
                if (!silent) _state.update { it.copy(loading = true, error = UiText.Empty) }
                runCatching { repository.snapshot() }.onSuccess { snapshot ->
                    val groups = snapshot.groups
                    val selection = snapshot.selection
                    _state.update { old ->
                        val selected = old.selectedGroupId
                            .takeIf { id -> groups.any { it.group.id == id } }
                            ?: selection.activeGroupId.takeIf(String::isNotBlank)
                            ?: groups.firstOrNull()?.group?.id.orEmpty()
                        old.copy(
                            groups = groups,
                            selection = selection,
                            selectedGroupId = selected,
                            loading = false,
                            error = UiText.Empty
                        )
                    }
                    loaded = true
                }.onFailure { error ->
                    _state.update {
                        it.copy(
                            loading = false,
                            error = error.userMessage().toUiText(),
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

    fun selectGroup(id: String) {
        _state.update { it.copy(selectedGroupId = id) }
    }

    fun useAuto(groupId: String) = runOperation("select", refreshAfter = false) {
        repository.selectAuto(groupId)
        _state.update {
            it.copy(
                selection = CurrentNodeSelection(groupId, "urltest", "Auto/$groupId"),
                selectedGroupId = groupId
            )
        }
        UiText.Resource(R.string.node_switched_auto)
    }

    fun useNode(groupId: String, tag: String) = runOperation("select", refreshAfter = false) {
        repository.select("$groupId/$tag")
        _state.update {
            it.copy(
                selection = CurrentNodeSelection(groupId, "manual", "$groupId/$tag"),
                selectedGroupId = groupId
            )
        }
        UiText.Resource(R.string.node_switched, listOf(tag))
    }

    fun addNode(link: String) {
        if (link.isBlank()) {
            publishError(UiText.Resource(R.string.node_link_empty))
            return
        }
        runOperation("add") {
            repository.add(link.trim())
            UiText.Resource(R.string.node_added)
        }
    }

    fun removeNode(groupId: String, tag: String) = runOperation("remove") {
        repository.remove("$groupId/$tag")
        UiText.Resource(R.string.node_deleted)
    }

    suspend fun loadNodeConfigContent(nodeRef: String): String =
        repository.get(nodeRef)

    fun saveNodeConfigContent(
        nodeRef: String,
        content: String,
        onResult: (Boolean) -> Unit
    ) {
        if (_state.value.operation.isNotEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(operation = "edit", error = UiText.Empty) }
            runCatching { repository.editJson(nodeRef, content) }
                .onSuccess {
                    _state.update { it.copy(operation = "") }
                    refresh(silent = true)
                    onResult(true)
                }
                .onFailure { error ->
                    _state.update {
                        it.copy(
                            operation = "",
                            error = error.userMessage().toUiText(),
                            noticeId = it.noticeId + 1
                        )
                    }
                    onResult(false)
                }
        }
    }

    fun exportNode(groupId: String, tag: String) {
        if (_state.value.operation.isNotEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(operation = "export", error = UiText.Empty) }
            runCatching { repository.export("$groupId/$tag") }
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

    fun testDelay(target: String) {
        testDelays(requestTarget = target, targets = listOf(target))
    }

    fun testGroupDelay(groupId: String) {
        val targets = _state.value.groups
            .firstOrNull { it.group.id == groupId }
            ?.nodes
            .orEmpty()
            .map { "$groupId/${it.tag}" }
        if (targets.isEmpty()) return
        testDelays(
            requestTarget = "auto",
            requestGroupId = groupId,
            targets = targets + "Auto/$groupId"
        )
    }

    fun importFile(uri: Uri) = runOperation(
        name = "import",
        action = {
            importStore.withImportedFile(uri) { temporary ->
                repository.import(temporary.absolutePath)
            }
            UiText.Resource(R.string.node_file_added)
        },
        onSuccess = {
            _state.update {
                it.copy(
                    selectedGroupId = DEFAULT_GROUP_ID,
                    focusGroupId = DEFAULT_GROUP_ID,
                    focusGroupRevision = it.focusGroupRevision + 1
                )
            }
        }
    )

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

    private fun testDelays(
        requestTarget: String,
        targets: List<String>,
        requestGroupId: String = ""
    ) {
        if (_state.value.operation.isNotEmpty()) return
        viewModelScope.launch {
            _state.update { current ->
                current.copy(
                    operation = "delay",
                    error = UiText.Empty,
                    latencies = current.latencies + targets.associateWith { "testing..." }
                )
            }
            runCatching { repository.testDelay(requestTarget, requestGroupId) }
                .onSuccess { result ->
                    val persistentGroupId = requestGroupId.ifBlank {
                        requestTarget.takeIf { '/' in it }?.substringBefore('/').orEmpty()
                    }
                    val runtimeGroupTag = result.target
                        .removePrefix("Auto/")
                        .removePrefix("Select/")
                        .substringBefore('/')
                    val measured = result.groups
                        .asSequence()
                        .flatMap { it.items.asSequence() }
                        .mapNotNull { item ->
                            item.urlTestDelay?.takeIf { it > 0 }?.let { delay ->
                                persistentDelayKey(
                                    tag = item.tag,
                                    persistentGroupId = persistentGroupId,
                                    runtimeGroupTag = runtimeGroupTag
                                ) to "$delay"
                            }
                        }
                        .toMap()
                        .toMutableMap()
                    val nodeTargets = targets.filterNot { it.startsWith("Auto/") }
                    targets.firstOrNull { it.startsWith("Auto/") }?.let { autoTarget ->
                        groupAutoDelay(measured, nodeTargets)?.let { delay ->
                            measured[autoTarget] = delay
                        }
                    }
                    _state.update { current ->
                        current.copy(
                            operation = "",
                            latencies = current.latencies + targets.associateWith { target ->
                                measured[target] ?: "timeout"
                            },
                            notice = UiText.Resource(R.string.node_latency_complete),
                            noticeId = current.noticeId + 1
                        )
                    }
                }
                .onFailure { error ->
                    _state.update { current ->
                        current.copy(
                            operation = "",
                            latencies = current.latencies + targets.associateWith { "failed" },
                            error = error.userMessage().toUiText(),
                            noticeId = current.noticeId + 1
                        )
                    }
                }
        }
    }

    private fun persistentDelayKey(
        tag: String,
        persistentGroupId: String,
        runtimeGroupTag: String
    ): String {
        if (persistentGroupId.isBlank() || runtimeGroupTag.isBlank()) return tag
        return when {
            tag == "Auto/$runtimeGroupTag" -> "Auto/$persistentGroupId"
            tag == "Select/$runtimeGroupTag" -> "Select/$persistentGroupId"
            tag.startsWith("$runtimeGroupTag/") ->
                "$persistentGroupId/${tag.removePrefix("$runtimeGroupTag/")}"

            else -> tag
        }
    }

    private fun runOperation(
        name: String,
        refreshAfter: Boolean = true,
        onSuccess: () -> Unit = {},
        action: suspend () -> UiText
    ) {
        if (_state.value.operation.isNotEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(operation = name, error = UiText.Empty) }
            runCatching { action() }
                .onSuccess { message ->
                    _state.update {
                        it.copy(
                            operation = "",
                            notice = message,
                            noticeId = it.noticeId + 1
                        )
                    }
                    onSuccess()
                    if (refreshAfter) refresh(silent = true)
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

    private companion object {
        const val DEFAULT_GROUP_ID = "default"
    }
}

internal fun groupAutoDelay(measured: Map<String, String>, nodeTargets: List<String>): String? =
    nodeTargets.mapNotNull { measured[it]?.toIntOrNull() }.minOrNull()?.toString()
