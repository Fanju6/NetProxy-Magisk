package com.fanjv.netproxy.feature.apps.presentation

import androidx.compose.runtime.State
import androidx.compose.runtime.mutableStateOf
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.ui.component.SearchStatus
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.apps.data.AppPackageRepository
import com.fanjv.netproxy.feature.apps.data.AppPolicyRepository
import com.fanjv.netproxy.feature.apps.model.AppProxyConfig
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
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
    private val packageCatalog: AppPackageRepository
) : ViewModel() {
    private val labels = ConcurrentHashMap<String, String>()
    private val packageLabels = ConcurrentHashMap<String, String>()
    private val _state = MutableStateFlow(AppsUiState())
    val state: StateFlow<AppsUiState> = _state.asStateFlow()
    private val _searchStatus = mutableStateOf(SearchStatus(""))
    val searchStatus: State<SearchStatus> = _searchStatus
    private var loadJob: Job? = null
    private var modelJob: Job? = null
    private var searchJob: Job? = null
    private val policyMutationMutex = Mutex()
    private val packageLookupDispatcher = Dispatchers.IO.limitedParallelism(4)
    private var loaded = false

    fun load(force: Boolean = false) {
        if (loadJob?.isActive == true || (!force && loaded)) return
        loadJob = viewModelScope.launch {
            _state.update { it.copy(isLoadingApps = true, error = "") }
            if (force) {
                packageCatalog.invalidatePackageListingCaches()
                loaded = false
            }
            runCatching {
                coroutineScope {
                    val config = async { repository.config() }
                    val users = async { packageCatalog.getUsers() }
                    config.await() to users.await()
                }
            }.onSuccess { (config, users) ->
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
                loaded = true
                _state.update {
                    it.copy(
                        appProxyEnabled = config.enabled,
                        appProxyMode = config.mode,
                        proxyApps = parsePackages(config.proxyApps),
                        bypassApps = parsePackages(config.bypassApps),
                        proxiedApps = selected,
                        masterAppList = master,
                        isLoadingApps = false,
                        hasLoadedApps = true,
                        error = ""
                    )
                }
                applyFilterAndSort()
            }.onFailure { error ->
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
        val previous = _state.value
        _state.update {
            it.copy(
                appProxyEnabled = enabled,
                appProxyMode = mode ?: it.appProxyMode,
                proxiedApps = if (mode == null) it.proxiedApps else activeItems(
                    mode,
                    it.proxyApps,
                    it.bypassApps
                ),
                error = ""
            )
        }
        viewModelScope.launch {
            policyMutationMutex.withLock {
                runCatching {
                    if (enabled && mode != null) repository.setMode(mode)
                    else repository.setEnabled(enabled)
                }.onFailure { error ->
                    _state.update {
                        it.copy(
                            appProxyEnabled = previous.appProxyEnabled,
                            appProxyMode = previous.appProxyMode,
                            proxyApps = previous.proxyApps,
                            bypassApps = previous.bypassApps,
                            proxiedApps = previous.proxiedApps,
                            error = error.userMessage()
                        )
                    }
                    refreshConfig()
                }.onSuccess { config ->
                    applyPolicyConfig(config)
                    applyFilterAndSort()
                }
            }
        }
    }

    fun toggle(appId: String) {
        val wasSelected = _state.value.proxiedApps.contains(appId)
        _state.update {
            val updated = if (wasSelected) {
                it.proxiedApps - appId
            } else {
                it.proxiedApps + appId
            }
            it.copy(proxiedApps = updated, error = "")
        }
        applyFilterAndSort()
        viewModelScope.launch {
            policyMutationMutex.withLock {
                runCatching {
                    if (wasSelected) repository.remove(appId) else repository.add(appId)
                }.onFailure { error ->
                    _state.update { it.copy(error = error.userMessage()) }
                    refreshConfig()
                }.onSuccess { config ->
                    applyPolicyConfig(config)
                    applyFilterAndSort()
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
        _state.update { it.copy(appSearchQuery = query) }
        _searchStatus.value.searchText = query
        searchJob?.cancel()
        if (query.isEmpty()) {
            _searchStatus.value.resultStatus = SearchStatus.ResultStatus.DEFAULT
            _state.update { it.copy(searchResults = emptyList()) }
            return
        }
        searchJob = viewModelScope.launch(Dispatchers.Default) {
            _searchStatus.value.resultStatus = SearchStatus.ResultStatus.LOAD
            val result = _state.value.allApps.filter {
                it.label.contains(query, ignoreCase = true) ||
                        it.packageName.contains(query, ignoreCase = true)
            }
            if (_state.value.appSearchQuery != query) return@launch
            _state.update { it.copy(searchResults = result) }
            _searchStatus.value.resultStatus = if (result.isEmpty()) {
                SearchStatus.ResultStatus.EMPTY
            } else {
                SearchStatus.ResultStatus.SHOW
            }
        }
    }

    private fun refreshConfig() {
        viewModelScope.launch {
            runCatching { repository.config() }
                .onSuccess { config ->
                    val selected = activeItems(config).toSet()
                    resolveLabels(selected.mapNotNull(::splitAppId))
                    applyPolicyConfig(config)
                    applyFilterAndSort()
                }
                .onFailure { error -> _state.update { it.copy(error = error.userMessage()) } }
        }
    }

    private fun applyFilterAndSort() {
        modelJob?.cancel()
        modelJob = viewModelScope.launch(Dispatchers.Default) {
            val snapshot = _state.value
            var apps = snapshot.masterAppList
                .asSequence()
                .filter { snapshot.showSystemApps || !it.isSystem }
                .map { app ->
                    app.copy(
                        isProxied = snapshot.proxiedApps.contains(app.id),
                        label = labels["${app.userId}:${app.packageName}"] ?: app.label
                    )
                }
                .toList()
            val comparator = if (snapshot.appSelectedFirst) {
                compareByDescending<AppInfoModel> { it.isProxied }
                    .thenBy { it.label.lowercase() }
            } else {
                compareBy { it.label.lowercase() }
            }
            apps = apps.sortedWith(comparator)
            if (snapshot.appReverseSort) apps = apps.reversed()
            val query = snapshot.appSearchQuery
            val search = if (query.isBlank()) emptyList() else apps.filter {
                it.label.contains(query, true) || it.packageName.contains(query, true)
            }
            _state.update { it.copy(allApps = apps, searchResults = search) }
        }
    }

    private suspend fun resolveLabels(packageIds: List<Pair<String, String>>) {
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

    private fun applyPolicyConfig(config: AppProxyConfig) {
        val proxyApps = parsePackages(config.proxyApps)
        val bypassApps = parsePackages(config.bypassApps)
        _state.update {
            it.copy(
                appProxyEnabled = config.enabled,
                appProxyMode = config.mode,
                proxyApps = proxyApps,
                bypassApps = bypassApps,
                proxiedApps = activeItems(config.mode, proxyApps, bypassApps),
                error = ""
            )
        }
    }

    private fun splitAppId(value: String): Pair<String, String>? {
        val separator = value.indexOf(':')
        if (separator <= 0 || separator == value.lastIndex) return null
        return value.substring(separator + 1) to value.substring(0, separator)
    }
}
