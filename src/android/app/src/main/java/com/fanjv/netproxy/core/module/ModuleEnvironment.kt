package com.fanjv.netproxy.core.module

import android.app.ActivityManager
import android.content.Context
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.ModuleApiException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.cancelAndJoin

internal enum class ModuleConnectionState { READY, UNAVAILABLE, UNTRUSTED }
internal data class ModuleAvailability(val state: ModuleConnectionState) {
    val available: Boolean get() = state == ModuleConnectionState.READY
}

internal interface ModuleEnvironment {
    val totalMemoryBytes: Long
    suspend fun availability(): ModuleAvailability
    suspend fun reconnect(address: String)
}

internal class ModuleAccessViewModel(
    private val environment: ModuleEnvironment,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) : ViewModel(scope) {
    private val _state = MutableStateFlow<ModuleAvailability?>(null)
    val state = _state.asStateFlow()
    private var refreshJob: Job? = null

    suspend fun reconnect(address: String) {
        refreshJob?.cancelAndJoin()
        environment.reconnect(address)
        _state.value = ModuleAvailability(ModuleConnectionState.READY)
    }

    fun refresh() {
        if (refreshJob?.isActive == true) return
        refreshJob = viewModelScope.launch {
            _state.value = try {
                environment.availability()
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                ModuleAvailability(ModuleConnectionState.UNAVAILABLE)
            }
        }
    }
}

internal class AndroidModuleEnvironment(
    context: Context,
    private val client: ModuleApiClient
) : ModuleEnvironment {
    override val totalMemoryBytes: Long = ActivityManager.MemoryInfo().also { info ->
        (context.getSystemService(Context.ACTIVITY_SERVICE) as? ActivityManager)
            ?.getMemoryInfo(info)
    }.totalMem

    override suspend fun availability(): ModuleAvailability {
        return try {
            client.get("/info")
            ModuleAvailability(ModuleConnectionState.READY)
        } catch (error: CancellationException) { throw error }
        catch (error: ModuleApiException) {
            ModuleAvailability(if (error.resultCode == "api.unauthorized") ModuleConnectionState.UNTRUSTED else ModuleConnectionState.UNAVAILABLE)
        }
    }

    override suspend fun reconnect(address: String) = client.reconnect(address)
}
