package com.fanjv.netproxy.feature.settings.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.command.ConfigurationWrites
import com.fanjv.netproxy.core.command.ShellConfigFile
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import com.fanjv.netproxy.feature.settings.model.ConfigSnapshot
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
    private var snapshot: ConfigSnapshot? = null
    private var confirmedWifi = WifiPolicySettings()
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

    fun setWifiAutoSwitch(value: Boolean) = edit { it.copy(enabled = value) }
    fun setWifiSsidMode(value: String) = edit { it.copy(mode = value) }
    fun setWifiSsidList(value: String) = edit { it.copy(ssids = value.replace('，', ',').split(',')
        .map(String::trim).filter(String::isNotEmpty).distinct().joinToString(",")) }
    fun setProxyOnCellular(value: Boolean) = edit { it.copy(proxyOnCellular = value) }

    private fun edit(transform: (WifiPolicySettings) -> WifiPolicySettings) {
        if (!state.value.hasLoaded || state.value.requiresReload) return
        refreshJob?.cancel()
        mutableState.update {
            val wifi = transform(it.wifi)
            it.copy(wifi = wifi, isLoading = false, hasPendingWifi = wifi != confirmedWifi || it.isSaving, error = "")
        }
    }

    fun requestWifiFlush() {
        if (!state.value.hasPendingWifi || state.value.requiresReload) return
        requestedWifi = state.value.wifi
        if (saveJob?.isActive == true) return
        mutableState.update { it.copy(isSaving = true) }
        saveJob = writes.launch("module", write = {
            try {
                while (requestedWifi != null) {
                    val saved = requestedWifi!!
                    requestedWifi = null
                    if (saved == confirmedWifi) continue
                    val previous = checkNotNull(snapshot)
                    val values = listOf(
                        Triple("WIFI_AUTO_SWITCH", if (saved.enabled) "1" else "0", false),
                        Triple("WIFI_SSID_MODE", saved.mode, true),
                        Triple("WIFI_SSID_LIST", saved.ssids, true),
                        Triple("PROXY_ON_CELLULAR", if (saved.proxyOnCellular) "1" else "0", false),
                    )
                    val content = values.fold(previous.content) { content, (key, value, quoted) ->
                        ShellConfigFile.updateValue(content, key, value, quoted)
                    }
                    snapshot = ConfigSnapshot(content, repository.apply("module", content, previous.revision))
                    confirmedWifi = saved
                }
            } finally {
                mutableState.update { it.copy(isSaving = false, hasPendingWifi = it.wifi != confirmedWifi) }
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
        if (!state.value.hasLoaded || state.value.isSaving || state.value.isLoading || state.value.hasPendingWifi) return
        mutableState.value = state.value.copy(isSaving = true, error = "")
        viewModelScope.launch {
            try {
                repository.updateValue("module", "AUTO_START", if (value) "1" else "0")
                mutableState.value = readSettings()
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                mutableState.value = state.value.copy(isSaving = false, error = error.userMessage())
            }
        }
    }

    private suspend fun readSettings(): SettingsUiState {
        val read = repository.readSnapshot("module")
        currentCoroutineContext().ensureActive()
        val module = ShellConfigFile.parse(read.content)
        val settings = SettingsUiState(
            hasLoaded = true,
            autoStartEnabled = ShellConfigFile.boolean(module["AUTO_START"]),
            wifi = WifiPolicySettings(
                enabled = ShellConfigFile.boolean(module["WIFI_AUTO_SWITCH"]),
                mode = module["WIFI_SSID_MODE"] ?: "blacklist",
                ssids = module["WIFI_SSID_LIST"].orEmpty(),
                proxyOnCellular = ShellConfigFile.boolean(module["PROXY_ON_CELLULAR"], true)
            )
        )
        snapshot = read
        confirmedWifi = settings.wifi
        return settings
    }
}
