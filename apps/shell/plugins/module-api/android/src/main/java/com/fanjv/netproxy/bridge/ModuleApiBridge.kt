package com.fanjv.netproxy.bridge

import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.api.ModuleApiTransport
import com.fanjv.netproxy.core.api.ModuleEnvelope
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.isActive
import kotlinx.coroutines.job
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.channels.Channel
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.builtins.nullable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.decodeFromStream
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import java.io.InputStream
import java.io.FilterInputStream
import java.lang.ref.WeakReference
import java.util.Base64

internal data class ReadResult(val status: Int, val body: JsonObject,
    val afterDelivery: DiagnosticShare.Prepared? = null)
internal data class EventResult(val name: String, val body: JsonObject)

internal class BridgeFailure(val code: String, val status: Int = failureStatus(code)) :
    IllegalStateException(SAFE_FAILURE_MESSAGE)

internal const val SAFE_FAILURE_MESSAGE = "Native module request failed"
private const val SAFE_SUCCESS_MESSAGE = "Native module request completed"
private val codePattern = Regex("[a-z][a-z0-9_]*(\\.[a-z][a-z0-9_]*)+")

private fun failureStatus(code: String): Int = when (code) {
    "api.unauthorized" -> 401
    "api.invalid_request", "api.import_invalid" -> 400
    "api.import_unreadable" -> 403
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
    private val workDispatcher: CoroutineDispatcher = Dispatchers.IO,
    private val prepareDiagnostics: (suspend () -> DiagnosticShare.Prepared)? = null,
) {
    private val lock = Any()
    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
    private val requests = HashMap<String, Job>()
    private var serviceWrite: Job? = null
    private var subscriptionCancel: Job? = null
    private val cancellations = LinkedHashMap<String, Long>()
    private var cancellationOverflowUntil = Long.MIN_VALUE
    private var loadedView: WeakReference<Any>? = null
    private var active: Subscription? = null
    private val producers = HashSet<Job>()
    private var foreground = false
    private var lifecycleVisibilityKnown = false
    private var destroyed = false
    private var generation = 0L
    private var dispatches = 0
    private val dispatchedJobs = HashSet<Job>()

    val visible: Boolean get() = synchronized(lock) { foreground && !destroyed }

    fun <T> dispatch(action: suspend () -> T, complete: (Result<T>) -> Unit) {
        val admitted = runCatching {
            synchronized(lock) {
                requireForeground()
                if (dispatches >= 10) throw BridgeFailure("api.busy")
                dispatches++
                generation
            }
        }
        if (admitted.isFailure) {
            runCatching { complete(Result.failure(admitted.exceptionOrNull()!!)) }
            return
        }
        val expected = admitted.getOrThrow()
        // 调度前进入协程，确保尚未执行 IO 时取消也能完成 Invoke。
        scope.launch(start = CoroutineStart.UNDISPATCHED) {
            val job = currentCoroutineContext().job
            var prepared: T? = null
            try {
                val result = runCatching {
                    synchronized(lock) {
                        requireGeneration(expected)
                        dispatchedJobs.add(job)
                    }
                    val value = withContext(workDispatcher) {
                        synchronized(lock) { requireGeneration(expected) }
                        action().also { prepared = it }
                    }
                    currentCoroutineContext().ensureActive()
                    synchronized(lock) { requireGeneration(expected) }
                    value
                }
                // 旧文档可能已释放回调，交付异常时不能再次完成同一个 Invoke。
                val delivered = runCatching {
                    complete(result)
                    if (result.isSuccess) (prepared as? ReadResult)?.afterDelivery?.let {
                        // Complete IPC first; opening Android's chooser then pauses this Activity.
                        currentCoroutineContext().ensureActive()
                        synchronized(lock) { requireGeneration(expected) }
                        it.deliver()
                    }
                }.isSuccess
                if (result.isFailure || !delivered) (prepared as? ReadResult)?.afterDelivery?.discard()
            } finally {
                synchronized(lock) {
                    dispatchedJobs.remove(job)
                    dispatches--
                }
            }
        }
    }

    suspend fun read(requestId: String, resource: String): ReadResult {
        val path = when (resource) {
            "info" -> "/info"
            "service" -> "/service"
            "nodes" -> "/nodes/snapshot"
            "policy_groups" -> "/policy-groups"
            "mode" -> "/mode"
            "catalog" -> "/catalog"
            "subscriptions" -> "/subscriptions"
            else -> throw BridgeFailure("api.read_only", 403)
        }
        return request(requestId, path, mutation = false)
    }

    suspend fun subscriptionRead(requestId: String, groupId: String, resource: String): ReadResult {
        if (!groupId.matches(Regex("[A-Za-z0-9][A-Za-z0-9._-]*")) || groupId == "staging" || groupId.contains(".."))
            throw BridgeFailure("api.invalid_request")
        if (groupId.toByteArray(Charsets.UTF_8).size > BODY_LIMIT) throw BridgeFailure("api.body_too_large")
        val suffix = when (resource) {
            "detail" -> ""
            "history" -> "/history"
            "progress" -> "/progress"
            else -> throw BridgeFailure("api.read_only", 403)
        }
        return request(requestId, "/subscriptions/$groupId$suffix", mutation = false)
    }

    suspend fun serviceAction(requestId: String, action: String): ReadResult {
        val path = when (action) {
            "start" -> "/service/start"
            "stop" -> "/service/stop"
            "restart" -> "/service/restart"
            "reload" -> "/service/reload"
            "check" -> "/service/check"
            else -> throw BridgeFailure("api.invalid_request", 400)
        }
        return request(requestId, path, mutation = true)
    }

    suspend fun subscriptionAction(raw: String): ReadResult {
        val args = SubscriptionActions.parse(raw)
        val path = "/subscriptions/${args.groupId}/${args.action}"
        return if (args.action == "cancel") {
            // A control request must not wait for the update's shared business-write slot.
            withTimeout(30_000) {
                request(args.requestId, path, mutation = true, control = true, subscriptionGroup = args.groupId)
            }
        } else request(args.requestId, path, mutation = true, subscriptionGroup = args.groupId)
    }

    suspend fun subscriptionWrite(raw: String): ReadResult {
        val args = SubscriptionActions.parseWrite(raw)
        val (method, path) = when (args.action) {
            "add" -> "POST" to "/subscriptions"
            "edit" -> "PUT" to "/subscriptions/${args.groupId}"
            "remove" -> "DELETE" to "/subscriptions/${args.groupId}"
            "update_all" -> "POST" to "/subscriptions/update-all"
            else -> throw BridgeFailure("api.invalid_request", 400)
        }
        return request(args.requestId, path, mutation = true, method = method,
            body = args.input ?: JsonObject(emptyMap()),
            subscriptionGroup = args.groupId, subscriptionMutation = true,
            subscriptionSummary = args.action == "update_all")
    }

    suspend fun management(raw: String): ReadResult {
        val args = Management.parse(raw)
        if (args.operation == "icon") return perform(args.requestId, mutation = false) {
            val key = args.input!!.getValue("icon_key").jsonPrimitive.content
            val bytes = transport.image("/android/icons/$key")
            val signature = byteArrayOf(0x89.toByte(), 0x50, 0x4e, 0x47, 13, 10, 26, 10)
            if (bytes.size > 256 * 1024) throw BridgeFailure("api.body_too_large")
            if (bytes.size < signature.size || !bytes.copyOfRange(0, signature.size).contentEquals(signature)) throw BridgeFailure("api.invalid_response")
            ReadResult(200, envelope(true, "android.icon", buildJsonObject { put("content", Base64.getEncoder().encodeToString(bytes)) }))
        }
        if (args.operation == "diagnostics") return perform(args.requestId, mutation = false) {
            val prepared = (prepareDiagnostics ?: throw BridgeFailure("api.connection_unavailable"))()
            ReadResult(200, envelope(true, "diagnostics.prepared", buildJsonObject { put("prepared", true) }), prepared)
        }
        val route = Management.route(args)
        return request(args.requestId, route.path, route.mutation, route.method, route.body, route.query, management = args)
    }

    suspend fun policySelect(raw: String): ReadResult {
        val args = PolicyGroups.select(raw)
        return request(args.requestId, "/policy-groups/select", mutation = true, body = buildJsonObject {
            put("group_tag", args.groupTag); put("member_tag", args.memberTag)
        })
    }

    suspend fun changeMode(requestId: String, mode: Any?): ReadResult {
        if (mode !is String || mode.isBlank()) throw BridgeFailure("api.invalid_request")
        return request(requestId, "/mode", mutation = true, method = "PUT", body = buildJsonObject { put("mode", mode) })
    }

    suspend fun policyDelay(raw: String): ReadResult {
        val args = PolicyGroups.delay(raw)
        return request(args.requestId, "/policy-groups/delay", mutation = true, body = buildJsonObject {
            put("group_tag", args.groupTag)
        })
    }

    private suspend fun request(
        requestId: String,
        path: String,
        mutation: Boolean,
        method: String = if (mutation) "POST" else "GET",
        body: JsonObject? = if (mutation) JsonObject(emptyMap()) else null,
        query: Map<String, String> = emptyMap(),
        control: Boolean = false,
        subscriptionGroup: String? = null,
        subscriptionMutation: Boolean = subscriptionGroup != null,
        subscriptionSummary: Boolean = false,
        management: Management.Request? = null,
    ): ReadResult {
        if (body != null && body.toString().toByteArray(Charsets.UTF_8).size > BODY_LIMIT) {
            throw BridgeFailure("api.body_too_large")
        }
        return perform(requestId, mutation, control) {
            transport.request(method, path, body, query, mutation) { status, input ->
                ReadResult(status, decode(status, input, subscriptionGroup = subscriptionGroup,
                    subscriptionMutation = subscriptionMutation, subscriptionSummary = subscriptionSummary,
                    management = management,
                    policy = path in setOf("/policy-groups", "/policy-groups/select", "/policy-groups/delay")) { PublicCatalog.read(path, it) })
            }
        }
    }

    private suspend fun <T> perform(requestId: String, mutation: Boolean, control: Boolean = false,
        action: suspend () -> T): T = coroutineScope {
        validateId(requestId)
        val job = currentCoroutineContext().job
        var mutationLease: MutationAdmissionGate.Lease? = null
        synchronized(lock) {
            requireForeground()
            job.ensureActive()
            pruneCancellations()
            if (cancellations.remove(requestId) != null || nowMillis() < cancellationOverflowUntil) {
                throw BridgeFailure("api.cancelled")
            }
            if (requests.containsKey(requestId)) throw BridgeFailure("api.request_exists", 409)
            if (control) {
                if (subscriptionCancel != null) throw BridgeFailure("api.busy", 429)
                subscriptionCancel = job
            } else if (mutation) {
                if (serviceWrite != null) throw BridgeFailure("api.busy", 429)
                mutationLease = MutationAdmissionGate.process.tryAcquire() ?: throw BridgeFailure("api.busy", 429)
                serviceWrite = job
            } else if (requests.size - (if (serviceWrite != null) 1 else 0) - (if (subscriptionCancel != null) 1 else 0) >= 8) {
                throw BridgeFailure("api.busy", 429)
            }
            requests[requestId] = job
        }
        var prepared: T? = null
        try {
            val response = action().also { prepared = it }
            currentCoroutineContext().ensureActive()
            synchronized(lock) { requireForeground() }
            response
        } catch (error: CancellationException) {
            (prepared as? ReadResult)?.afterDelivery?.discard()
            throw error
        } catch (error: Exception) {
            (prepared as? ReadResult)?.afterDelivery?.discard()
            throw safeFailure(error)
        } finally {
            synchronized(lock) {
                if (requests[requestId] === job) requests.remove(requestId)
                if (serviceWrite === job) serviceWrite = null
                if (subscriptionCancel === job) subscriptionCancel = null
            }
            mutationLease?.close()
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
            // 最多保留一个正在收尾的旧流与一个新流，避免快速替换堆积取消中的协程。
            if (producers.size >= 2) throw BridgeFailure("api.busy")
            // A new document cannot release the previous document's IPC handles after WebView reload.
            active?.let {
                it.stop(BridgeFailure("api.subscription_closed"))
                it.producer.cancel()
            }
            Subscription(subscriptionId, topics.toSet()).also { session ->
                session.producer = scope.launch(workDispatcher, start = CoroutineStart.LAZY) {
                    try {
                        transport.watch(session.topics) { event ->
                            if (event.name !in session.topics &&
                                !(event.name == "traffic" && "status" in session.topics)) {
                                throw BridgeFailure("api.invalid_response", 502)
                            }
                            if (event.content.toByteArray(Charsets.UTF_8).size > BODY_LIMIT) {
                                throw BridgeFailure("api.body_too_large", 413)
                            }
                            val body = decode(200, event.content.byteInputStream(), allowBusinessFailure = false) { PublicCatalog.event(event.name, it) }
                            session.queue.send(EventResult(event.name, body))
                        }
                        throw BridgeFailure("api.disconnected")
                    } catch (error: Exception) {
                        synchronized(lock) { session.stop(safeFailure(error)) }
                    }
                }
                producers.add(session.producer)
                session.producer.invokeOnCompletion { synchronized(lock) { producers.remove(session.producer) } }
                active = session
            }
        }
        subscription.producer.start()
    }

    suspend fun nextEvent(subscriptionId: String): EventResult {
        validateId(subscriptionId)
        val job = currentCoroutineContext().job
        val session = synchronized(lock) {
            requireForeground()
            job.ensureActive()
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
        lifecycleVisibilityKnown = true
        if (!destroyed) foreground = true
    }

    fun load(view: Any, visible: Boolean) = synchronized(lock) {
        if (destroyed) return
        if (loadedView != null && loadedView?.get() !== view) stopWork()
        loadedView = WeakReference(view)
        // load 时 AndroidX 的状态可能尚未追上 onResume，不能覆盖已收到的生命周期回调。
        if (!lifecycleVisibilityKnown) foreground = visible
        if (!foreground) stopWork()
    }

    fun pause() = synchronized(lock) {
        lifecycleVisibilityKnown = true
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
        generation++
        // Cancel under the same lock as admission, so a late cancel cannot affect a new request.
        dispatchedJobs.toList().forEach { it.cancel() }
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

    private fun requireGeneration(expected: Long) {
        requireForeground()
        if (generation != expected) throw BridgeFailure("api.cancelled")
    }

    private fun pruneCancellations() {
        val now = nowMillis()
        cancellations.entries.removeAll { it.value <= now }
    }

    @OptIn(ExperimentalSerializationApi::class)
    private fun decode(status: Int, input: InputStream, allowBusinessFailure: Boolean = true,
        subscriptionGroup: String? = null,
        subscriptionMutation: Boolean = subscriptionGroup != null,
        subscriptionSummary: Boolean = false,
        management: Management.Request? = null,
        policy: Boolean = false,
        project: (JsonElement) -> JsonElement = { it }): JsonObject {
        val header = try {
            BodyLimitInputStream(input).use {
                // 复用共享 Envelope 的严格字段校验；Go 错误响应可省略 data。
                json.decodeFromStream(ModuleEnvelope.serializer(JsonElement.serializer().nullable), it)
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
        val data = if (management != null) {
            if (ok) Management.success(management, header.data ?: throw BridgeFailure("api.invalid_response"))
            else Management.failure(management, header.data)
        } else if (subscriptionMutation) {
            if (ok && header.data !is JsonObject) throw BridgeFailure("api.invalid_response", 502)
            SubscriptionActions.result(header.data, subscriptionGroup, subscriptionSummary)
        } else if (ok) {
            project(header.data?.takeUnless { it is JsonNull } ?: throw BridgeFailure("api.invalid_response"))
        } else if (policy) PolicyGroups.failure(header.data) else buildJsonObject {
            for (key in listOf("accepted", "persisted")) {
                val flag = ((header.data as? JsonObject)?.get(key) as? JsonPrimitive)
                    ?.takeUnless { it.isString }?.booleanOrNull
                if (flag != null) put(key, flag)
            }
        }
        return envelope(ok, header.code, data)
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
