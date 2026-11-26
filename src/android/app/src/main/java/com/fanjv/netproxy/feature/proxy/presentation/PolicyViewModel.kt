package com.fanjv.netproxy.feature.proxy.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.toUiText
import com.fanjv.netproxy.feature.proxy.data.PolicyDelayStore
import com.fanjv.netproxy.feature.proxy.data.PolicyDelays
import com.fanjv.netproxy.feature.proxy.data.PolicyGroup
import com.fanjv.netproxy.feature.proxy.data.PolicyMember
import com.fanjv.netproxy.feature.proxy.data.PolicyRepository
import com.fanjv.netproxy.feature.proxy.data.PolicyRegexConflict
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
    val delays: PolicyDelays = PolicyDelays(),
    val manualTests: Map<String, ManualPolicyTest> = emptyMap(),
    val nodeLatency: Map<String, String> = emptyMap(),
    val nodeLatencyIdentity: Long = 0,
    val membershipNotice: Int? = null,
) {
    val loading: Boolean get() = refreshing || (configured == null && !stale)
    val live: Boolean get() = !stale && runtimeConfirmed && snapshot?.live == true
    val stopped: Boolean get() = !stale && runtimeConfirmed && runtime?.live == false

    fun selectedMember(group: PolicyGroup): String =
        if (stale) "" else if (stopped && group.type == "selector") group.configuredDefault else group.selected

    fun canSelect(group: PolicyGroup, member: PolicyMember): Boolean = !operating &&
        (live && group.selectable && member.runtimeAvailable ||
            stopped && group.type == "selector" && member.type.isNotEmpty())

    val snapshot: PolicySnapshot? by lazy {
        configured?.withRuntime(runtime.takeUnless { runtimeUnavailable }) { tag, member ->
            if (runtimeConfirmed && delays.coreIdentity == runtime?.coreIdentity) {
                delays.groups[tag]?.get(member.tag) ?: 0
            } else 0
        }
    }
}

internal data class ManualPolicyTest(
    val coreIdentity: Long,
    val requestedAtSeconds: Long,
    val previous: Map<String, PolicyMember>,
) {
    fun isNew(member: PolicyMember): Boolean {
        if (member.delay <= 0 || member.delayTime < requestedAtSeconds) return false
        val old = previous[member.tag] ?: return false
        // The core exposes whole seconds, so a changed result can also arrive in the same second.
        return member.delayTime > old.delayTime || (member.delayTime == old.delayTime && member.delay != old.delay)
    }
}

internal class PolicyViewModel(
    private val repository: PolicyRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
    private val delayStore: PolicyDelayStore? = null,
) : ViewModel(scope) {
    private val mutable = MutableStateFlow(PolicyState(delays = delayStore?.load() ?: PolicyDelays()))
    val state = mutable.asStateFlow()
    private var changes: Job? = null
    private var read: Job? = null
    private var runtimeRead: Job? = null
    private var visible = false
    private var visibilityGeneration = 0L
    private var runtimeRevision = 0L
    private var configuredEventRevision = ""
    private var configuredRefreshPending = false

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
                val revision = snapshot?.configRevision.orEmpty()
                if (revision.isEmpty() || revision != configuredEventRevision || mutable.value.stale) {
                    configuredEventRevision = revision
                    configuredRefreshPending = true
                    refresh()
                }
                if (snapshot != null && (snapshot.live || mutable.value.runtime?.live != true)) {
                    runtimeRevision++
                    applyRuntime(snapshot)
                } else {
                    mutable.update { it.copy(runtimeConfirmed = false) }
                    refreshRuntime()
                }
            }
        }
    }

    fun refresh(showProgress: Boolean = false) {
        if (!visible || mutable.value.operating) return
        configuredRefreshPending = false
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

    private fun applyRuntime(snapshot: PolicySnapshot, newTest: Pair<String, ManualPolicyTest>? = null) {
        val current = mutable.value
        var delays = current.delays
        val tests = current.manualTests.filterValues { snapshot.live && it.coreIdentity == snapshot.coreIdentity }
            .toMutableMap()
        if (snapshot.live && snapshot.coreIdentity > 0) {
            if (delays.coreIdentity != snapshot.coreIdentity) delays = PolicyDelays(snapshot.coreIdentity)
            if (newTest != null) {
                tests[newTest.first] = newTest.second
                delays = delays.copy(groups = delays.groups - newTest.first)
            }
            for (group in snapshot.groups) {
                val test = tests[group.tag] ?: continue
                val fresh = group.items.filter(test::isNew).associate { it.tag to it.delay }
                if (fresh.isEmpty()) continue
                val results = delays.groups[group.tag].orEmpty() + fresh
                delays = delays.copy(groups = delays.groups + (group.tag to results))
                // Later history updates must not overwrite an already captured manual result.
                val remaining = test.previous - fresh.keys
                if (remaining.isEmpty()) tests.remove(group.tag)
                else tests[group.tag] = test.copy(previous = remaining)
            }
        }
        if (delays != current.delays) delayStore?.save(delays)
        mutable.value = current.copy(
            runtime = snapshot,
            runtimeConfirmed = true,
            runtimeUnavailable = false,
            delays = delays,
            manualTests = tests,
        )
    }

    private fun markRuntimeUnavailable() = mutable.update {
        it.copy(runtimeConfirmed = false, runtimeUnavailable = true, manualTests = emptyMap())
    }

    fun select(group: String, member: String) {
        val current = mutable.value
        val target = current.snapshot?.groups?.firstOrNull { it.tag == group } ?: return
        val item = target.items.firstOrNull { it.tag == member } ?: return
        if (!current.canSelect(target, item) ||
            (current.selectedMember(target) == member && target.configuredDefault == member)
        ) return
        operate(group) { identity -> repository.select(identity, group, member) }
    }

    fun delay(group: String) = operate(group, testDelay = true) { identity ->
        repository.delay(identity, group)
        null
    }

    fun testNode(member: PolicyMember) {
        val state = mutable.value
        val identity = state.snapshot?.coreIdentity ?: return
        if (!visible || !state.live || !member.runtimeAvailable || state.operating ||
            (state.nodeLatencyIdentity == identity && state.nodeLatency[member.tag] == "testing...")
        ) return
        mutable.update {
            val previous = it.nodeLatency.takeIf { _ -> it.nodeLatencyIdentity == identity }.orEmpty()
            it.copy(nodeLatency = previous + (member.tag to "testing..."), nodeLatencyIdentity = identity)
        }
        viewModelScope.launch {
            val value = try {
                repository.testNodeLatency(identity, member).toString()
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                postNotice(error.toUiText(), isError = true)
                "failed"
            }
            mutable.update {
                if (it.nodeLatencyIdentity == identity) it.copy(nodeLatency = it.nodeLatency + (member.tag to value)) else it
            }
        }
    }

    fun addNodeToGroups(member: PolicyMember, groups: Set<String>) = updateGroups {
        try {
            val count = repository.addToGroups(member, groups)
            mutable.update { it.copy(membershipNotice = if (count == 0) R.string.policy_node_already_added else R.string.policy_node_added) }
            UiText.Empty
        } catch (_: PolicyRegexConflict) {
            postNotice(UiText.Resource(R.string.policy_provider_regex_conflict), isError = true)
            UiText.Empty
        }
    }

    fun clearMembershipNotice() = mutable.update { it.copy(membershipNotice = null) }

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
        if (!visible || current.operating || (!current.live && (testDelay || !current.stopped)) ||
            (testDelay && snapshot.groups.none { it.tag == group && it.items.any { member -> member.runtimeAvailable } })
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
                    val latest = mutable.value
                    val runtime = latest.runtime
                    if (runtime != null && latest.runtimeConfirmed && runtime.coreIdentity == test.coreIdentity) {
                        // An event may have delivered results before the request was acknowledged.
                        applyRuntime(runtime, group to test)
                    }
                }
                if (visible && generation == visibilityGeneration && confirmed != null) {
                    val latest = mutable.value
                    if (!confirmed.live && latest.stopped) {
                        mutable.update { it.copy(configured = confirmed) }
                    } else if (confirmed.live && confirmed.coreIdentity == snapshot.coreIdentity &&
                        latest.runtime?.coreIdentity == confirmed.coreIdentity && latest.runtimeConfirmed
                    ) {
                        runtimeRevision++
                        applyRuntime(confirmed)
                    }
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                ensureActive()
                postNotice(error.toUiText(), isError = true)
            } finally {
                mutable.update { it.copy(operating = false) }
                if (!testDelay || configuredRefreshPending || mutable.value.configured == null) refresh()
                if (!mutable.value.runtimeConfirmed) refreshRuntime()
            }
        }
    }

    private fun postNotice(message: UiText, isError: Boolean = false) =
        mutable.update { it.copy(notice = message, noticeId = it.noticeId + 1, noticeIsError = isError) }

    fun clearNotice() = mutable.update { it.copy(notice = UiText.Empty) }
}
