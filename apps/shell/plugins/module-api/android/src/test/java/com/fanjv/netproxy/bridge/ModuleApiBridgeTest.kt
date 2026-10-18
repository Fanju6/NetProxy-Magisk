package com.fanjv.netproxy.bridge

import com.fanjv.netproxy.core.api.ConnectionStore
import com.fanjv.netproxy.core.api.LocalHttpTransport
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.api.ModuleApiTransport
import com.fanjv.netproxy.core.api.ModuleEvent
import com.fanjv.netproxy.core.api.ModuleConnection
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonPrimitive
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
class ModuleApiBridgeTest {
    private val bridges = mutableListOf<ModuleApiBridge>()

    @After fun destroyBridges() = bridges.forEach { it.destroy() }

    @Test fun onlyWhitelistedResourcesProduceReadonlyGETs() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        for (resource in listOf("info", "service", "nodes", "policy_groups", "mode")) {
            transport.content = envelope(data = if (resource == "nodes") """{"groups":[]}""" else POLICY)
            assertEquals(200, bridge.read(resource, resource).status)
        }
        assertEquals(listOf("/info", "/service", "/nodes/snapshot", "/policy-groups", "/mode"), transport.paths)
        for (resource in listOf("selection", "/info", "config", "service/start", "../info", "info?secret=x", "")) {
            assertFailure("api.read_only", runCatching { bridge.read("forbidden", resource) })
        }
        assertEquals(5, transport.paths.size)
    }

    @Test fun publicSubscriptionReadsHaveFixedGETPathsAndRejectPrivateAndWriteResources() = runTest {
        val transport = FakeTransport().apply { content = envelope(data = """{"groups":[]}""") }
        val bridge = bridge(transport)
        bridge.read("catalog", "catalog")
        transport.content = envelope(data = "[]")
        bridge.read("subscriptions", "subscriptions")
        bridge.subscriptionRead("history", "a.b-C_1", "history")
        transport.content = envelope(data = """{"id":"a.b-C_1","progress":null}""")
        bridge.subscriptionRead("progress", "a.b-C_1", "progress")
        assertEquals(listOf("/catalog", "/subscriptions", "/subscriptions/a.b-C_1/history", "/subscriptions/a.b-C_1/progress"), transport.paths)
        for (id in listOf("", "staging", "a..b", "a/b", "a%2fb", "a?private=true", "a#b", "_a", "中文")) {
            assertFailure("api.invalid_request", runCatching { bridge.subscriptionRead("bad", id, "detail") })
        }
        for (resource in listOf("private", "detail?private=true", "update", "update-all", "cancel", "add", "edit", "delete", "POST", "/subscriptions/default")) {
            assertFailure("api.read_only", runCatching { bridge.subscriptionRead("bad", "default", resource) })
        }
        assertEquals(4, transport.paths.size)
    }

    @Test fun newResourceReadsAreCancelledOnBackgroundAndNeverReplayed() = runTest {
        val transport = FakeTransport().apply { holdReads = true }
        val bridge = bridge(transport)
        val pending = async { runCatching { bridge.subscriptionRead("history", "default", "history") } }
        runCurrent()
        bridge.pause()
        runCurrent()
        assertTrue(pending.await().exceptionOrNull() is CancellationException)
        assertEquals(1, transport.cancelledReads)
        assertFailure("api.background", runCatching { bridge.subscriptionRead("next", "default", "detail") })
        bridge.resume()
        runCurrent()
        assertEquals(1, transport.paths.size)
    }

    @Test fun modeOperationsRetainFixedMethodAndBodyValidation() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        bridge.changeMode("mode", "Rule")
        assertEquals("PUT", transport.methods.last())
        assertEquals("/mode", transport.paths.last())
        assertEquals(Json.parseToJsonElement("""{"mode":"Rule"}"""), transport.bodies.last())
        for (mode in listOf(null, 1, true, emptyMap<String, String>(), "", "  ", "\n")) {
            assertFailure("api.invalid_request", runCatching { bridge.changeMode("bad", mode) })
        }
        assertFailure("api.body_too_large", runCatching { bridge.changeMode("large", "\n".repeat(2_100_000) + "Rule") })
        assertEquals(1, transport.paths.size)
    }

    @Test fun m42FailuresCancellationAndBackgroundNeverReplayMutations() = runTest {
        val operations: List<suspend (ModuleApiBridge, String) -> ReadResult> = listOf(
            { bridge, id -> bridge.policySelect(policyRequest(id, true)) },
            { bridge, id -> bridge.policyDelay(policyRequest(id, false)) },
            { bridge, id -> bridge.changeMode(id, "Rule") },
        )
        for (operation in operations) {
            val transport = FakeTransport().apply { requestFailure = IOException(PRIVATE) }
            val bridge = bridge(transport)
            assertTrue(runCatching { operation(bridge, "error") }.isFailure)
            bridge.pause()
            bridge.resume()
            runCurrent()
            assertEquals(1, transport.paths.size)
            transport.requestFailure = null
            transport.status = 502
            transport.content = envelope(ok = false, code = "node.runtime_sync_failed", data = """{"persisted":true}""")
            assertEquals(Json.parseToJsonElement("""{"persisted":true}"""), operation(bridge, "persisted").body["data"])
            transport.holdWrites = true
            val pending = async { runCatching { operation(bridge, "cancel") } }
            runCurrent()
            bridge.cancel("cancel")
            runCurrent()
            assertTrue(pending.await().exceptionOrNull() is CancellationException)
            bridge.pause()
            bridge.resume()
            runCurrent()
            assertEquals(3, transport.paths.size)
            assertEquals(1, transport.cancelledWrites)
        }
    }

    @Test fun m42MutationsShareServiceAdmissionAndCannotOverlapAnAcceptedWrite() = runTest {
        val transport = FakeTransport().apply { holdWrites = true }
        val bridge = bridge(transport)
        val service = async { runCatching { bridge.serviceAction("service", "start") } }
        runCurrent()
        assertFailure("api.busy", runCatching { bridge.policySelect(policyRequest("select", true)) })
        assertFailure("api.busy", runCatching { bridge.changeMode("mode", "Rule") })
        assertFailure("api.busy", runCatching { bridge.policyDelay(policyRequest("delay", false)) })
        transport.content = envelope(data = """{"groups":[]}""")
        assertEquals(200, bridge.read("read", "nodes").status)
        bridge.cancel("service")
        runCurrent()
        assertTrue(service.await().exceptionOrNull() is CancellationException)
        val selection = async { runCatching { bridge.policySelect(policyRequest("select", true)) } }
        runCurrent()
        assertFailure("api.busy", runCatching { bridge.serviceAction("service2", "stop") })
        bridge.pause()
        runCurrent()
        assertTrue(selection.await().exceptionOrNull() is CancellationException)
    }

    @Test fun successIsAnEnvelopeObjectWithFixedMessage() = runTest {
        val bridge = bridge(FakeTransport().apply { content = envelope(message = PRIVATE) })
        val result = bridge.read("one", "info")
        assertEquals(200, result.status)
        assertEquals("1", result.body.getValue("schema").jsonPrimitive.content)
        assertTrue(result.body.getValue("ok").jsonPrimitive.boolean)
        assertEquals("fixture.status", result.body.getValue("code").jsonPrimitive.content)
        assertEquals("Native module request completed", result.body.getValue("message").jsonPrimitive.content)
        assertFalse(result.body.toString().contains(PRIVATE))
    }

    @Test fun lifecycleOperationsProduceFixedEmptyPOSTsAndReturnActualGoActionAndStatus() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        for ((action, state) in listOf("start" to "ready", "stop" to "stopped", "restart" to "ready", "reload" to "ready", "check" to "ready")) {
            val data = """{"action":"$action","status":{"state":"$state","pid":0}}"""
            transport.content = envelope(code = "service.$action", message = PRIVATE, data = data)
            val result = bridge.serviceAction("action", action)
            assertEquals(200, result.status)
            assertEquals(Json.parseToJsonElement(data), result.body["data"])
            assertEquals("service.$action", result.body.getValue("code").jsonPrimitive.content)
            assertFalse(result.body.toString().contains(PRIVATE))
        }
        for (action in listOf("toggle", "status", "Start", " start", "stop ",
            "/service/start", "service/stop", "start?secret=x", "POST", PRIVATE, "")) {
            assertFailure("api.invalid_request", runCatching { bridge.serviceAction("forbidden", action) })
        }
        for (id in listOf("", "../action", "x".repeat(97))) {
            assertFailure("api.invalid_request", runCatching { bridge.serviceAction(id, "start") })
        }
        assertEquals(listOf("/service/start", "/service/stop", "/service/restart", "/service/reload", "/service/check"), transport.paths)
    }

    @Test fun serviceErrorsPreserveOnlyBooleanConfirmationFactsAlongsideHTTPStatusAndCode() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        for (status in listOf(200, 401, 409, 503)) {
            for (value in listOf("true", "false", "null", "\"true\"", "\"false\"", "1", "{}", "[]")) {
                transport.status = status
                transport.content = envelope(ok = false, code = "service.operation_failed", message = PRIVATE,
                    data = """{"accepted":$value,"persisted":$value,"url":"$PRIVATE","diagnostics":{"secret":"$PRIVATE"}}""")
                val result = bridge.serviceAction("error", "start")
                assertEquals(status, result.status)
                assertFalse(result.body.getValue("ok").jsonPrimitive.boolean)
                assertEquals("service.operation_failed", result.body.getValue("code").jsonPrimitive.content)
                assertEquals(SAFE_FAILURE_MESSAGE, result.body.getValue("message").jsonPrimitive.content)
                assertEquals(Json.parseToJsonElement(if (value in listOf("true", "false")) """{"accepted":$value,"persisted":$value}""" else "{}"), result.body["data"])
                assertFalse(result.body.toString().contains(PRIVATE))
            }
        }
        for (data in listOf("null", "[]", "true", "\"$PRIVATE\"", "{}")) {
            transport.content = envelope(ok = false, data = data)
            assertEquals(JsonObject(emptyMap()), bridge.serviceAction("error", "stop").body["data"])
        }
    }

    @Test fun failedServiceWritesAreNeverRetriedOrReplayedOnResume() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        for (error in listOf(IOException(PRIVATE), ModuleApiException("api.unauthorized", PRIVATE))) {
            transport.requestFailure = error
            val before = transport.paths.size
            assertTrue(runCatching { bridge.serviceAction("failure", "start") }.isFailure)
            bridge.pause()
            bridge.resume()
            runCurrent()
            assertEquals(before + 1, transport.paths.size)
        }
        transport.requestFailure = null
        for ((status, content) in listOf(
            401 to envelope(ok = false, code = "api.unauthorized", message = PRIVATE),
            503 to envelope(ok = false, code = "service.start_failed", message = PRIVATE),
            200 to "malformed $PRIVATE",
            200 to envelope().replace("\"schema\":1", "\"schema\":2"),
        )) {
            transport.status = status
            transport.content = content
            val before = transport.paths.size
            runCatching { bridge.serviceAction("failure", "start") }
            assertEquals(before + 1, transport.paths.size)
        }
    }

    @Test fun sharedTransportUses135SecondMutationPolicyAndNeverRetriesServiceWrites() = runTest {
        for (action in listOf("start", "stop")) {
            for (failNetwork in listOf(false, true)) {
                val credentials = ModuleConnection("https://127.0.0.1:9091", "a".repeat(64), "b".repeat(64))
                val transport = LocalHttpTransport(ConnectionStore({ Json.encodeToString(credentials) }, {}))
                val pinned = LocalHttpTransport::class.java.getDeclaredMethod("pinned", String::class.java)
                    .apply { isAccessible = true }.invoke(transport, credentials.certificate_sha256) as OkHttpClient
                val calls = AtomicInteger()
                // Probe after the shared policy interceptor without opening sockets or changing production hooks.
                val probe = pinned.newBuilder().addInterceptor { chain ->
                    calls.incrementAndGet()
                    assertEquals(135_000, chain.readTimeoutMillis())
                    assertEquals(TimeUnit.SECONDS.toNanos(140), chain.call().timeout().timeoutNanos())
                    val request = chain.request()
                    assertEquals("POST", request.method)
                    assertEquals("/api/v1/service/$action", request.url.encodedPath)
                    assertNull(request.url.query)
                    val body = Buffer()
                    requireNotNull(request.body).writeTo(body)
                    assertEquals("{}", body.readUtf8())
                    if (failNetwork) throw IOException(PRIVATE)
                    Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(503).message(PRIVATE)
                        .body(envelope(ok = false, code = "service.operation_failed", message = PRIVATE,
                            data = """{"persisted":true,"url":"$PRIVATE"}""").toResponseBody())
                        .build()
                }.build()
                LocalHttpTransport::class.java.getDeclaredField("pinnedClient").apply { isAccessible = true }
                    .set(transport, credentials.certificate_sha256 to probe)
                val result = runCatching { bridge(transport).serviceAction("write", action) }
                if (failNetwork) {
                    assertFailure("api.connection_failed", result)
                } else {
                    assertEquals(503, result.getOrThrow().status)
                    assertEquals("service.operation_failed", result.getOrThrow().body.getValue("code").jsonPrimitive.content)
                    assertEquals(Json.parseToJsonElement("""{"persisted":true}"""), result.getOrThrow().body["data"])
                    assertFalse(result.getOrThrow().body.toString().contains(PRIVATE))
                }
                assertEquals(1, calls.get())
            }
        }
    }

    @Test fun serviceWriteHasOneSlotIndependentOfEightReadsAndSSE() = runTest {
        val transport = FakeTransport().apply { holdReads = true; holdWrites = true }
        val bridge = bridge(transport)
        val completions = mutableListOf<Result<Any>>()
        repeat(8) { bridge.dispatch({ bridge.read("read$it", "info") }, completions::add) }
        bridge.dispatch({ bridge.serviceAction("write", "start") }, completions::add)
        bridge.openEvents("events", listOf("status"))
        bridge.dispatch({ bridge.nextEvent("events") }, completions::add)
        runCurrent()
        assertTrue(completions.isEmpty())
        assertEquals(9, transport.paths.size)
        repeat(128) {
            assertFailure("api.busy", runCatching { bridge.serviceAction("overflow$it", if (it % 2 == 0) "stop" else "start") })
        }
        assertFailure("api.busy", runCatching { bridge.read("overflow", "service") })
        assertFailure("api.request_exists", runCatching { bridge.read("write", "info") })
        assertFailure("api.request_exists", runCatching { bridge.serviceAction("read0", "stop") })
        transport.events.send(ModuleEvent("status", envelope()))
        runCurrent()
        assertEquals(1, completions.size)
        assertTrue(completions.single().isSuccess)
        assertEquals(9, transport.paths.size)
        bridge.cancel("read0")
        runCurrent()
        transport.holdReads = false
        assertEquals(200, bridge.read("fresh", "service").status)
        assertFailure("api.busy", runCatching { bridge.serviceAction("still-busy", "stop") })
        bridge.pause()
        runCurrent()
        assertEquals(1, transport.cancelledWrites)
        assertEquals(8, transport.cancelledReads)
    }

    @Test fun cancellationBeforeServiceAdmissionPreventsHTTPAndOverflowFailsClosed() = runTest {
        var time = 0L
        val transport = FakeTransport()
        val bridge = bridge(transport) { time }
        bridge.cancel("late")
        assertFailure("api.cancelled", runCatching { bridge.serviceAction("late", "start") })
        repeat(129) { bridge.cancel("cancel$it") }
        assertFailure("api.cancelled", runCatching { bridge.serviceAction("fresh", "stop") })
        assertTrue(transport.paths.isEmpty())
        time = 60_001
        assertEquals(200, bridge.serviceAction("fresh", "stop").status)
    }

    @Test fun cancelledWriteKeepsAdmissionUntilCleanupAndDoesNotCancelReadsOrEvents() = runTest {
        val cleanup = CompletableDeferred<Unit>()
        val transport = FakeTransport().apply { holdWrites = true; writeCleanup = cleanup }
        val bridge = bridge(transport)
        val write = async { runCatching { bridge.serviceAction("write", "start") } }
        bridge.openEvents("events", listOf("status"))
        runCurrent()
        bridge.cancel("write")
        runCurrent()
        assertFalse(write.isCompleted)
        assertFailure("api.busy", runCatching { bridge.serviceAction("next", "stop") })
        assertFailure("api.request_exists", runCatching { bridge.read("write", "info") })
        assertEquals(200, bridge.read("read", "info").status)
        transport.events.send(ModuleEvent("status", envelope()))
        runCurrent()
        assertEquals("status", bridge.nextEvent("events").name)
        assertEquals(0, transport.watchStops)
        cleanup.complete(Unit)
        runCurrent()
        assertTrue(write.await().exceptionOrNull() is CancellationException)
        assertEquals(1, transport.cancelledWrites)
        transport.holdWrites = false
        assertEquals(200, bridge.serviceAction("write", "stop").status)
    }

    @Test fun serviceWritesFollowPauseDestroyAndWebViewCancellationWithoutReplay() = runTest {
        for (boundary in listOf("pause", "destroy", "webview")) {
            val transport = FakeTransport().apply { holdWrites = true }
            val bridge = bridge(transport)
            bridge.load(Any(), true)
            val completions = mutableListOf<Result<ReadResult>>()
            bridge.dispatch({ bridge.serviceAction("running", "stop") }, completions::add)
            runCurrent()
            when (boundary) {
                "pause" -> bridge.pause()
                "destroy" -> bridge.destroy()
                else -> bridge.load(Any(), true)
            }
            runCurrent()
            assertEquals(1, completions.size)
            assertTrue(completions.single().isFailure)
            assertEquals(1, transport.cancelledWrites)
            if (boundary != "webview") {
                assertFailure(if (boundary == "destroy") "api.destroyed" else "api.background",
                    runCatching { bridge.serviceAction("blocked", "start") })
            }
            bridge.resume()
            runCurrent()
            assertEquals(listOf("/service/stop"), transport.paths)
            bridge.destroy()
        }
    }

    @Test fun queuedOrCompletedWritesCannotPublishIntoAnotherDocumentGeneration() = runTest {
        for (completed in listOf(false, true)) {
            val transport = FakeTransport()
            val bridge = bridge(transport)
            val completions = mutableListOf<Result<ReadResult>>()
            bridge.dispatch({
                bridge.serviceAction("write", "start").also {
                    if (completed) { bridge.pause(); bridge.resume() }
                }
            }, completions::add)
            if (!completed) { bridge.pause(); bridge.resume() }
            runCurrent()
            assertEquals(1, completions.size)
            assertTrue(completions.single().isFailure)
            assertEquals(if (completed) 1 else 0, transport.paths.size)
            bridge.destroy()
        }
    }

    @Test fun businessErrorsPreserveHTTPStatusAndCodeButDropUntrustedDetails() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        for ((status, ok) in listOf(200 to false, 401 to false, 409 to false, 503 to true)) {
            transport.status = status
            transport.content = envelope(ok = ok, code = "service.unavailable", message = PRIVATE, data = """{"url":"$PRIVATE"}""")
            val result = bridge.read("one", "service")
            assertEquals(status, result.status)
            assertFalse(result.body.getValue("ok").jsonPrimitive.boolean)
            assertEquals("service.unavailable", result.body.getValue("code").jsonPrimitive.content)
            assertEquals(SAFE_FAILURE_MESSAGE, result.body.getValue("message").jsonPrimitive.content)
            assertEquals(JsonObject(emptyMap()), result.body["data"])
            assertFalse(result.body.toString().contains(PRIVATE))
        }
    }

    @Test fun unauthorizedWithoutDataPreservesTheExactGoHTTP401Envelope() = runTest {
        val transport = FakeTransport().apply {
            status = 401
            content = """{"schema":1,"ok":false,"code":"api.unauthorized","message":"需要有效的管理接口凭据"}"""
        }
        val result = bridge(transport).read("unauthorized", "info")
        assertEquals(401, result.status)
        assertFalse(result.body.getValue("ok").jsonPrimitive.boolean)
        assertEquals("api.unauthorized", result.body.getValue("code").jsonPrimitive.content)
        assertEquals(SAFE_FAILURE_MESSAGE, result.body.getValue("message").jsonPrimitive.content)
        assertEquals(JsonObject(emptyMap()), result.body["data"])
    }

    @Test fun errorEnvelopesAllowAbsentOrNullDataAndPreserveStatusAndCode() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        for ((status, code) in listOf(403 to "api.forbidden", 409 to "config.conflict", 503 to "api.unavailable")) {
            for (data in listOf("", ""","data":null""")) {
                transport.status = status
                transport.content = """{"schema":1,"ok":false,"code":"$code","message":"$PRIVATE"$data}"""
                val result = bridge.read("error", "service")
                assertEquals(status, result.status)
                assertFalse(result.body.getValue("ok").jsonPrimitive.boolean)
                assertEquals(code, result.body.getValue("code").jsonPrimitive.content)
                assertEquals(SAFE_FAILURE_MESSAGE, result.body.getValue("message").jsonPrimitive.content)
                assertEquals(JsonObject(emptyMap()), result.body["data"])
                assertFalse(result.body.toString().contains(PRIVATE))
            }
        }
    }

    @Test fun successfulReadsAndEventsStillRejectAbsentOrNullData() = runTest {
        for (data in listOf("", ""","data":null""")) {
            val transport = FakeTransport().apply {
                content = """{"schema":1,"ok":true,"code":"fixture.status","message":"safe"$data}"""
            }
            val bridge = bridge(transport)
            assertFailure("api.invalid_response", runCatching { bridge.read("missing", "info") })
            bridge.openEvents("missing", listOf("status"))
            transport.events.send(ModuleEvent("status", transport.content))
            runCurrent()
            assertFailure("api.invalid_response", runCatching { bridge.nextEvent("missing") })
            bridge.destroy()
        }
    }

    @Test fun codecRejectsUnsupportedSchemaAndMalformedHeaders() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        for ((content, code) in listOf(
            envelope().replace("\"schema\":1", "\"schema\":2") to "api.unsupported_schema",
            envelope().replace("\"schema\":1", "\"schema\":\"1\"") to "api.unsupported_schema",
            envelope().replace("\"schema\":1,", "") to "api.unsupported_schema",
            envelope().replace("\"ok\":true", "\"ok\":\"true\"") to "api.invalid_response",
            envelope().replace("\"message\":\"safe\"", "\"message\":false") to "api.invalid_response",
            envelope(code = "") to "api.invalid_response",
            envelope(code = "https://untrusted.invalid/token") to "api.invalid_response",
            "[]" to "api.invalid_response",
            "not JSON $PRIVATE" to "api.invalid_response",
        )) {
            transport.content = content
            assertFailure(code, runCatching { bridge.read("one", "info") })
        }
    }

    @Test fun transportExceptionsCannotLeakURLsSecretsOrOriginalMessages() = runTest {
        val transport = FakeTransport().apply { requestFailure = IOException(PRIVATE) }
        val bridge = bridge(transport)
        assertFailure("api.native_failed", runCatching { bridge.read("one", "info") })
        for (error in listOf(
            ModuleApiException("api.unauthorized", PRIVATE),
            ModuleApiException(PRIVATE, PRIVATE),
            IOException(PRIVATE), CancellationException(PRIVATE),
        )) {
            val failure = safeFailure(error)
            assertEquals(SAFE_FAILURE_MESSAGE, failure.message)
            assertNull(failure.cause)
            assertFalse(failure.code.contains(PRIVATE))
            assertTrue(failure.code.matches(Regex("api\\.[a-z_]+")))
        }
    }

    @Test fun explicitTransportProvisioningOptOutRemainsFailClosed() = runTest {
        val transport = LocalHttpTransport(ConnectionStore({ null }, { fail("Must not persist credentials") }), allowProvisioning = false)
        val bridge = bridge(transport)
        assertFailure("api.connection_unavailable", runCatching { bridge.read("one", "info") })
        val connect = runCatching { transport.connect() }.exceptionOrNull() as ModuleApiException
        assertEquals("api.connection_unavailable", connect.resultCode)
        val reconnect = runCatching { transport.reconnect("https://127.0.0.1:1") }.exceptionOrNull() as ModuleApiException
        assertEquals("api.connection_unavailable", reconnect.resultCode)
    }

    @Test fun cancelTargetsOnlyMatchingReadAndReleasesIDAfterCleanup() = runTest {
        val transport = FakeTransport().apply { holdReads = true }
        val bridge = bridge(transport)
        val first = async { runCatching { bridge.read("first", "info") } }
        val second = async { runCatching { bridge.read("second", "service") } }
        runCurrent()
        assertFailure("api.request_exists", runCatching { bridge.read("first", "info") })
        bridge.cancel("first")
        bridge.cancel("first")
        bridge.cancel("unknown")
        runCurrent()
        assertTrue(first.await().exceptionOrNull() is CancellationException)
        assertFalse(second.isCompleted)
        assertEquals(1, transport.cancelledReads)
        transport.holdReads = false
        assertEquals(200, bridge.read("first", "info").status)
        bridge.cancel("second")
        runCurrent()
        assertTrue(second.await().exceptionOrNull() is CancellationException)
        assertEquals(2, transport.cancelledReads)
    }

    @Test fun concurrentReadsAreBounded() = runTest {
        val transport = FakeTransport().apply { holdReads = true }
        val bridge = bridge(transport)
        val reads = (1..8).map { id -> async { runCatching { bridge.read("id$id", "info") } } }
        runCurrent()
        assertFailure("api.busy", runCatching { bridge.read("overflow", "info") })
        bridge.pause()
        runCurrent()
        reads.forEach { assertTrue(it.await().isFailure) }
        assertEquals(8, transport.cancelledReads)
    }

    @Test fun queuedInvokesCannotEnterTheNextForegroundOrWebViewSession() = runTest {
        for (replaceView in listOf(false, true)) {
            val transport = FakeTransport()
            val bridge = bridge(transport)
            val view = Any()
            bridge.load(view, true)
            val completions = mutableListOf<Result<ReadResult>>()
            bridge.dispatch({ bridge.read("queued", "info") }, completions::add)
            if (replaceView) bridge.load(Any(), true) else {
                bridge.pause()
                bridge.resume()
            }
            runCurrent()
            assertEquals(1, completions.size)
            assertTrue(completions.single().isFailure)
            assertTrue(transport.paths.isEmpty())
            assertTrue(bridge.visible)
            bridge.destroy()
        }
    }

    @Test fun dispatchCancellationSettlesOnceAndDeadCallbacksAreNotRetried() = runTest {
        val transport = FakeTransport().apply { holdReads = true }
        val bridge = bridge(transport)
        val completions = mutableListOf<Result<ReadResult>>()
        bridge.dispatch({ bridge.read("running", "info") }) {
            completions.add(it)
            throw IllegalStateException("Dead callback")
        }
        runCurrent()
        bridge.destroy()
        runCurrent()
        assertEquals(1, completions.size)
        assertTrue(completions.single().isFailure)
        assertEquals(1, transport.cancelledReads)
        bridge.dispatch({ error("Destroyed bridge must not dispatch") }) {
            completions.add(Result.failure(it.exceptionOrNull()!!))
            throw IllegalStateException("Dead callback")
        }
        assertEquals(2, completions.size)
        assertFailure("api.destroyed", completions.last())
    }

    @Test fun queuedInvokesAreBoundedBeforeTheyReachIO() = runTest {
        val bridge = bridge(FakeTransport())
        val completions = mutableListOf<Result<Unit>>()
        repeat(10) { bridge.dispatch({ awaitCancellation() }, completions::add) }
        bridge.dispatch({ fail("Overflow must not run") }, completions::add)
        assertFailure("api.busy", completions.single())
        bridge.pause()
        runCurrent()
        assertEquals(11, completions.size)
        assertTrue(completions.all { it.isFailure })
        bridge.resume()
        bridge.dispatch({ Unit }, completions::add)
        runCurrent()
        assertTrue(completions.last().isSuccess)
    }

    @Test fun completedReadCannotPublishAfterReentrantPauseAndResume() = runTest {
        val bridge = bridge(FakeTransport())
        val completions = mutableListOf<Result<ReadResult>>()
        bridge.dispatch({
            val result = bridge.read("one", "info")
            bridge.pause()
            bridge.resume()
            result
        }, completions::add)
        runCurrent()
        assertEquals(1, completions.size)
        assertTrue(completions.single().isFailure)
    }

    @Test fun recreatedActivityGetsFreshWorkAndLateOldCallbacksCannotStopIt() = runTest {
        val transport = FakeTransport().apply { holdReads = true }
        val oldOwner = Any()
        val newOwner = Any()
        val unrelated = Any()
        val lifecycle = ModuleApiLifecycle(oldOwner) { bridge(transport) }
        val oldBridge = lifecycle.bridge
        val read = async { runCatching { oldBridge.read("old", "info") } }
        oldBridge.openEvents("old", listOf("status"))
        val pull = async { runCatching { oldBridge.nextEvent("old") } }
        runCurrent()
        assertFalse(lifecycle.pause(unrelated))
        assertFalse(lifecycle.destroy(unrelated))
        assertFalse(lifecycle.resume(unrelated, replaceOwner = false))
        assertTrue(lifecycle.destroy(oldOwner))
        assertNull(lifecycle.owner)
        assertTrue(lifecycle.resume(newOwner, replaceOwner = true))
        val replacement = lifecycle.bridge
        assertNotSame(oldBridge, replacement)
        assertFalse(lifecycle.destroy(oldOwner))
        assertFalse(lifecycle.pause(oldOwner))
        assertFalse(lifecycle.resume(oldOwner, replaceOwner = false))
        runCurrent()
        assertTrue(read.await().isFailure)
        assertFailure("api.subscription_closed", pull.await())
        assertFalse(oldBridge.visible)
        assertTrue(replacement.visible)
        assertEquals(1, transport.watchStarts)
        assertEquals(1, transport.watchStops)
        transport.holdReads = false
        assertEquals(200, replacement.read("new", "info").status)
        assertTrue(lifecycle.destroy(newOwner))
        assertFalse(lifecycle.destroy(newOwner))
    }

    @Test fun replacementResumeBeforeOldDestroyStillRetiresTheOldScope() = runTest {
        val transport = FakeTransport()
        val oldOwner = Any()
        val newOwner = Any()
        val lifecycle = ModuleApiLifecycle(oldOwner) { bridge(transport) }
        val previous = lifecycle.bridge
        previous.openEvents("old", listOf("status"))
        runCurrent()
        assertTrue(lifecycle.resume(newOwner, replaceOwner = true))
        assertFalse(lifecycle.destroy(oldOwner))
        runCurrent()
        assertFalse(previous.visible)
        assertTrue(lifecycle.bridge.visible)
        assertEquals(1, transport.watchStarts)
        assertEquals(1, transport.watchStops)
        assertFailure("api.destroyed", runCatching { previous.read("old", "info") })
    }

    @Test fun newOwnerCanAttachBeforeResumeWithoutStartingAnyWork() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        val transport = FakeTransport()
        val oldOwner = Any()
        val newOwner = Any()
        val lifecycle = ModuleApiLifecycle(oldOwner) {
            ModuleApiBridge(transport, CoroutineScope(SupervisorJob() + dispatcher), workDispatcher = dispatcher)
                .also(bridges::add)
        }
        lifecycle.destroy(oldOwner)
        assertTrue(lifecycle.attach(newOwner, replaceOwner = true))
        lifecycle.bridge.load(Any(), false)
        val visible = mutableListOf<Boolean>()
        lifecycle.registerVisibility(1, visible::add)
        assertFalse(lifecycle.bridge.visible)
        lifecycle.resume(newOwner, replaceOwner = false)
        lifecycle.publishVisibility()
        assertEquals(listOf(true), visible)
        assertTrue(transport.paths.isEmpty())
        assertEquals(0, transport.watchStarts)
    }

    @Test fun newDocumentCanRegisterBeforeRetainedPluginReceivesResumeOrLoad() = runTest {
        val oldOwner = Any()
        val newOwner = Any()
        val lifecycle = ModuleApiLifecycle(oldOwner) { bridge(FakeTransport()) }
        val old = mutableListOf<Boolean>()
        val current = mutableListOf<Boolean>()
        lifecycle.registerVisibility(1, old::add)
        lifecycle.destroy(oldOwner)
        lifecycle.registerVisibility(2, current::add)
        lifecycle.resume(newOwner, replaceOwner = true)
        lifecycle.removeVisibility(1)
        lifecycle.publishVisibility()
        assertTrue(old.isEmpty())
        assertEquals(listOf(true), current)
    }

    @Test fun visibilitySlotReplacesOldDocumentsAndIgnoresTheirLateRemoval() = runTest {
        val owner = Any()
        val lifecycle = ModuleApiLifecycle(owner) { bridge(FakeTransport()) }
        val old = mutableListOf<Boolean>()
        val current = mutableListOf<Boolean>()
        lifecycle.registerVisibility(1, old::add)
        lifecycle.registerVisibility(2, current::add)
        lifecycle.removeVisibility(1)
        lifecycle.publishVisibility()
        lifecycle.pause(owner)
        lifecycle.publishVisibility()
        assertTrue(old.isEmpty())
        assertEquals(listOf(true, false), current)
        lifecycle.destroy(owner)
        lifecycle.publishVisibility()
        lifecycle.resume(Any(), replaceOwner = true)
        lifecycle.publishVisibility()
        assertEquals(2, current.size)
        lifecycle.registerVisibility(3) { throw IllegalStateException("Dead document") }
        lifecycle.publishVisibility()
        lifecycle.removeVisibility(3)
        lifecycle.publishVisibility()
    }

    @Test fun eventReplacementBoundsProducersStillFinishingCancellation() = runTest {
        val cleanup = CompletableDeferred<Unit>()
        val transport = FakeTransport().apply { watchCleanup = cleanup }
        val bridge = bridge(transport)
        bridge.openEvents("first", listOf("status"))
        runCurrent()
        bridge.openEvents("second", listOf("groups"))
        runCurrent()
        repeat(20) {
            assertFailure("api.busy", runCatching { bridge.openEvents("overflow$it", listOf("status")) })
        }
        assertEquals(2, transport.watchStarts)
        cleanup.complete(Unit)
        runCurrent()
        bridge.openEvents("third", listOf("status"))
        runCurrent()
        assertEquals(3, transport.watchStarts)
    }

    @Test fun cancelBeforeReadPreventsAnyHTTPWork() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        bridge.cancel("late")
        assertFailure("api.cancelled", runCatching { bridge.read("late", "info") })
        assertTrue(transport.paths.isEmpty())
        assertEquals(200, bridge.read("other", "info").status)
    }

    @Test fun cancellationHistoryIsBoundedAndOverflowFailsClosedForTheGraceWindow() = runTest {
        var time = 0L
        val transport = FakeTransport()
        val bridge = bridge(transport) { time }
        repeat(129) { bridge.cancel("id$it") }
        assertFailure("api.cancelled", runCatching { bridge.read("id0", "info") })
        assertFailure("api.cancelled", runCatching { bridge.read("id128", "info") })
        assertFailure("api.cancelled", runCatching { bridge.read("new", "info") })
        assertTrue(transport.paths.isEmpty())
        time = 60_001
        assertEquals(200, bridge.read("fresh", "info").status)
    }

    @Test fun singleSubscriptionAndSinglePendingPullAreEnforced() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        bridge.openEvents("first", listOf("status"))
        assertFailure("api.subscription_exists", runCatching { bridge.openEvents("first", listOf("status")) })
        val pull = async { runCatching { bridge.nextEvent("first") } }
        runCurrent()
        assertFailure("api.next_pending", runCatching { bridge.nextEvent("first") })
        bridge.closeEvents("first")
        runCurrent()
        assertFailure("api.subscription_closed", pull.await())
        assertEquals(1, transport.watchStarts)
        assertEquals(1, transport.watchStops)
    }

    @Test fun newDocumentReplacesOrphanedSubscriptionAndLateCloseIsHarmless() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        bridge.openEvents("old-document", listOf("status"))
        val oldPull = async { runCatching { bridge.nextEvent("old-document") } }
        runCurrent()
        bridge.openEvents("new-document", listOf("groups"))
        bridge.closeEvents("old-document")
        runCurrent()
        assertFailure("api.subscription_closed", oldPull.await())
        assertEquals(2, transport.watchStarts)
        assertEquals(1, transport.watchStops)
        val newPull = async { bridge.nextEvent("new-document") }
        runCurrent()
        transport.events.send(ModuleEvent("groups", envelope(data = POLICY)))
        runCurrent()
        assertEquals("groups", newPull.await().name)
    }

    @Test fun eventQueueHasCapacityOneAndBackpressuresWithoutDropping() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        bridge.openEvents("one", listOf("status"))
        repeat(3) { transport.events.send(ModuleEvent("status", envelope(data = """{"pid":$it}"""))) }
        runCurrent()
        assertEquals(1, transport.delivered)
        repeat(3) { index ->
            val result = bridge.nextEvent("one")
            assertEquals("status", result.name)
            assertEquals(Json.parseToJsonElement("""{"pid":$index}"""), result.body["data"])
            runCurrent()
            assertEquals((index + 2).coerceAtMost(3), transport.delivered)
        }
        bridge.closeEvents("one")
    }

    @Test fun closeBeforeProducerStartsAndLateCloseCannotAffectReplacement() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        bridge.openEvents("old", listOf("status"))
        bridge.closeEvents("old")
        bridge.closeEvents("old")
        runCurrent()
        assertEquals(0, transport.watchStarts)
        bridge.openEvents("new", listOf("groups"))
        bridge.closeEvents("old")
        val pull = async { bridge.nextEvent("new") }
        runCurrent()
        transport.events.send(ModuleEvent("groups", envelope(data = POLICY)))
        runCurrent()
        assertEquals("groups", pull.await().name)
        assertEquals(1, transport.watchStarts)
    }

    @Test fun closeBeforeOpenIsSettledByFrontendCompensatingClose() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        bridge.closeEvents("late")
        bridge.openEvents("late", listOf("status"))
        val pull = async { runCatching { bridge.nextEvent("late") } }
        runCurrent()
        bridge.closeEvents("late")
        runCurrent()
        assertFailure("api.subscription_closed", pull.await())
        assertEquals(1, transport.watchStarts)
        assertEquals(1, transport.watchStops)
    }

    @Test fun replacingWebViewCancelsOldWorkWithoutRestartingTheSubscription() = runTest {
        val transport = FakeTransport().apply { holdReads = true }
        val bridge = bridge(transport)
        val view = Any()
        bridge.load(view, true)
        val read = async { runCatching { bridge.read("old", "info") } }
        bridge.openEvents("events", listOf("status"))
        val pull = async { runCatching { bridge.nextEvent("events") } }
        runCurrent()
        bridge.load(view, true)
        runCurrent()
        assertFalse(read.isCompleted)
        assertFalse(pull.isCompleted)
        bridge.load(Any(), true)
        runCurrent()
        assertTrue(read.await().exceptionOrNull() is CancellationException)
        assertFailure("api.subscription_closed", pull.await())
        assertTrue(bridge.visible)
        assertEquals(1, transport.watchStarts)
        assertEquals(1, transport.watchStops)
        bridge.pause()
        bridge.load(Any(), false)
        assertFalse(bridge.visible)
    }

    @Test fun loadCannotOverrideExplicitResumeOrPauseWithAStaleLifecycleSnapshot() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        bridge.load(Any(), false)
        assertTrue(bridge.visible)
        assertEquals(200, bridge.read("resumed-before-load", "info").status)
        bridge.pause()
        bridge.load(Any(), true)
        assertFalse(bridge.visible)
        assertFailure("api.background", runCatching { bridge.read("paused-before-load", "info") })
        bridge.resume()
        bridge.load(Any(), false)
        assertTrue(bridge.visible)
    }

    @Test fun loadUsesInitialVisibilityOnlyBeforeAnyActivityCallback() = runTest {
        val dispatcher = StandardTestDispatcher(testScheduler)
        val bridge = ModuleApiBridge(FakeTransport(), CoroutineScope(SupervisorJob() + dispatcher), workDispatcher = dispatcher)
        bridges.add(bridge)
        bridge.load(Any(), false)
        assertFalse(bridge.visible)
        bridge.load(Any(), true)
        assertTrue(bridge.visible)
        bridge.pause()
        bridge.load(Any(), true)
        assertFalse(bridge.visible)
    }

    @Test fun invalidResponsesAndOversizedBodiesHaveExplicitErrorStatus() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        transport.content = "malformed"
        assertEquals(502, (runCatching { bridge.read("bad", "info") }.exceptionOrNull() as BridgeFailure).status)
        transport.content = envelope(data = """{"value":"${"x".repeat(4 * 1024 * 1024)}"}""")
        val failure = runCatching { bridge.read("large", "info") }
        assertFailure("api.body_too_large", failure)
        assertEquals(413, (failure.exceptionOrNull() as BridgeFailure).status)
        assertEquals(401, safeFailure(ModuleApiException("api.unauthorized", PRIVATE)).status)
        assertEquals(503, safeFailure(IOException(PRIVATE)).status)
    }

    @Test fun closingDiscardsQueuedEventsAndOldPendingPullCannotConsumeNewSession() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        bridge.openEvents("old", listOf("status"))
        val oldPull = async { runCatching { bridge.nextEvent("old") } }
        runCurrent()
        bridge.closeEvents("old")
        bridge.openEvents("new", listOf("status"))
        transport.events.send(ModuleEvent("status", envelope()))
        runCurrent()
        assertFailure("api.subscription_closed", oldPull.await())
        assertEquals("status", bridge.nextEvent("new").name)
        bridge.closeEvents("new")
        assertFailure("api.subscription_closed", runCatching { bridge.nextEvent("new") })
    }

    @Test fun eachFrameIsValidatedAndFailuresAreSanitizedWithoutReconnect() = runTest {
        for ((name, content, code) in listOf(
            Triple("status", envelope().replace("\"schema\":1", "\"schema\":7"), "api.unsupported_schema"),
            Triple("status", "{ $PRIVATE", "api.invalid_response"),
            Triple("groups", envelope(), "api.invalid_response"),
            Triple("status", envelope(ok = false, code = "api.events_failed", message = PRIVATE), "api.events_failed"),
        )) {
            val transport = FakeTransport()
            val bridge = bridge(transport)
            bridge.openEvents("one", listOf("status"))
            transport.events.send(ModuleEvent(name, content))
            runCurrent()
            assertFailure(code, runCatching { bridge.nextEvent("one") })
            assertEquals(1, transport.watchStarts)
            assertEquals(1, transport.watchStops)
            bridge.destroy()
        }
    }

    @Test fun statusTopicAllowsValidatedTrafficFrames() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        bridge.openEvents("one", listOf("status"))
        transport.events.send(ModuleEvent("traffic", envelope(message = PRIVATE)))
        runCurrent()
        val result = bridge.nextEvent("one")
        assertEquals("traffic", result.name)
        assertFalse(result.body.toString().contains(PRIVATE))
    }

    @Test fun invalidTopicsAndIDsNeverReachTransport() = runTest {
        val transport = FakeTransport()
        val bridge = bridge(transport)
        for (topics in listOf(emptyList(), listOf("traffic"), listOf("status", "status"), listOf("../events"))) {
            assertFailure("api.invalid_request", runCatching { bridge.openEvents("one", topics) })
        }
        for (id in listOf("", PRIVATE, "x".repeat(97))) {
            assertFailure("api.invalid_request", runCatching { bridge.read(id, "info") })
            assertFailure("api.invalid_request", runCatching { bridge.openEvents(id, listOf("status")) })
        }
        assertEquals(0, transport.watchStarts)
        assertTrue(transport.paths.isEmpty())
    }

    @Test fun pauseCancelsReadsAndPullAndResumeDoesNotRestartWork() = runTest {
        val transport = FakeTransport().apply { holdReads = true }
        val bridge = bridge(transport)
        val read = async { runCatching { bridge.read("read", "info") } }
        bridge.openEvents("events", listOf("status"))
        val pull = async { runCatching { bridge.nextEvent("events") } }
        runCurrent()
        bridge.pause()
        runCurrent()
        assertFalse(bridge.visible)
        assertTrue(read.await().exceptionOrNull() is CancellationException)
        assertFailure("api.subscription_closed", pull.await())
        assertEquals(1, transport.cancelledReads)
        assertEquals(1, transport.watchStops)
        assertFailure("api.background", runCatching { bridge.read("blocked", "info") })
        assertFailure("api.background", runCatching { bridge.openEvents("blocked", listOf("status")) })
        bridge.resume()
        runCurrent()
        assertTrue(bridge.visible)
        assertEquals(1, transport.watchStarts)
        assertFailure("api.subscription_closed", runCatching { bridge.nextEvent("events") })
        transport.holdReads = false
        assertEquals(200, bridge.read("read", "info").status)
    }

    @Test fun destructionIsIdempotentAndCannotBeUndoneByResume() = runTest {
        val bridge = bridge(FakeTransport())
        bridge.openEvents("one", listOf("status"))
        val pull = async { runCatching { bridge.nextEvent("one") } }
        runCurrent()
        bridge.destroy()
        bridge.destroy()
        bridge.resume()
        runCurrent()
        assertFalse(bridge.visible)
        assertFailure("api.subscription_closed", pull.await())
        assertFailure("api.destroyed", runCatching { bridge.read("read", "info") })
        assertFailure("api.destroyed", runCatching { bridge.openEvents("two", listOf("status")) })
    }

    @Test fun networkFailureWakesPendingPullWithSafeCodeAndDoesNotReconnect() = runTest {
        val transport = FakeTransport().apply { watchFailure = IOException(PRIVATE) }
        val bridge = bridge(transport)
        bridge.openEvents("one", listOf("status"))
        val pull = async { runCatching { bridge.nextEvent("one") } }
        runCurrent()
        assertFailure("api.native_failed", pull.await())
        assertEquals(1, transport.watchStarts)
    }

    private fun TestScope.bridge(transport: ModuleApiTransport, clock: () -> Long = { System.nanoTime() / 1_000_000 }): ModuleApiBridge = ModuleApiBridge(
        transport, CoroutineScope(SupervisorJob() + StandardTestDispatcher(testScheduler)), clock,
        StandardTestDispatcher(testScheduler),
    ).also { bridges.add(it); it.resume() }

    private fun assertFailure(code: String, result: Result<*>) {
        val failure = result.exceptionOrNull()
        assertTrue("Expected $code, got $failure", failure is BridgeFailure)
        failure as BridgeFailure
        assertEquals(code, failure.code)
        assertEquals(SAFE_FAILURE_MESSAGE, failure.message)
        assertNull(failure.cause)
    }

    private class FakeTransport : ModuleApiTransport {
        val paths = mutableListOf<String>()
        val methods = mutableListOf<String>()
        val bodies = mutableListOf<JsonObject?>()
        val events = Channel<ModuleEvent>(8)
        var content = envelope()
        var status = 200
        var holdReads = false
        var holdWrites = false
        var cancelledWrites = 0
        var writeCleanup: CompletableDeferred<Unit>? = null
        var cancelledReads = 0
        var delivered = 0
        var watchStarts = 0
        var watchStops = 0
        var requestFailure: Exception? = null
        var watchFailure: Exception? = null
        var watchCleanup: CompletableDeferred<Unit>? = null

        override suspend fun <T> request(method: String, path: String, body: JsonObject?, query: Map<String, String>, mutation: Boolean,
            decode: (Int, InputStream) -> T): T {
            assertTrue(query.isEmpty())
            if (path in setOf("/service/start", "/service/stop", "/service/restart", "/service/reload", "/service/check")) {
                assertEquals("POST", method)
                assertEquals(JsonObject(emptyMap()), body)
                assertTrue(mutation)
            } else if (path == "/mode" && mutation || path in setOf("/policy-groups/select", "/policy-groups/delay")) {
                assertEquals(if (path == "/mode") "PUT" else "POST", method)
                assertNotNull(body)
                assertTrue(mutation)
            } else {
                assertEquals("GET", method)
                assertNull(body)
                assertFalse(mutation)
            }
            paths.add(path)
            methods.add(method)
            bodies.add(body)
            requestFailure?.let { throw it }
            if (mutation && holdWrites) try { awaitCancellation() } finally {
                writeCleanup?.let { withContext(NonCancellable) { it.await() } }
                cancelledWrites++
            }
            if (!mutation && holdReads) try { awaitCancellation() } finally { cancelledReads++ }
            return decode(status, content.byteInputStream())
        }

        override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) {
            watchStarts++
            try {
                watchFailure?.let { throw it }
                for (event in events) {
                    consume(event)
                    delivered++
                }
            } finally {
                watchCleanup?.let { withContext(NonCancellable) { it.await() } }
                watchStops++
            }
        }

        override suspend fun reconnect(address: String): Unit = error("No native reconnection")
        override suspend fun image(path: String): ByteArray = error("Read-only")
        override suspend fun download(path: String, query: Map<String, String>, destination: File): Unit = error("Read-only")
    }

    companion object {
        private const val POLICY = """{"groups":[],"live":false,"core_started_at_millis":0,"default_outbound":"Proxy"}"""
        private fun policyRequest(id: String, select: Boolean) =
            """{"requestId":"$id","groupTag":"Proxy"${if (select) ",\"memberTag\":\"a\"" else ""}}"""
        private const val PRIVATE = "https://private.invalid/Bearer-private-secret"

        private fun envelope(ok: Boolean = true, code: String = "fixture.status", message: String = "safe", data: String = "{}") =
            """{"schema":1,"ok":$ok,"code":"$code","message":"$message","data":$data}"""
    }
}
