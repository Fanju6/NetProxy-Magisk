package com.fanjv.netproxy.feature.apps.presentation

import androidx.compose.runtime.Immutable

@Immutable
data class AppInfoModel(
    val packageName: String,
    val label: String,
    val userId: String = "0",
    val isSystem: Boolean = false
) {
    val id: String get() = "$userId:$packageName"
}

@Immutable
data class AppsUiState(
    val appProxyEnabled: Boolean = true,
    val appProxyMode: String = "blacklist",
    val proxyApps: Set<String> = emptySet(),
    val bypassApps: Set<String> = emptySet(),
    val proxiedApps: Set<String> = emptySet(),
    val allApps: List<AppInfoModel> = emptyList(),
    val masterAppList: List<AppInfoModel> = emptyList(),
    val appSearchQuery: String = "",
    val searchResults: List<AppInfoModel> = emptyList(),
    val showSystemApps: Boolean = false,
    val appSelectedFirst: Boolean = true,
    val appReverseSort: Boolean = false,
    val appShowPackageName: Boolean = true,
    val isLoadingApps: Boolean = false,
    val isFilteringApps: Boolean = false,
    val hasLoadedApps: Boolean = false,
    val hasPendingPolicy: Boolean = false,
    val isSavingPolicy: Boolean = false,
    val requiresPolicyReload: Boolean = false,
    val error: String = ""
)

internal fun AppsUiState.orderedApps(items: List<AppInfoModel> = allApps): List<AppInfoModel> {
    if (!appSelectedFirst) return items
    val (selected, other) = items.partition { it.id in proxiedApps }
    return if (appReverseSort) other + selected else selected + other
}
