package com.fanjv.netproxy.feature.settings.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.command.ShellConfigFile
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.core.ui.userMessage
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import com.fanjv.netproxy.feature.settings.data.ConfigValueUpdate
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** 通过 netproxyctl 事务管理模块与 eBPF 设置。 */
internal class SettingsViewModel(
    private val configRepository: ConfigRepository,
    private val serviceRepository: ServiceRepository
) : ViewModel() {
    private val _state = MutableStateFlow(SettingsUiState())
    val state: StateFlow<SettingsUiState> = _state.asStateFlow()
    private var pendingSaves = 0
    private var hasLoaded = false

    fun setVisible(visible: Boolean) {
        if (visible && !_state.value.isLoading) refresh()
    }

    fun refresh() {
        if (pendingSaves > 0) return
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = "") }
            runCatching { loadSettings() }
                .onSuccess { loaded ->
                    hasLoaded = true
                    _state.value = loaded.copy(isSaving = pendingSaves > 0)
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

    fun ensureLoaded() {
        if (!hasLoaded && !_state.value.isLoading) refresh()
    }

    fun setAutoStartEnabled(enabled: Boolean) =
        updateModuleSetting("AUTO_START", if (enabled) "1" else "0")

    fun setWifiAutoSwitch(enabled: Boolean) =
        updateModuleSetting("WIFI_AUTO_SWITCH", if (enabled) "1" else "0")

    fun setWifiSsidMode(mode: String) =
        updateModuleSetting("WIFI_SSID_MODE", mode, forceQuotes = true)

    fun setWifiSsidList(value: String) =
        updateModuleSetting("WIFI_SSID_LIST", normalizeCommaSeparated(value), forceQuotes = true)

    fun setProxyOnCellular(enabled: Boolean) =
        updateModuleSetting("PROXY_ON_CELLULAR", if (enabled) "1" else "0")

    fun setNetwork(network: String) {
        val settings = _state.value.proxySettings
        val updates = mutableListOf("EBPF_NETWORK" to network)
        if (network == "tcp" && requiresSharedUdp(settings.mode, settings.dnsMode)) {
            updates += "EBPF_DNS_MODE" to "off"
        }
        updateProxySettings(updates)
    }

    fun setDnsMode(mode: String) {
        val settings = _state.value.proxySettings
        val updates = mutableListOf("EBPF_DNS_MODE" to mode)
        if (settings.network == "tcp" && requiresSharedUdp(settings.mode, mode)) {
            updates += "EBPF_NETWORK" to ""
        }
        updateProxySettings(updates)
    }

    fun setMode(mode: String) {
        val settings = _state.value.proxySettings
        val updates = mutableListOf("EBPF_MODE" to mode)
        if (settings.network == "tcp" && requiresSharedUdp(mode, settings.dnsMode)) {
            updates += "EBPF_NETWORK" to ""
        }
        updateProxySettings(updates)
    }

    fun setBypassPrivateAddress(enabled: Boolean) {
        val value = if (enabled) "1" else "0"
        val keys = when (_state.value.proxySettings.mode) {
            "shared" -> listOf("EBPF_SHARED_BYPASS_PRIVATE_ADDRESS")
            "hybrid" -> listOf(
                "EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS",
                "EBPF_SHARED_BYPASS_PRIVATE_ADDRESS"
            )

            else -> listOf("EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS")
        }
        updateProxySettings(keys.map { it to value })
    }

    fun updateProxySetting(key: String, value: String) {
        val normalizedValue = if (key in commaSeparatedKeys) {
            normalizeCommaSeparated(value)
        } else {
            value
        }
        updateProxySettings(listOf(key to normalizedValue))
    }

    private fun updateProxySettings(updates: List<Pair<String, String>>) = updateSettings(
        target = "ebpf",
        updates = updates.map { (key, value) ->
            ConfigValueUpdate(key, value, key in quotedEbpfKeys)
        }
    )

    fun restartService() {
        viewModelScope.launch {
            runCatching { serviceRepository.action("restart") }
                .onFailure { error -> _state.update { it.copy(error = error.userMessage()) } }
        }
    }

    fun diagnoseEbpf() {
        viewModelScope.launch {
            _state.update { it.copy(isDiagnosingEbpf = true, ebpfDiagnostic = null, error = "") }
            runCatching { configRepository.ebpfStatus() }
                .onSuccess { output ->
                    _state.update {
                        it.copy(isDiagnosingEbpf = false, ebpfDiagnostic = output)
                    }
                }
                .onFailure { error ->
                    val output = (error as? NetProxyCtlException)?.data
                        ?.jsonObject?.get("content")?.jsonPrimitive?.content
                        ?: error.userMessage()
                    _state.update {
                        it.copy(isDiagnosingEbpf = false, ebpfDiagnostic = output)
                    }
                }
        }
    }

    fun dismissEbpfDiagnostic() {
        _state.update { it.copy(ebpfDiagnostic = null) }
    }

    private fun updateModuleSetting(
        key: String,
        value: String,
        forceQuotes: Boolean = false
    ) = updateSettings("module", listOf(ConfigValueUpdate(key, value, forceQuotes)))

    private fun updateSettings(
        target: String,
        updates: List<ConfigValueUpdate>
    ) {
        updates.forEach { updateLocalSetting(it.key, it.value) }
        pendingSaves += 1
        _state.update { it.copy(isSaving = true, error = "") }
        viewModelScope.launch {
            runCatching {
                configRepository.updateValues(target, updates)
            }.onSuccess {
                pendingSaves = (pendingSaves - 1).coerceAtLeast(0)
                updates.forEach { updateLocalSetting(it.key, it.value) }
                _state.update { it.copy(isSaving = pendingSaves > 0) }
            }.onFailure { error ->
                pendingSaves = (pendingSaves - 1).coerceAtLeast(0)
                val message = error.userMessage()
                runCatching { loadSettings() }
                    .onSuccess { loaded ->
                        _state.value = loaded.copy(
                            isSaving = pendingSaves > 0,
                            error = message
                        )
                    }.onFailure {
                        _state.update { current ->
                            current.copy(isSaving = pendingSaves > 0, error = message)
                        }
                    }
            }
        }
    }

    private suspend fun loadSettings(): SettingsUiState = coroutineScope {
        val moduleContent = async { configRepository.read("module") }
        val ebpfContent = async { configRepository.read("ebpf") }
        val module = ShellConfigFile.parse(moduleContent.await())
        val ebpf = ShellConfigFile.parse(ebpfContent.await())
        SettingsUiState(
            autoStartEnabled = module["AUTO_START"] == "1",
            proxySettings = parseProxySettings(module, ebpf),
            isLoading = false
        )
    }

    private fun updateLocalSetting(key: String, value: String) {
        _state.update { current ->
            val settings = current.proxySettings
            when (key) {
                "AUTO_START" -> current.copy(autoStartEnabled = value == "1")
                "EBPF_NETWORK" -> current.copy(proxySettings = settings.copy(network = value))
                "EBPF_DNS_MODE" -> current.copy(proxySettings = settings.copy(dnsMode = value))
                "EBPF_MODE" -> current.copy(
                    proxySettings = settings.copy(mode = value)
                )

                "EBPF_LOCAL_IPV6_MODE" -> current.copy(
                    proxySettings = settings.copy(localIpv6Mode = value)
                )

                "EBPF_SHARED_IPV6_MODE" -> current.copy(
                    proxySettings = settings.copy(sharedIpv6Mode = value)
                )

                "EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS" -> current.copy(
                    proxySettings = settings.copy(localBypassPrivateAddress = value == "1")
                )

                "EBPF_SHARED_BYPASS_PRIVATE_ADDRESS" -> current.copy(
                    proxySettings = settings.copy(sharedBypassPrivateAddress = value == "1")
                )

                "EBPF_BYPASS_RULE_SET" -> current.copy(
                    proxySettings = settings.copy(bypassRuleSet = value)
                )

                "EBPF_SHARED_INTERFACES" -> current.copy(
                    proxySettings = settings.copy(sharedInterfaces = value)
                )

                "EBPF_SHARED_INCLUDE_SOURCE_CIDR" -> current.copy(
                    proxySettings = settings.copy(sharedIncludeSourceCidrs = value)
                )

                "EBPF_SHARED_EXCLUDE_SOURCE_CIDR" -> current.copy(
                    proxySettings = settings.copy(sharedExcludeSourceCidrs = value)
                )

                "EBPF_SHARED_INCLUDE_MAC_ADDRESS" -> current.copy(
                    proxySettings = settings.copy(sharedIncludeMacAddresses = value)
                )

                "EBPF_SHARED_EXCLUDE_MAC_ADDRESS" -> current.copy(
                    proxySettings = settings.copy(sharedExcludeMacAddresses = value)
                )

                "WIFI_AUTO_SWITCH" -> current.copy(
                    proxySettings = settings.copy(wifiAutoSwitch = value == "1")
                )

                "WIFI_SSID_MODE" -> current.copy(
                    proxySettings = settings.copy(wifiSsidMode = value)
                )

                "WIFI_SSID_LIST" -> current.copy(
                    proxySettings = settings.copy(wifiSsidList = value)
                )

                "PROXY_ON_CELLULAR" -> current.copy(
                    proxySettings = settings.copy(proxyOnCellular = value == "1")
                )

                else -> current
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

        val mode = value("EBPF_MODE", "local").takeIf { it in modes } ?: "local"
        return ProxySettings(
            mode = mode,
            network = value("EBPF_NETWORK", ""),
            dnsMode = value("EBPF_DNS_MODE", "hijack"),
            localIpv6Mode = value("EBPF_LOCAL_IPV6_MODE", "auto")
                .takeIf { it in ipv6Modes } ?: "auto",
            sharedIpv6Mode = value("EBPF_SHARED_IPV6_MODE", "always")
                .takeIf { it in sharedIpv6Modes } ?: "always",
            localBypassPrivateAddress = enabled("EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS", true),
            sharedBypassPrivateAddress = enabled("EBPF_SHARED_BYPASS_PRIVATE_ADDRESS", true),
            bypassRuleSet = value("EBPF_BYPASS_RULE_SET", "direct,ChinaIP"),
            sharedInterfaces = value("EBPF_SHARED_INTERFACES", "wlan2"),
            sharedIncludeSourceCidrs = value("EBPF_SHARED_INCLUDE_SOURCE_CIDR", ""),
            sharedExcludeSourceCidrs = value("EBPF_SHARED_EXCLUDE_SOURCE_CIDR", ""),
            sharedIncludeMacAddresses = value("EBPF_SHARED_INCLUDE_MAC_ADDRESS", ""),
            sharedExcludeMacAddresses = value("EBPF_SHARED_EXCLUDE_MAC_ADDRESS", ""),
            wifiAutoSwitch = module["WIFI_AUTO_SWITCH"] == "1",
            wifiSsidMode = module["WIFI_SSID_MODE"] ?: "blacklist",
            wifiSsidList = module["WIFI_SSID_LIST"].orEmpty(),
            proxyOnCellular = module["PROXY_ON_CELLULAR"] != "0"
        )
    }

    private companion object {
        val quotedEbpfKeys = setOf(
            "EBPF_MODE",
            "EBPF_NETWORK",
            "EBPF_DNS_MODE",
            "EBPF_LOCAL_IPV6_MODE",
            "EBPF_SHARED_IPV6_MODE",
            "EBPF_BYPASS_RULE_SET",
            "EBPF_SHARED_INTERFACES",
            "EBPF_SHARED_INCLUDE_SOURCE_CIDR",
            "EBPF_SHARED_EXCLUDE_SOURCE_CIDR",
            "EBPF_SHARED_INCLUDE_MAC_ADDRESS",
            "EBPF_SHARED_EXCLUDE_MAC_ADDRESS"
        )
        val ipv6Modes = setOf("always", "auto", "off")
        val sharedIpv6Modes = setOf("always", "off")
        val modes = setOf("local", "shared", "hybrid")
        val commaSeparatedKeys = setOf(
            "EBPF_BYPASS_RULE_SET",
            "EBPF_SHARED_INTERFACES",
            "EBPF_SHARED_INCLUDE_SOURCE_CIDR",
            "EBPF_SHARED_EXCLUDE_SOURCE_CIDR",
            "EBPF_SHARED_INCLUDE_MAC_ADDRESS",
            "EBPF_SHARED_EXCLUDE_MAC_ADDRESS",
            "WIFI_SSID_LIST"
        )

        fun normalizeCommaSeparated(value: String): String = value
            .replace('，', ',')
            .split(',')
            .map(String::trim)
            .filter(String::isNotEmpty)
            .joinToString(",")
    }
}

internal fun requiresSharedUdp(mode: String, dnsMode: String): Boolean =
    mode in setOf("shared", "hybrid") && dnsMode != "off"
