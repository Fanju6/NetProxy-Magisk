package www.netproxy.web.ui

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.content.pm.PackageInfo
import android.content.pm.PackageManager
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import com.topjohnwu.superuser.ipc.RootService
import www.netproxy.web.ui.services.RootServices
import kotlin.coroutines.resume
import kotlin.coroutines.suspendCoroutine

data class AppInfo(val packageInfo: PackageInfo, val label: String) {
    val packageName: String get() = packageInfo.packageName
}

object AppList {
    private var cachedApps: List<AppInfo>? = null

    fun getApplist(): List<AppInfo> {
        return cachedApps ?: emptyList()
    }

    suspend fun getApps(context: Context): List<AppInfo> {
        cachedApps?.let { return it }
        val result = connectRootServiceAsync(context) ?: return emptyList()

        val (binder, _) = result
        try {
            val rootService = IKsuWebuiStandaloneInterface.Stub.asInterface(binder)
            val slice = rootService.getPackages(PackageManager.GET_META_DATA)
            val packages = slice.list
            val pm = context.packageManager
            val apps = packages.map { pkg ->
                val label = pm.getApplicationLabel(pkg.applicationInfo!!).toString()
                AppInfo(pkg, label)
            }
            cachedApps = apps
            return apps
        } catch (_: Exception) {
            return emptyList()
        } finally {
            val handler = Handler(Looper.getMainLooper())
            handler.post {
                stopRootService(context)
            }
        }
    }

    private suspend fun connectRootServiceAsync(context: Context): Pair<IBinder, ServiceConnection>? = suspendCoroutine { continuation ->
        var connectedBinder: IBinder? = null
        var connectedConnection: ServiceConnection?

        val connection = object : ServiceConnection {
            override fun onServiceConnected(name: ComponentName?, binder: IBinder?) {
                connectedBinder = binder
                connectedConnection = this
                continuation.resume(connectedBinder!! to connectedConnection)
            }

            override fun onServiceDisconnected(name: ComponentName?) {
                if (connectedBinder == null) {
                    continuation.resume(null)
                }
            }
        }

        val intent = Intent(context, RootServices::class.java)
        val handler = Handler(Looper.getMainLooper())
        handler.post {
            RootService.bind(intent, connection)
        }
    }

    private fun stopRootService(context: Context) {
        val intent = Intent(context, RootServices::class.java)
        RootService.stop(intent)
    }
}
