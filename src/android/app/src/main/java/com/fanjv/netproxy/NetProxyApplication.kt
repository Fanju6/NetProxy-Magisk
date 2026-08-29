package com.fanjv.netproxy

import android.app.Application
import android.content.pm.ApplicationInfo
import android.os.Build
import com.fanjv.netproxy.core.di.AppContainer
import com.fanjv.netproxy.core.shell.ShellUtil
import com.fanjv.netproxy.feature.apps.data.AppIconCache
import org.lsposed.hiddenapibypass.HiddenApiBypass

class NetProxyApplication : Application() {

    internal lateinit var container: AppContainer
        private set

    companion object {
        fun setEnableOnBackInvokedCallback(appInfo: ApplicationInfo, enable: Boolean) {
            runCatching {
                val method = ApplicationInfo::class.java.getDeclaredMethod(
                    "setEnableOnBackInvokedCallback",
                    Boolean::class.javaPrimitiveType
                )
                method.isAccessible = true
                method.invoke(appInfo, enable)
            }
        }
    }

    override fun onCreate() {
        super.onCreate()
        ShellUtil.configure()
        container = AppContainer(this)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            HiddenApiBypass.addHiddenApiExemptions("Landroid/content/pm/ApplicationInfo;->setEnableOnBackInvokedCallback")
            val prefs = getSharedPreferences("settings", MODE_PRIVATE)
            val enablePredictiveBack = prefs.getBoolean("enable_predictive_back", false)
            setEnableOnBackInvokedCallback(applicationInfo, enablePredictiveBack)
        }
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
