package com.fanjv.netproxy

import android.graphics.drawable.Icon
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
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
        refreshTile()
    }

    override fun onClick() {
        super.onClick()
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
            try {
                repository.action(if (targetRunning) "start" else "stop")
            } catch (error: Exception) {
                if (error is CancellationException) throw error
            }
            syncTileState()
        }
    }

    private fun refreshTile() {
        if (toggleJob?.isActive == true) return
        refreshJob?.cancel()
        refreshJob = serviceScope.launch {
            syncTileState()
        }
    }

    private suspend fun syncTileState(): Boolean {
        return try {
            val isRunning = repository.status().state in
                    setOf("preparing", "starting", "ready")
            lastKnownRunning = isRunning
            applyTileState(isRunning)
            isRunning
        } catch (error: Exception) {
            if (error is CancellationException) throw error
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

    override fun onDestroy() {
        super.onDestroy()
        serviceScope.cancel()
    }
}
