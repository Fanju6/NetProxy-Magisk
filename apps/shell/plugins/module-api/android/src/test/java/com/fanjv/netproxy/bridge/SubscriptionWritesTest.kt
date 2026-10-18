package com.fanjv.netproxy.bridge

import com.fanjv.netproxy.core.api.ModuleApiTransport
import com.fanjv.netproxy.core.api.ModuleEvent
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import kotlinx.serialization.json.*
import org.junit.After
import org.junit.Assert.*
import org.junit.Test
import java.io.File
import java.io.IOException
import java.io.InputStream

@OptIn(ExperimentalCoroutinesApi::class)
class SubscriptionWritesTest {
    private val bridges = mutableListOf<ModuleApiBridge>()
    @After fun close() = bridges.forEach { it.destroy() }

    @Test fun fixedMethodsPathsAndBodiesPreserveOmittedAndClearedFieldsWithoutReplacementQuery() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        for (value in validWrites()) bridge.subscriptionWrite(value.toString())
        assertEquals(listOf("POST", "PUT", "PUT", "DELETE", "POST"), transport.calls.map { it.method })
        assertEquals(listOf("/subscriptions", "/subscriptions/sub-a", "/subscriptions/sub-a", "/subscriptions/sub-a", "/subscriptions/update-all"), transport.calls.map { it.path })
        assertEquals(Json.parseToJsonElement("""{"url":"https://example.invalid/feed"}"""), transport.calls[0].body)
        assertEquals(JsonObject(emptyMap()), transport.calls[1].body)
        assertEquals(Json.parseToJsonElement("""{"user_agent":"","hwid":"","custom_headers":{},"include":"","exclude":"","auto_update":false}"""), transport.calls[2].body)
        assertTrue(transport.calls.all { it.query.isEmpty() })
        assertTrue(transport.calls.all { it.mutation })
    }

    @Test fun everyEnvelopeRejectsMissingUnknownNullWrongTypeAndUnrelatedArguments() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        for (value in validWrites()) {
            for (key in listOf("method", "path", "query", "body", "headers", "url", "private", "unknown")) {
                rejected(bridge, JsonObject(value + (key to JsonObject(emptyMap()))))
            }
            for (key in value.keys) {
                rejected(bridge, JsonObject(value + (key to JsonNull)))
                rejected(bridge, JsonObject(value - key))
            }
            rejected(bridge, JsonObject(value + ("requestId" to JsonPrimitive(1))))
            rejected(bridge, JsonObject(value + ("requestId" to JsonPrimitive("bad/id"))))
        }
        for (value in listOf(
            """{"action":"update_all","requestId":"r","groupId":"sub-a"}""",
            """{"action":"remove","requestId":"r","groupId":"sub-a","input":{}}""",
            """{"action":"add","requestId":"r","groupId":"sub-a","input":{"url":"https://example.invalid"}}""",
            """{"action":"edit","requestId":"r","groupId":"sub-a","input":{},"replacementGroupId":"default"}""",
            """{"action":"remove","requestId":"r","groupId":"sub-a","replacementGroupId":"sub-a"}""",
        )) rejected(bridge, Json.parseToJsonElement(value))
        for (group in listOf("", "staging", "a..b", "a/b", "a%2fb", "a?x=1", "_a")) {
            rejected(bridge, JsonObject(validWrites()[1] + ("groupId" to JsonPrimitive(group))))
            rejected(bridge, JsonObject(validWrites()[3] + ("replacementGroupId" to JsonPrimitive(group))))
        }
        for (action in listOf("update", "cancel", "delete", "update-all", "POST", "private")) {
            rejected(bridge, buildJsonObject { put("requestId", "r"); put("action", action) })
        }
        assertTrue(transport.calls.isEmpty())
    }

    @Test fun inputValidationMatchesFrontendAndNeverTreatsNullAsOmission() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        for (key in listOf("name", "url", "user_agent", "hwid", "custom_headers", "auto_update", "update_interval", "update_via_proxy", "include", "exclude", "allow_insecure", "download_timeout")) {
            rejected(bridge, write("edit", input = buildJsonObject { put(key, JsonNull) }))
        }
        for (input in listOf(
            """{"unknown":true}""", """{"name":" "}""", """{"name":1}""", """{"url":""}""",
            """{"url":"ftp://example.invalid"}""", """{"url":"https:///"}""", """{"url":"https://example.invalid/a b"}""",
            """{"user_agent":"a\nb"}""", """{"hwid":"a\tb"}""", """{"include":"a\rb"}""", """{"exclude":"a\u007fb"}""",
            """{"auto_update":"false"}""", """{"allow_insecure":1}""", """{"update_interval":899}""",
            """{"update_interval":"900"}""", """{"update_interval":900.5}""", """{"download_timeout":0}""",
            """{"download_timeout":-1}""", """{"download_timeout":9007199254740992}""", """{"update_via_proxy":"direct"}""",
            """{"custom_headers":[]}""", """{"custom_headers":{"":"value"}}""", """{"custom_headers":{"Bad Header":"value"}}""",
            """{"custom_headers":{"X-Test":"a\rb"}}""", """{"custom_headers":{"X-Test":null}}""", """{"custom_headers":{"X-Test":false}}""",
        )) rejected(bridge, write("edit", input = Json.parseToJsonElement(input) as JsonObject))
        rejected(bridge, write("add", input = JsonObject(emptyMap())))
        val complete = Json.parseToJsonElement("""{"name":"Feed","url":"HTTP://127.0.0.1/feed","user_agent":"","hwid":"","custom_headers":{"X-Token":"a\tb"},"auto_update":false,"update_interval":900,"update_via_proxy":"never","include":"","exclude":"","allow_insecure":true,"download_timeout":1}""") as JsonObject
        bridge.subscriptionWrite(write("add", input = complete).toString())
        assertEquals(complete, transport.calls.single().body)
        assertFailure("api.body_too_large", runCatching {
            bridge.subscriptionWrite(write("edit", input = buildJsonObject { put("user_agent", "x".repeat(4 * 1024 * 1024)) }).toString())
        })
        assertEquals(1, transport.calls.size)
    }

    @Test fun addEditAndRemoveResultsOnlyExposeValidatedPublicOutcomeFacts() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        val safe = Json.parseToJsonElement("""{"group_id":"sub-a","node_count":2,"revision":4,"name_changed":true,"requires_update":false,"persisted":true,"runtime_synced":false,"runtime_sync_state":"failed","runtime_sync_pending":true}""") as JsonObject
        for (action in listOf("add", "edit")) {
            for ((status, ok) in listOf(200 to true, 200 to false, 409 to false, 502 to false, 504 to false)) {
                transport.status = status
                transport.content = envelope(JsonObject(safe + ("cause" to JsonPrimitive(PRIVATE)) + ("url" to JsonPrimitive(PRIVATE))), ok)
                val result = bridge.subscriptionWrite(write(action).toString())
                assertEquals(status, result.status)
                assertEquals(ok, result.body.getValue("ok").jsonPrimitive.boolean)
                assertEquals(safe, result.body["data"])
                assertFalse(result.body.toString().contains(PRIVATE))
            }
        }
        transport.status = 200
        transport.content = envelope(Json.parseToJsonElement("""{"id":"sub-a","url":"$PRIVATE"}"""))
        assertEquals(Json.parseToJsonElement("""{"id":"sub-a"}"""), bridge.subscriptionWrite(write("remove").toString()).body["data"])
        transport.content = envelope(Json.parseToJsonElement("""{"group_id":"other"}"""))
        assertFailure("api.invalid_response", runCatching { bridge.subscriptionWrite(write("edit").toString()) })
        assertEquals(200, bridge.subscriptionWrite(write("add").toString()).status)
        transport.content = envelope(Json.parseToJsonElement("""{"group_id":"bad/id"}"""))
        assertFailure("api.invalid_response", runCatching { bridge.subscriptionWrite(write("add").toString()) })
    }

    @Test fun updateAllPartialFailuresKeepIndependentOutcomesAndFirstErrorFactsWithoutPrivateCauses() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        val original = Json.parseToJsonElement("""{"updated":["sub-a"],"failed":["sub-b"],"persisted":true,"first_error":{"group_id":"sub-b","persisted":true,"runtime_synced":false,"runtime_sync_state":"failed","runtime_sync_pending":true,"cause":"$PRIVATE","url":"$PRIVATE","headers":{"Authorization":"$PRIVATE"},"first_error":{"cause":"$PRIVATE"}},"cause":"$PRIVATE"}""")
        transport.status = 502
        transport.content = envelope(original, false)
        val result = bridge.subscriptionWrite(write("update_all").toString())
        assertEquals(502, result.status)
        assertEquals("subscription.runtime_sync_failed", result.body.getValue("code").jsonPrimitive.content)
        assertFalse(result.body.getValue("ok").jsonPrimitive.boolean)
        assertEquals(Json.parseToJsonElement("""{"updated":["sub-a"],"failed":["sub-b"],"persisted":true,"first_error":{"group_id":"sub-b","persisted":true,"runtime_synced":false,"runtime_sync_state":"failed","runtime_sync_pending":true}}"""), result.body["data"])
        assertFalse(result.body.toString().contains(PRIVATE))
        transport.status = 200
        transport.content = envelope(Json.parseToJsonElement("""{"updated":["sub-a"],"failed":[],"nearest":1}"""))
        assertEquals(Json.parseToJsonElement("""{"updated":["sub-a"],"failed":[]}"""), bridge.subscriptionWrite(write("update_all").toString()).body["data"])
        for (data in listOf("""{"updated":null}""", """{"failed":"sub-a"}""", """{"updated":[1]}""", """{"failed":["bad/id"]}""", """{"first_error":{"persisted":"true"}}""")) {
            transport.content = envelope(Json.parseToJsonElement(data), false)
            assertFailure("api.invalid_response", runCatching { bridge.subscriptionWrite(write("update_all").toString()) })
        }
    }

    @Test fun everySubscriptionWriteSharesBusinessAdmissionAndNeverBlocksIndependentCancel() = runTest {
        for (operation in validWrites()) {
            val transport = Transport().apply { gate = CompletableDeferred() }
            val bridge = bridge(transport)
            val active = async { runCatching { bridge.subscriptionWrite(operation.toString()) } }
            runCurrent()
            assertFailure("api.busy", runCatching { bridge.serviceAction("service", "start") })
            assertFailure("api.busy", runCatching { bridge.changeMode("mode", "Rule") })
            assertFailure("api.busy", runCatching { bridge.policySelect("""{"requestId":"node","groupTag":"Proxy","memberTag":"a"}""") })
            assertFailure("api.busy", runCatching { bridge.subscriptionAction("""{"requestId":"update","groupId":"sub-a","action":"update"}""") })
            assertFailure("api.busy", runCatching { bridge.subscriptionWrite(write("update_all", id = "other").toString()) })
            assertEquals(200, bridge.subscriptionAction("""{"requestId":"cancel","groupId":"sub-a","action":"cancel"}""").status)
            assertFalse(active.isCompleted)
            bridge.cancel("r")
            runCurrent()
            assertTrue(active.await().exceptionOrNull() is CancellationException)
            assertEquals(2, transport.calls.size)
        }
        val transport = Transport().apply { gate = CompletableDeferred() }
        val bridge = bridge(transport)
        val service = async { runCatching { bridge.serviceAction("service", "start") } }
        runCurrent()
        for (operation in validWrites()) assertFailure("api.busy", runCatching { bridge.subscriptionWrite(operation.toString()) })
        bridge.cancel("service")
        runCurrent()
        assertTrue(service.await().exceptionOrNull() is CancellationException)
    }

    @Test fun lifecycleAndTransportFailuresNeverReplayAcceptedStageBWrites() = runTest {
        for (action in listOf("add", "edit", "remove", "update_all")) {
            for (boundary in listOf("pause", "destroy", "webview")) {
                val transport = Transport().apply { gate = CompletableDeferred() }
                val bridge = bridge(transport)
                val oldView = Any()
                bridge.load(oldView, true)
                val results = mutableListOf<Result<ReadResult>>()
                bridge.dispatch({ bridge.subscriptionWrite(write(action).toString()) }, results::add)
                runCurrent()
                when (boundary) {
                    "pause" -> bridge.pause()
                    "destroy" -> bridge.destroy()
                    else -> bridge.load(Any(), true)
                }
                runCurrent()
                assertEquals(1, results.size)
                assertTrue(results.single().isFailure)
                bridge.resume()
                runCurrent()
                assertEquals(1, transport.calls.size)
            }
            val transport = Transport().apply { fail = true }
            val bridge = bridge(transport)
            assertFailure("api.native_failed", runCatching { bridge.subscriptionWrite(write(action).toString()) })
            bridge.pause(); bridge.resume(); runCurrent()
            assertEquals(1, transport.calls.size)
        }
    }

    private suspend fun rejected(bridge: ModuleApiBridge, value: JsonElement) =
        assertFailure("api.invalid_request", runCatching { bridge.subscriptionWrite(value.toString()) })

    private fun assertFailure(code: String, result: Result<*>) {
        val error = result.exceptionOrNull() as? BridgeFailure
        assertNotNull("Expected $code, got ${result.exceptionOrNull()}", error)
        assertEquals(code, error!!.code)
        assertEquals(SAFE_FAILURE_MESSAGE, error.message)
        assertNull(error.cause)
    }
    private fun TestScope.bridge(transport: ModuleApiTransport) = ModuleApiBridge(
        transport, CoroutineScope(SupervisorJob() + StandardTestDispatcher(testScheduler)),
        workDispatcher = StandardTestDispatcher(testScheduler),
    ).also { bridges.add(it); it.resume() }

    private data class Call(val method: String, val path: String, val body: JsonObject?, val query: Map<String, String>, val mutation: Boolean)
    private class Transport : ModuleApiTransport {
        val calls = mutableListOf<Call>()
        var content = envelope(JsonObject(emptyMap()))
        var status = 200
        var gate: CompletableDeferred<Unit>? = null
        var fail = false
        override suspend fun <T> request(method: String, path: String, body: JsonObject?, query: Map<String, String>, mutation: Boolean, decode: (Int, InputStream) -> T): T {
            calls.add(Call(method, path, body, query, mutation))
            if (fail) throw IOException(PRIVATE)
            if (!path.endsWith("/cancel")) gate?.await()
            return decode(status, content.byteInputStream())
        }
        override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) = awaitCancellation()
        override suspend fun reconnect(address: String): Unit = error("No reconnect")
        override suspend fun image(path: String): ByteArray = error("No images")
        override suspend fun download(path: String, query: Map<String, String>, destination: File): Unit = error("No downloads")
    }

    companion object {
        private const val PRIVATE = "https://private.invalid/input-secret"
        private fun envelope(data: JsonElement, ok: Boolean = true) =
            """{"schema":1,"ok":$ok,"code":"subscription.runtime_sync_failed","message":"$PRIVATE","data":$data}"""
        private fun write(action: String, input: JsonObject? = null, id: String = "r") = buildJsonObject {
            put("action", action); put("requestId", id)
            if (action in setOf("edit", "remove")) put("groupId", "sub-a")
            if (action in setOf("add", "edit")) put("input", input ?: if (action == "add") buildJsonObject { put("url", "https://example.invalid/feed") } else JsonObject(emptyMap()))
        }
        private fun validWrites() = listOf(write("add"), write("edit"), write("edit", input = Json.parseToJsonElement("""{"user_agent":"","hwid":"","custom_headers":{},"include":"","exclude":"","auto_update":false}""") as JsonObject),
            write("remove"), write("update_all"))
    }
}
