package com.fanjv.netproxy.feature.settings.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.command.ConfigurationWrites
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import com.fanjv.netproxy.feature.settings.model.ModuleAutoStartConfig
import com.fanjv.netproxy.feature.settings.model.ModuleWifiConfig
import com.fanjv.netproxy.feature.settings.model.WifiPolicySettings
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.Job
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

internal class SettingsViewModel(
    private val repository: ConfigRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
    private val writes: ConfigurationWrites = ConfigurationWrites(scope)
) : ViewModel(scope) {
    private val mutableState = MutableStateFlow(SettingsUiState())
    val state = mutableState.asStateFlow()
    private var wifiSnapshot: ModuleWifiConfig? = null
    private var autoStartSnapshot: ModuleAutoStartConfig? = null
    private val confirmedWifi get() = wifiSnapshot?.wifi ?: WifiPolicySettings()
    private var saveJob: Job? = null
    private var requestedWifi: WifiPolicySettings? = null
    private var refreshJob: Job? = null

    fun setVisible(visible: Boolean) {
        if (visible) refresh()
    }

    fun ensureLoaded() {
        if (!state.value.hasLoaded) refresh()
    }

    fun refresh() {
        if (state.value.isLoading || state.value.isSaving || state.value.hasPendingWifi || state.value.requiresReload) return
        mutableState.value = state.value.copy(isLoading = true, error = "")
        refreshJob = viewModelScope.launch {
            try {
                mutableState.value = readSettings()
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                mutableState.value = state.value.copy(isLoading = false, error = error.userMessage())
            }
        }
    }

    fun setWifiSsidMode(value: String) = edit {
        if (value == "off") it.copy(enabled = false) else it.copy(enabled = true, mode = value)
    }
    fun setWifiSsids(values: List<String>) = edit { it.withSsids(values.distinct()) }
    fun setProxyOnNonWifi(value: Boolean) = edit { it.copy(proxyOnNonWifi = value) }
    suspend fun savedWifiNetworks(): List<String> = repository.savedWifiNetworks()

    private fun edit(transform: (WifiPolicySettings) -> WifiPolicySettings) {
        if (!state.value.hasLoaded || state.value.requiresReload) return
        refreshJob?.cancel()
        mutableState.update {
            val wifi = transform(it.wifi)
            it.copy(wifi = wifi, isLoading = false, hasPendingWifi = wifi != confirmedWifi || it.isSavingWifi, error = "")
        }
    }

    fun requestWifiFlush() {
        if (!state.value.hasPendingWifi || state.value.requiresReload) return
        requestedWifi = state.value.wifi
        if (saveJob?.isActive == true) return
        mutableState.update { it.copy(isSavingWifi = true) }
        saveJob = writes.launch("module/wifi", write = {
            try {
                while (requestedWifi != null) {
                    val saved = requestedWifi!!
                    requestedWifi = null
                    if (saved == confirmedWifi) continue
                    val revision = repository.applyWifi(saved, checkNotNull(wifiSnapshot).revision)
                    wifiSnapshot = ModuleWifiConfig(saved, revision)
                }
            } finally {
                mutableState.update { it.copy(isSavingWifi = false, hasPendingWifi = it.wifi != confirmedWifi) }
            }
        }, onFailure = { error ->
            requestedWifi = null
            mutableState.update { it.copy(requiresReload = true, error = error.userMessage()) }
        })
    }

    fun discardWifiAndReload() {
        if (saveJob?.isActive == true || state.value.isSaving) return
        mutableState.update { it.copy(wifi = confirmedWifi, hasPendingWifi = false, requiresReload = false) }
        refresh()
    }

    fun setAutoStartEnabled(value: Boolean) {
        if (!state.value.hasLoaded || state.value.isSavingAutoStart || state.value.isLoading) return
        val previous = autoStartSnapshot ?: return
        if (value == previous.enabled) return
        mutableState.update { it.copy(isSavingAutoStart = true, error = "") }
        writes.launch("module/auto_start", write = {
            try {
                val revision = repository.applyAutoStart(value, previous.revision)
                autoStartSnapshot = ModuleAutoStartConfig(value, revision)
                mutableState.update { it.copy(autoStartEnabled = value) }
            } finally {
                mutableState.update { it.copy(isSavingAutoStart = false) }
            }
        }, onFailure = { error ->
            autoStartSnapshot = null
            mutableState.update { it.copy(error = error.userMessage()) }
        })
    }

    private suspend fun readSettings(): SettingsUiState {
        val wifi = repository.readWifi()
        val autoStart = repository.readAutoStart()
        currentCoroutineContext().ensureActive()
        wifiSnapshot = wifi
        autoStartSnapshot = autoStart
        val settings = SettingsUiState(
            hasLoaded = true,
            autoStartEnabled = autoStart.enabled,
            wifi = wifi.wifi
        )
        return settings
    }
}
