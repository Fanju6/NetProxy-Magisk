package com.fanjv.netproxy.feature.kernel.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** 通过 netproxyctl 配置事务驱动 sing-box 配置工作台。 */
internal class SingBoxConfigViewModel(
    private val repository: ConfigRepository,
    private val serviceRepository: ServiceRepository
) : ViewModel() {
    private val _state = MutableStateFlow(SingBoxConfigUiState())
    val state: StateFlow<SingBoxConfigUiState> = _state.asStateFlow()

    fun refreshDocuments() {
        viewModelScope.launch {
            _state.update { it.copy(isLoadingDocuments = true, documentsError = false) }
            runCatching { repository.listDocuments() }
                .onSuccess { documents ->
                    _state.update {
                        it.copy(
                            documents = documents.map { document ->
                                SingBoxDocument(
                                    id = document.id,
                                    filename = document.filename,
                                    category = when (document.category) {
                                        "rules" -> SingBoxDocumentCategory.LocalRule
                                        "runtime" -> SingBoxDocumentCategory.Runtime
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
                .onFailure {
                    _state.update {
                        it.copy(isLoadingDocuments = false, documentsError = true)
                    }
                }
        }
    }

    fun openDocument(id: String) {
        viewModelScope.launch {
            _state.update {
                it.copy(
                    activeDocumentId = id,
                    activeDocumentContent = "",
                    activeDocumentRevision = "",
                    isLoadingDocument = true,
                    documentLoadError = false
                )
            }
            runCatching { repository.readSnapshot(id) }
                .onSuccess { snapshot ->
                    _state.update { state ->
                        if (state.activeDocumentId != id) state else state.copy(
                            activeDocumentContent = snapshot.content,
                            activeDocumentRevision = snapshot.revision,
                            isLoadingDocument = false,
                            documentLoadError = false
                        )
                    }
                }
                .onFailure {
                    _state.update { state ->
                        if (state.activeDocumentId != id) state else state.copy(
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
        onComplete: (SingBoxDocumentSaveResult) -> Unit = {}
    ) {
        viewModelScope.launch {
            runCatching {
                val state = _state.value
                check(state.activeDocumentId == id && expectedRevision.isNotEmpty())
                repository.apply(id, content, expectedRevision)
            }
                .onSuccess { revision ->
                    _state.update { state ->
                        if (state.activeDocumentId != id) state else state.copy(
                            activeDocumentContent = content,
                            activeDocumentRevision = revision,
                            documentLoadError = false
                        )
                    }
                    onComplete(SingBoxDocumentSaveResult(success = true, revision = revision))
                }
                .onFailure { error ->
                    onComplete(
                        SingBoxDocumentSaveResult(
                            success = false,
                            errorMessage = error.userMessage(),
                            restored = true
                        )
                    )
                }
        }
    }

    fun checkConfig(onComplete: (Boolean) -> Unit = {}) {
        viewModelScope.launch {
            onComplete(runCatching { repository.check() }.isSuccess)
        }
    }

    fun restartService() {
        viewModelScope.launch {
            runCatching { serviceRepository.action("restart") }
        }
    }
}
