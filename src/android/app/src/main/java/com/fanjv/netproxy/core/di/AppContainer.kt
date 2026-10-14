package com.fanjv.netproxy.core.di

import android.content.Context
import android.widget.Toast
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.config.ConfigurationWrites
import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.module.AndroidModuleEnvironment
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.apps.data.AppPackageRepository
import com.fanjv.netproxy.feature.apps.data.AppPolicyRepository
import com.fanjv.netproxy.feature.catalog.data.NodeImportStore
import com.fanjv.netproxy.feature.catalog.data.NodeRepository
import com.fanjv.netproxy.feature.catalog.data.SubscriptionRepository
import com.fanjv.netproxy.feature.logs.data.LogRepository
import com.fanjv.netproxy.feature.inbound.data.InboundRepository
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import com.fanjv.netproxy.feature.theme.presentation.ThemeManager

/** 应用级依赖容器，保持 Repository 与 HTTP 客户端共享。 */
internal class AppContainer(context: Context) {
    private val appContext = context.applicationContext
    private val apiClient = ModuleApiClient(appContext)
    val configurationWrites = ConfigurationWrites(reportFailure = { reportConfigurationSaveFailure(it.userMessage()) })

    val serviceRepository = ServiceRepository(apiClient)
    val nodeRepository = NodeRepository(apiClient)
    val subscriptionRepository = SubscriptionRepository(apiClient)
    val configRepository = ConfigRepository(apiClient,
        awaitPendingWrites = configurationWrites::await)
    val appPolicyRepository = AppPolicyRepository(configRepository)
    val inboundRepository = InboundRepository(configRepository, serviceRepository)
    val logRepository = LogRepository(apiClient, appContext)
    val moduleEnvironment = AndroidModuleEnvironment(appContext, apiClient)
    val nodeImportStore = NodeImportStore(appContext)
    val appPackageRepository = AppPackageRepository(apiClient)
    val themeManager = ThemeManager(
        appContext.getSharedPreferences("settings", Context.MODE_PRIVATE)
    )
    val viewModelFactory = NetProxyViewModelFactory(this)

    private fun reportConfigurationSaveFailure(message: String) {
        Toast.makeText(appContext, "${appContext.getString(R.string.json_save_failed)}: $message", Toast.LENGTH_LONG).show()
    }
}
