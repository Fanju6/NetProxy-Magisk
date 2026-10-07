package com.fanjv.netproxy.feature.settings.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.command.ShellConfigFile
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

internal class SettingsViewModel(
    private val repository: ConfigRepository,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
) : ViewModel(scope) {
    private val mutableState = MutableStateFlow(SettingsUiState())
    val state = mutableState.asStateFlow()

    fun setVisible(visible: Boolean) {
        if (visible) refresh()
    }

    fun ensureLoaded() {
        if (!state.value.hasLoaded) refresh()
    }

    fun refresh() {
        if (state.value.isLoading || state.value.isSaving) return
        mutableState.value = state.value.copy(isLoading = true, error = "")
        viewModelScope.launch {
            try {
                mutableState.value = readSettings()
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                mutableState.value = state.value.copy(isLoading = false, error = error.userMessage())
            }
        }
    }

    fun setAutoStartEnabled(value: Boolean) = update("AUTO_START", if (value) "1" else "0")
    fun setWifiAutoSwitch(value: Boolean) = update("WIFI_AUTO_SWITCH", if (value) "1" else "0")
    fun setWifiSsidMode(value: String) = update("WIFI_SSID_MODE", value, true)
    fun setWifiSsidList(value: String) = update(
        "WIFI_SSID_LIST", value.replace('，', ',').split(',').map(String::trim)
            .filter(String::isNotEmpty).joinToString(","), true
    )
    fun setProxyOnCellular(value: Boolean) = update("PROXY_ON_CELLULAR", if (value) "1" else "0")

    private fun update(key: String, value: String, quoted: Boolean = false) {
        if (!state.value.hasLoaded || state.value.isSaving || state.value.isLoading) return
        mutableState.value = state.value.copy(isSaving = true, error = "")
        viewModelScope.launch {
            try {
                repository.updateValue("module", key, value, quoted)
                mutableState.value = readSettings()
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                mutableState.value = state.value.copy(isSaving = false, error = error.userMessage())
            }
        }
    }

    private suspend fun readSettings(): SettingsUiState {
        val module = ShellConfigFile.parse(repository.read("module"))
        return SettingsUiState(
            hasLoaded = true,
            autoStartEnabled = ShellConfigFile.boolean(module["AUTO_START"]),
            wifi = WifiPolicySettings(
                enabled = ShellConfigFile.boolean(module["WIFI_AUTO_SWITCH"]),
                mode = module["WIFI_SSID_MODE"] ?: "blacklist",
                ssids = module["WIFI_SSID_LIST"].orEmpty(),
                proxyOnCellular = ShellConfigFile.boolean(module["PROXY_ON_CELLULAR"], true)
            )
        )
    }
}
