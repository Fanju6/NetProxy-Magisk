package com.fanjv.netproxy

import android.app.ActivityOptions
import android.app.PendingIntent
import android.content.Intent
import android.graphics.drawable.Icon
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import com.fanjv.netproxy.bridge.HttpServiceTileApi
import com.fanjv.netproxy.bridge.R
import com.fanjv.netproxy.bridge.ServiceTileClick
import com.fanjv.netproxy.bridge.ServiceTileController
import com.fanjv.netproxy.bridge.ServiceTileState
import com.fanjv.netproxy.core.api.ConnectionStore
import com.fanjv.netproxy.core.api.LocalHttpTransport
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.launch

/** 保留磁贴组件身份，但不依赖 V8 Application 容器。 */
class NetProxyTileService : TileService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private lateinit var controller: ServiceTileController
    private var refresh: Job? = null
    private var click: Job? = null
    private var listening = false
    private var destroyed = false
    private var unlockPending = false
    private var unlockEpoch = 0L

    override fun onCreate() {
        super.onCreate()
        val transport = LocalHttpTransport(ConnectionStore(applicationContext), allowProvisioning = false)
        controller = ServiceTileController(HttpServiceTileApi(transport), ::render)
    }

    override fun onStartListening() {
        super.onStartListening()
        listening = true
        if (click?.isActive == true) return
        refresh?.cancel()
        refresh = scope.launch { controller.refresh() }
    }

    override fun onStopListening() {
        listening = false
        unlockPending = false
        refresh?.cancel()
        super.onStopListening()
    }

    override fun onClick() {
        super.onClick()
        if (!listening || destroyed || unlockPending || click?.isActive == true) return
        if (isLocked) {
            unlockPending = true
            val epoch = ++unlockEpoch
            unlockAndRun {
                if (epoch != unlockEpoch) return@unlockAndRun
                unlockPending = false
                // 解锁界面可能已关闭 QS，不能因此丢弃用户已授权的点击。
                if (!destroyed && !isLocked) performClick()
            }
        } else performClick()
    }

    private fun performClick() {
        if (click?.isActive == true) return
        val previousRefresh = refresh
        click = scope.launch {
            previousRefresh?.cancelAndJoin()
            try {
                if (controller.click() == ServiceTileClick.OPEN_APP && !destroyed) openApp()
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                render(ServiceTileState.UNKNOWN)
            }
        }
    }

    private fun render(value: ServiceTileState) {
        if (!listening || destroyed) return
        val tile = qsTile ?: return
        val subtitle = getString(when (value) {
            ServiceTileState.READY -> R.string.netproxy_tile_ready
            ServiceTileState.STOPPED -> R.string.netproxy_tile_stopped
            ServiceTileState.PREPARING -> R.string.netproxy_tile_preparing
            ServiceTileState.STARTING -> R.string.netproxy_tile_starting
            ServiceTileState.STOPPING -> R.string.netproxy_tile_stopping
            ServiceTileState.FAILED -> R.string.netproxy_tile_failed
            ServiceTileState.UNKNOWN -> R.string.netproxy_tile_unknown
            ServiceTileState.CHECKING -> R.string.netproxy_tile_checking
            ServiceTileState.WORKING -> R.string.netproxy_tile_working
            ServiceTileState.BUSY -> R.string.netproxy_tile_busy
        })
        tile.state = when (value) {
            ServiceTileState.READY -> Tile.STATE_ACTIVE
            // 未知与失败必须可点击打开应用，不能使用禁用态或冒充已停止。
            ServiceTileState.UNKNOWN, ServiceTileState.FAILED, ServiceTileState.STOPPED -> Tile.STATE_INACTIVE
            else -> Tile.STATE_UNAVAILABLE
        }
        val title = getString(R.string.netproxy_tile_label)
        tile.label = if (Build.VERSION.SDK_INT >= 29) title else "$title: $subtitle"
        if (Build.VERSION.SDK_INT >= 29) tile.subtitle = subtitle
        tile.contentDescription = "$title: $subtitle"
        tile.icon = Icon.createWithResource(this, R.drawable.ic_netproxy_tile)
        tile.updateTile()
    }

    // The Intent overload is required only on API 26-33; newer systems use PendingIntent.
    @android.annotation.SuppressLint("StartActivityAndCollapseDeprecated")
    @Suppress("DEPRECATION")
    private fun openApp() {
        val intent = packageManager.getLaunchIntentForPackage(packageName) ?: return
        // 只打开本安装包的显式启动组件，不接受 URL、任意组件或中转组件。
        if (intent.component?.packageName != packageName) return
        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP)
        if (Build.VERSION.SDK_INT >= 34) {
            val options = if (Build.VERSION.SDK_INT >= 35) ActivityOptions.makeBasic().apply {
                // 仅用户点击时向 SystemUI 授予后台打开本应用的能力。
                setPendingIntentCreatorBackgroundActivityStartMode(ActivityOptions.MODE_BACKGROUND_ACTIVITY_START_ALLOWED)
            }.toBundle() else null
            val pending = PendingIntent.getActivity(this, 0, intent,
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT, options)
            startActivityAndCollapse(pending)
        } else startActivityAndCollapse(intent)
    }

    override fun onDestroy() {
        destroyed = true
        listening = false
        scope.cancel()
        super.onDestroy()
    }
}
