package com.fanjv.netproxy.feature.routing.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.routing.model.LocalRuleDocument
import com.fanjv.netproxy.feature.routing.model.LocalRuleSet
import com.fanjv.netproxy.feature.routing.model.RuleField
import com.fanjv.netproxy.feature.routing.model.RuleInputError
import com.fanjv.netproxy.feature.routing.model.simpleRule
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

internal data class RuleDraft(val index: Int? = null, val field: RuleField? = RuleField.DomainSuffix, val text: String = "")

internal data class RuleFileState(
    val document: LocalRuleDocument? = null,
    val revision: String = "",
    val requiresReload: Boolean = false,
    val invalidDocument: Boolean = false,
    val error: String = "",
)

internal data class RoutingRulesState(
    val selected: LocalRuleSet = LocalRuleSet.Proxy,
    val files: Map<LocalRuleSet, RuleFileState> = LocalRuleSet.entries.associateWith { RuleFileState() },
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val draft: RuleDraft? = null,
    val deleting: Int? = null,
    val inputError: RuleInputError? = null,
) {
    val file: RuleFileState get() = files.getValue(selected)
    val document: LocalRuleDocument? get() = file.document
    val revision: String get() = file.revision
    val requiresReload: Boolean get() = file.requiresReload
    val invalidDocument: Boolean get() = file.invalidDocument
    val error: String get() = file.error
    val editable: Boolean get() = document != null && !isLoading && !isSaving && !requiresReload

    fun withFile(file: RuleFileState): RoutingRulesState = copy(files = files + (selected to file))
}

internal class RoutingRulesViewModel(
    private val repository: ConfigRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) : ViewModel(scope) {
    private val mutableState = MutableStateFlow(RoutingRulesState())
    val state = mutableState.asStateFlow()

    fun select(selected: LocalRuleSet) {
        if (selected == state.value.selected || state.value.isLoading || state.value.isSaving || state.value.draft != null || state.value.deleting != null) return
        mutableState.update { it.copy(selected = selected, inputError = null) }
    }

    fun refresh(discardDraft: Boolean = false) {
        val current = state.value
        if (current.isLoading || current.isSaving || (!discardDraft && (current.draft != null || current.deleting != null))) return
        mutableState.update { it.copy(isLoading = true, draft = null, deleting = null, inputError = null) }
        viewModelScope.launch {
            try {
                val files = LocalRuleSet.entries.associateWith { selected ->
                    try {
                        val snapshot = repository.readSnapshot(selected.target)
                        val document = try {
                            withContext(Dispatchers.Default) { LocalRuleDocument.parse(snapshot.content) }
                        } catch (error: Exception) {
                            if (error is CancellationException) throw error
                            null
                        }
                        RuleFileState(document, snapshot.revision, requiresReload = document == null, invalidDocument = document == null)
                    } catch (error: Exception) {
                        if (error is CancellationException) throw error
                        current.files.getValue(selected).copy(requiresReload = true, error = error.userMessage())
                    }
                }
                mutableState.update { it.copy(files = files) }
            } finally {
                mutableState.update { it.copy(isLoading = false) }
            }
        }
    }

    fun edit(index: Int? = null) {
        if (!state.value.editable) return
        val simple = index?.let { state.value.document!!.rules[it].simpleRule() }
        mutableState.update { it.copy(draft = RuleDraft(index, if (index == null) RuleField.DomainSuffix else simple?.field,
            simple?.values?.joinToString("\n").orEmpty()), inputError = null).withFile(it.file.copy(error = "")) }
    }

    fun updateDraft(field: RuleField, text: String) {
        if (state.value.isSaving || state.value.draft == null) return
        mutableState.update { it.copy(draft = it.draft?.copy(field = field, text = text), inputError = null).withFile(it.file.copy(error = "")) }
    }

    fun requestDelete(index: Int) {
        if (state.value.editable) mutableState.update { it.copy(draft = null, deleting = index, inputError = null).withFile(it.file.copy(error = "")) }
    }

    fun dismiss() {
        if (!state.value.isSaving) mutableState.update { it.copy(draft = null, deleting = null, inputError = null) }
    }

    fun save() {
        val current = state.value
        val draft = current.draft ?: return
        val field = draft.field ?: return
        if (!current.editable) return
        val error = field.inputError(draft.text)
        if (error != null) {
            mutableState.update { it.copy(inputError = error) }
            return
        }
        apply(current.document!!.replace(draft.index, field.rule(draft.text)))
    }

    fun delete() {
        val current = state.value
        if (!current.editable) return
        apply(current.document!!.remove(current.deleting ?: return))
    }

    private fun apply(document: LocalRuleDocument) {
        val current = state.value
        mutableState.update { it.copy(isSaving = true).withFile(it.file.copy(error = "")) }
        viewModelScope.launch {
            try {
                val content = withContext(Dispatchers.Default) { document.content() }
                val revision = repository.apply(current.selected.target, content, current.revision)
                mutableState.update { it.copy(isSaving = false, draft = null, deleting = null, inputError = null)
                    .withFile(RuleFileState(document, revision)) }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                val unchanged = if ((error as? NetProxyCtlException)?.resultCode == "config.conflict") false else try {
                    repository.readSnapshot(current.selected.target).revision == current.revision
                } catch (readError: Exception) {
                    if (readError is CancellationException) throw readError
                    false
                }
                // 仅确认原 revision 未变化时允许修正草稿重试，不拿新 revision 覆盖旧快照。
                mutableState.update { it.copy(isSaving = false).withFile(it.file.copy(requiresReload = !unchanged, error = error.userMessage())) }
            }
        }
    }
}
