package com.fanjv.netproxy.core.module

import android.app.ActivityManager
import android.content.Context
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.shell.ShellUtil
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

internal data class ModuleAvailability(
    val rootGranted: Boolean,
    val moduleInstalled: Boolean
) {
    val available: Boolean get() = rootGranted && moduleInstalled
}

internal interface ModuleEnvironment {
    val totalMemoryBytes: Long
    suspend fun availability(): ModuleAvailability
}

internal class ModuleAccessViewModel(
    private val environment: ModuleEnvironment,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) : ViewModel(scope) {
    private val _state = MutableStateFlow<ModuleAvailability?>(null)
    val state = _state.asStateFlow()
    private var refreshJob: Job? = null

    fun refresh() {
        if (refreshJob?.isActive == true) return
        refreshJob = viewModelScope.launch {
            _state.value = try {
                environment.availability()
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                ModuleAvailability(false, false)
            }
        }
    }
}

internal class AndroidModuleEnvironment(
    context: Context,
    private val client: NetProxyCtlClient
) : ModuleEnvironment {
    override val totalMemoryBytes: Long = ActivityManager.MemoryInfo().also { info ->
        (context.getSystemService(Context.ACTIVITY_SERVICE) as? ActivityManager)
            ?.getMemoryInfo(info)
    }.totalMem

    override suspend fun availability(): ModuleAvailability {
        val root = ShellUtil.isRootAvailable()
        return ModuleAvailability(
            rootGranted = root,
            moduleInstalled = root && client.isAvailable()
        )
    }
}
