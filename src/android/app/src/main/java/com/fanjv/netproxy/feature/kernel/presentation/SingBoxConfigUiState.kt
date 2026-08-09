package com.fanjv.netproxy.feature.kernel.presentation

import androidx.compose.runtime.Immutable

@Immutable
data class SingBoxConfigUiState(
    val documents: List<SingBoxDocument> = emptyList(),
    val isLoadingDocuments: Boolean = false,
    val documentsError: Boolean = false,
    val activeDocumentId: String? = null,
    val activeDocumentContent: String = "",
    val isLoadingDocument: Boolean = false,
    val documentLoadError: Boolean = false
)


