package com.fanjv.netproxy.core.di

import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewmodel.CreationExtras
import androidx.lifecycle.viewmodel.compose.viewModel
import com.fanjv.netproxy.NetProxyApplication
import com.fanjv.netproxy.feature.apps.presentation.AppsViewModel
import com.fanjv.netproxy.feature.catalog.presentation.nodes.CatalogNodesViewModel
import com.fanjv.netproxy.feature.catalog.presentation.subscriptions.SubscriptionDetailsViewModel
import com.fanjv.netproxy.feature.catalog.presentation.subscriptions.SubscriptionEditorViewModel
import com.fanjv.netproxy.feature.catalog.presentation.subscriptions.SubscriptionsViewModel
import com.fanjv.netproxy.feature.dashboard.presentation.CatalogDashboardViewModel
import com.fanjv.netproxy.feature.kernel.presentation.SingBoxConfigViewModel
import com.fanjv.netproxy.feature.logs.presentation.LogsViewModel
import com.fanjv.netproxy.feature.settings.presentation.SettingsViewModel
import com.fanjv.netproxy.feature.theme.presentation.ThemeViewModel

/** 在应用组合根集中创建 ViewModel，业务类不再依赖 Application 或服务定位器。 */
internal class NetProxyViewModelFactory(
    private val container: AppContainer
) : ViewModelProvider.Factory {
    @Suppress("UNCHECKED_CAST")
    override fun <T : ViewModel> create(modelClass: Class<T>, extras: CreationExtras): T =
        when (modelClass) {
            CatalogDashboardViewModel::class.java -> CatalogDashboardViewModel(
                container.serviceRepository,
                container.moduleEnvironment
            )

            CatalogNodesViewModel::class.java -> CatalogNodesViewModel(
                container.nodeRepository,
                container.nodeImportStore
            )

            SubscriptionsViewModel::class.java ->
                SubscriptionsViewModel(container.subscriptionRepository)

            SubscriptionDetailsViewModel::class.java ->
                SubscriptionDetailsViewModel(
                    container.subscriptionRepository,
                    container.nodeRepository
                )

            SubscriptionEditorViewModel::class.java ->
                SubscriptionEditorViewModel(container.subscriptionRepository)

            AppsViewModel::class.java -> AppsViewModel(
                container.appPolicyRepository,
                container.appPackageRepository
            )

            SettingsViewModel::class.java -> SettingsViewModel(
                container.configRepository,
                container.serviceRepository
            )

            SingBoxConfigViewModel::class.java -> SingBoxConfigViewModel(
                container.configRepository,
                container.serviceRepository
            )

            LogsViewModel::class.java -> LogsViewModel(container.logRepository)
            ThemeViewModel::class.java -> ThemeViewModel(container.themeManager)
            else -> error("不支持的 ViewModel: ${modelClass.name}")
        } as T
}

/** 使用当前 Navigation3 条目的 ViewModelStore 获取已注入依赖的 ViewModel。 */
@Composable
internal inline fun <reified T : ViewModel> netProxyViewModel(): T {
    val application = LocalContext.current.applicationContext as NetProxyApplication
    return viewModel(factory = application.container.viewModelFactory)
}
