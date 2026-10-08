package com.fanjv.netproxy.feature.apps.presentation

import androidx.compose.runtime.State
import androidx.compose.runtime.mutableStateOf
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.ui.component.SearchStatus
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
) : ViewModel(scope) {
    private var labels = ConcurrentHashMap<String, String>()
    private var packageLabels = ConcurrentHashMap<String, String>()
    private val _state = MutableStateFlow(AppsUiState())
    val state: StateFlow<AppsUiState> = _state.asStateFlow()
    private val _searchStatus = mutableStateOf(SearchStatus(""))
    val searchStatus: State<SearchStatus> = _searchStatus
    private var loadJob: Job? = null
    private var modelJob: Job? = null
    private var mutationJob: Job? = null
    private var loadRevision = 0L
    private var modelRevision = 0L
    private var confirmedConfig = AppProxyConfig()
    private var policyConfirmed = false
    private val pending = mutableListOf<PolicyIntent>()
    private val policyMutationMutex = Mutex()
    private val packageLookupDispatcher = Dispatchers.IO.limitedParallelism(4)
    private var loaded = false

    fun load(force: Boolean = false) {
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
        loadJob = viewModelScope.launch {
            _state.update { it.copy(isLoadingApps = true, error = "") }
            try {
                if (force) packageCatalog.invalidatePackageListingCaches()
                val (config, users) = coroutineScope {
                    val config = async {
                        policyMutationMutex.withLock {
                            val result = repository.config()
                            currentCoroutineContext().ensureActive()
                            if (revision == loadRevision) {
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
                        hasLoadedApps = true,
                        error = error.userMessage()
                    )
                }
            }
        }
    }

    fun setProxySettings(enabled: Boolean, mode: String? = null) {
        enqueue(PolicyIntent.Settings(enabled, mode))
    }

    fun toggle(appId: String) {
        val current = _state.value
        enqueue(PolicyIntent.Selection(appId, !current.proxiedApps.contains(appId), current.appProxyMode))
    }

    private fun enqueue(intent: PolicyIntent) {
        pending += intent
        publishPolicy()
        if (mutationJob?.isActive == true) return
        mutationJob = viewModelScope.launch {
            var failure = ""
            while (pending.isNotEmpty()) {
                policyMutationMutex.withLock {
                    val intent = pending.first()
                    // add/remove 操作当前确认模式，不能把失败模式下的勾选换成另一份名单。
                    if (intent is PolicyIntent.Selection && (!policyConfirmed || intent.mode != confirmedConfig.mode)) {
                        pending.removeAt(0)
                        publishPolicy(failure)
                        return@withLock
                    }
                    try {
                        val config = when (intent) {
                            is PolicyIntent.Settings -> if (intent.enabled && intent.mode != null) {
                                repository.setMode(intent.mode)
                            } else repository.setEnabled(intent.enabled)
                            is PolicyIntent.Selection -> if (intent.selected) repository.add(intent.id)
                                else repository.remove(intent.id)
                        }
                        currentCoroutineContext().ensureActive()
                        confirmedConfig = config
                        policyConfirmed = true
                        failure = ""
                    } catch (error: Exception) {
                        if (error is CancellationException) throw error
                        currentCoroutineContext().ensureActive()
                        failure = error.userMessage()
                        policyConfirmed = false
                        try {
                            val config = repository.config()
                            currentCoroutineContext().ensureActive()
                            confirmedConfig = config
                            policyConfirmed = true
                        } catch (readError: Exception) {
                            if (readError is CancellationException) throw readError
                            currentCoroutineContext().ensureActive()
                        }
                    }
                    // 回读只确认已执行的写入，尚未执行的意图始终叠加在确认配置之上。
                    pending.removeAt(0)
                    publishPolicy(failure)
                }
            }
        }
    }

    fun setShowSystemApps(show: Boolean) {
        _state.update { it.copy(showSystemApps = show) }
        applyFilterAndSort()
    }

    fun setSelectedFirst(enabled: Boolean) {
        _state.update { it.copy(appSelectedFirst = enabled) }
        applyFilterAndSort()
    }

    fun setReverseSort(enabled: Boolean) {
        _state.update { it.copy(appReverseSort = enabled) }
        applyFilterAndSort()
    }

    fun setShowPackageName(enabled: Boolean) {
        _state.update { it.copy(appShowPackageName = enabled) }
    }

    fun updateSearch(query: String) {
        _state.update {
            it.copy(appSearchQuery = query, searchResults = if (query.isBlank()) emptyList() else it.searchResults)
        }
        _searchStatus.value.searchText = query
        applyFilterAndSort()
    }

    private fun applyFilterAndSort() {
        val revision = ++modelRevision
        modelJob?.cancel()
        val snapshot = _state.value
        val currentLabels = labels
        _searchStatus.value.resultStatus = if (snapshot.appSearchQuery.isBlank()) {
            SearchStatus.ResultStatus.DEFAULT
        } else SearchStatus.ResultStatus.LOAD
        modelJob = viewModelScope.launch {
            val (apps, search) = withContext(modelDispatcher) {
                calculateAppsList(snapshot, currentLabels)
            }
            currentCoroutineContext().ensureActive()
            if (revision != modelRevision) return@launch
            _state.update { it.copy(allApps = apps, searchResults = search) }
            _searchStatus.value.resultStatus = when {
                snapshot.appSearchQuery.isBlank() -> SearchStatus.ResultStatus.DEFAULT
                search.isEmpty() -> SearchStatus.ResultStatus.EMPTY
                else -> SearchStatus.ResultStatus.SHOW
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
            isProxied = false,
            userId = userId,
            isSystem = isSystem
        )

    private fun activeItems(config: AppProxyConfig): Set<String> =
        activeItems(config.mode, parsePackages(config.proxyApps), parsePackages(config.bypassApps))

    private fun activeItems(
        mode: String,
        proxyApps: Set<String>,
        bypassApps: Set<String>
    ): Set<String> = if (mode == "blacklist") bypassApps else proxyApps

    private fun parsePackages(value: String): Set<String> =
        value.split(',').map(String::trim).filter(String::isNotBlank).toSet()

    private fun publishPolicy(error: String = "") {
        val config = pending.fold(confirmedConfig) { config, intent -> intent.applyTo(config) }
        val proxyApps = parsePackages(config.proxyApps)
        val bypassApps = parsePackages(config.bypassApps)
        _state.update {
            it.copy(
                appProxyEnabled = config.enabled,
                appProxyMode = config.mode,
                proxyApps = proxyApps,
                bypassApps = bypassApps,
                proxiedApps = activeItems(config.mode, proxyApps, bypassApps),
                error = error
            )
        }
        applyFilterAndSort()
    }

    private fun splitAppId(value: String): Pair<String, String>? {
        val separator = value.indexOf(':')
        if (separator <= 0 || separator == value.lastIndex) return null
        return value.substring(separator + 1) to value.substring(0, separator)
    }

    private sealed interface PolicyIntent {
        fun applyTo(config: AppProxyConfig): AppProxyConfig

        data class Settings(val enabled: Boolean, val mode: String?) : PolicyIntent {
            override fun applyTo(config: AppProxyConfig) =
                if (enabled && mode != null) config.copy(enabled = true, mode = mode)
                else config.copy(enabled = enabled)
        }

        data class Selection(val id: String, val selected: Boolean, val mode: String) : PolicyIntent {
            override fun applyTo(config: AppProxyConfig): AppProxyConfig {
                if (config.mode != mode) return config
                val items = (if (config.mode == "blacklist") config.bypassApps else config.proxyApps)
                    .split(',').filter(String::isNotBlank).toSet()
                val updated = (if (selected) items + id else items - id).joinToString(",")
                return if (config.mode == "blacklist") config.copy(bypassApps = updated)
                    else config.copy(proxyApps = updated)
            }
        }
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
            app.copy(isProxied = snapshot.proxiedApps.contains(app.id), label = labels[app.id] ?: app.label)
        }
        .toList()
    val comparator = if (snapshot.appSelectedFirst) {
        compareByDescending<AppInfoModel> { it.isProxied }.then(appLabelComparator)
    } else appLabelComparator
    apps = apps.sortedWith { left, right ->
        context.ensureActive()
        comparator.compare(left, right)
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
