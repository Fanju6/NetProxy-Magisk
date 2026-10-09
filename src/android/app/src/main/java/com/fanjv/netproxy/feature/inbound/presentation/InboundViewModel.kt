package com.fanjv.netproxy.feature.inbound.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.command.ConfigurationWrites
import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.inbound.data.*
import com.fanjv.netproxy.feature.settings.model.ConfigSnapshot
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.isActive
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

internal data class InboundDraft(val partition: String, val snapshot: ConfigSnapshot)

internal data class InboundUiState(
    val snapshot: InboundSnapshot? = null,
    val choices: InboundChoices = InboundChoices(emptyList(), emptyList()),
    val choicesError: Boolean = false,
    val isLoading: Boolean = false,
    val applyingField: String? = null,
    val requiresReload: Boolean = false,
    val pendingBackend: String? = null,
    val failedDraft: InboundDraft? = null,
    val draft: JsonObject? = null,
    val diagnostic: String? = null,
    val isDiagnosing: Boolean = false,
    val errorCode: String = "",
    val error: String = ""
) {
    val isSaving: Boolean get() = applyingField != null
    val isInitialLoading: Boolean get() = snapshot == null && (isLoading || !requiresReload)
    val backend: String get() = snapshot?.backend.orEmpty()
    val native: JsonObject get() = draft ?: snapshot?.native?.get(backend) ?: JsonObject(emptyMap())
    val hasPendingChanges: Boolean get() = draft != null || failedDraft != null
    val canReviewDraft: Boolean get() = failedDraft != null || (requiresReload && draft != null)
    val hasConfiguration: Boolean get() = snapshot?.status != null && !requiresReload
    val editable: Boolean get() = hasConfiguration && (applyingField == null || applyingField == backend) && pendingBackend == null

    fun draftForReview(): InboundDraft? {
        failedDraft?.let { return it }
        val candidate = draft?.takeIf { requiresReload } ?: return null
        val revision = snapshot?.partitions?.get(backend)?.revision ?: return null
        return InboundDraft(backend, ConfigSnapshot(inboundJson.encodeToString(JsonObject(mapOf(backend to candidate))), revision))
    }
}

internal class InboundViewModel(
    private val repository: InboundRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
    private val writes: ConfigurationWrites = ConfigurationWrites(scope)
) : ViewModel(scope) {
    private val mutableState = MutableStateFlow(InboundUiState())
    val state = mutableState.asStateFlow()
    private var refreshJob: Job? = null
    private var mutationJob: Job? = null
    private var requestedNative: JsonObject? = null

    fun refresh() {
        if (state.value.isLoading || state.value.isSaving || state.value.hasPendingChanges ||
            (state.value.requiresReload && state.value.snapshot != null)) return
        mutableState.update { it.copy(isLoading = true, error = "", errorCode = "", pendingBackend = null) }
        refreshJob = viewModelScope.launch {
            try {
                val snapshot = repository.load()
                val choices = try {
                    repository.choices()
                } catch (error: Exception) {
                    if (error is CancellationException) throw error
                    null
                }
                mutableState.update {
                    it.copy(snapshot = snapshot, choices = choices ?: it.choices, choicesError = choices == null,
                        requiresReload = false, isLoading = false, failedDraft = null)
                }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                mutableState.update {
                    it.copy(snapshot = it.snapshot?.copy(status = null), isLoading = false,
                        requiresReload = true, error = error.userMessage())
                }
            }
        }
    }

    fun requestBackend(backend: String) {
        if (!state.value.editable || state.value.isSaving) return
        require(backend in setOf("ebpf", "tun"))
        mutate("backend", write = {
            applyPending()
            val status = repository.status()
            if (status.requiresBackendSwitch(backend)) {
                if (viewModelScope.isActive) mutableState.update { it.copy(pendingBackend = backend) }
            } else if (backend != state.value.backend) applyBackend(backend)
            else mutableState.update { it.copy(snapshot = it.snapshot?.copy(status = status)) }
        }, afterWrite = ::restoreFailedBackend)
    }

    fun cancelBackendSwitch() = mutableState.update { it.copy(pendingBackend = null) }
    fun confirmBackendSwitch() {
        val backend = state.value.pendingBackend ?: return
        cancelBackendSwitch()
        mutate("backend", write = { applyBackend(backend, confirmed = true) }, afterWrite = ::restoreFailedBackend)
    }

    fun setField(path: String, value: JsonElement?, backend: String = state.value.backend, revision: String? = null) =
        edit(backend, revision) { it.withPath(path.split('.'), value) }
    fun setDataPaths(value: String) = edit {
        it.withPath(listOf("local", "enabled"), JsonPrimitive(value != "shared"))
            .withPath(listOf("shared", "enabled"), JsonPrimitive(value != "local"))
    }
    fun setTunIpv6(enabled: Boolean) = edit { it.withTunIpv6(enabled) }
    fun setFilter(include: String, exclude: String, mode: String, values: List<String>, backend: String = state.value.backend, revision: String? = null) =
        edit(backend, revision) { it.withFilter(include, exclude, mode, values) }

    private fun edit(backend: String = state.value.backend, revision: String? = null, transform: (JsonObject) -> JsonObject) {
        if (!state.value.editable) return
        if (backend != state.value.backend) {
            mutableState.update { it.copy(requiresReload = true, errorCode = "config.conflict") }
            return
        }
        val snapshot = state.value.snapshot!!.partitions.getValue(backend)
        try {
            if (revision != null && revision != snapshot.revision) {
                mutableState.update { it.copy(requiresReload = true, errorCode = "config.conflict") }
                return
            }
            cancelRefresh()
            val updated = transform(state.value.native)
            mutableState.update { current -> current.copy(
                draft = updated.takeUnless { it == current.snapshot!!.native.getValue(backend) }, error = "") }
        } catch (error: Exception) {
            mutableState.update { it.copy(error = error.userMessage()) }
        }
    }

    fun requestFlush() {
        if (state.value.requiresReload || (state.value.draft == null && state.value.applyingField != state.value.backend)) return
        requestedNative = state.value.native
        if (mutationJob?.isActive == true) return
        mutate(state.value.backend, write = { applyPending(drain = false) })
    }

    suspend fun flush(): Boolean {
        do {
            requestFlush()
            while (mutationJob?.isActive == true) mutationJob?.join()
        } while (state.value.draft != null && !state.value.requiresReload)
        return !state.value.requiresReload && !state.value.hasPendingChanges && !state.value.isSaving
    }

    private suspend fun applyPending(drain: Boolean = true) {
        while (true) {
            val current = state.value
            val partition = current.backend
            val snapshot = current.snapshot!!.partitions.getValue(partition)
            val saved = if (drain) current.draft ?: break else requestedNative ?: break
            requestedNative = null
            if (saved == current.snapshot.native.getValue(partition)) continue
            val content = inboundJson.encodeToString(JsonObject(mapOf(partition to saved)))
            val result = repository.apply("inbound/$partition", content, snapshot.revision)
            mutableState.update {
                val latest = it.native
                val previous = it.snapshot!!
                it.copy(snapshot = previous.copy(
                    partitions = previous.partitions + (partition to ConfigSnapshot(content, result.revision)),
                    native = previous.native + (partition to saved), status = result.status,
                ), draft = latest.takeUnless { native -> native == saved })
            }
        }
    }

    private suspend fun applyBackend(backend: String, confirmed: Boolean = false) {
        val snapshot = state.value.snapshot!!.partitions.getValue("backend")
        val content = inboundJson.encodeToString(JsonObject(mapOf("backend" to JsonPrimitive(backend))))
        val draft = InboundDraft("backend", ConfigSnapshot(content, snapshot.revision))
        try {
            val result = repository.apply("inbound/backend", content, snapshot.revision, confirmed)
            mutableState.update { current ->
                val previous = current.snapshot!!
                current.copy(snapshot = previous.copy(
                    backend = backend,
                    partitions = previous.partitions + ("backend" to ConfigSnapshot(content, result.revision)),
                    status = result.status
                ))
            }
        } catch (error: InboundSwitchConfirmationRequired) {
            if (viewModelScope.isActive) mutableState.update { it.copy(pendingBackend = backend) }
        } catch (error: Exception) {
            if (error is CancellationException) throw error
            mutableState.update { it.copy(failedDraft = draft) }
            throw error
        }
    }

    private suspend fun restoreFailedBackend() {
        if (state.value.failedDraft?.partition != "backend") return
        val restored = try { repository.load() } catch (error: Exception) {
            if (error is CancellationException) throw error
            null
        }
        if (restored != null) mutableState.update { it.copy(snapshot = restored) }
    }

    fun restart() {
        if (!state.value.editable || state.value.isSaving) return
        mutate("restart", write = {
            applyPending()
            repository.restart()
        }, afterWrite = {
            if (!state.value.requiresReload) {
                val snapshot = repository.load()
                mutableState.update { it.copy(snapshot = snapshot) }
            }
        })
    }

    private fun mutate(field: String, write: suspend () -> Unit, afterWrite: suspend () -> Unit = {}) {
        cancelRefresh()
        mutableState.update { it.copy(applyingField = field, error = "", errorCode = "") }
        mutationJob = writes.launch("inbound", write, afterWrite = {
            try {
                afterWrite()
            } finally { mutableState.update { it.copy(applyingField = null) } }
        }, onFailure = { error ->
            requestedNative = null
            mutableState.update { it.copy(snapshot = it.snapshot?.copy(status = null), requiresReload = true,
                errorCode = (error as? NetProxyCtlException)?.resultCode.orEmpty(), error = error.userMessage()) }
        })
    }

    fun diagnose() {
        if (state.value.isDiagnosing) return
        mutableState.update { it.copy(isDiagnosing = true) }
        viewModelScope.launch {
            val output = try { repository.diagnose() } catch (error: Exception) {
                if (error is CancellationException) throw error
                ((error as? NetProxyCtlException)?.data as? JsonObject)?.textAt("content")?.takeIf(String::isNotBlank)
                    ?: error.userMessage()
            }
            mutableState.update { it.copy(isDiagnosing = false, diagnostic = output) }
        }
    }

    fun dismissDiagnostic() = mutableState.update { it.copy(diagnostic = null) }
    fun discardDraft() = mutableState.update { it.copy(failedDraft = null, draft = null) }
    fun discardAndReload() {
        if (state.value.isSaving) return
        discardDraft()
        mutableState.update { it.copy(requiresReload = false) }
        refresh()
    }

    private fun cancelRefresh() {
        // 保存后不能再由较早发起的刷新覆盖新 revision。
        refreshJob?.cancel()
        refreshJob = null
        mutableState.update { it.copy(isLoading = false) }
    }
}
