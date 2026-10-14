package com.fanjv.netproxy.core.api

import android.content.Context
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import kotlinx.serialization.serializer
import kotlinx.serialization.json.*
import java.io.File
import java.io.InputStream

internal data class ModuleApiResponse(val code: String, val message: String, val data: JsonElement)
internal data class ModuleEvent(val name: String, val content: String)

internal class ModuleApiException(
    val resultCode: String,
    override val message: String,
    val data: JsonElement = JsonObject(emptyMap()),
) : IllegalStateException(message) {
    val persisted: Boolean get() = ((data as? JsonObject)?.get("persisted") as? JsonPrimitive)?.let { !it.isString && it.booleanOrNull == true } == true
}

internal interface ModuleApiTransport {
    suspend fun reconnect(address: String)
    suspend fun <T> request(method: String, path: String, body: JsonObject?, query: Map<String, String>, mutation: Boolean,
        decode: (Int, InputStream) -> T): T
    suspend fun download(path: String, query: Map<String, String>, destination: File)
    suspend fun image(path: String): ByteArray
    suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit)
}

@OptIn(ExperimentalCoroutinesApi::class)
internal class ModuleApiClient(
    private val transport: ModuleApiTransport,
    private val eventScope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO),
) {
    constructor(context: Context) : this(LocalHttpTransport(context.applicationContext))
    internal val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
    private val codec = ModuleApiCodec(json)
    private val interests = HashMap<String, Int>()
    private val topics = MutableStateFlow<Set<String>>(emptySet())
    private val snapshots = topics.flatMapLatest { active ->
        if (active.isEmpty()) emptyFlow() else channelFlow {
            var wait = 1000L
            while (isActive) {
                send(ModuleEvent("reset", ""))
                try {
                    transport.watch(active) { send(it); wait = 1000L }
                } catch (error: CancellationException) { throw error }
                catch (_: Exception) { }
                send(ModuleEvent("", ""))
                delay(wait)
                wait = (wait * 2).coerceAtMost(15_000L)
            }
        }
    }.scan(ModuleEventSnapshot()) { previous, event ->
        when {
            event.name == "reset" -> ModuleEventSnapshot()
            event.name.isEmpty() -> ModuleEventSnapshot(error = ModuleApiException("api.disconnected", "模块事件连接已断开"))
            else -> previous.copy(frames = previous.frames + (event.name to ModuleEventFrame(event.content, codec)))
        }
    }.onCompletion {
        // 真正停止共享流后才释放本次前台会话的主题并丢弃快照。
        synchronized(interests) { topics.value = interests.keys.toSet() }
    }.stateIn(eventScope, SharingStarted.WhileSubscribed(2_000, 0), ModuleEventSnapshot())

    fun observe(names: Set<String>): Flow<Result<Map<String, ModuleEventFrame>>> = flow {
        require(names.isNotEmpty() && names.all { it in setOf("status", "traffic", "groups", "subscriptions") })
        updateInterests(names, 1)
        try {
            snapshots.collect { snapshot ->
                val failure = snapshot.error
                if (failure != null) emit(Result.failure(failure))
                else if (names.all { snapshot.frames.containsKey(it) }) emit(Result.success(names.associateWith { snapshot.frames.getValue(it) }))
            }
        } finally { updateInterests(names, -1) }
    }.distinctUntilChanged { previous, next ->
        if (previous.isFailure || next.isFailure) previous.exceptionOrNull() === next.exceptionOrNull()
        else names.all { previous.getOrThrow()[it] === next.getOrThrow()[it] }
    }

    private fun updateInterests(names: Set<String>, change: Int) = synchronized(interests) {
        names.map { if (it == "traffic") "status" else it }.forEach { topic ->
            require(topic in setOf("status", "groups", "subscriptions"))
            val count = (interests[topic] ?: 0) + change
            if (count == 0) interests.remove(topic) else interests[topic] = count
        }
        if (change > 0) topics.value = topics.value + interests.keys
    }

    suspend fun get(path: String, query: Map<String, String> = emptyMap()) = request("GET", path, query = query)
    suspend fun reconnect(address: String) {
        transport.reconnect(address)
        get("/info")
    }
    suspend fun post(path: String, body: JsonObject = JsonObject(emptyMap()), mutation: Boolean = true,
        query: Map<String, String> = emptyMap()) = request("POST", path, body, query, mutation)
    suspend fun put(path: String, body: JsonObject, query: Map<String, String> = emptyMap()) = request("PUT", path, body, query, true)
    suspend fun delete(path: String, query: Map<String, String> = emptyMap()) = request("DELETE", path, query = query, mutation = true)
    suspend inline fun <reified T> read(path: String, query: Map<String, String> = emptyMap()): T =
        read(path, query, serializer<T>())
    suspend fun <T> read(path: String, query: Map<String, String>, serializer: kotlinx.serialization.KSerializer<T>): T =
        request("GET", path, query = query) { status, input -> codec.decode(status, input, serializer) }
    suspend fun download(path: String, query: Map<String, String>, destination: File) = transport.download(path, query, destination)
    suspend fun image(path: String) = transport.image(path)

    private suspend fun request(method: String, path: String, body: JsonObject? = null,
        query: Map<String, String> = emptyMap(), mutation: Boolean = false): ModuleApiResponse {
        return request(method, path, body, query, mutation) { status, input -> codec.decode(status, input) }
    }

    private suspend fun <T> request(method: String, path: String, body: JsonObject? = null,
        query: Map<String, String> = emptyMap(), mutation: Boolean = false, decode: (Int, InputStream) -> T): T {
        currentCoroutineContext().ensureActive()
        return withContext(if (mutation) NonCancellable + Dispatchers.IO else Dispatchers.IO) {
            transport.request(method, path, body, query, mutation, decode)
        }
    }
}
