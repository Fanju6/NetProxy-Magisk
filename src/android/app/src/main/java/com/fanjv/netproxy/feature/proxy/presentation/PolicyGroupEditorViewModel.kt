package com.fanjv.netproxy.feature.proxy.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.toUiText
import com.fanjv.netproxy.feature.proxy.data.PolicyGroupDocument
import com.fanjv.netproxy.feature.proxy.data.PolicyGroupDraft
import com.fanjv.netproxy.feature.proxy.data.PolicyRepository
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

internal class PolicyGroupEditorViewModel(
    private val repository: PolicyRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) : ViewModel(scope) {
    private val editState = MutableStateFlow<PolicyEditorState?>(null)
    val editor = editState.asStateFlow()

    fun openEditor(tag: String?) {
        if (editState.value?.let { it.loading || it.document != null || it.saved } == true) return
        editState.value = PolicyEditorState(tag)
        viewModelScope.launch {
            try {
                val document = repository.openEditor(tag)
                ensureActive()
                editState.value = PolicyEditorState(tag, document, document.initial, loading = false)
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                ensureActive()
                editState.value = PolicyEditorState(tag, loading = false, error = error.toUiText())
            }
        }
    }

    fun changeGroupDraft(change: (PolicyGroupDraft) -> PolicyGroupDraft) {
        editState.update { current ->
            if (current == null || current.saving || current.conflict) current
            else current.copy(draft = current.draft?.let(change), error = UiText.Empty)
        }
    }

    fun saveEditor() {
        val current = editState.value ?: return
        val document = current.document ?: return
        val draft = current.draft ?: return
        if (!current.canSave) return
        editState.value = current.copy(saving = true, error = UiText.Empty)
        viewModelScope.launch {
            try {
                repository.saveGroup(document, draft)
                ensureActive()
                editState.value = current.copy(saved = true)
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                ensureActive()
                val conflict = (error as? ModuleApiException)?.resultCode == "config.conflict"
                editState.value = current.copy(error = error.toUiText(), conflict = conflict)
            }
        }
    }
}

internal data class PolicyEditorState(
    val originalTag: String?,
    val document: PolicyGroupDocument? = null,
    val draft: PolicyGroupDraft? = null,
    val loading: Boolean = true,
    val saving: Boolean = false,
    val conflict: Boolean = false,
    val saved: Boolean = false,
    val error: UiText = UiText.Empty,
) {
    val canSave: Boolean get() {
        val value = draft ?: return false
        val base = document ?: return false
        val tag = originalTag ?: value.tag.trim()
        return !loading && !saving && !conflict && !saved && value != base.initial &&
            tag.isNotEmpty() && value.hasMembers && tag !in value.outbounds &&
            (originalTag != null || tag !in base.memberTags)
    }
}
