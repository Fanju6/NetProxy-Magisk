package com.fanjv.netproxy.core.di

import android.content.Context
import android.widget.Toast
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.command.CommandFileStore
import com.fanjv.netproxy.core.command.ConfigurationWrites
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.module.AndroidModuleEnvironment
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.apps.data.AppPackageRepository
import com.fanjv.netproxy.feature.apps.data.AppPolicyRepository
import com.fanjv.netproxy.feature.catalog.data.CatalogRepository
import com.fanjv.netproxy.feature.catalog.data.NodeImportStore
import com.fanjv.netproxy.feature.catalog.data.NodeRepository
import com.fanjv.netproxy.feature.catalog.data.SubscriptionRepository
import com.fanjv.netproxy.feature.logs.data.LogRepository
import com.fanjv.netproxy.feature.inbound.data.InboundRepository
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import com.fanjv.netproxy.feature.theme.presentation.ThemeManager

/** 应用级依赖容器，保持 Repository 单例并避免页面重复创建 Shell 客户端。 */
internal class AppContainer(context: Context) {
    private val appContext = context.applicationContext
    private val netProxyCtlClient = NetProxyCtlClient()
    private val commandFileStore = CommandFileStore(appContext.cacheDir)
    val configurationWrites = ConfigurationWrites(reportFailure = { reportConfigurationSaveFailure(it.userMessage()) })

    val serviceRepository = ServiceRepository(netProxyCtlClient)
    private val catalogRepository = CatalogRepository(netProxyCtlClient, commandFileStore)
    val nodeRepository = NodeRepository(catalogRepository)
    val subscriptionRepository = SubscriptionRepository(catalogRepository)
    val configRepository = ConfigRepository(netProxyCtlClient, commandFileStore,
        awaitPendingWrites = configurationWrites::await)
    val appPolicyRepository = AppPolicyRepository(configRepository)
    val inboundRepository = InboundRepository(configRepository, serviceRepository)
    val logRepository = LogRepository(netProxyCtlClient, appContext)
    val moduleEnvironment = AndroidModuleEnvironment(appContext, netProxyCtlClient)
    val nodeImportStore = NodeImportStore(appContext)
    val appPackageRepository = AppPackageRepository(appContext)
    val themeManager = ThemeManager(
        appContext.getSharedPreferences("settings", Context.MODE_PRIVATE)
    )
    val viewModelFactory = NetProxyViewModelFactory(this)

    private fun reportConfigurationSaveFailure(message: String) {
        Toast.makeText(appContext, "${appContext.getString(R.string.json_save_failed)}: $message", Toast.LENGTH_LONG).show()
    }
}
