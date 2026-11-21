package com.fanjv.netproxy.feature.proxy.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.R
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
    val runtimeConfirmed: Boolean = runtime != null,
    val runtimeUnavailable: Boolean = false,
    val refreshing: Boolean = false,
    val stale: Boolean = false,
    val operating: Boolean = false,
    val notice: UiText = UiText.Empty,
    val noticeId: Long = 0,
    val noticeIsError: Boolean = false,
    val manualTests: Map<String, ManualPolicyTest> = emptyMap(),
) {
    val loading: Boolean get() = refreshing || (configured == null && !stale)
    val live: Boolean get() = !stale && runtimeConfirmed && snapshot?.live == true
    val snapshot: PolicySnapshot? by lazy {
        configured?.withRuntime(runtime.takeUnless { runtimeUnavailable }) { tag, member ->
            val test = manualTests[tag]
            runtimeConfirmed && test != null && test.coreIdentity == runtime?.coreIdentity && test.isNew(member)
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
    private var runtimeRead: Job? = null
    private var visible = false
    private var visibilityGeneration = 0L
    private var runtimeRevision = 0L

    fun setVisible(value: Boolean) {
        if (visible == value) return
        visible = value
        visibilityGeneration++
        if (!value) {
            changes?.cancel()
            read?.cancel()
            runtimeRead?.cancel()
            mutable.update { it.copy(runtimeConfirmed = false, refreshing = false, manualTests = emptyMap()) }
            return
        }
        refresh()
        refreshRuntime()
        changes = viewModelScope.launch {
            repository.changes().collect { result ->
                val snapshot = result.getOrNull()
                if (snapshot != null && (snapshot.live || mutable.value.runtime?.live != true)) {
                    runtimeRevision++
                    applyRuntime(snapshot)
                } else {
                    mutable.update { it.copy(runtimeConfirmed = false) }
                    refreshRuntime()
                }
                // This topic also announces changes to persisted config and Catalog.
                refresh()
            }
        }
    }

    fun refresh(showProgress: Boolean = false) {
        if (!visible || mutable.value.operating) return
        if (showProgress) refreshRuntime()
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

    private fun refreshRuntime() {
        if (!visible || mutable.value.operating) return
        runtimeRead?.cancel()
        val revision = runtimeRevision
        runtimeRead = viewModelScope.launch {
            try {
                val snapshot = repository.runtimeSnapshot()
                ensureActive()
                if (revision == runtimeRevision) applyRuntime(snapshot)
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                ensureActive()
                if (revision == runtimeRevision && !mutable.value.runtimeConfirmed) markRuntimeUnavailable()
            }
        }
    }

    private fun applyRuntime(snapshot: PolicySnapshot) {
        mutable.update {
            it.copy(
                runtime = snapshot,
                runtimeConfirmed = true,
                runtimeUnavailable = false,
                manualTests = it.manualTests.filterValues { test -> test.coreIdentity == snapshot.coreIdentity },
            )
        }
    }

    private fun markRuntimeUnavailable() = mutable.update {
        it.copy(runtimeConfirmed = false, runtimeUnavailable = true, manualTests = emptyMap())
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
        if (type !in editablePolicyTypes ||
            current.configured?.groups?.none { it.tag == tag && it.type == type } != false
        ) return
        updateGroups {
            repository.deleteGroup(tag, type)
            UiText.Empty
        }
    }

    fun createSubscriptionGroups(suffix: String) = updateGroups {
        val count = repository.createSubscriptionGroups(suffix)
        if (count == 0) UiText.Resource(R.string.policy_subscription_groups_none)
        else UiText.Resource(R.string.policy_subscription_groups_created, listOf(count))
    }

    private fun updateGroups(change: suspend () -> UiText) {
        if (!visible || mutable.value.stale || mutable.value.operating) return
        read?.cancel()
        mutable.update { it.copy(operating = true, notice = UiText.Empty) }
        viewModelScope.launch {
            try {
                val message = change()
                ensureActive()
                if (message != UiText.Empty) postNotice(message)
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                ensureActive()
                postNotice(error.toUiText(), isError = true)
            } finally {
                mutable.update { it.copy(operating = false) }
                refresh()
            }
        }
    }

    private fun operate(group: String, testDelay: Boolean = false, action: suspend (Long) -> PolicySnapshot?) {
        val current = mutable.value
        val snapshot = current.snapshot ?: return
        if (!visible || !current.live || current.operating ||
            snapshot.groups.none { it.tag == group && it.items.any { member -> member.runtimeAvailable } }
        ) return
        val generation = visibilityGeneration
        val test = if (testDelay) ManualPolicyTest(
            snapshot.coreIdentity,
            Instant.now().epochSecond,
            current.runtime?.groups?.firstOrNull { it.tag == group }?.items.orEmpty().associateBy { it.tag },
        ) else null
        read?.cancel()
        runtimeRead?.cancel()
        mutable.update { it.copy(operating = true, notice = UiText.Empty) }
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
                if (visible && generation == visibilityGeneration && confirmed?.coreIdentity == snapshot.coreIdentity &&
                    mutable.value.runtime?.coreIdentity == confirmed.coreIdentity && mutable.value.runtimeConfirmed
                ) {
                    runtimeRevision++
                    applyRuntime(confirmed)
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                ensureActive()
                postNotice(error.toUiText(), isError = true)
            } finally {
                mutable.update { it.copy(operating = false) }
                refresh()
                if (!mutable.value.runtimeConfirmed) refreshRuntime()
            }
        }
    }

    private fun postNotice(message: UiText, isError: Boolean = false) =
        mutable.update { it.copy(notice = message, noticeId = it.noticeId + 1, noticeIsError = isError) }

    fun clearNotice() = mutable.update { it.copy(notice = UiText.Empty) }
}
