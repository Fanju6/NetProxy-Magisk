package com.fanjv.netproxy.feature.routing.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.apps.data.AppIconCache
import com.fanjv.netproxy.feature.apps.data.AppPackageRepository
import com.fanjv.netproxy.feature.apps.presentation.AppInfoModel
import com.fanjv.netproxy.feature.proxy.data.editablePolicyTypes
import com.fanjv.netproxy.feature.proxy.data.parsePolicySection
import com.fanjv.netproxy.feature.proxy.data.text
import com.fanjv.netproxy.feature.routing.model.LocalRuleDocument
import com.fanjv.netproxy.feature.routing.model.LocalRuleSet
import com.fanjv.netproxy.feature.routing.model.RuleField
import com.fanjv.netproxy.feature.routing.model.RuleInputError
import com.fanjv.netproxy.feature.routing.model.policyRule
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

internal data class RuleIdentity(val target: LocalRuleSet, val index: Int)

internal data class RuleEditor(val identity: RuleIdentity?, val visible: Boolean = true)

internal data class RuleDraft(
    val target: LocalRuleSet,
    val index: Int? = null,
    val field: RuleField? = RuleField.DomainSuffix,
    val text: String = "",
    val policyTag: String? = null,
    val insertion: Int? = null,
    val reviewInsertion: Boolean = false,
)

internal data class RuleFileState(
    val document: LocalRuleDocument? = null,
    val revision: String = "",
    val requiresReload: Boolean = false,
    val invalidDocument: Boolean = false,
    val error: String = "",
)

internal data class RoutingRulesState(
    val files: Map<LocalRuleSet, RuleFileState> = LocalRuleSet.entries.associateWith { RuleFileState() },
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val draft: RuleDraft? = null,
    val deleting: RuleIdentity? = null,
    val inputError: RuleInputError? = null,
    val policyTargets: List<String> = emptyList(),
    val editor: RuleEditor? = null,
) {
    val rules: List<RuleIdentity> get() = LocalRuleSet.entries.flatMap { target ->
        files.getValue(target).document?.editableIndices?.map { RuleIdentity(target, it) }.orEmpty()
    }
    val hasDocuments: Boolean get() = files.values.any { it.document != null }
    val isEmpty: Boolean get() = rules.isEmpty() && files.values.all { it.document != null && !it.requiresReload && it.error.isEmpty() }
    val canAdd: Boolean get() = LocalRuleSet.entries.any(::canAddTo)

    fun canAddTo(target: LocalRuleSet): Boolean = editable(target) && (target != LocalRuleSet.Policy ||
        policyTargets.isNotEmpty())

    fun canSave(draft: RuleDraft): Boolean = editable(draft.target) && (draft.target != LocalRuleSet.Policy ||
        (draft.policyTag in policyTargets && (draft.index != null || policyInsertion(draft) != null)))

    fun needsInsertion(draft: RuleDraft): Boolean = draft.target == LocalRuleSet.Policy && draft.index == null &&
        (draft.reviewInsertion || files.getValue(draft.target).document?.automaticPolicyInsertion == null)

    fun policyInsertion(draft: RuleDraft): Int? {
        val document = files.getValue(LocalRuleSet.Policy).document ?: return null
        return if (needsInsertion(draft)) draft.insertion?.takeIf { it in 0..document.rules.size }
        else document.automaticPolicyInsertion
    }

    fun showPageError(target: LocalRuleSet): Boolean = target != draft?.target && target != deleting?.target &&
        files.getValue(target).let { it.error.isNotEmpty() || it.requiresReload }

    fun editable(target: LocalRuleSet): Boolean = files.getValue(target).let {
        it.document != null && !isLoading && !isSaving && !it.requiresReload
    }

    fun withFile(target: LocalRuleSet, file: RuleFileState): RoutingRulesState = copy(files = files + (target to file))
}

internal class RoutingRulesViewModel(
    private val repository: ConfigRepository,
    private val packages: AppPackageRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) : ViewModel(scope) {
    private val mutableState = MutableStateFlow(RoutingRulesState())
    val state = mutableState.asStateFlow()

    suspend fun installedApps(): List<AppInfoModel> {
        val snapshot = packages.snapshot()
        return withContext(Dispatchers.Default) {
            // package_name matches all profiles; use the owner entry when available.
            snapshot.users.sortedBy { it.id }.asSequence()
                .flatMap { it.apps.asSequence() }
                .distinctBy { it.packageName }
                .map { AppInfoModel(it.packageName, it.label.ifBlank { it.packageName }, iconKey = it.iconKey) }
                .sortedWith(compareBy(String.CASE_INSENSITIVE_ORDER, AppInfoModel::label).thenBy(AppInfoModel::packageName))
                .toList()
        }
    }

    suspend fun loadAppIcons(keys: Collection<String>) {
        AppIconCache.loadMissing(keys, packages)
    }

    fun selectTarget(target: LocalRuleSet) {
        val current = state.value
        val draft = current.draft ?: return
        if (draft.index != null || current.isLoading || current.isSaving) return
        mutableState.update {
            it.copy(draft = draft.copy(target = target, policyTag = null, insertion = null, reviewInsertion = false))
        }
    }

    fun selectPolicy(tag: String) {
        val current = state.value
        val draft = current.draft ?: return
        if (draft.target != LocalRuleSet.Policy || current.isLoading || current.isSaving || tag !in current.policyTargets) return
        mutableState.update { it.copy(draft = draft.copy(policyTag = tag)) }
    }

    fun selectInsertion(position: Int, revision: String) {
        val current = state.value
        val draft = current.draft ?: return
        val file = current.files.getValue(LocalRuleSet.Policy)
        if (!current.editable(LocalRuleSet.Policy) || !current.needsInsertion(draft) ||
            file.revision != revision || position !in 0..file.document!!.rules.size) return
        mutableState.update { it.copy(draft = draft.copy(insertion = position)) }
    }

    fun reloadPolicyDraft() {
        val current = state.value
        val draft = current.draft ?: return
        val target = LocalRuleSet.Policy
        val file = current.files.getValue(target)
        if (draft.target != target || draft.index != null || !file.requiresReload ||
            current.isLoading || current.isSaving) return
        mutableState.update {
            it.copy(isLoading = true, draft = draft.copy(insertion = null, reviewInsertion = true))
        }
        viewModelScope.launch {
            try {
                val updated = readFile(target, file)
                val targets = readPolicyTargets()
                mutableState.update { it.copy(policyTargets = targets).withFile(target, updated) }
            } finally {
                mutableState.update { it.copy(isLoading = false) }
            }
        }
    }

    fun openEditor(identity: RuleIdentity?, revision: String?, discardDraft: Boolean = false) {
        val current = state.value
        if (current.isLoading || current.isSaving || (!discardDraft && current.editor != null)) return
        val editor = RuleEditor(identity)
        mutableState.update { it.copy(editor = editor) }
        refresh(discardDraft) {
            if (state.value.editor != editor) return@refresh
            if (identity == null) add()
            else if (revision != null && state.value.files.getValue(identity.target).revision == revision) edit(identity)
        }
    }

    fun refresh(discardDraft: Boolean = false) {
        if (state.value.editor == null || discardDraft) refresh(discardDraft, onLoaded = {})
    }

    private fun refresh(discardDraft: Boolean, onLoaded: () -> Unit) {
        val current = state.value
        if (current.isLoading || current.isSaving || (!discardDraft && (current.draft != null || current.deleting != null))) return
        mutableState.update { it.copy(isLoading = true, draft = null, deleting = null, inputError = null) }
        viewModelScope.launch {
            try {
                val files = LocalRuleSet.entries.associateWith { target ->
                    readFile(target, current.files.getValue(target))
                }
                val policyTargets = readPolicyTargets()
                mutableState.update { it.copy(files = files, policyTargets = policyTargets) }
            } finally {
                mutableState.update { it.copy(isLoading = false) }
            }
            onLoaded()
        }
    }

    private suspend fun readFile(target: LocalRuleSet, fallback: RuleFileState): RuleFileState = try {
        val snapshot = repository.readSnapshot(target.target)
        val document = withContext(Dispatchers.Default) {
            try {
                LocalRuleDocument.parse(snapshot.content, target == LocalRuleSet.Policy)
            } catch (error: IllegalArgumentException) {
                null
            } catch (error: IllegalStateException) {
                null
            }
        }
        RuleFileState(document, snapshot.revision, requiresReload = document == null, invalidDocument = document == null)
    } catch (error: Exception) {
        if (error is CancellationException) throw error
        fallback.copy(requiresReload = true, error = error.userMessage())
    }

    private suspend fun readPolicyTargets(): List<String> = try {
        parsePolicySection(repository.read("singbox/outbounds"), "outbounds").mapNotNull { outbound ->
            outbound.text("tag").takeIf { it.isNotBlank() && outbound.text("type") in editablePolicyTypes }
        }.distinct()
    } catch (error: Exception) {
        if (error is CancellationException) throw error
        emptyList()
    }

    fun add() {
        val current = state.value
        if (current.draft != null || current.deleting != null) return
        val target = LocalRuleSet.entries.firstOrNull { current.canAddTo(it) } ?: return
        mutableState.update { it.copy(draft = RuleDraft(target), inputError = null)
            .withFile(target, it.files.getValue(target).copy(error = "")) }
    }

    fun edit(identity: RuleIdentity) {
        val current = state.value
        if (!current.editable(identity.target) || current.draft != null || current.deleting != null) return
        val document = current.files.getValue(identity.target).document!!
        if (identity.index !in document.editableIndices) return
        val rule = document.rules[identity.index]
        val policy = if (identity.target == LocalRuleSet.Policy) rule.policyRule() else null
        val simple = policy?.second ?: rule.simpleRule()
        mutableState.update { it.copy(draft = RuleDraft(identity.target, identity.index, simple?.field,
            simple?.values?.joinToString("\n").orEmpty(), policy?.first), inputError = null)
            .withFile(identity.target, it.files.getValue(identity.target).copy(error = "")) }
    }

    fun updateDraft(field: RuleField, text: String) {
        val current = state.value
        val draft = current.draft ?: return
        if (current.isSaving || current.deleting != null || draft.field == null) return
        mutableState.update {
            val file = it.files.getValue(draft.target)
            it.copy(draft = draft.copy(field = field, text = text), inputError = null)
                .withFile(draft.target, file.copy(error = if (file.requiresReload) file.error else ""))
        }
    }

    fun requestDelete(identity: RuleIdentity) {
        val current = state.value
        if (!current.editable(identity.target) || identity.index !in current.files.getValue(identity.target).document!!.editableIndices) return
        mutableState.update { it.copy(
            draft = it.draft?.takeIf { draft -> draft.target == identity.target && draft.index == identity.index },
            deleting = identity, inputError = null,
        )
            .withFile(identity.target, it.files.getValue(identity.target).copy(error = "")) }
    }

    fun cancelDelete() {
        if (!state.value.isSaving) mutableState.update { it.copy(deleting = null) }
    }

    fun closeEditor() {
        if (!state.value.isSaving) mutableState.update {
            it.copy(editor = it.editor?.copy(visible = false), deleting = null)
        }
    }

    fun dismiss() {
        if (!state.value.isSaving) mutableState.update {
            it.copy(editor = null, draft = null, deleting = null, inputError = null)
        }
    }

    fun save() {
        val current = state.value
        val draft = current.draft ?: return
        val field = draft.field ?: return
        if (current.deleting != null || !current.canSave(draft)) return
        val error = field.inputError(draft.text)
        if (error != null) {
            mutableState.update { it.copy(inputError = error) }
            return
        }
        val rule = if (draft.target == LocalRuleSet.Policy) field.policyRule(draft.text, requireNotNull(draft.policyTag))
            else field.rule(draft.text)
        val insertion = if (draft.target == LocalRuleSet.Policy && draft.index == null) current.policyInsertion(draft) else null
        apply(draft.target, current.files.getValue(draft.target).document!!.replace(draft.index, rule, insertion))
    }

    fun delete() {
        val current = state.value
        val identity = current.deleting ?: return
        if (!current.editable(identity.target)) return
        apply(identity.target, current.files.getValue(identity.target).document!!.remove(identity.index))
    }

    private fun apply(target: LocalRuleSet, document: LocalRuleDocument) {
        val current = state.value
        val file = current.files.getValue(target)
        mutableState.update { it.copy(isSaving = true).withFile(target, file.copy(error = "")) }
        viewModelScope.launch {
            try {
                val content = withContext(Dispatchers.Default) { document.content() }
                val revision = repository.apply(target.target, content, file.revision)
                mutableState.update { it.copy(
                    isSaving = false, draft = if (it.editor != null) it.draft else null, deleting = null,
                    inputError = null, editor = it.editor?.copy(visible = false),
                )
                    .withFile(target, RuleFileState(document, revision)) }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                val unchanged = if ((error as? ModuleApiException)?.resultCode == "config.conflict") false else try {
                    repository.readSnapshot(target.target).revision == file.revision
                } catch (readError: Exception) {
                    if (readError is CancellationException) throw readError
                    false
                }
                // 仅确认原 revision 未变化时允许修正草稿重试，不拿新 revision 覆盖旧快照。
                mutableState.update { it.copy(isSaving = false).withFile(target, file.copy(requiresReload = !unchanged, error = error.userMessage())) }
            }
        }
    }
}
