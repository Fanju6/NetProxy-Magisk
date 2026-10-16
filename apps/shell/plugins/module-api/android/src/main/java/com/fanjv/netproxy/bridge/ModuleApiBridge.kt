package com.fanjv.netproxy.bridge

import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.api.ModuleApiTransport
import com.fanjv.netproxy.core.api.ModuleEnvelope
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.isActive
import kotlinx.coroutines.job
import kotlinx.coroutines.launch
import kotlinx.coroutines.channels.Channel
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.decodeFromStream
import kotlinx.serialization.json.put
import java.io.InputStream
import java.io.FilterInputStream
import java.lang.ref.WeakReference

internal data class ReadResult(val status: Int, val body: JsonObject)
internal data class EventResult(val name: String, val body: JsonObject)

internal class BridgeFailure(val code: String, val status: Int = failureStatus(code)) :
    IllegalStateException(SAFE_FAILURE_MESSAGE)

internal const val SAFE_FAILURE_MESSAGE = "Native module request failed"
private const val SAFE_SUCCESS_MESSAGE = "Native module request completed"
private val codePattern = Regex("[a-z][a-z0-9_]*(\\.[a-z][a-z0-9_]*)+")

private fun failureStatus(code: String): Int = when (code) {
    "api.unauthorized" -> 401
    "api.invalid_request" -> 400
    "api.read_only" -> 403
    "api.body_too_large" -> 413
    "api.invalid_response", "api.unsupported_schema" -> 502
    "api.request_exists", "api.subscription_exists", "api.next_pending" -> 409
    "api.busy" -> 429
    "api.cancelled", "api.background", "api.destroyed", "api.subscription_closed" -> 0
    else -> 503
}

internal fun safeFailure(error: Exception): BridgeFailure = when (error) {
    is BridgeFailure -> error
    is CancellationException -> BridgeFailure("api.cancelled")
    is ModuleApiException -> BridgeFailure(safeCode(error.resultCode).takeIf { it.startsWith("api.") } ?: "api.request_failed")
    else -> BridgeFailure("api.native_failed")
}

private fun safeCode(code: String): String =
    code.takeIf { it.length <= 96 && it.matches(codePattern) } ?: "api.invalid_response"

/** Owns native work only. The frontend owns reconnection and foreground rebuilding. */
internal class ModuleApiBridge(
    private val transport: ModuleApiTransport,
    private val scope: CoroutineScope,
    private val nowMillis: () -> Long = { System.nanoTime() / 1_000_000 },
) {
    private val lock = Any()
    private val json = Json { ignoreUnknownKeys = true }
    private val requests = HashMap<String, Job>()
    private val cancellations = LinkedHashMap<String, Long>()
    private var cancellationOverflowUntil = Long.MIN_VALUE
    private var loadedView: WeakReference<Any>? = null
    private var active: Subscription? = null
    private var foreground = false
    private var destroyed = false

    val visible: Boolean get() = synchronized(lock) { foreground && !destroyed }

    suspend fun read(requestId: String, resource: String): ReadResult = coroutineScope {
        val path = when (resource) {
            "info" -> "/info"
            "service" -> "/service"
            else -> throw BridgeFailure("api.read_only", 403)
        }
        validateId(requestId)
        val job = currentCoroutineContext().job
        synchronized(lock) {
            requireForeground()
            pruneCancellations()
            if (cancellations.remove(requestId) != null || nowMillis() < cancellationOverflowUntil) {
                throw BridgeFailure("api.cancelled")
            }
            if (requests.containsKey(requestId)) throw BridgeFailure("api.request_exists", 409)
            if (requests.size >= 8) throw BridgeFailure("api.busy", 429)
            requests[requestId] = job
        }
        try {
            val response = transport.request("GET", path, null, emptyMap(), false) { status, input ->
                ReadResult(status, decode(status, input))
            }
            currentCoroutineContext().ensureActive()
            synchronized(lock) { requireForeground() }
            response
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            throw safeFailure(error)
        } finally {
            synchronized(lock) {
                if (requests[requestId] === job) requests.remove(requestId)
            }
        }
    }

    fun cancel(requestId: String) {
        validateId(requestId)
        synchronized(lock) {
            if (destroyed) return
            requests[requestId]?.let { it.cancel(); return@synchronized }
            pruneCancellations()
            if (requestId in cancellations || cancellations.size < MAX_CANCELLATIONS) {
                cancellations[requestId] = nowMillis() + CANCELLATION_WINDOW_MS
            } else {
                // Fail closed during a cancellation flood rather than evicting a pending cancellation.
                cancellationOverflowUntil = nowMillis() + CANCELLATION_WINDOW_MS
            }
        }
    }

    fun openEvents(subscriptionId: String, topics: List<String>) {
        validateId(subscriptionId)
        if (topics.isEmpty() || topics.size > 3 || topics.toSet().size != topics.size ||
            topics.any { it !in TOPICS }) throw BridgeFailure("api.invalid_request", 400)
        val subscription = synchronized(lock) {
            requireForeground()
            if (active?.id == subscriptionId) throw BridgeFailure("api.subscription_exists", 409)
            // A new document cannot release the previous document's IPC handles after WebView reload.
            active?.let {
                it.stop(BridgeFailure("api.subscription_closed"))
                it.producer.cancel()
            }
            Subscription(subscriptionId, topics.toSet()).also { session ->
                session.producer = scope.launch(start = CoroutineStart.LAZY) {
                    try {
                        transport.watch(session.topics) { event ->
                            if (event.name !in session.topics &&
                                !(event.name == "traffic" && "status" in session.topics)) {
                                throw BridgeFailure("api.invalid_response", 502)
                            }
                            if (event.content.toByteArray(Charsets.UTF_8).size > BODY_LIMIT) {
                                throw BridgeFailure("api.body_too_large", 413)
                            }
                            val body = decode(200, event.content.byteInputStream(), allowBusinessFailure = false)
                            session.queue.send(EventResult(event.name, body))
                        }
                        throw BridgeFailure("api.disconnected")
                    } catch (error: Exception) {
                        synchronized(lock) { session.stop(safeFailure(error)) }
                    }
                }
                active = session
            }
        }
        subscription.producer.start()
    }

    suspend fun nextEvent(subscriptionId: String): EventResult {
        validateId(subscriptionId)
        val session = synchronized(lock) {
            requireForeground()
            val current = active?.takeIf { it.id == subscriptionId }
                ?: throw BridgeFailure("api.subscription_closed")
            if (current.pulling) throw BridgeFailure("api.next_pending", 409)
            current.pulling = true
            current
        }
        try {
            val event = try {
                session.queue.receive()
            } catch (error: Exception) {
                throw synchronized(lock) { session.failure } ?: error
            }
            currentCoroutineContext().ensureActive()
            synchronized(lock) {
                requireForeground()
                if (active !== session) throw BridgeFailure("api.subscription_closed")
                session.failure?.let { throw it }
            }
            return event
        } finally {
            synchronized(lock) { session.pulling = false }
        }
    }

    fun closeEvents(subscriptionId: String) {
        validateId(subscriptionId)
        synchronized(lock) {
            active?.takeIf { it.id == subscriptionId }?.let {
                active = null
                it.stop(BridgeFailure("api.subscription_closed"))
                it.producer.cancel()
            }
        }
    }

    fun resume() = synchronized(lock) {
        if (!destroyed) foreground = true
    }

    fun load(view: Any, visible: Boolean) = synchronized(lock) {
        if (destroyed) return
        if (loadedView != null && loadedView?.get() !== view) stopWork()
        loadedView = WeakReference(view)
        foreground = visible
        if (!visible) stopWork()
    }

    fun pause() = synchronized(lock) {
        foreground = false
        stopWork()
    }

    fun destroy() {
        synchronized(lock) {
            destroyed = true
            foreground = false
            stopWork()
            cancellations.clear()
            loadedView?.clear()
            loadedView = null
        }
        scope.cancel()
    }

    private fun stopWork() {
        // Cancel under the same lock as admission, so a late cancel cannot affect a new request.
        requests.values.toList().forEach { it.cancel() }
        active?.let {
            active = null
            it.stop(BridgeFailure("api.subscription_closed"))
            it.producer.cancel()
        }
    }

    private fun requireForeground() {
        if (destroyed || !scope.isActive) throw BridgeFailure("api.destroyed")
        if (!foreground) throw BridgeFailure("api.background")
    }

    private fun pruneCancellations() {
        val now = nowMillis()
        cancellations.entries.removeAll { it.value <= now }
    }

    @OptIn(ExperimentalSerializationApi::class)
    private fun decode(status: Int, input: InputStream, allowBusinessFailure: Boolean = true): JsonObject {
        val header = try {
            BodyLimitInputStream(input).use {
                // Reuse ModuleApiCodec's strict schema/boolean/string serializers in one stream decode.
                json.decodeFromStream(ModuleEnvelope.serializer(JsonElement.serializer()), it)
            }
        } catch (error: Exception) {
            if (error is BridgeFailure) throw error
            throw BridgeFailure(if (error is ModuleApiException) safeFailure(error).code else "api.invalid_response")
        }
        if (status !in 100..599) throw BridgeFailure("api.invalid_response")
        if (header.schema != 1) throw BridgeFailure("api.unsupported_schema")
        if (safeCode(header.code) != header.code) throw BridgeFailure("api.invalid_response")
        val ok = status in 200..299 && header.ok
        if (!ok && !allowBusinessFailure) throw safeFailure(ModuleApiException(header.code, SAFE_FAILURE_MESSAGE))
        return envelope(ok, header.code, if (ok) header.data else JsonObject(emptyMap()))
    }

    private fun envelope(ok: Boolean, code: String, data: kotlinx.serialization.json.JsonElement) = buildJsonObject {
        put("schema", 1)
        put("ok", ok)
        put("code", code)
        put("message", if (ok) SAFE_SUCCESS_MESSAGE else SAFE_FAILURE_MESSAGE)
        put("data", data)
    }

    private fun validateId(id: String) {
        if (!id.matches(Regex("[A-Za-z0-9_-]{1,96}"))) throw BridgeFailure("api.invalid_request", 400)
    }

    private class Subscription(val id: String, val topics: Set<String>) {
        val queue = Channel<EventResult>(1)
        lateinit var producer: Job
        var pulling = false
        var failure: BridgeFailure? = null

        fun stop(error: BridgeFailure) {
            if (failure == null) failure = error
            queue.cancel(CancellationException(SAFE_FAILURE_MESSAGE))
        }
    }

    private class BodyLimitInputStream(input: InputStream) : FilterInputStream(input) {
        private var count = 0

        override fun read(): Int = `in`.read().also { if (it >= 0) record(1) }

        override fun read(buffer: ByteArray, offset: Int, length: Int): Int =
            `in`.read(buffer, offset, minOf(length, BODY_LIMIT - count + 1)).also { if (it > 0) record(it) }

        private fun record(size: Int) {
            count += size
            if (count > BODY_LIMIT) throw BridgeFailure("api.body_too_large")
        }
    }

    private companion object {
        val TOPICS = setOf("status", "groups", "subscriptions")
        const val BODY_LIMIT = 4 * 1024 * 1024
        const val MAX_CANCELLATIONS = 128
        const val CANCELLATION_WINDOW_MS = 60_000L
    }
}
