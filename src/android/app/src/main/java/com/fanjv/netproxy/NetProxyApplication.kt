package com.fanjv.netproxy

import android.app.Application
import com.fanjv.netproxy.core.di.AppContainer
import com.fanjv.netproxy.feature.apps.data.AppIconCache

class NetProxyApplication : Application() {

    internal lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
    }

    override fun onTrimMemory(level: Int) {
        super.onTrimMemory(level)
        AppIconCache.trimMemory(level)
    }

    override fun onLowMemory() {
        super.onLowMemory()
        AppIconCache.clear()
    }
}
