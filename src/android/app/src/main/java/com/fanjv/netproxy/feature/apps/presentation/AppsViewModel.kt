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
                resolveLabels(selected.map(::splitAppId))
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
                        proxiedApps = selected,
                        masterAppList = master,
                        users = users,
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
        viewModelScope.launch {
            runCatching {
                repository.setEnabled(enabled)
                if (enabled && mode != null) repository.setMode(mode)
            }.onSuccess { refreshConfig() }
                .onFailure { error -> _state.update { it.copy(error = error.userMessage()) } }
        }
    }

    fun toggle(packageName: String, userId: String = "0") {
        viewModelScope.launch {
            val id = if (userId == "0") packageName else "$userId:$packageName"
            runCatching {
                if (isSelected(_state.value.proxiedApps, packageName, userId)) {
                    repository.remove(id)
                } else {
                    repository.add(id)
                }
            }.onSuccess { refreshConfig() }
                .onFailure { error -> _state.update { it.copy(error = error.userMessage()) } }
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
                    resolveLabels(selected.map(::splitAppId))
                    _state.update {
                        it.copy(
                            appProxyEnabled = config.enabled,
                            appProxyMode = config.mode,
                            proxiedApps = selected,
                            error = ""
                        )
                    }
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
                        isProxied = isSelected(
                            snapshot.proxiedApps,
                            app.packageName,
                            app.userId
                        ),
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
                async(Dispatchers.IO) {
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

    private fun activeItems(config: AppProxyConfig): List<String> =
        (if (config.mode == "blacklist") config.bypassApps else config.proxyApps)
            .split(' ')
            .filter(String::isNotBlank)

    private fun splitAppId(id: String): Pair<String, String> =
        if (':' in id) id.substringAfter(':') to id.substringBefore(':') else id to "0"

    private fun isSelected(selected: Set<String>, packageName: String, userId: String): Boolean =
        selected.contains(if (userId == "0") packageName else "$userId:$packageName") ||
            selected.contains("$userId:$packageName")
}
