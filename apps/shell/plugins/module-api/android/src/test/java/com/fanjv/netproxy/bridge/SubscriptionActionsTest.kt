package com.fanjv.netproxy.bridge

import com.fanjv.netproxy.core.api.ConnectionStore
import com.fanjv.netproxy.core.api.LocalHttpTransport
import com.fanjv.netproxy.core.api.ModuleApiTransport
import com.fanjv.netproxy.core.api.ModuleConnection
import com.fanjv.netproxy.core.api.ModuleEvent
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import kotlinx.serialization.json.*
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import okio.Buffer
import org.junit.After
import org.junit.Assert.*
import org.junit.Test
import java.io.File
import java.io.IOException
import java.io.InputStream
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

@OptIn(ExperimentalCoroutinesApi::class)
class SubscriptionActionsTest {
    private val bridges = mutableListOf<ModuleApiBridge>()
    @After fun close() = bridges.forEach { it.destroy() }

    @Test fun strictArgumentsRejectUnknownFieldsWrongTypesAndUnsafeIdentity() {
        for (action in listOf("update", "cancel")) {
            assertEquals(action, SubscriptionActions.parse(args(action)).action)
        }
        for (action in listOf("add", "edit", "delete", "update-all", "Update", "POST", "/update", "update?x=1")) {
            failure("api.invalid_request") { SubscriptionActions.parse(args(action)) }
        }
        for (key in listOf("path", "method", "query", "body", "headers", "url", "private")) {
            failure("api.invalid_request") { SubscriptionActions.parse(args().dropLast(1) + ",\"$key\":{} }") }
        }
        for (group in listOf("", "staging", "..", "a..b", "a/b", "a%2fb", "a?private=true", "_a", "中文")) {
            failure("api.invalid_request") { SubscriptionActions.parse(args(group = group)) }
        }
        for (id in listOf("", "bad/id", "a".repeat(97))) {
            failure("api.invalid_request") { SubscriptionActions.parse(args(id = id)) }
        }
        for (raw in listOf("{}", "[]", "null", "bad", args().replace("\"update\"", "null"),
            args().replace("\"default\"", "123"), args().replace("\"request\"", "false"))) {
            failure("api.invalid_request") { SubscriptionActions.parse(raw) }
        }
        failure("api.body_too_large") { SubscriptionActions.parse(args(group = "a".repeat(LIMIT))) }
    }

    @Test fun updateAndCancelOnlySendFixedPOSTsWithEmptyBodyAndNoQuery() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        for (action in listOf("update", "cancel")) {
            val result = bridge.subscriptionAction(args(action, group = "a.b-C_1"))
            assertEquals(200, result.status)
            assertEquals(Json.parseToJsonElement("{}"), result.body["data"])
        }
        assertEquals(listOf("/subscriptions/a.b-C_1/update", "/subscriptions/a.b-C_1/cancel"), transport.paths)
        assertEquals(listOf("POST", "POST"), transport.methods)
        assertEquals(listOf(true, true), transport.mutations)
        for (raw in listOf(args("add"), args("delete"), args(group = "a/b"), args().dropLast(1) + ",\"body\":{}}")) {
            assertFailure("api.invalid_request", runCatching { bridge.subscriptionAction(raw) })
        }
        assertEquals(2, transport.paths.size)
    }

    @Test fun successAndErrorsProjectOnlyTypedSafeFactsAndKeepHTTPStatusCodeAndPersisted() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        val safe = """{"id":"default","group_id":"default","node_count":3,"revision":12,"persisted":true,"not_modified":true,"structure_changed":false,"runtime_synced":false,"runtime_sync_pending":true,"runtime_sync_state":"failed"}"""
        val unsafe = safe.dropLast(1) + """, "url":"$PRIVATE","headers":{"Authorization":"$PRIVATE"},"hwid":"$PRIVATE","cause":"$PRIVATE","used_proxy":true,"metadata":{"secret":"$PRIVATE"}}"""
        for (action in listOf("update", "cancel")) {
            for ((status, ok) in listOf(200 to true, 200 to false, 401 to false, 409 to false, 422 to false, 502 to false, 504 to false)) {
                transport.status = status
                transport.content = envelope(unsafe, ok, if (ok) "subscription.updated" else "subscription.runtime_sync_failed")
                val result = bridge.subscriptionAction(args(action))
                assertEquals(status, result.status)
                assertEquals(ok, result.body.getValue("ok").jsonPrimitive.boolean)
                assertEquals(if (ok) "subscription.updated" else "subscription.runtime_sync_failed", result.body.getValue("code").jsonPrimitive.content)
                assertEquals(Json.parseToJsonElement(safe), result.body["data"])
                assertFalse(result.body.toString().contains(PRIVATE))
            }
        }
        // Go errors may have a raw string cause or no data, not a structured result.
        for (data in listOf("null", "[]", "\"$PRIVATE\"")) {
            transport.status = 409
            transport.content = envelope(data, false, "subscription.busy")
            assertEquals(JsonObject(emptyMap()), bridge.subscriptionAction(args()).body["data"])
        }
        transport.content = """{"schema":1,"ok":false,"code":"api.unauthorized","message":"$PRIVATE"}"""
        transport.status = 401
        assertEquals(JsonObject(emptyMap()), bridge.subscriptionAction(args()).body["data"])
    }

    @Test fun malformedSafeFieldsAndOversizedResponseNeverReachJavaScript() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        for (data in listOf("[]", "null", """{"persisted":"true"}""", """{"revision":"1"}""",
            """{"revision":1.5}""", """{"node_count":-1}""", """{"id":"other"}""",
            """{"group_id":{"url":"$PRIVATE"}}""", """{"runtime_sync_state":"$PRIVATE"}""",
            """{"runtime_synced":null}""")) {
            transport.content = envelope(data)
            assertFailure("api.invalid_response", runCatching { bridge.subscriptionAction(args()) })
        }
        transport.content = envelope("""{"cause":"${"x".repeat(LIMIT)}"}""")
        assertFailure("api.body_too_large", runCatching { bridge.subscriptionAction(args()) })
    }

    @Test fun cancellationExecutesIndependentlyWhileUpdateUsesTheSharedWriteSlot() = runTest {
        val transport = Transport().apply { updateGate = CompletableDeferred(); cancelGate = CompletableDeferred() }
        val bridge = bridge(transport)
        val update = async { bridge.subscriptionAction(args("update", "update")) }
        runCurrent()
        advanceTimeBy(151_000)
        runCurrent()
        assertFalse(update.isCompleted)
        assertFailure("api.busy", runCatching { bridge.serviceAction("service", "start") })
        assertFailure("api.busy", runCatching { bridge.changeMode("mode", "Rule") })
        assertFailure("api.busy", runCatching { bridge.policySelect("""{"requestId":"node","groupTag":"Proxy","memberTag":"a"}""") })
        assertFailure("api.busy", runCatching { bridge.subscriptionAction(args("update", "other-update")) })
        val cancel = async { bridge.subscriptionAction(args("cancel", "cancel")) }
        runCurrent()
        assertEquals(listOf("/subscriptions/default/update", "/subscriptions/default/cancel"), transport.paths)
        assertFailure("api.busy", runCatching { bridge.subscriptionAction(args("cancel", "duplicate-cancel")) })
        assertEquals(200, bridge.read("read", "info").status)
        transport.cancelGate!!.complete(Unit)
        runCurrent()
        assertEquals(200, cancel.await().status)
        assertFalse(update.isCompleted) // Cancel acknowledgment is not transaction completion.
        transport.updateGate!!.complete(Unit)
        runCurrent()
        assertEquals(200, update.await().status)
        assertEquals(200, bridge.serviceAction("after", "stop").status)
        assertEquals(0, transport.cancelled)
    }

    @Test fun cancelHasThirtySecondBoundAndDoesNotCancelThePendingUpdate() = runTest {
        val transport = Transport().apply { updateGate = CompletableDeferred(); cancelGate = CompletableDeferred() }
        val bridge = bridge(transport)
        val update = async { runCatching { bridge.subscriptionAction(args("update", "update")) } }
        runCurrent()
        val cancel = async { runCatching { bridge.subscriptionAction(args("cancel", "cancel")) } }
        runCurrent()
        advanceTimeBy(30_000)
        runCurrent()
        assertTrue(cancel.await().exceptionOrNull() is TimeoutCancellationException)
        assertFalse(update.isCompleted)
        assertEquals(1, transport.cancelled)
        // The bounded control slot is released; no automatic replay occurred.
        assertEquals(2, transport.paths.size)
        transport.cancelGate!!.complete(Unit)
        assertEquals(200, bridge.subscriptionAction(args("cancel", "next-cancel")).status)
        bridge.cancel("update")
        runCurrent()
        assertTrue(update.await().exceptionOrNull() is CancellationException)
    }

    @Test fun totalDispatchBudgetStaysBoundedWithAnIndependentControlSlot() = runTest {
        val transport = Transport().apply { updateGate = CompletableDeferred(); cancelGate = CompletableDeferred(); holdReads = true }
        val bridge = bridge(transport)
        val results = mutableListOf<Result<ReadResult>>()
        repeat(8) { bridge.dispatch({ bridge.read("read$it", "info") }, results::add) }
        bridge.dispatch({ bridge.subscriptionAction(args("update", "update")) }, results::add)
        bridge.dispatch({ bridge.subscriptionAction(args("cancel", "cancel")) }, results::add)
        runCurrent()
        assertEquals(10, transport.paths.size)
        assertTrue(results.isEmpty())
        bridge.dispatch({ bridge.read("overflow", "info") }, results::add)
        assertFailure("api.busy", results.single())
        bridge.pause()
        runCurrent()
        assertEquals(11, results.size)
        assertEquals(10, transport.cancelled)
        bridge.resume()
        runCurrent()
        assertEquals(10, transport.paths.size)
    }

    @Test fun pendingUpdateAndCancelFollowLifecycleAndNeverReplay() = runTest {
        for (boundary in listOf("pause", "destroy", "webview")) {
            val transport = Transport().apply { updateGate = CompletableDeferred(); cancelGate = CompletableDeferred() }
            val bridge = bridge(transport)
            val oldView = Any()
            bridge.load(oldView, true)
            val results = mutableListOf<Result<ReadResult>>()
            bridge.dispatch({ bridge.subscriptionAction(args("update", "update")) }, results::add)
            bridge.dispatch({ bridge.subscriptionAction(args("cancel", "cancel")) }, results::add)
            runCurrent()
            when (boundary) {
                "pause" -> bridge.pause()
                "destroy" -> bridge.destroy()
                else -> bridge.load(Any(), true)
            }
            runCurrent()
            assertEquals(2, results.size)
            assertTrue(results.all { it.isFailure })
            assertEquals(2, transport.cancelled)
            bridge.resume()
            runCurrent()
            assertEquals(2, transport.paths.size)
        }
    }

    @Test fun cancellingClientWaitingNeverSendsTheServerCancelOperation() = runTest {
        val transport = Transport().apply { updateGate = CompletableDeferred() }
        val bridge = bridge(transport)
        bridge.cancel("before")
        assertFailure("api.cancelled", runCatching { bridge.subscriptionAction(args(id = "before")) })
        val update = async { runCatching { bridge.subscriptionAction(args(id = "pending")) } }
        runCurrent()
        bridge.cancel("pending")
        runCurrent()
        assertTrue(update.await().exceptionOrNull() is CancellationException)
        assertEquals(listOf("/subscriptions/default/update"), transport.paths)
    }

    @Test fun sharedHttpsTransportRetains615SecondBudgetAndNeverRetriesSubscriptionPOSTs() = runTest {
        for (action in listOf("update", "cancel")) {
            for (fail in listOf(false, true)) {
                val credentials = ModuleConnection("https://127.0.0.1:9091", "a".repeat(64), "b".repeat(64))
                val transport = LocalHttpTransport(ConnectionStore({ Json.encodeToString(credentials) }, {}))
                val pinned = LocalHttpTransport::class.java.getDeclaredMethod("pinned", String::class.java)
                    .apply { isAccessible = true }.invoke(transport, credentials.certificate_sha256) as OkHttpClient
                val calls = AtomicInteger()
                val probe = pinned.newBuilder().addInterceptor { chain ->
                    calls.incrementAndGet()
                    assertEquals(615_000, chain.readTimeoutMillis())
                    assertEquals(TimeUnit.SECONDS.toNanos(620), chain.call().timeout().timeoutNanos())
                    val request = chain.request()
                    assertEquals("POST", request.method)
                    assertEquals("/api/v1/subscriptions/default/$action", request.url.encodedPath)
                    assertNull(request.url.query)
                    val body = Buffer()
                    requireNotNull(request.body).writeTo(body)
                    assertEquals("{}", body.readUtf8())
                    if (fail) throw IOException(PRIVATE)
                    Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(503).message(PRIVATE)
                        .body(envelope("""{"persisted":true,"cause":"$PRIVATE","runtime_synced":false}""", false, "subscription.runtime_sync_failed").toResponseBody())
                        .build()
                }.build()
                LocalHttpTransport::class.java.getDeclaredField("pinnedClient").apply { isAccessible = true }
                    .set(transport, credentials.certificate_sha256 to probe)
                val bridge = bridge(transport)
                // OkHttp callbacks run on real threads; a virtual timer would jump straight to 30s.
                val result = withContext(Dispatchers.IO) { runCatching { bridge.subscriptionAction(args(action)) } }
                if (fail) assertFailure("api.connection_failed", result)
                else {
                    assertEquals(503, result.getOrThrow().status)
                    assertEquals(Json.parseToJsonElement("""{"persisted":true,"runtime_synced":false}"""), result.getOrThrow().body["data"])
                    assertFalse(result.getOrThrow().body.toString().contains(PRIVATE))
                }
                assertEquals(1, calls.get())
            }
        }
    }

    private fun TestScope.bridge(transport: ModuleApiTransport): ModuleApiBridge = ModuleApiBridge(
        transport, CoroutineScope(SupervisorJob() + StandardTestDispatcher(testScheduler)),
        workDispatcher = StandardTestDispatcher(testScheduler),
    ).also { bridges.add(it); it.resume() }

    private fun failure(code: String, block: () -> Any?) = assertFailure(code, runCatching(block))
    private fun assertFailure(code: String, result: Result<*>) {
        val error = result.exceptionOrNull() as? BridgeFailure
        assertNotNull("Expected $code, got ${result.exceptionOrNull()}", error)
        assertEquals(code, error!!.code)
        assertEquals(SAFE_FAILURE_MESSAGE, error.message)
        assertNull(error.cause)
    }

    private class Transport : ModuleApiTransport {
        val paths = mutableListOf<String>()
        val methods = mutableListOf<String>()
        val mutations = mutableListOf<Boolean>()
        var content = envelope()
        var status = 200
        var updateGate: CompletableDeferred<Unit>? = null
        var cancelGate: CompletableDeferred<Unit>? = null
        var holdReads = false
        var cancelled = 0
        override suspend fun <T> request(method: String, path: String, body: JsonObject?, query: Map<String, String>, mutation: Boolean,
            decode: (Int, InputStream) -> T): T {
            assertTrue(query.isEmpty())
            if (mutation) assertEquals(JsonObject(emptyMap()), body) else assertNull(body)
            paths.add(path); methods.add(method); mutations.add(mutation)
            try {
                if (path.endsWith("/update")) updateGate?.await()
                if (path.endsWith("/cancel")) cancelGate?.await()
                if (!mutation && holdReads) awaitCancellation()
                return decode(status, content.byteInputStream())
            } catch (error: CancellationException) { cancelled++; throw error }
        }
        override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) = awaitCancellation()
        override suspend fun reconnect(address: String): Unit = error("No reconnect")
        override suspend fun image(path: String): ByteArray = error("No images")
        override suspend fun download(path: String, query: Map<String, String>, destination: File): Unit = error("No download")
    }

    companion object {
        private const val LIMIT = 4 * 1024 * 1024
        private const val PRIVATE = "https://private.invalid/token-secret"
        private fun args(action: String = "update", id: String = "request", group: String = "default") =
            Json.encodeToString(SubscriptionActions.Request(id, group, action))
        private fun envelope(data: String = "{}", ok: Boolean = true, code: String = "subscription.updated") =
            """{"schema":1,"ok":$ok,"code":"$code","message":"$PRIVATE","data":$data}"""
    }
}
