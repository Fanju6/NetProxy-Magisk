package com.fanjv.netproxy.feature.catalog.presentation.nodes

import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.toUiText
import com.fanjv.netproxy.feature.catalog.data.NodeImportStore
import com.fanjv.netproxy.feature.catalog.data.NodeRepository
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

internal data class CatalogNodesUiState(
    val operation: String = "",
    val notice: UiText = UiText.Empty,
    val error: UiText = UiText.Empty,
    val noticeId: Long = 0,
)

internal class CatalogNodesViewModel(
    private val repository: NodeRepository,
    private val importStore: NodeImportStore,
) : ViewModel() {
    private val mutable = MutableStateFlow(CatalogNodesUiState())
    val state = mutable.asStateFlow()

    fun addNode(link: String) {
        if (link.isBlank()) return
        runOperation("add", UiText.Resource(R.string.node_added)) { repository.add(link.trim()) }
    }

    fun importFile(uri: Uri) = runOperation("import", UiText.Resource(R.string.node_file_added)) {
        repository.import(importStore.read(uri))
    }

    suspend fun loadNodeConfigContent(reference: String): String = repository.get(reference)

    fun saveNodeConfigContent(nodeRef: String, content: String, onResult: (Boolean) -> Unit) =
        runOperation("edit", onResult = onResult) { repository.editJson(nodeRef, content) }

    fun clearNotice() = mutable.update { it.copy(notice = UiText.Empty, error = UiText.Empty) }

    private fun runOperation(
        operation: String,
        success: UiText = UiText.Empty,
        onResult: (Boolean) -> Unit = {},
        action: suspend () -> Unit,
    ) {
        if (mutable.value.operation.isNotEmpty()) return
        mutable.update { it.copy(operation = operation, error = UiText.Empty) }
        viewModelScope.launch {
            val error = try {
                action()
                UiText.Empty
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                error.toUiText()
            } finally {
                mutable.update { it.copy(operation = "") }
            }
            val ok = error == UiText.Empty
            val notice = if (ok) success else UiText.Empty
            if (error != UiText.Empty || notice != UiText.Empty) {
                mutable.update { it.copy(error = error, notice = notice, noticeId = it.noticeId + 1) }
            }
            onResult(ok)
        }
    }
}
