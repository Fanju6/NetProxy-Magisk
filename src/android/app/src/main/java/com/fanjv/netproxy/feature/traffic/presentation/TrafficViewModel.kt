package com.fanjv.netproxy.feature.traffic.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.feature.traffic.data.CaptureCertificate
import com.fanjv.netproxy.feature.traffic.data.CaptureDetail
import com.fanjv.netproxy.feature.traffic.data.CaptureRecord
import com.fanjv.netproxy.feature.traffic.data.CaptureStatus
import com.fanjv.netproxy.feature.traffic.data.MitmPolicy
import com.fanjv.netproxy.feature.traffic.data.RewriteRule
import com.fanjv.netproxy.feature.traffic.data.TrafficRepository
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

internal data class TrafficContent<T>(val value: T? = null, val error: Throwable? = null) {
    fun refreshed(result: Result<T>): TrafficContent<T> =
        result.fold({ TrafficContent(it) }, { copy(error = it) })
}

internal data class RuleDraft(
    val original: List<RewriteRule>,
    val index: Int?,
    val rule: RewriteRule,
)

internal data class RecordDetailState(
    val record: CaptureRecord,
    val loading: Boolean = false,
    val detail: CaptureDetail? = null,
    val error: Throwable? = null,
)

internal data class TrafficState(
    val status: TrafficContent<CaptureStatus> = TrafficContent(),
    val records: TrafficContent<List<CaptureRecord>> = TrafficContent(),
    val rules: TrafficContent<List<RewriteRule>> = TrafficContent(),
    val policy: TrafficContent<MitmPolicy> = TrafficContent(),
    val loading: Boolean = true,
    val saving: Boolean = false,
    val failure: Throwable? = null,
    val draftError: Throwable? = null,
    val ruleDraft: RuleDraft? = null,
    val selectedRecord: RecordDetailState? = null,
    val fingerprint: String? = null,
) {
    val busy: Boolean get() = loading || saving
}

internal class TrafficViewModel(
    private val repository: TrafficRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) : ViewModel(scope) {
    private val _state = MutableStateFlow(TrafficState())
    val state = _state.asStateFlow()
    private var started = false
    private var pendingCertificate: CaptureCertificate? = null
    private var detailJob: Job? = null

    fun openRecord(record: CaptureRecord) {
        detailJob?.cancel()
        val id = record.id
        _state.update { it.copy(selectedRecord = RecordDetailState(record, loading = id != null)) }
        if (id == null) return
        detailJob = viewModelScope.launch {
            val result = attempt { repository.detail(id) }
            _state.update {
                it.copy(selectedRecord = RecordDetailState(record,
                    detail = result.getOrNull(), error = result.exceptionOrNull()))
            }
        }
    }

    fun closeRecord() {
        detailJob?.cancel()
        _state.update { it.copy(selectedRecord = null) }
    }

    fun refresh() {
        if (started && state.value.busy) return
        started = true
        _state.update { it.copy(loading = true, failure = null) }
        viewModelScope.launch {
            coroutineScope {
                val status = async { attempt { repository.status() } }
                val records = async { attempt { repository.records() } }
                val rules = async { attempt { repository.rules() } }
                val policy = async { attempt { repository.policy() } }
                val nextStatus = status.await()
                val nextRecords = records.await()
                val nextRules = rules.await()
                val nextPolicy = policy.await()
                _state.update { current -> current.copy(
                    status = current.status.refreshed(nextStatus),
                    records = current.records.refreshed(nextRecords),
                    rules = current.rules.refreshed(nextRules),
                    policy = current.policy.refreshed(nextPolicy),
                    loading = false,
                ) }
            }
        }
    }

    fun setCapture(enabled: Boolean) = mutate {
        val status = repository.setCapture(enabled)
        val records = attempt { repository.records() }
        _state.update { it.copy(status = TrafficContent(status), records = it.records.refreshed(records)) }
    }

    fun clear() = mutate {
        val status = repository.clear()
        _state.update { it.copy(status = TrafficContent(status), records = TrafficContent(emptyList())) }
    }

    fun editRule(index: Int? = null) {
        if (state.value.busy) return
        val rules = state.value.rules.value ?: return
        val rule = if (index == null) RewriteRule("", "request", "") else rules.getOrNull(index) ?: return
        _state.update { it.copy(ruleDraft = RuleDraft(rules, index, rule), draftError = null) }
    }

    fun updateRule(rule: RewriteRule) {
        if (state.value.saving) return
        _state.update { it.copy(ruleDraft = it.ruleDraft?.copy(rule = rule), draftError = null) }
    }

    fun saveRule(delete: Boolean = false) {
        val draft = state.value.ruleDraft ?: return
        val updated = draft.original.toMutableList()
        val index = draft.index
        when {
            delete && index != null -> updated.removeAt(index)
            delete -> return
            index == null -> updated.add(draft.rule)
            else -> updated[index] = draft.rule
        }
        mutate(inDraft = true) {
            val saved = repository.saveRules(draft.original, updated)
            _state.update { it.copy(rules = TrafficContent(saved), ruleDraft = null) }
        }
    }

    fun setHosts(hosts: List<String>) {
        val policy = state.value.policy.value ?: return
        savePolicy(policy, policy.copy(hosts = hosts))
    }

    fun setMitmEnabled(enabled: Boolean) {
        val policy = state.value.policy.value ?: return
        savePolicy(policy, policy.copy(enabled = enabled))
    }

    fun setSystemTrust(enabled: Boolean) {
        val policy = state.value.policy.value ?: return
        savePolicy(policy, policy.copy(systemTrust = enabled))
    }

    private fun savePolicy(original: MitmPolicy, policy: MitmPolicy) =
        mutate {
            val saved = try {
                repository.savePolicy(original, policy)
            } catch (error: ModuleApiException) {
                repository.persistedPolicy(error)?.let { persisted ->
                    _state.update { it.copy(policy = TrafficContent(persisted)) }
                }
                throw error
            }
            _state.update { it.copy(policy = TrafficContent(saved)) }
        }

    fun prepareCertificate(launchExport: (String) -> Unit) = mutate {
        val cert = repository.certificate()
        pendingCertificate = cert
        _state.update { it.copy(fingerprint = cert.sha256) }
        launchExport(cert.filename)
    }

    fun readCertificateStatus() {
        viewModelScope.launch {
            val cert = attempt { repository.certificate(initialize = false) }.getOrNull()
            if (cert != null) _state.update { it.copy(fingerprint = cert.sha256) }
        }
    }

    fun takeCertificate(): CaptureCertificate? = pendingCertificate.also { pendingCertificate = null }

    private fun mutate(inDraft: Boolean = false, action: suspend () -> Unit) {
        if (state.value.busy) return
        _state.update { it.copy(saving = true, failure = null, draftError = null) }
        viewModelScope.launch {
            try {
                action()
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _state.update { if (inDraft) it.copy(draftError = error) else it.copy(failure = error) }
            } finally {
                _state.update { it.copy(saving = false) }
            }
        }
    }
}

private suspend fun <T> attempt(action: suspend () -> T): Result<T> = try {
    Result.success(action())
} catch (error: CancellationException) {
    throw error
} catch (error: Exception) {
    Result.failure(error)
}
