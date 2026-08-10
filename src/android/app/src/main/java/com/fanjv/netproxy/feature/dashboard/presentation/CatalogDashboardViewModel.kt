package com.fanjv.netproxy.feature.dashboard.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.core.module.ModuleEnvironment
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.core.module.ServiceStatusSnapshot
import com.fanjv.netproxy.core.ui.userMessage
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.net.NetworkInterface

internal data class CatalogDashboardUiState(
    val rootChecked: Boolean = false,
    val rootGranted: Boolean = false,
    val moduleInstalled: Boolean = false,
    val loading: Boolean = true,
    val serviceState: String = "stopped",
    val serviceError: String = "",
    val readyAt: Long = 0,
    val uptimeSeconds: Long = 0,
    val outboundMode: String = "rule",
    val activeGroupId: String = "",
    val currentNode: String = "",
    val downloadBytesPerSecond: Long = 0,
    val uploadBytesPerSecond: Long = 0,
    val downloadTotal: Long = 0,
    val uploadTotal: Long = 0,
    val cpuUsage: Float = 0f,
    val memoryUsage: Float = 0f,
    val downloadHistory: List<Float> = emptyList(),
    val uploadHistory: List<Float> = emptyList(),
    val internalIp: String = "--",
    val operation: String = "",
    val notice: String = "",
    val noticeId: Long = 0
) {
    val isServiceTransitioning: Boolean
        get() = operation == "start" || operation == "stop"
    val isReady: Boolean
        get() = serviceState == "ready" && !isServiceTransitioning
    val isStarting: Boolean
        get() = operation == "start" || serviceState in setOf("preparing", "starting")
    val isStopping: Boolean
        get() = operation == "stop" || serviceState == "stopping"
    val isServiceControlBusy: Boolean
        get() = isStarting || isStopping || operation.isNotEmpty()
}

/** 仅消费 netproxyctl 与运行时 API 的仪表盘状态，不读取旧配置或 PID。 */
internal class CatalogDashboardViewModel(
    private val repository: ServiceRepository,
    private val environment: ModuleEnvironment
) : ViewModel() {
    private val _state = MutableStateFlow(CatalogDashboardUiState())
    val state: StateFlow<CatalogDashboardUiState> = _state.asStateFlow()
    private var refreshJob: Job? = null
    private var visible = false
    private var serviceTransitionRevision = 0L
    private val totalMemoryBytes = environment.totalMemoryBytes
    private val snapshotReducer = DashboardSnapshotReducer(totalMemoryBytes)

    init {
        viewModelScope.launch {
            val availability = environment.availability()
            _state.update {
                it.copy(
                    rootChecked = true,
                    rootGranted = availability.rootGranted,
                    moduleInstalled = availability.moduleInstalled,
                    loading = availability.moduleInstalled
                )
            }
            if (availability.moduleInstalled && visible) startPolling()
        }
        viewModelScope.launch {
            while (isActive) {
                delay(1000)
                _state.update { current ->
                    if (current.readyAt <= 0 || current.serviceState != "ready") current
                    else current.copy(
                        uptimeSeconds = (System.currentTimeMillis() / 1000 - current.readyAt)
                            .coerceAtLeast(0)
                    )
                }
            }
        }
    }

    fun refresh() {
        if (_state.value.moduleInstalled) {
            viewModelScope.launch { refreshSnapshot() }
        }
    }

    fun setVisible(visible: Boolean) {
        this.visible = visible
        if (visible && _state.value.moduleInstalled) {
            startPolling()
        } else {
            refreshJob?.cancel()
            refreshJob = null
        }
    }

    fun toggleService() {
        if (_state.value.operation.isNotEmpty()) return
        val action = if (_state.value.serviceState in setOf("ready", "starting", "preparing")) {
            "stop"
        } else {
            "start"
        }
        runOperation(action) {
            repository.action(action)
            if (action == "start") "服务已启动" else "服务已停止"
        }
    }

    fun setMode(mode: String) = runOperation("mode") {
        repository.setMode(mode)
        "出站模式已切换"
    }

    fun clearNotice() {
        _state.update { it.copy(notice = "") }
    }

    private fun startPolling() {
        refreshJob?.cancel()
        refreshJob = viewModelScope.launch {
            while (isActive) {
                refreshSnapshot()
                delay(5000)
            }
        }
    }

    private suspend fun refreshSnapshot() {
        if (_state.value.isServiceTransitioning) return
        val requestRevision = serviceTransitionRevision

        runCatching { repository.status() }.onSuccess { service ->
            if (!shouldApplyDashboardSnapshot(
                    requestRevision = requestRevision,
                    currentRevision = serviceTransitionRevision,
                    operation = _state.value.operation
                )
            ) return@onSuccess

            _state.update { current ->
                snapshotReducer.reduce(
                    current = current,
                    service = service,
                    nowMillis = System.currentTimeMillis(),
                    localAddress = localAddress()
                )
            }
        }.onFailure { error ->
            if (!shouldApplyDashboardSnapshot(
                    requestRevision = requestRevision,
                    currentRevision = serviceTransitionRevision,
                    operation = _state.value.operation
                )
            ) return@onFailure

            _state.update {
                it.copy(
                    loading = false,
                    serviceState = if (it.serviceState == "ready") "failed" else it.serviceState,
                    serviceError = error.userMessage()
                )
            }
        }
    }

    private fun runOperation(name: String, action: suspend () -> String) {
        viewModelScope.launch {
            val changesServiceState = name == "start" || name == "stop"
            if (changesServiceState) serviceTransitionRevision++
            _state.update { current ->
                current.copy(
                    operation = name,
                    serviceState = when (name) {
                        "start" -> "starting"
                        "stop" -> "stopping"
                        else -> current.serviceState
                    }
                )
            }
            runCatching { action() }
                .onSuccess { message ->
                    if (changesServiceState) serviceTransitionRevision++
                    _state.update {
                        it.copy(
                            operation = "",
                            serviceState = when (name) {
                                "start" -> "ready"
                                "stop" -> "stopped"
                                else -> it.serviceState
                            },
                            notice = message,
                            noticeId = it.noticeId + 1
                        )
                    }
                    refreshSnapshot()
                }
                .onFailure { error ->
                    if (changesServiceState) serviceTransitionRevision++
                    _state.update {
                        it.copy(
                            operation = "",
                            notice = error.userMessage(),
                            noticeId = it.noticeId + 1
                        )
                    }
                    refreshSnapshot()
                }
        }
    }

    private fun localAddress(): String = runCatching {
        NetworkInterface.getNetworkInterfaces().toList()
            .asSequence()
            .filter { it.isUp && !it.isLoopback }
            .flatMap { it.inetAddresses.toList().asSequence() }
            .firstOrNull { !it.isLoopbackAddress && it.hostAddress?.contains(':') == false }
            ?.hostAddress
            ?: "--"
    }.getOrDefault("--")
}

/** 仅接受当前启停代次且不处于过渡操作中的服务快照。 */
internal fun shouldApplyDashboardSnapshot(
    requestRevision: Long,
    currentRevision: Long,
    operation: String
): Boolean = requestRevision == currentRevision && operation != "start" && operation != "stop"

/** 将持久选择状态转换为适合仪表盘展示的节点名称。 */
internal fun dashboardNodeName(service: ServiceStatusSnapshot): String {
    if (service.activeGroupNodeCount <= 0) return ""

    val groupName = service.activeGroupName.ifBlank { service.activeGroupId }
    val automatic = service.selectorMode == "urltest" || service.selectorMode == "auto"
    val nodeName = if (automatic) {
        "Auto-Fastest"
    } else {
        service.selectedNodeRef
            .substringAfter('/', service.selectedNodeRef)
            .ifBlank { service.runtimeSelected.substringAfter('/', service.runtimeSelected) }
    }
    return listOf(groupName, nodeName).filter(String::isNotBlank).joinToString("/")
}
