package com.fanjv.netproxy.feature.apps.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.command.ConfigurationWrites
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.apps.data.AppIconCache
import com.fanjv.netproxy.feature.apps.data.AppPackageRepository
import com.fanjv.netproxy.feature.apps.data.AppPolicyRepository
import com.fanjv.netproxy.feature.apps.model.AppProxyConfig
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import java.util.concurrent.ConcurrentHashMap

/** 管理 Android 应用清单与 netproxyctl 分应用策略。 */
internal class AppsViewModel(
    private val repository: AppPolicyRepository,
    private val packageCatalog: AppPackageRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
    private val modelDispatcher: CoroutineDispatcher = Dispatchers.Default,
    private val writes: ConfigurationWrites = ConfigurationWrites(scope),
) : ViewModel(scope) {
    private var labels = ConcurrentHashMap<String, String>()
    private var packageLabels = ConcurrentHashMap<String, String>()
    private val _state = MutableStateFlow(AppsUiState())
    val state: StateFlow<AppsUiState> = _state.asStateFlow()
    private var loadJob: Job? = null
    private var modelJob: Job? = null
    private var mutationJob: Job? = null
    private var loadRevision = 0L
    private var modelRevision = 0L
    private var confirmedConfig = AppProxyConfig()
    private var policyConfirmed = false
    private var draftConfig: AppProxyConfig? = null
    private var requestedConfig: AppProxyConfig? = null
    private val policyMutationMutex = Mutex()
    private val packageLookupDispatcher = Dispatchers.IO.limitedParallelism(4)
    private var loaded = false

    fun load(force: Boolean = false) {
        if (_state.value.requiresPolicyReload) return
        if (!force && (loadJob?.isActive == true || loaded)) return
        val revision = ++loadRevision
        loadJob?.cancel()
        if (force) {
            modelRevision++
            modelJob?.cancel()
            labels = ConcurrentHashMap()
            packageLabels = ConcurrentHashMap()
            AppIconCache.clear()
            loaded = false
        }
        _state.update { it.copy(isLoadingApps = true, error = "") }
        loadJob = viewModelScope.launch {
            try {
                if (force) packageCatalog.invalidatePackageListingCaches()
                val (config, users) = coroutineScope {
                    val config = async {
                        policyMutationMutex.withLock {
                            if (draftConfig != null || _state.value.isSavingPolicy) return@withLock draftConfig ?: confirmedConfig
                            val result = repository.config()
                            currentCoroutineContext().ensureActive()
                            if (revision == loadRevision && draftConfig == null && !_state.value.isSavingPolicy) {
                                confirmedConfig = result
                                policyConfirmed = true
                                publishPolicy()
                            }
                            result
                        }
                    }
                    val users = async { packageCatalog.getUsers() }
                    config.await() to users.await()
                }
                val selected = activeItems(config).toSet()
                resolveLabels(selected.mapNotNull(::splitAppId))
                val master = withContext(Dispatchers.IO) {
                    users.map { user ->
                        async {
                            val system = packageCatalog.getInstalledPackages(user.id, "system")
                            val regular = packageCatalog.getInstalledPackages(user.id, "user")
                            resolveLabels((system + regular).map { it to user.id })
                            buildList {
                                system.forEach { add(appModel(it, user.id, true)) }
                                regular.forEach { add(appModel(it, user.id, false)) }
                            }
                        }
                    }.awaitAll().flatten()
                }
                currentCoroutineContext().ensureActive()
                if (revision != loadRevision) return@launch
                loaded = true
                _state.update {
                    it.copy(
                        masterAppList = master,
                        isLoadingApps = false,
                        hasLoadedApps = true
                    )
                }
                applyFilterAndSort()
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                currentCoroutineContext().ensureActive()
                if (revision != loadRevision) return@launch
                _state.update {
                    it.copy(
                        isLoadingApps = false,
                        isFilteringApps = false,
                        hasLoadedApps = true,
                        error = error.userMessage()
                    )
                }
            }
        }
    }

    fun setProxySettings(enabled: Boolean, mode: String? = null) {
        editPolicy { it.copy(enabled = enabled, mode = if (enabled && mode != null) mode else it.mode) }
    }

    fun toggle(appId: String) {
        editPolicy { config ->
            val items = activeItems(config)
            val updated = (if (appId in items) items - appId else items + appId).toList()
            if (config.mode == "blacklist") config.copy(bypassApps = updated) else config.copy(proxyApps = updated)
        }
    }

    private fun editPolicy(transform: (AppProxyConfig) -> AppProxyConfig) {
        if (!policyConfirmed) return
        draftConfig = transform(draftConfig ?: confirmedConfig).takeIf(::policyChanged)
        publishPolicy()
    }

    fun requestPolicyFlush() {
        if (!policyConfirmed || (draftConfig == null && mutationJob?.isActive != true)) return
        requestedConfig = draftConfig ?: confirmedConfig
        if (mutationJob?.isActive == true) return
        _state.update { it.copy(isSavingPolicy = true) }
        mutationJob = writes.launch("inbound/app", write = {
            try {
                policyMutationMutex.withLock {
                    while (requestedConfig != null) {
                        val requested = requestedConfig!!.copy(revision = confirmedConfig.revision)
                        requestedConfig = null
                        if (!policyChanged(requested)) continue
                        val applied = repository.apply(requested)
                        currentCoroutineContext().ensureActive()
                        val latest = draftConfig ?: confirmedConfig
                        confirmedConfig = applied
                        // 确认结果只推进 revision，不能覆盖写入期间的新意图或撤销。
                        draftConfig = latest.copy(revision = applied.revision).takeIf(::policyChanged)
                        publishPolicy()
                    }
                }
            } finally {
                _state.update { it.copy(isSavingPolicy = false) }
                publishPolicy(_state.value.error)
            }
        }, onFailure = { error ->
            requestedConfig = null
            policyConfirmed = false
            _state.update { it.copy(requiresPolicyReload = true, error = error.userMessage()) }
        })
    }

    fun discardPolicyAndReload() {
        if (mutationJob?.isActive == true) return
        draftConfig = null
        _state.update { it.copy(requiresPolicyReload = false, hasPendingPolicy = false) }
        load(force = true)
    }

    fun setShowSystemApps(show: Boolean) {
        _state.update { it.copy(showSystemApps = show) }
        applyFilterAndSort()
    }

    fun setSelectedFirst(enabled: Boolean) {
        _state.update { it.copy(appSelectedFirst = enabled) }
    }

    fun setReverseSort(enabled: Boolean) {
        _state.update { it.copy(appReverseSort = enabled) }
        applyFilterAndSort()
    }

    fun setShowPackageName(enabled: Boolean) {
        _state.update { it.copy(appShowPackageName = enabled) }
    }

    fun updateSearch(query: String) {
        if (_state.value.appSearchQuery == query) return
        _state.update {
            it.copy(appSearchQuery = query, searchResults = emptyList())
        }
        applyFilterAndSort()
    }

    private fun applyFilterAndSort() {
        val revision = ++modelRevision
        modelJob?.cancel()
        val snapshot = _state.value
        val currentLabels = labels
        _state.update { it.copy(isFilteringApps = true) }
        modelJob = viewModelScope.launch {
            val (apps, search) = withContext(modelDispatcher) {
                calculateAppsList(snapshot, currentLabels)
            }
            currentCoroutineContext().ensureActive()
            if (revision != modelRevision) return@launch
            _state.update {
                it.copy(allApps = apps, searchResults = search, isFilteringApps = false)
            }
        }
    }

    private suspend fun resolveLabels(packageIds: List<Pair<String, String>>) {
        val labels = labels
        val packageLabels = packageLabels
        coroutineScope {
            packageIds.distinct().map { (packageName, userId) ->
                async(packageLookupDispatcher) {
                    val key = "$userId:$packageName"
                    if (labels.containsKey(key)) return@async
                    val label = packageLabels.computeIfAbsent(packageName) {
                        packageCatalog.label(packageName)
                    }
                    labels[key] = label
                }
            }.awaitAll()
        }
    }

    private fun appModel(packageName: String, userId: String, isSystem: Boolean) =
        AppInfoModel(
            packageName = packageName,
            label = labels["$userId:$packageName"] ?: packageName,
            userId = userId,
            isSystem = isSystem
        )

    private fun activeItems(config: AppProxyConfig): Set<String> =
        activeItems(config.mode, config.proxyApps.toSet(), config.bypassApps.toSet())

    private fun activeItems(
        mode: String,
        proxyApps: Set<String>,
        bypassApps: Set<String>
    ): Set<String> = if (mode == "blacklist") bypassApps else proxyApps

    private fun publishPolicy(error: String = "") {
        val config = draftConfig ?: confirmedConfig
        val proxyApps = config.proxyApps.toSet()
        val bypassApps = config.bypassApps.toSet()
        _state.update {
            it.copy(
                appProxyEnabled = config.enabled,
                appProxyMode = config.mode,
                proxyApps = proxyApps,
                bypassApps = bypassApps,
                proxiedApps = activeItems(config.mode, proxyApps, bypassApps),
                hasPendingPolicy = draftConfig != null || it.isSavingPolicy,
                error = error
            )
        }
    }

    private fun policyChanged(config: AppProxyConfig) =
        config.enabled != confirmedConfig.enabled || config.mode != confirmedConfig.mode ||
            config.proxyApps.toSet() != confirmedConfig.proxyApps.toSet() ||
            config.bypassApps.toSet() != confirmedConfig.bypassApps.toSet()

    private fun splitAppId(value: String): Pair<String, String>? {
        val separator = value.indexOf(':')
        if (separator <= 0 || separator == value.lastIndex) return null
        return value.substring(separator + 1) to value.substring(0, separator)
    }

}

internal suspend fun calculateAppsList(
    snapshot: AppsUiState,
    labels: Map<String, String>,
): Pair<List<AppInfoModel>, List<AppInfoModel>> {
    val context = currentCoroutineContext()
    var apps = snapshot.masterAppList
        .asSequence()
        .filter {
            context.ensureActive()
            snapshot.showSystemApps || !it.isSystem
        }
        .map { app ->
            val label = labels[app.id] ?: app.label
            if (label == app.label) app else app.copy(label = label)
        }
        .toList()
    apps = apps.sortedWith { left, right ->
        context.ensureActive()
        appLabelComparator.compare(left, right)
    }
    if (snapshot.appReverseSort) apps = apps.reversed()
    val query = snapshot.appSearchQuery
    val search = if (query.isBlank()) emptyList() else apps.filter {
        context.ensureActive()
        it.label.contains(query, true) || it.packageName.contains(query, true)
    }
    context.ensureActive()
    return apps to search
}

private val appLabelComparator = Comparator<AppInfoModel> { left, right ->
    val labelComparison = left.label.compareTo(right.label, ignoreCase = true)
    if (labelComparison != 0) labelComparison else left.id.compareTo(right.id)
}
