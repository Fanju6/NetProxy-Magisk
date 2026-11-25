package com.fanjv.netproxy.feature.traffic.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.feature.apps.data.AndroidApp
import com.fanjv.netproxy.feature.apps.data.AppIconCache
import com.fanjv.netproxy.feature.apps.data.AppPackageRepository
import com.fanjv.netproxy.feature.apps.presentation.AppInfoModel
import com.fanjv.netproxy.feature.traffic.data.CaptureApps
import com.fanjv.netproxy.feature.traffic.data.CaptureCertificate
import com.fanjv.netproxy.feature.traffic.data.CaptureDetail
import com.fanjv.netproxy.feature.traffic.data.CaptureRecord
import com.fanjv.netproxy.feature.traffic.data.CaptureStatus
import com.fanjv.netproxy.feature.traffic.data.MitmPolicy
import com.fanjv.netproxy.feature.traffic.data.PinnedBypass
import com.fanjv.netproxy.feature.traffic.data.RewriteRule
import com.fanjv.netproxy.feature.traffic.data.TrafficChangedException
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
    val app: AndroidApp? = null,
)

internal data class TrafficState(
    val status: TrafficContent<CaptureStatus> = TrafficContent(),
    val records: TrafficContent<List<CaptureRecord>> = TrafficContent(),
    val rules: TrafficContent<List<RewriteRule>> = TrafficContent(),
    val policy: TrafficContent<MitmPolicy> = TrafficContent(),
    val pinned: TrafficContent<List<PinnedBypass>> = TrafficContent(),
    val loading: Boolean = true,
    val saving: Boolean = false,
    val failure: Throwable? = null,
    val draftError: Throwable? = null,
    val ruleDraft: RuleDraft? = null,
    val selectedRecord: RecordDetailState? = null,
    val fingerprint: String? = null,
    val captureApps: List<String>? = null,
    val captureAppPolicy: CaptureApps? = null,
    val appCatalog: Map<String, AppInfoModel> = emptyMap(),
    val appDraft: Set<String>? = null,
) {
    val busy: Boolean get() = loading || saving
}

internal class TrafficViewModel(
    private val repository: TrafficRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
    private val packages: AppPackageRepository? = null,
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
            launch {
                val result = attempt { repository.detail(id) }
                _state.update {
                    it.copy(selectedRecord = it.selectedRecord?.copy(loading = false,
                        detail = result.getOrNull(), error = result.exceptionOrNull()))
                }
            }
            val exit = record.exit
            val catalog = packages
            val name = record.sourceApp?.packageName ?: exit?.packageName
            val uid = record.sourceApp?.uid ?: exit?.uid
            if (catalog != null && name != null) {
                val app = attempt {
                    val matches = catalog.snapshot().users.asSequence().flatMap { it.apps.asSequence() }
                        .filter { it.packageName == name && (uid == null || it.uid == uid) }
                    matches.firstOrNull()
                }.getOrNull()
                if (app != null) {
                    _state.update { it.copy(selectedRecord = it.selectedRecord?.copy(app = app)) }
                    loadAppIcon(app.iconKey)
                }
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
                val apps = async { attempt { repository.apps() } }
                val nextStatus = status.await()
                val nextRecords = records.await()
                val nextRules = rules.await()
                val nextPolicy = policy.await()
                val nextApps = apps.await()
                _state.update { current -> current.copy(
                    status = current.status.refreshed(nextStatus),
                    records = current.records.refreshed(nextRecords),
                    rules = current.rules.refreshed(nextRules),
                    policy = current.policy.refreshed(nextPolicy),
                    captureAppPolicy = nextApps.getOrNull(),
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

    fun readPinned() = viewModelScope.launch {
        val result = attempt { repository.pinned() }
        _state.update { it.copy(pinned = it.pinned.refreshed(result)) }
    }

    fun clearPinned() = mutate {
        val entries = repository.clearPinned()
        _state.update { it.copy(pinned = TrafficContent(entries)) }
    }

    fun setCaptureDisableQuic(disabled: Boolean) = mutate {
        val current = state.value.captureAppPolicy ?: return@mutate
        if (repository.apps() != current) throw TrafficChangedException()
        val saved = repository.saveApps(current.apps, disabled)
        _state.update { it.copy(captureAppPolicy = saved) }
    }

    fun clear() = mutate {
        val status = repository.clear()
        _state.update { it.copy(status = TrafficContent(status), records = TrafficContent(emptyList())) }
    }

    fun loadAppCatalog() {
        viewModelScope.launch {
            val catalog = attempt { readAppCatalog() }.getOrNull() ?: return@launch
            _state.update { it.copy(appCatalog = catalog) }
        }
    }

    suspend fun loadAppIcon(key: String) {
        val catalog = packages ?: return
        if (key.isNotEmpty() && AppIconCache.cachedIcon(key) == null) {
            val generation = AppIconCache.generation
            attempt { AppIconCache.putIcon(key, catalog.icon(key), generation) }
        }
    }

    private suspend fun readAppCatalog(): Map<String, AppInfoModel> =
        packages?.snapshot()?.users.orEmpty().flatMap { user ->
            user.apps.map { AppInfoModel(it.packageName, it.label, user.id.toString(), it.system, it.iconKey) }
        }.associateBy { it.id }

    fun openAppPicker() = mutate {
        val policy = repository.apps()
        val apps = policy.apps
        val catalog = readAppCatalog()
        _state.update { it.copy(captureApps = apps, captureAppPolicy = policy, appCatalog = catalog, appDraft = apps.toSet()) }
    }

    fun editCaptureApps(apps: Set<String>?) {
        if (!state.value.saving) _state.update { it.copy(appDraft = apps) }
    }

    fun saveCaptureApps() = mutate {
        val draft = state.value.appDraft ?: return@mutate
        if (repository.apps().apps != state.value.captureApps) throw TrafficChangedException()
        val saved = repository.saveApps(draft.sorted())
        _state.update { it.copy(captureApps = saved.apps, captureAppPolicy = saved, appDraft = null) }
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
        savePolicy(policy, policy.copy(hosts = hosts.ifEmpty { listOf("*") }))
    }

    fun setBypassHosts(hosts: List<String>) {
        val policy = state.value.policy.value ?: return
        savePolicy(policy, policy.copy(bypassHosts = hosts))
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

    fun removeUserCertificate() = mutate { repository.removeUserCertificate() }

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
