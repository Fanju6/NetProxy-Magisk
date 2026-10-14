package com.fanjv.netproxy.feature.dashboard.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.module.ModuleAvailability
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.core.module.ServiceStatusSnapshot
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.core.ui.toUiText
import com.fanjv.netproxy.core.ui.userMessage
import kotlinx.coroutines.Job
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import java.net.NetworkInterface

internal data class CatalogDashboardUiState(
    val connection: ModuleAvailability? = null,
    val loading: Boolean = true,
    val serviceState: String = "stopped",
    val serviceError: String = "",
    val readyAt: Long = 0,
    val uptimeSeconds: Long = 0,
    val outboundMode: String = "unknown",
    val availableOutboundModes: List<String> = emptyList(),
    val activeGroupId: String = "",
    val currentNode: String = "",
    val downloadBytesPerSecond: Long = 0,
    val uploadBytesPerSecond: Long = 0,
    val downloadTotal: Long = 0,
    val uploadTotal: Long = 0,
    val cpuUsage: Float = 0f,
    val memoryUsage: Float = 0f,
    val trafficSamples: List<TrafficSample> = emptyList(),
    val internalIp: String = "--",
    val operation: String = "",
    val notice: UiText = UiText.Empty,
    val noticeId: Long = 0
) {
    val connected: Boolean get() = connection?.available == true
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

/** 仅消费模块 API 与共享事件快照，不读取设备配置或 PID。 */
internal class CatalogDashboardViewModel(
    private val repository: ServiceRepository,
    totalMemoryBytes: Long,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) : ViewModel(scope) {
    private val _state = MutableStateFlow(CatalogDashboardUiState())
    val state: StateFlow<CatalogDashboardUiState> = _state.asStateFlow()
    private var eventsJob: Job? = null
    private var snapshotJob: Job? = null
    private var uptimeJob: Job? = null
    private var visible = false
    private var stateRevision = 0L
    private val controlMutex = Mutex()
    private val snapshotReducer = DashboardSnapshotReducer(totalMemoryBytes)
    private val trafficReducer = TrafficTimelineReducer()

    fun setAvailability(availability: ModuleAvailability?) {
        val previous = _state.value
        val available = availability?.available == true
        _state.update {
            it.copy(connection = availability,
                loading = available && (!previous.connected || previous.loading))
        }
        if (available && visible) {
            startUptimeTicker()
            startEvents()
        } else if (!available) {
            eventsJob?.cancel()
            eventsJob = null
            snapshotJob?.cancel()
            uptimeJob?.cancel()
            if (previous.operation.isEmpty()) stateRevision++
        }
    }

    fun refresh() {
        if (_state.value.connected) {
            refreshSnapshot()
        }
    }

    fun setVisible(visible: Boolean) {
        this.visible = visible
        if (visible) {
            if (_state.value.connected) {
                refreshSnapshot()
                startUptimeTicker()
                startEvents()
            }
        } else {
            eventsJob?.cancel()
            eventsJob = null
            snapshotJob?.cancel()
            if (_state.value.operation.isEmpty()) stateRevision++
            uptimeJob?.cancel()
            uptimeJob = null
        }
    }

    fun toggleService() {
        if (!canControlService()) return
        val action = if (_state.value.serviceState in setOf("ready", "starting", "preparing")) {
            "stop"
        } else {
            "start"
        }
        runOperation(action) {
            repository.action(action)
            if (action == "start") {
                UiText.Resource(R.string.dashboard_service_started)
            } else {
                UiText.Resource(R.string.dashboard_service_stopped)
            }
        }
    }

    fun setMode(mode: String) {
        val current = _state.value
        if (!current.connected || current.loading || mode !in current.availableOutboundModes) return
        runOperation("mode") {
            repository.setMode(mode)
            UiText.Resource(R.string.dashboard_mode_changed)
        }
    }

    private fun canControlService(): Boolean =
        _state.value.connected &&
            !_state.value.loading &&
            _state.value.operation.isEmpty()

    fun clearNotice() {
        _state.update { it.copy(notice = UiText.Empty) }
    }

    private fun startEvents() {
        if (eventsJob?.isActive == true) return
        eventsJob = viewModelScope.launch {
            repository.observe().collect { result ->
                if (_state.value.operation.isEmpty()) {
                    val revision = ++stateRevision
                    result.fold(
                        onSuccess = { readSnapshot(revision, it) },
                        onFailure = { error -> _state.update { it.copy(loading = false, serviceError = error.userMessage()) } }
                    )
                }
            }
        }
    }

    private fun startUptimeTicker() {
        uptimeJob?.cancel()
        uptimeJob = viewModelScope.launch {
            while (isActive) {
                _state.update { current ->
                    if (current.readyAt <= 0 || current.serviceState != "ready") current
                    else current.copy(
                        uptimeSeconds = (System.currentTimeMillis() / 1000 - current.readyAt)
                            .coerceAtLeast(0)
                    )
                }
                delay(1000)
            }
        }
    }

    private fun refreshSnapshot() {
        if (!_state.value.connected || _state.value.operation.isNotEmpty()) return
        val requestRevision = ++stateRevision
        snapshotJob?.cancel()
        snapshotJob = viewModelScope.launch { readSnapshot(requestRevision) }
    }

    private suspend fun readSnapshot(requestRevision: Long, event: ServiceStatusSnapshot? = null) {
        runCatching { event ?: repository.status() }.onSuccess { service ->
            val localAddress = withContext(Dispatchers.IO) { localAddress() }
            currentCoroutineContext().ensureActive()
            if (!shouldApplyDashboardSnapshot(
                    requestRevision = requestRevision,
                    currentRevision = stateRevision,
                    operation = _state.value.operation
                )
            ) return@onSuccess

            val nowMillis = System.currentTimeMillis()
            val timeline = if (service.state == "ready") {
                trafficReducer.reduce(service, nowMillis)
            } else {
                trafficReducer.reset()
                TrafficTimelineState()
            }
            _state.update { current ->
                snapshotReducer.reduce(
                    current = current,
                    service = service,
                    nowMillis = nowMillis,
                    localAddress = localAddress
                ).copy(
                    downloadBytesPerSecond = timeline.downloadBytesPerSecond,
                    uploadBytesPerSecond = timeline.uploadBytesPerSecond,
                    trafficSamples = timeline.samples
                )
            }
        }.onFailure { error ->
            if (error is CancellationException) throw error
            currentCoroutineContext().ensureActive()
            if (!shouldApplyDashboardSnapshot(
                    requestRevision = requestRevision,
                    currentRevision = stateRevision,
                    operation = _state.value.operation
                )
            ) return@onFailure

            _state.update {
                it.copy(
                    loading = false,
                    serviceState = "failed",
                    serviceError = error.userMessage()
                )
            }
        }
    }

    private fun runOperation(name: String, action: suspend () -> UiText) {
        viewModelScope.launch(start = CoroutineStart.UNDISPATCHED) {
            controlMutex.withLock {
                val requestRevision = ++stateRevision
                snapshotJob?.cancel()
                val previousServiceState = _state.value.serviceState
                _state.update { current ->
                    current.copy(
                        operation = name,
                        serviceError = "",
                        serviceState = when (name) {
                            "start" -> "starting"
                            "stop" -> "stopping"
                            else -> current.serviceState
                        }
                    )
                }
                val result = runCatching { action() }
                val error = result.exceptionOrNull()
                if (error is CancellationException) throw error
                currentCoroutineContext().ensureActive()
                if (requestRevision != stateRevision) return@withLock
                _state.update {
                    it.copy(
                        operation = "",
                        serviceState = when {
                            error != null && name in setOf("start", "stop") -> previousServiceState
                            error == null && name == "start" -> "ready"
                            error == null && name == "stop" -> "stopped"
                            else -> it.serviceState
                        },
                        notice = result.fold({ message -> message }, { it.userMessage().toUiText() }),
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

/** 刷新、模式和启停共享代次，控制操作期间不接受任何服务快照。 */
internal fun shouldApplyDashboardSnapshot(
    requestRevision: Long,
    currentRevision: Long,
    operation: String
): Boolean = requestRevision == currentRevision && operation.isEmpty()

/** 将持久选择状态转换为适合仪表盘展示的节点名称。 */
internal fun dashboardNodeName(service: ServiceStatusSnapshot): String {
    if (service.activeGroupNodeCount <= 0) return ""

    val groupName = service.activeGroupName.ifBlank { service.activeGroupId }
    val automatic = service.selectorMode == "urltest"
    if (automatic) {
        val selected = if (service.state == "ready") {
            runtimeSelectedNodeTag(
                service.activeGroupRuntimeTag,
                service.runtimeSelected
            )
        } else {
            ""
        }
        if (selected.isNotBlank()) return "$selected · Auto-Fastest"
        return "$groupName/Auto-Fastest"
    }
    val nodeName = service.selectedNodeRef
        .substringAfter('/', service.selectedNodeRef)
        .ifBlank { service.runtimeSelected.substringAfter('/', service.runtimeSelected) }
    return listOf(groupName, nodeName).filter(String::isNotBlank).joinToString("/")
}

internal fun runtimeSelectedNodeTag(runtimeTag: String, selected: String): String {
    if (runtimeTag.isBlank()) return ""
    val prefix = "$runtimeTag/"
    return selected.removePrefix(prefix)
        .takeIf { selected.startsWith(prefix) && it.isNotBlank() }
        .orEmpty()
}
