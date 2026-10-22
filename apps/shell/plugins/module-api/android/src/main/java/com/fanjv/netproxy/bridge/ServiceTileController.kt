package com.fanjv.netproxy.bridge

import com.fanjv.netproxy.core.api.LimitedInputStream
import com.fanjv.netproxy.core.api.ModuleApiCodec
import com.fanjv.netproxy.core.api.ModuleApiTransport
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.TimeoutCancellationException
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

internal enum class ServiceTileState {
    STOPPED, PREPARING, STARTING, READY, STOPPING, FAILED, UNKNOWN, CHECKING, WORKING, BUSY;

    companion object {
        fun fromWire(value: String?): ServiceTileState = when (value) {
            "stopped" -> STOPPED
            "preparing" -> PREPARING
            "starting" -> STARTING
            "ready" -> READY
            "stopping" -> STOPPING
            "failed" -> FAILED
            else -> UNKNOWN
        }
    }
}

internal interface ServiceTileApi {
    suspend fun read(): ServiceTileState
    suspend fun start()
    suspend fun stop()
}

/** 磁贴仅开放固定启停路径，不配对、不订阅事件、不重试写入。 */
internal class HttpServiceTileApi(private val transport: ModuleApiTransport) : ServiceTileApi {
    private val codec = ModuleApiCodec(Json { ignoreUnknownKeys = true; explicitNulls = false })

    override suspend fun read(): ServiceTileState = transport.request("GET", "/service", null, emptyMap(), false) { status, input ->
        val data = codec.decode(status, LimitedInputStream(input, BODY_LIMIT), JsonObject.serializer())
        val state = (data["state"] as? JsonPrimitive)?.takeIf { it.isString }?.content
        ServiceTileState.fromWire(state)
    }

    override suspend fun start() = write("/service/start")
    override suspend fun stop() = write("/service/stop")

    private suspend fun write(path: String) {
        transport.request("POST", path, JsonObject(emptyMap()), emptyMap(), true) { status, input ->
            codec.decode(status, LimitedInputStream(input, BODY_LIMIT), JsonObject.serializer())
        }
    }

    private companion object {
        const val BODY_LIMIT = 4 * 1024 * 1024
    }
}

internal enum class ServiceTileClick { DONE, OPEN_APP }

/** 刷新与点击串行，服务状态只来自本次读取。 */
internal class ServiceTileController(
    private val api: ServiceTileApi,
    private val publish: (ServiceTileState) -> Unit,
    private val gate: MutationAdmissionGate = MutationAdmissionGate.process,
) {
    private val operation = Mutex()

    suspend fun refresh() {
        if (!operation.tryLock()) return
        try {
            publish(ServiceTileState.CHECKING)
            publish(read())
        } finally {
            operation.unlock()
        }
    }

    suspend fun click(): ServiceTileClick {
        if (!operation.tryLock()) return ServiceTileClick.DONE
        try {
            val lease = gate.tryAcquire()
            if (lease == null) {
                publish(ServiceTileState.BUSY)
                return ServiceTileClick.DONE
            }
            try {
                publish(ServiceTileState.CHECKING)
                val before = read()
                publish(before)
                when (before) {
                    ServiceTileState.READY, ServiceTileState.STOPPED -> Unit
                    ServiceTileState.UNKNOWN, ServiceTileState.FAILED -> return ServiceTileClick.OPEN_APP
                    else -> return ServiceTileClick.DONE
                }
                // 关闭 QS 或销毁组件不能在写入中途释放门禁；传输与整体收尾均有时限。
                return withContext(NonCancellable) {
                    try {
                        withTimeout(185_000) {
                            publish(ServiceTileState.WORKING)
                            var failed = false
                            try {
                                if (before == ServiceTileState.READY) api.stop() else api.start()
                            } catch (error: CancellationException) {
                                throw error
                            } catch (_: Exception) {
                                failed = true
                            }
                            val after = read()
                            publish(after)
                            if (failed || after == ServiceTileState.UNKNOWN || after == ServiceTileState.FAILED)
                                ServiceTileClick.OPEN_APP else ServiceTileClick.DONE
                        }
                    } catch (_: TimeoutCancellationException) {
                        publish(ServiceTileState.UNKNOWN)
                        ServiceTileClick.OPEN_APP
                    }
                }
            } finally {
                lease.close()
            }
        } finally {
            operation.unlock()
        }
    }

    private suspend fun read(): ServiceTileState = try {
        api.read()
    } catch (error: CancellationException) {
        throw error
    } catch (_: Exception) {
        ServiceTileState.UNKNOWN
    }
}
