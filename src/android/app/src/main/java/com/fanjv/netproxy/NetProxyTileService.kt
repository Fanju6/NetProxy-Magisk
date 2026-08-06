package com.fanjv.netproxy

import android.graphics.drawable.Icon
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** 快捷设置磁贴：一键切换代理服务的启停，并同步磁贴状态。 */
class NetProxyTileService : TileService() {

    private val serviceScope = CoroutineScope(Dispatchers.Main + SupervisorJob())
    private var refreshJob: Job? = null
    private var toggleJob: Job? = null
    private var lastKnownRunning: Boolean? = null

    private val repository
        get() = (application as NetProxyApplication).container.serviceRepository

    override fun onStartListening() {
        super.onStartListening()
        if (AlphaExpiry.isExpired()) {
            applyExpiredState()
        } else {
            refreshTile()
        }
    }

    override fun onClick() {
        super.onClick()
        if (AlphaExpiry.isExpired()) {
            applyExpiredState()
            return
        }
        if (toggleJob?.isActive == true) return

        val currentRunning = lastKnownRunning ?: when (qsTile?.state) {
            Tile.STATE_ACTIVE -> true
            Tile.STATE_INACTIVE -> false
            else -> false
        }
        val targetRunning = !currentRunning

        refreshJob?.cancel()
        lastKnownRunning = targetRunning
        applyTileState(targetRunning)

        toggleJob = serviceScope.launch {
            val commandAccepted = runCatching {
                repository.action(if (targetRunning) "start" else "stop")
            }.isSuccess

            if (!commandAccepted) {
                syncTileState()
                return@launch
            }

            repeat(8) {
                delay(500)
                val actualRunning = syncTileState()
                if (actualRunning == targetRunning) return@launch
            }
        }
    }

    private fun refreshTile() {
        refreshJob?.cancel()
        refreshJob = serviceScope.launch {
            syncTileState()
        }
    }

    private suspend fun syncTileState(): Boolean {
        if (AlphaExpiry.isExpired()) {
            applyExpiredState()
            return false
        }
        return try {
            val isRunning = repository.status().state in
                    setOf("preparing", "starting", "ready")
            lastKnownRunning = isRunning
            applyTileState(isRunning)
            isRunning
        } catch (_: Exception) {
            lastKnownRunning ?: false
        }
    }

    private fun applyTileState(isRunning: Boolean) {
        val tile = qsTile ?: return
        tile.state = if (isRunning) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
        tile.icon = Icon.createWithResource(
            this,
            if (isRunning) R.drawable.ic_qs_active else R.drawable.ic_qs_inactive
        )
        tile.updateTile()
    }

    private fun applyExpiredState() {
        lastKnownRunning = null
        qsTile?.apply {
            state = Tile.STATE_UNAVAILABLE
            icon = Icon.createWithResource(this@NetProxyTileService, R.drawable.ic_qs_inactive)
            updateTile()
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        serviceScope.cancel()
    }
}
