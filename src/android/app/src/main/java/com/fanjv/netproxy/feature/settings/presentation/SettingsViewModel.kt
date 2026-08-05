package com.fanjv.netproxy.feature.settings.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.core.command.ShellConfigFile
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** 通过 netproxyctl 事务管理模块与 eBPF 设置。 */
internal class SettingsViewModel(
    private val configRepository: ConfigRepository,
    private val serviceRepository: ServiceRepository
) : ViewModel() {
    private val _state = MutableStateFlow(SettingsUiState())
    val state: StateFlow<SettingsUiState> = _state.asStateFlow()

    fun setVisible(visible: Boolean) {
        if (visible && !_state.value.isLoading) refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = "") }
            runCatching {
                coroutineScope {
                    val module = async { configRepository.read("module") }
                    val ebpf = async { configRepository.read("ebpf") }
                    module.await() to ebpf.await()
                }
            }.onSuccess { (moduleContent, ebpfContent) ->
                val module = ShellConfigFile.parse(moduleContent)
                val ebpf = ShellConfigFile.parse(ebpfContent)
                _state.value = SettingsUiState(
                    autoStartEnabled = module["AUTO_START"] == "1",
                    gmsFixEnabled = module["GMS_FIX"] == "1",
                    proxySettings = parseProxySettings(module, ebpf),
                    isLoading = false
                )
            }.onFailure { error ->
                _state.update {
                    it.copy(
                        isLoading = false,
                        error = error.userMessage()
                    )
                }
            }
        }
    }

    fun setAutoStartEnabled(enabled: Boolean) =
        updateModuleSetting("AUTO_START", if (enabled) "1" else "0")

    fun setGmsFixEnabled(enabled: Boolean) =
        updateModuleSetting("GMS_FIX", if (enabled) "1" else "0")

    fun setWifiAutoSwitch(enabled: Boolean) =
        updateModuleSetting("WIFI_AUTO_SWITCH", if (enabled) "1" else "0")

    fun setWifiSsidMode(mode: String) =
        updateModuleSetting("WIFI_SSID_MODE", mode, forceQuotes = true)

    fun setWifiSsidList(value: String) =
        updateModuleSetting("WIFI_SSID_LIST", value, forceQuotes = true)

    fun setProxyOnCellular(enabled: Boolean) =
        updateModuleSetting("PROXY_ON_CELLULAR", if (enabled) "1" else "0")

    fun setDnsHijackEnabled(enabled: Boolean) =
        updateProxySetting("EBPF_DNS_MODE", if (enabled) "hijack" else "off")

    fun updateProxySetting(key: String, value: String) {
        updateSetting(
            target = "ebpf",
            key = key,
            value = value,
            forceQuotes = key in quotedEbpfKeys
        )
    }

    fun restartService() {
        viewModelScope.launch {
            runCatching { serviceRepository.action("restart") }
                .onFailure { error -> _state.update { it.copy(error = error.userMessage()) } }
        }
    }

    private fun updateModuleSetting(
        key: String,
        value: String,
        forceQuotes: Boolean = false
    ) = updateSetting("module", key, value, forceQuotes)

    private fun updateSetting(
        target: String,
        key: String,
        value: String,
        forceQuotes: Boolean
    ) {
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = "") }
            runCatching {
                configRepository.updateValue(target, key, value, forceQuotes)
            }.onSuccess {
                refresh()
            }.onFailure { error ->
                _state.update {
                    it.copy(isSaving = false, error = error.userMessage())
                }
            }
        }
    }

    private fun parseProxySettings(
        module: Map<String, String>,
        ebpf: Map<String, String>
    ): ProxySettings {
        fun value(key: String, default: String) = ebpf[key] ?: default
        fun enabled(key: String, default: Boolean = false) =
            ebpf[key]?.let { it == "1" } ?: default

        return ProxySettings(
            network = value("EBPF_NETWORK", ""),
            udpTimeout = value("EBPF_UDP_TIMEOUT", "5m"),
            dnsMode = value("EBPF_DNS_MODE", "hijack"),
            cgroupPath = value("EBPF_CGROUP_PATH", ""),
            ipv6Enabled = enabled("EBPF_IPV6", true),
            bypassRuleSets = value("EBPF_BYPASS_RULE_SETS", "direct ChinaIP"),
            sharedNetworkEnabled = enabled("EBPF_SHARED_NETWORK"),
            sharedInterfaces = value("EBPF_SHARED_INTERFACES", "wlan2"),
            tcpMapCapacity = value("EBPF_TCP_MAP_CAPACITY", "65536"),
            udpMapCapacity = value("EBPF_UDP_MAP_CAPACITY", "65536"),
            socketMapCapacity = value("EBPF_SOCKET_MAP_CAPACITY", "65536"),
            sharedMapCapacity = value("EBPF_SHARED_MAP_CAPACITY", "65536"),
            wifiAutoSwitch = module["WIFI_AUTO_SWITCH"] == "1",
            wifiSsidMode = module["WIFI_SSID_MODE"] ?: "blacklist",
            wifiSsidList = module["WIFI_SSID_LIST"].orEmpty(),
            proxyOnCellular = module["PROXY_ON_CELLULAR"] != "0"
        )
    }

    private companion object {
        val quotedEbpfKeys = setOf(
            "EBPF_NETWORK",
            "EBPF_UDP_TIMEOUT",
            "EBPF_DNS_MODE",
            "EBPF_CGROUP_PATH",
            "EBPF_BYPASS_RULE_SETS",
            "EBPF_SHARED_INTERFACES"
        )
    }
}
