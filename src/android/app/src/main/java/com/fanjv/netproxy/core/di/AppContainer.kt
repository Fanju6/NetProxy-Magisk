package com.fanjv.netproxy.core.di

import android.content.Context
import com.fanjv.netproxy.core.command.CommandFileStore
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.module.AndroidModuleEnvironment
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.feature.apps.data.AppPackageRepository
import com.fanjv.netproxy.feature.apps.data.AppPolicyRepository
import com.fanjv.netproxy.feature.logs.data.LogRepository
import com.fanjv.netproxy.feature.nodes.data.NodeImportStore
import com.fanjv.netproxy.feature.nodes.data.NodeRepository
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import com.fanjv.netproxy.feature.settings.theme.ThemeManager
import com.fanjv.netproxy.feature.subscriptions.data.SubscriptionRepository

/** 应用级依赖容器，保持 Repository 单例并避免页面重复创建 Shell 客户端。 */
internal class AppContainer(context: Context) {
    private val appContext = context.applicationContext
    private val netProxyCtlClient = NetProxyCtlClient()
    private val commandFileStore = CommandFileStore(appContext)

    val serviceRepository = ServiceRepository(netProxyCtlClient)
    val nodeRepository = NodeRepository(netProxyCtlClient)
    val subscriptionRepository = SubscriptionRepository(netProxyCtlClient, commandFileStore)
    val appPolicyRepository = AppPolicyRepository(netProxyCtlClient)
    val configRepository = ConfigRepository(netProxyCtlClient, commandFileStore)
    val logRepository = LogRepository(netProxyCtlClient, appContext)
    val moduleEnvironment = AndroidModuleEnvironment(appContext)
    val nodeImportStore = NodeImportStore(appContext)
    val appPackageRepository = AppPackageRepository(appContext)
    val themeManager = ThemeManager(
        appContext.getSharedPreferences("settings", Context.MODE_PRIVATE)
    )
    val viewModelFactory = NetProxyViewModelFactory(this)
}
