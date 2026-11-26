package com.fanjv.netproxy.feature.traffic.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.feature.apps.data.AndroidApp
import com.fanjv.netproxy.feature.apps.data.AppIconCache
import com.fanjv.netproxy.feature.apps.data.AppPackageRepository
import com.fanjv.netproxy.feature.apps.presentation.AppInfoModel
import com.fanjv.netproxy.feature.traffic.data.CaptureApps
import com.fanjv.netproxy.feature.traffic.data.CaptureBatch
import com.fanjv.netproxy.feature.traffic.data.CaptureCertificate
import com.fanjv.netproxy.feature.traffic.data.CaptureDetail
import com.fanjv.netproxy.feature.traffic.data.CapturePage
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
import java.io.File

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
    val batches: TrafficContent<List<CaptureBatch>> = TrafficContent(),
    val selectedBatch: String? = null,
    val page: CapturePage? = null,
    val loadingOlder: Boolean = false,
    val olderError: Throwable? = null,
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
    val captureAppPolicy: CaptureApps? = null,
    val appCatalog: Map<String, AppInfoModel> = emptyMap(),
    val appDraft: CaptureApps? = null,
    val appDraftBase: CaptureApps? = null,
) {
    val busy: Boolean get() = loading || saving
}

internal class TrafficViewModel(
    private val repository: TrafficRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
    private val packages: AppPackageRepository? = null,
) : ViewModel(scope) {
    suspend fun downloadBody(id: String, side: String, destination: File, size: Int) =
        repository.downloadBody(id, side, destination, size)

    private val _state = MutableStateFlow(TrafficState())
    val state = _state.asStateFlow()
    private var started = false
    private var pendingCertificate: CaptureCertificate? = null
    private var detailJob: Job? = null
    private var olderJob: Job? = null
    private var pollJob: Job? = null

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
        cancelPaging()
        started = true
        _state.update { it.copy(loading = true, failure = null) }
        viewModelScope.launch {
            coroutineScope {
                val status = async { attempt { repository.status() } }
                val batches = async { attempt { repository.batches() } }
                val rules = async { attempt { repository.rules() } }
                val policy = async { attempt { repository.policy() } }
                val apps = async { attempt { repository.apps() } }
                val nextStatus = status.await()
                val nextBatches = batches.await()
                val selected = state.value.selectedBatch?.takeUnless { id ->
                    nextBatches.getOrNull()?.none { it.id == id } == true
                }
                val head = attempt { captureHead(selected) }
                val nextRules = rules.await()
                val nextPolicy = policy.await()
                val nextApps = apps.await()
                _state.update { current -> applyHead(current, head).copy(
                    status = current.status.refreshed(nextStatus),
                    batches = current.batches.refreshed(nextBatches),
                    selectedBatch = selected,
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
        val head = attempt { captureHead() }
        val batches = attempt { repository.batches() }
        _state.update { applyHead(it.copy(page = null), head).copy(status = TrafficContent(status),
            batches = it.batches.refreshed(batches), selectedBatch = null) }
    }

    fun selectBatch(id: String?) = mutate {
        val head = captureHead(id)
        _state.update { applyHead(it.copy(page = null), Result.success(head)).copy(selectedBatch = id) }
    }

    fun deleteBatch(id: String) = mutate {
        repository.deleteBatch(id)
        val batches = attempt { repository.batches() }
        val head = attempt { captureHead() }
        _state.update { applyHead(it.copy(page = null, records = TrafficContent()), head).copy(selectedBatch = null,
            batches = it.batches.refreshed(batches)) }
    }

    fun loadOlder() {
        val current = state.value
        val page = current.page ?: return
        val before = page.before ?: return
        if (current.busy || current.loadingOlder) return
        _state.update { it.copy(loadingOlder = true, olderError = null) }
        olderJob = viewModelScope.launch {
            val result = attempt {
                val older = repository.page(batch = page.batch, before = before, oldest = page.oldest)
                if (older.oldest != page.oldest) captureHead(current.selectedBatch) to true else older to false
            }
            _state.update { latest ->
                if (latest.page?.batch != page.batch || latest.page?.before != before) {
                    latest.copy(loadingOlder = false)
                } else result.fold(
                    { (older, reset) ->
                        if (reset) applyHead(latest, Result.success(older)).copy(loadingOlder = false)
                        else latest.copy(
                            page = latest.page?.copy(before = older.before),
                            records = TrafficContent(mergeRecords(latest.records.value.orEmpty(), older.entries)),
                            loadingOlder = false,
                        )
                    },
                    { latest.copy(loadingOlder = false, olderError = it) },
                )
            }
        }
    }

    fun pollRecords() {
        val current = state.value
        if (current.busy || current.loadingOlder || current.selectedBatch != null ||
            current.status.value?.running != true || pollJob?.isActive == true) return
        val previous = current.page ?: return
        pollJob = viewModelScope.launch {
            val update = attempt { repository.page(since = previous.since) }.getOrNull() ?: return@launch
            val reset = previous.since == null || previous.batch != update.batch || previous.oldest != update.oldest
            val head = if (reset && previous.since != null) attempt { captureHead() }.getOrNull() ?: return@launch else update
            _state.update { latest ->
                if (latest.page != previous || latest.selectedBatch != null) latest
                else if (reset) applyHead(latest, Result.success(head))
                else latest.copy(
                    page = previous.copy(since = head.since),
                    records = TrafficContent(mergeRecords(head.entries, latest.records.value.orEmpty())),
                )
            }
        }
    }

    private suspend fun captureHead(id: String? = null): CapturePage = try {
        repository.page(batch = id)
    } catch (error: ModuleApiException) {
        if (error.resultCode != "api.not_found") throw error
        CapturePage(repository.records(id), batch = id)
    }

    private fun applyHead(state: TrafficState, result: Result<CapturePage>): TrafficState = result.fold(
        { head ->
            val prior = state.records.value.orEmpty()
            val previous = state.page
            val headIDs = head.entries.mapNotNullTo(mutableSetOf()) { it.id }
            val retain = previous?.batch != null && previous.batch == head.batch && previous.oldest == head.oldest &&
                prior.any { it.id in headIDs }
            state.copy(
                page = if (retain) head.copy(before = previous?.before) else head,
                records = TrafficContent(if (retain) mergeRecords(head.entries, prior) else head.entries),
                olderError = null,
            )
        },
        { state.copy(records = state.records.copy(error = it)) },
    )

    private fun mergeRecords(newer: List<CaptureRecord>, older: List<CaptureRecord>): List<CaptureRecord> =
        (newer + older).distinctBy { it.id }

    private fun cancelPaging() {
        olderJob?.cancel()
        pollJob?.cancel()
        _state.update { it.copy(loadingOlder = false, olderError = null) }
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
        val saved = repository.saveApps(current.copy(disableQuic = disabled))
        _state.update { it.copy(captureAppPolicy = saved) }
    }

    fun clear() = mutate {
        val status = repository.clear()
        val batches = attempt { repository.batches() }
        val head = attempt { captureHead() }
        _state.update { it.copy(status = TrafficContent(status), records = TrafficContent(emptyList()),
            batches = it.batches.refreshed(batches), selectedBatch = null, page = head.getOrNull()) }
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
        if (state.value.appDraft != null) return@mutate
        val policy = repository.apps()
        val catalog = readAppCatalog()
        _state.update { it.copy(captureAppPolicy = policy, appCatalog = catalog, appDraft = policy, appDraftBase = policy) }
    }

    fun editCaptureApps(apps: Set<String>?) {
        if (!state.value.saving) _state.update {
            if (apps == null) it.copy(appDraft = null, appDraftBase = null)
            else it.copy(appDraft = it.appDraft?.withSelection(apps))
        }
    }

    fun setCaptureAppMode(mode: String) {
        if (!state.value.busy) _state.update { it.copy(appDraft = it.appDraft?.copy(mode = mode)) }
    }

    fun saveCaptureApps(onSaved: () -> Unit) = mutate {
        val draft = state.value.appDraft ?: return@mutate
        if (repository.apps() != state.value.appDraftBase) throw TrafficChangedException()
        val saved = repository.saveApps(draft)
        _state.update { it.copy(captureAppPolicy = saved, appDraft = null, appDraftBase = null) }
        onSaved()
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

    fun setRuleEnabled(index: Int, enabled: Boolean) {
        val rules = state.value.rules.value ?: return
        val rule = rules.getOrNull(index) ?: return
        if (rule.enabled == enabled) return
        val updated = rules.toMutableList().apply { this[index] = rule.copy(enabled = enabled) }
        mutate {
            val saved = repository.saveRules(rules, updated)
            _state.update { it.copy(rules = TrafficContent(saved)) }
        }
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
        cancelPaging()
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
