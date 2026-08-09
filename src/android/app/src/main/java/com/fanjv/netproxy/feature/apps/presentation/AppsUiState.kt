package com.fanjv.netproxy.feature.apps.presentation

import androidx.compose.runtime.Immutable
import com.fanjv.netproxy.feature.apps.model.UserInfo

@Immutable
data class AppInfoModel(
    val packageName: String,
    val label: String,
    val isProxied: Boolean,
    val userId: String = "0",
    val userIds: List<String> = listOf(userId),
    val isSystem: Boolean = false
)

@Immutable
data class AppsUiState(
    val appProxyEnabled: Boolean = true,
    val appProxyMode: String = "blacklist",
    val appAndroidUsers: Set<String> = emptySet(),
    val proxyApps: Set<String> = emptySet(),
    val bypassApps: Set<String> = emptySet(),
    val proxiedApps: Set<String> = emptySet(),
    val allApps: List<AppInfoModel> = emptyList(),
    val masterAppList: List<AppInfoModel> = emptyList(),
    val appSearchQuery: String = "",
    val searchResults: List<AppInfoModel> = emptyList(),
    val showSystemApps: Boolean = false,
    val users: List<UserInfo> = listOf(UserInfo("0", "Owner")),
    val appSelectedFirst: Boolean = true,
    val appReverseSort: Boolean = false,
    val appShowPackageName: Boolean = true,
    val isLoadingApps: Boolean = false,
    val hasLoadedApps: Boolean = false,
    val error: String = ""
)
