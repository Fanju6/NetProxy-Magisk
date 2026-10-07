package com.fanjv.netproxy.feature.inbound.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
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
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject

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
    val diagnostic: String? = null,
    val isDiagnosing: Boolean = false,
    val errorCode: String = "",
    val error: String = ""
) {
    val isSaving: Boolean get() = applyingField != null
    val isInitialLoading: Boolean get() = snapshot == null && (isLoading || !requiresReload)
    val backend: String get() = snapshot?.backend.orEmpty()
    val native: JsonObject get() = snapshot?.partitions?.get(backend)?.let {
        inboundJson.parseToJsonElement(it.content).jsonObject.objectAt(backend)
    } ?: JsonObject(emptyMap())
    val hasConfiguration: Boolean get() = snapshot?.status != null && !requiresReload
    val editable: Boolean get() = hasConfiguration && !isSaving && pendingBackend == null
}

internal class InboundViewModel(
    private val repository: InboundRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
) : ViewModel(scope) {
    private val mutableState = MutableStateFlow(InboundUiState())
    val state = mutableState.asStateFlow()
    private var refreshJob: Job? = null

    fun refresh() {
        if (state.value.isLoading || state.value.isSaving) return
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
                        requiresReload = false, isLoading = false)
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
        if (!state.value.editable) return
        require(backend in setOf("ebpf", "tun"))
        cancelRefresh()
        mutableState.update { it.copy(applyingField = "backend", error = "") }
        viewModelScope.launch {
            try {
                val status = repository.status()
                mutableState.update { it.copy(applyingField = null) }
                if (status.requiresBackendSwitch(backend)) mutableState.update { it.copy(pendingBackend = backend) }
                else if (backend != state.value.backend) saveBackend(backend)
                else mutableState.update { it.copy(snapshot = it.snapshot?.copy(status = status)) }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                mutableState.update {
                    it.copy(snapshot = it.snapshot?.copy(status = null), applyingField = null,
                        requiresReload = true, error = error.userMessage())
                }
            }
        }
    }

    fun cancelBackendSwitch() = mutableState.update { it.copy(pendingBackend = null) }
    fun confirmBackendSwitch() {
        val backend = state.value.pendingBackend ?: return
        cancelBackendSwitch()
        saveBackend(backend, confirmed = true)
    }

    private fun saveBackend(backend: String, confirmed: Boolean = false) {
        val snapshot = state.value.snapshot?.partitions?.get("backend") ?: return
        save("backend", snapshot, JsonObject(mapOf("backend" to JsonPrimitive(backend))), confirmed)
    }

    fun setField(path: String, value: JsonElement?, backend: String = state.value.backend, revision: String? = null) =
        edit(path, backend, revision) { it.withPath(path.split('.'), value) }
    fun setDataPaths(value: String) = edit("paths") {
        it.withPath(listOf("local", "enabled"), JsonPrimitive(value != "shared"))
            .withPath(listOf("shared", "enabled"), JsonPrimitive(value != "local"))
    }
    fun setTunIpv6(enabled: Boolean) = edit("ipv6") { it.withTunIpv6(enabled) }
    fun setFilter(include: String, exclude: String, mode: String, values: List<String>, backend: String = state.value.backend, revision: String? = null) =
        edit(include, backend, revision) { it.withFilter(include, exclude, mode, values) }

    private fun edit(field: String, backend: String = state.value.backend, revision: String? = null, transform: (JsonObject) -> JsonObject) {
        if (!state.value.editable) return
        if (backend != state.value.backend) {
            mutableState.update { it.copy(requiresReload = true, errorCode = "config.conflict") }
            return
        }
        val snapshot = state.value.snapshot!!.partitions.getValue(backend)
        try {
            val updated = transform(state.value.native)
            save(backend, if (revision == null) snapshot else snapshot.copy(revision = revision), JsonObject(mapOf(backend to updated)), field = field)
        } catch (error: Exception) {
            mutableState.update { it.copy(error = error.userMessage()) }
        }
    }

    private fun save(partition: String, snapshot: ConfigSnapshot, value: JsonObject, confirmed: Boolean = false, field: String = partition) {
        if (!state.value.editable) return
        cancelRefresh()
        val content = inboundJson.encodeToString(value)
        val draft = InboundDraft(partition, ConfigSnapshot(content, snapshot.revision))
        mutableState.update { it.copy(applyingField = field, error = "", errorCode = "") }
        viewModelScope.launch {
            try {
                val revision = repository.apply("inbound/$partition", content, snapshot.revision, confirmed)
                val status = repository.status()
                mutableState.update { current ->
                    val previous = current.snapshot!!
                    val updated = previous.copy(
                        backend = if (partition == "backend") value.textAt("backend") else previous.backend,
                        partitions = previous.partitions + (partition to ConfigSnapshot(content, revision)),
                        status = status
                    )
                    current.copy(snapshot = updated, applyingField = null)
                }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                if (error is InboundSwitchConfirmationRequired) {
                    mutableState.update { it.copy(applyingField = null, pendingBackend = value.textAt("backend")) }
                    return@launch
                }
                val restored = try { repository.load() } catch (loadError: Exception) {
                    if (loadError is CancellationException) throw loadError
                    null
                }
                mutableState.update {
                    it.copy(snapshot = restored ?: it.snapshot?.copy(status = null), failedDraft = draft,
                        applyingField = null, requiresReload = true,
                        errorCode = (error as? NetProxyCtlException)?.resultCode.orEmpty(), error = error.userMessage())
                }
            }
        }
    }

    fun restart() {
        if (!state.value.editable) return
        cancelRefresh()
        mutableState.update { it.copy(applyingField = "restart", error = "") }
        viewModelScope.launch {
            try {
                repository.restart()
                val snapshot = repository.load()
                mutableState.update { it.copy(snapshot = snapshot, applyingField = null) }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                mutableState.update {
                    it.copy(snapshot = it.snapshot?.copy(status = null), applyingField = null,
                        requiresReload = true, error = error.userMessage())
                }
            }
        }
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
    fun discardDraft() = mutableState.update { it.copy(failedDraft = null) }

    private fun cancelRefresh() {
        // 保存后不能再由较早发起的刷新覆盖新 revision。
        refreshJob?.cancel()
        refreshJob = null
        mutableState.update { it.copy(isLoading = false) }
    }
}
