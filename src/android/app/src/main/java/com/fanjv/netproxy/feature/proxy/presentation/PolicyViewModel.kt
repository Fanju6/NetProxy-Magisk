package com.fanjv.netproxy.feature.proxy.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.toUiText
import com.fanjv.netproxy.feature.proxy.data.PolicyMember
import com.fanjv.netproxy.feature.proxy.data.PolicyRepository
import com.fanjv.netproxy.feature.proxy.data.PolicySnapshot
import com.fanjv.netproxy.feature.proxy.data.editablePolicyTypes
import com.fanjv.netproxy.feature.proxy.data.withRuntime
import java.time.Instant
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

internal data class PolicyState(
    val configured: PolicySnapshot? = null,
    val runtime: PolicySnapshot? = null,
    val refreshing: Boolean = false,
    val stale: Boolean = false,
    val pendingGroup: String? = null,
    val error: UiText = UiText.Empty,
    val noticeId: Long = 0,
    val manualTests: Map<String, ManualPolicyTest> = emptyMap(),
) {
    val loading: Boolean get() = refreshing || (configured == null && !stale)
    val live: Boolean get() = !stale && configured != null && runtime?.let { it.live && it.coreIdentity > 0 } == true
    val snapshot: PolicySnapshot? by lazy {
        configured?.withRuntime(runtime.takeIf { live }) { tag, member ->
            val test = manualTests[tag]
            test != null && test.coreIdentity == runtime?.coreIdentity && test.isNew(member)
        }
    }
}

internal data class ManualPolicyTest(
    val coreIdentity: Long,
    val requestedAtSeconds: Long,
    val previous: Map<String, PolicyMember>,
) {
    fun isNew(member: PolicyMember): Boolean {
        if (member.delayTime < requestedAtSeconds) return false
        val old = previous[member.tag] ?: return true
        // The core exposes whole seconds, so a changed result can also arrive in the same second.
        return member.delayTime > old.delayTime || (member.delayTime == old.delayTime && member.delay != old.delay)
    }
}

internal class PolicyViewModel(
    private val repository: PolicyRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) : ViewModel(scope) {
    private val mutable = MutableStateFlow(PolicyState())
    val state = mutable.asStateFlow()
    private var changes: Job? = null
    private var read: Job? = null
    private var visible = false
    private var visibilityGeneration = 0L

    fun setVisible(value: Boolean) {
        if (visible == value) return
        visible = value
        visibilityGeneration++
        if (!value) {
            changes?.cancel()
            read?.cancel()
            mutable.update { it.copy(runtime = null, refreshing = false, manualTests = emptyMap()) }
            return
        }
        refresh()
        changes = viewModelScope.launch {
            repository.changes().collect { result ->
                val runtime = result.getOrNull()
                mutable.update {
                    it.copy(runtime = runtime, manualTests = if (runtime == null) it.manualTests
                        else it.manualTests.filterValues { test -> test.coreIdentity == runtime.coreIdentity })
                }
                // This topic also announces changes to persisted config and Catalog.
                refresh()
            }
        }
    }

    fun refresh(showProgress: Boolean = false) {
        if (!visible || mutable.value.pendingGroup != null) return
        read?.cancel()
        if (showProgress) mutable.update { it.copy(refreshing = true) }
        read = viewModelScope.launch {
            try {
                val snapshot = repository.snapshot()
                ensureActive()
                mutable.update { it.copy(configured = snapshot, stale = false, refreshing = false) }
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                ensureActive()
                mutable.update { it.copy(stale = true, refreshing = false) }
            }
        }
    }

    fun select(group: String, member: String) {
        val current = mutable.value.snapshot?.groups?.firstOrNull { it.tag == group } ?: return
        if (!current.selectable || current.selected == member ||
            current.items.none { it.tag == member && it.runtimeAvailable }
        ) return
        operate(group) { identity -> repository.select(identity, group, member) }
    }

    fun delay(group: String) = operate(group, testDelay = true) { identity ->
        repository.delay(identity, group)
        null
    }

    fun deleteGroup(tag: String, type: String) {
        val current = mutable.value
        if (!visible || current.stale || current.pendingGroup != null || type !in editablePolicyTypes ||
            current.configured?.groups?.none { it.tag == tag && it.type == type } != false
        ) return
        read?.cancel()
        mutable.update { it.copy(pendingGroup = tag, error = UiText.Empty) }
        viewModelScope.launch {
            try {
                repository.deleteGroup(tag, type)
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                ensureActive()
                mutable.update { it.copy(error = error.toUiText(), noticeId = it.noticeId + 1) }
            } finally {
                mutable.update { it.copy(pendingGroup = null) }
                refresh()
            }
        }
    }

    private fun operate(group: String, testDelay: Boolean = false, action: suspend (Long) -> PolicySnapshot?) {
        val current = mutable.value
        val snapshot = current.snapshot ?: return
        if (!visible || !current.live || current.pendingGroup != null ||
            snapshot.groups.none { it.tag == group && it.items.any { member -> member.runtimeAvailable } }
        ) return
        val generation = visibilityGeneration
        val test = if (testDelay) ManualPolicyTest(
            snapshot.coreIdentity,
            Instant.now().epochSecond,
            current.runtime?.groups?.firstOrNull { it.tag == group }?.items.orEmpty().associateBy { it.tag },
        ) else null
        read?.cancel()
        mutable.update { it.copy(pendingGroup = group, error = UiText.Empty) }
        viewModelScope.launch {
            try {
                val confirmed = action(snapshot.coreIdentity)
                ensureActive()
                if (test != null && generation == visibilityGeneration) {
                    mutable.update {
                        if (it.runtime == null || it.runtime.coreIdentity == test.coreIdentity) {
                            it.copy(manualTests = it.manualTests + (group to test))
                        } else it
                    }
                }
                if (visible && confirmed?.coreIdentity == snapshot.coreIdentity) {
                    mutable.update {
                        if (it.runtime?.coreIdentity == confirmed.coreIdentity) it.copy(runtime = confirmed) else it
                    }
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                ensureActive()
                mutable.update { it.copy(error = error.toUiText(), noticeId = it.noticeId + 1) }
            } finally {
                mutable.update { it.copy(pendingGroup = null) }
                refresh()
            }
        }
    }

    fun clearNotice() = mutable.update { it.copy(error = UiText.Empty) }
}
