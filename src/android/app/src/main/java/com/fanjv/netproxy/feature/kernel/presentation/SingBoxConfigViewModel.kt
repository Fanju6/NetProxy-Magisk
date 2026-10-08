package com.fanjv.netproxy.feature.kernel.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import com.fanjv.netproxy.feature.inbound.data.InboundRepository
import com.fanjv.netproxy.feature.inbound.data.InboundSwitchConfirmationRequired
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** 通过 netproxyctl 配置事务驱动 sing-box 配置工作台。 */
internal class SingBoxConfigViewModel(
    private val repository: ConfigRepository,
    private val serviceRepository: ServiceRepository,
    private val inboundRepository: InboundRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) : ViewModel(scope) {
    private val _state = MutableStateFlow(SingBoxConfigUiState())
    val state: StateFlow<SingBoxConfigUiState> = _state.asStateFlow()
    private var documentsRequest = 0L
    private var documentRequest = 0L
    private var documentsJob: Job? = null
    private var documentJob: Job? = null
    private val saveMutex = Mutex()

    fun refreshDocuments() {
        val request = ++documentsRequest
        documentsJob?.cancel()
        _state.update { it.copy(isLoadingDocuments = true, documentsError = false) }
        documentsJob = viewModelScope.launch {
            runCatching { repository.listDocuments() }
                .onSuccess { documents ->
                    ensureActive()
                    if (request != documentsRequest) return@onSuccess
                    _state.update {
                        it.copy(
                            documents = documents.map { document ->
                                SingBoxDocument(
                                    id = document.id,
                                    filename = document.filename,
                                    category = when {
                                        document.id == "inbound" || document.id.startsWith("inbound/") -> SingBoxDocumentCategory.Inbound
                                        document.category == "rules" -> SingBoxDocumentCategory.LocalRule
                                        document.category == "runtime" -> SingBoxDocumentCategory.Runtime
                                        else -> SingBoxDocumentCategory.Config
                                    },
                                    editable = document.editable,
                                    section = document.section
                                )
                            },
                            isLoadingDocuments = false,
                            documentsError = false
                        )
                    }
                }
                .onFailure { error ->
                    if (error is CancellationException) throw error
                    ensureActive()
                    if (request != documentsRequest) return@onFailure
                    _state.update {
                        it.copy(isLoadingDocuments = false, documentsError = true)
                    }
                }
        }
    }

    fun openDocument(id: String) {
        val request = ++documentRequest
        documentJob?.cancel()
        _state.update {
            it.copy(
                activeDocumentId = id,
                activeDocumentContent = "",
                activeDocumentRevision = "",
                isLoadingDocument = true,
                documentLoadError = false
            )
        }
        documentJob = viewModelScope.launch {
            runCatching { repository.readSnapshot(id) }
                .onSuccess { snapshot ->
                    ensureActive()
                    _state.update { state ->
                        if (request != documentRequest || state.activeDocumentId != id) state else state.copy(
                            activeDocumentContent = snapshot.content,
                            activeDocumentRevision = snapshot.revision,
                            isLoadingDocument = false,
                            documentLoadError = false
                        )
                    }
                }
                .onFailure { error ->
                    if (error is CancellationException) throw error
                    ensureActive()
                    _state.update { state ->
                        if (request != documentRequest || state.activeDocumentId != id) state else state.copy(
                            isLoadingDocument = false,
                            documentLoadError = true
                        )
                    }
                }
        }
    }

    fun saveDocument(
        id: String,
        content: String,
        expectedRevision: String,
        confirmBackendSwitch: Boolean = false,
        onComplete: (SingBoxDocumentSaveResult) -> Unit = {}
    ) {
        val request = ++documentRequest
        documentJob?.cancel()
        _state.update { it.copy(isLoadingDocument = false) }
        viewModelScope.launch {
            val result = runCatching {
                saveMutex.withLock {
                    ensureActive()
                    if (request != documentRequest) return@launch
                    val state = _state.value
                    check(state.activeDocumentId == id && expectedRevision.isNotEmpty())
                    if (id == "inbound" || id.startsWith("inbound/")) {
                        if (!confirmBackendSwitch && inboundRepository.requiresSwitchConfirmation(id, content)) {
                            ensureActive()
                            if (request == documentRequest) {
                                onComplete(SingBoxDocumentSaveResult(success = false, confirmationRequired = true))
                            }
                            return@launch
                        }
                        inboundRepository.apply(id, content, expectedRevision, confirmBackendSwitch).revision
                    } else repository.apply(id, content, expectedRevision)
                }
            }
            ensureActive()
            if (request != documentRequest) return@launch
            result
                .onSuccess { revision ->
                    _state.update { state ->
                        if (state.activeDocumentId != id) state else state.copy(
                            activeDocumentContent = content,
                            activeDocumentRevision = revision,
                            isLoadingDocument = false,
                            documentLoadError = false
                        )
                    }
                    onComplete(SingBoxDocumentSaveResult(success = true, revision = revision))
                }
                .onFailure { error ->
                    if (error is CancellationException) throw error
                    if (error is InboundSwitchConfirmationRequired) {
                        onComplete(SingBoxDocumentSaveResult(success = false, confirmationRequired = true))
                        return@onFailure
                    }
                    onComplete(
                        SingBoxDocumentSaveResult(
                            success = false,
                            errorMessage = error.userMessage(),
                            errorCode = (error as? NetProxyCtlException)?.resultCode.orEmpty(),
                            restored = false
                        )
                    )
                }
        }
    }

    fun checkConfig(onComplete: (Boolean) -> Unit = {}) {
        viewModelScope.launch {
            val result = runCatching { repository.check() }
            ensureActive()
            result.exceptionOrNull()?.let { if (it is CancellationException) throw it }
            onComplete(result.isSuccess)
        }
    }

    fun restartService() {
        viewModelScope.launch {
            runCatching { serviceRepository.action("restart") }
                .onFailure { if (it is CancellationException) throw it }
            ensureActive()
        }
    }
}
