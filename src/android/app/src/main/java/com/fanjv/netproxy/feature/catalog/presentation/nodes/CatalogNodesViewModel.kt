package com.fanjv.netproxy.feature.catalog.presentation.nodes

import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.toUiText
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.catalog.data.NodeImportStore
import com.fanjv.netproxy.feature.catalog.data.NodeRepository
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.feature.catalog.model.CatalogNodesSnapshot
import com.fanjv.netproxy.feature.catalog.model.CurrentNodeSelection
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
) {
    fun withSnapshot(snapshot: CatalogNodesSnapshot): CatalogNodesUiState = copy(
        groups = snapshot.groups,
        selection = snapshot.selection,
        selectedGroupId = selectedGroupId.takeIf { id -> snapshot.groups.any { it.group.id == id } }
            ?: snapshot.selection.activeGroupId.takeIf(String::isNotBlank)
            ?: snapshot.groups.firstOrNull()?.group?.id.orEmpty(),
        loading = false
    )
}

internal class CatalogNodesViewModel(
    private val repository: NodeRepository,
    private val importNode: suspend (Uri) -> Unit,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) : ViewModel(scope) {
    constructor(
        repository: NodeRepository,
        importStore: NodeImportStore,
        scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
    ) : this(repository, { uri ->
        importStore.withImportedFile(uri) { repository.import(it.absolutePath) }
        Unit
    }, scope)

    private val _state = MutableStateFlow(CatalogNodesUiState())
    val state: StateFlow<CatalogNodesUiState> = _state.asStateFlow()
    private var refreshJob: Job? = null
    private var refreshRevision = 0L
    private var loaded = false

    fun setVisible(visible: Boolean) {
        if (visible) refresh(silent = loaded)
    }

    fun refresh(silent: Boolean = false) {
        if (_state.value.operation.isNotEmpty()) return
        val revision = ++refreshRevision
        refreshJob?.cancel()
        if (!silent) _state.update { it.copy(loading = true, error = UiText.Empty) }
        refreshJob = viewModelScope.launch {
            try {
                val snapshot = repository.snapshot()
                currentCoroutineContext().ensureActive()
                if (revision != refreshRevision) return@launch
                _state.update { it.withSnapshot(snapshot) }
                loaded = true
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                currentCoroutineContext().ensureActive()
                if (revision == refreshRevision) {
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
    }

    fun selectGroup(id: String) {
        _state.update { it.copy(selectedGroupId = id) }
    }

    fun useAuto(groupId: String) = runOperation("select", refreshAfter = false) {
        repository.selectAuto(groupId)
        val snapshot = repository.snapshot()
        currentCoroutineContext().ensureActive()
        _state.update {
            it.withSnapshot(snapshot).copy(selectedGroupId = groupId)
        }
        loaded = true
        UiText.Resource(R.string.node_switched_auto)
    }

    fun useNode(groupId: String, tag: String) = runOperation("select", refreshAfter = false) {
        repository.select("$groupId/$tag")
        val snapshot = repository.snapshot()
        currentCoroutineContext().ensureActive()
        _state.update {
            it.withSnapshot(snapshot).copy(selectedGroupId = groupId)
        }
        loaded = true
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
                    currentCoroutineContext().ensureActive()
                    _state.update { it.copy(operation = "") }
                    refresh(silent = true)
                    onResult(true)
                }
                .onFailure { error ->
                    if (error is CancellationException) throw error
                    currentCoroutineContext().ensureActive()
                    _state.update {
                        it.copy(
                            operation = "",
                            error = error.userMessage().toUiText(),
                            noticeId = it.noticeId + 1
                        )
                    }
                    if ((error as? NetProxyCtlException)?.persisted == true) refresh(silent = true)
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
                    currentCoroutineContext().ensureActive()
                    _state.update {
                        it.copy(
                            operation = "",
                            exportedNodeLink = exported.link,
                            exportedNodeLinkId = it.exportedNodeLinkId + 1
                        )
                    }
                }
                .onFailure { error ->
                    currentCoroutineContext().ensureActive()
                    publishError(error)
                }
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
            importNode(uri)
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
        if (error is CancellationException) throw error
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
                    currentCoroutineContext().ensureActive()
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
                    if (error is CancellationException) throw error
                    currentCoroutineContext().ensureActive()
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
        refreshRevision++
        refreshJob?.cancel()
        _state.update { it.copy(operation = name, loading = false, error = UiText.Empty) }
        viewModelScope.launch {
            runCatching { action() }
                .onSuccess { message ->
                    currentCoroutineContext().ensureActive()
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
                    if (error is CancellationException) throw error
                    currentCoroutineContext().ensureActive()
                    _state.update {
                        it.copy(
                            operation = "",
                            error = error.userMessage().toUiText(),
                            noticeId = it.noticeId + 1
                        )
                    }
                    if (name == "select" || (error as? NetProxyCtlException)?.persisted == true) refresh(silent = true)
                }
        }
    }

    private companion object {
        const val DEFAULT_GROUP_ID = "default"
    }
}

internal fun groupAutoDelay(measured: Map<String, String>, nodeTargets: List<String>): String? =
    nodeTargets.mapNotNull { measured[it]?.toIntOrNull() }.minOrNull()?.toString()

internal fun selectedAutoNodeTag(
    group: CatalogNodeGroup,
    selection: CurrentNodeSelection
): String {
    if (selection.selectorMode != "urltest" || selection.activeGroupId != group.group.id) return ""
    val prefix = selection.activeGroupRuntimeTag.takeIf(String::isNotBlank)?.let { "$it/" } ?: return ""
    val nodeTag = selection.runtimeSelected
        .takeIf { it.startsWith(prefix) }
        ?.removePrefix(prefix)
        ?.takeIf(String::isNotBlank)
        ?: return ""
    return nodeTag.takeIf { tag -> group.nodes.any { it.tag == tag } }.orEmpty()
}
