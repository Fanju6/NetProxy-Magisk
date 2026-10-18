package com.fanjv.netproxy.bridge

import com.fanjv.netproxy.core.api.ModuleApiTransport
import com.fanjv.netproxy.core.api.ModuleEvent
import kotlinx.coroutines.*
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.test.*
import kotlinx.serialization.json.*
import org.junit.After
import org.junit.Assert.*
import org.junit.Test
import java.io.File
import java.io.IOException
import java.io.InputStream

@OptIn(ExperimentalCoroutinesApi::class)
class PolicyGroupsTest {
    private val bridges = mutableListOf<ModuleApiBridge>()
    @After fun close() = bridges.forEach { it.destroy() }

    @Test fun fixedReadAndPostsShareTheSamePublicSnapshotAsGroupsEvents() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        val expected = PolicyGroups.snapshot(snapshot)
        assertEquals(expected, bridge.read("read", "policy_groups").body["data"])
        for (tag in listOf("Proxy", "\u9999\u6e2f / \uD83D\uDE80", " name ", "../path?x=1")) {
            assertEquals(expected, bridge.policySelect(request(true, tag).toString()).body["data"])
            assertEquals(Call("POST", "/policy-groups/select", buildJsonObject {
                put("group_tag", tag); put("member_tag", tag)
            }, emptyMap(), true), transport.calls.last())
            assertEquals(expected, bridge.policyDelay(request(false, tag).toString()).body["data"])
            assertEquals(Call("POST", "/policy-groups/delay", buildJsonObject { put("group_tag", tag) }, emptyMap(), true), transport.calls.last())
        }
        assertEquals(Call("GET", "/policy-groups", null, emptyMap(), false), transport.calls.first())
        bridge.openEvents("events", listOf("groups"))
        transport.events.send(ModuleEvent("groups", envelope(snapshot)))
        runCurrent()
        val result = bridge.nextEvent("events")
        assertEquals(expected, result.body["data"])
        assertFalse(result.body.toString().contains(PRIVATE))
        assertEquals(setOf("groups", "live", "core_started_at_millis", "default_outbound"), expected.keys)
        val group = expected.getValue("groups").jsonArray.single().jsonObject
        assertEquals(setOf("tag", "type", "selectable", "expanded", "selected", "items"), group.keys)
        assertEquals(setOf("tag", "type", "url_test_time", "url_test_delay"), group.getValue("items").jsonArray.single().jsonObject.keys)
    }

    @Test fun kotlinIndependentlyRejectsUnknownMissingNullWrongTypeAndInvalidUnicodeTags() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        for (select in listOf(false, true)) {
            val value = request(select)
            for (key in listOf("method", "path", "url", "headers", "query", "body", "target", "group_id", "node_tag")) {
                rejected(bridge, select, JsonObject(value + (key to JsonPrimitive(PRIVATE))).toString())
            }
            if (!select) rejected(bridge, select, JsonObject(value + ("memberTag" to JsonPrimitive("a"))).toString())
            for (key in value.keys) {
                rejected(bridge, select, JsonObject(value - key).toString())
                for (item in listOf(JsonNull, JsonPrimitive(1), JsonPrimitive(false), JsonObject(emptyMap()), JsonArray(emptyList()))) {
                    rejected(bridge, select, JsonObject(value + (key to item)).toString())
                }
            }
            for (id in listOf("", "bad/id", "x".repeat(97))) {
                rejected(bridge, select, JsonObject(value + ("requestId" to JsonPrimitive(id))).toString())
            }
            for (key in if (select) listOf("groupTag", "memberTag") else listOf("groupTag")) {
                for (tag in listOf("", " ", "\u3000", "\u0000", "a\n", "\u007f", "\u0085", "\uD800", "\uDC00")) {
                    rejected(bridge, select, JsonObject(value + (key to JsonPrimitive(tag))).toString())
                }
            }
            for (raw in listOf("null", "[]", "malformed", """{"requestId":"r","groupTag":"\ud800"${if (select) ",\"memberTag\":\"a\"" else ""}}""")) {
                rejected(bridge, select, raw)
            }
            val oversized = request(select, "\u9f99".repeat(1_400_000)).toString()
            assertFailure("api.body_too_large", runCatching { execute(bridge, select, oversized) })
        }
        assertTrue(transport.calls.isEmpty())
    }

    @Test fun malformedSnapshotsFailClosedForReadPostAndSSEWhileStoppedSnapshotsAreValid() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        val stopped = Json.parseToJsonElement("""{"groups":[],"live":false,"core_started_at_millis":0,"default_outbound":"Proxy"}""")
        transport.content = envelope(stopped)
        assertEquals(stopped, bridge.read("stopped", "policy_groups").body["data"])
        val bad = mutableListOf<JsonElement>(JsonNull, JsonArray(emptyList()), JsonObject(emptyMap()))
        for (key in listOf("groups", "live", "core_started_at_millis", "default_outbound")) {
            bad.add(JsonObject(snapshot.jsonObject - key))
            bad.add(JsonObject(snapshot.jsonObject + (key to JsonNull)))
        }
        for ((key, value) in listOf("live" to JsonPrimitive("true"), "core_started_at_millis" to JsonPrimitive(-1), "core_started_at_millis" to JsonPrimitive(0),
            "core_started_at_millis" to JsonPrimitive(0.5), "default_outbound" to JsonPrimitive(1))) {
            bad.add(JsonObject(snapshot.jsonObject + (key to value)))
        }
        val group = snapshot.jsonObject.getValue("groups").jsonArray.single().jsonObject
        for (key in listOf("tag", "type", "selectable", "expanded", "items")) {
            bad.add(JsonObject(snapshot.jsonObject + ("groups" to JsonArray(listOf(JsonObject(group - key))))))
        }
        for (value in bad) {
            transport.content = envelope(value)
            assertFailure("api.invalid_response", runCatching { bridge.read("bad", "policy_groups") })
            for (select in listOf(false, true)) {
                assertFailure("api.invalid_response", runCatching { execute(bridge, select, request(select).toString()) })
            }
            bridge.openEvents("bad", listOf("groups"))
            transport.events.send(ModuleEvent("groups", envelope(value)))
            runCurrent()
            assertFailure("api.invalid_response", runCatching { bridge.nextEvent("bad") })
            bridge.closeEvents("bad")
            runCurrent()
        }
    }

    @Test fun policyErrorsPreserveOnlyBusinessCodeStatusAndBooleanPersistedAndNeverReplay() = runTest {
        for (select in listOf(false, true)) {
            val transport = Transport()
            val bridge = bridge(transport)
            transport.status = 409
            transport.content = envelope(Json.parseToJsonElement("""{"accepted":true,"persisted":true,"runtime_synced":false,"core_started_at_millis":1234,"secret":"$PRIVATE","cause":"$PRIVATE","path":"$PRIVATE"}"""), false)
            val result = execute(bridge, select, request(select).toString())
            assertEquals(409, result.status)
            assertEquals("policy.fixture", result.body.getValue("code").jsonPrimitive.content)
            assertEquals(Json.parseToJsonElement("""{"accepted":true,"persisted":true,"runtime_synced":false,"core_started_at_millis":1234}"""), result.body["data"])
            assertFalse(result.body.toString().contains(PRIVATE))
            transport.fail = true
            assertFailure("api.native_failed", runCatching { execute(bridge, select, request(select).toString()) })
            bridge.pause(); bridge.resume(); runCurrent()
            assertEquals(2, transport.calls.size)
            transport.fail = false
            transport.gate = CompletableDeferred()
            val pending = async { runCatching { execute(bridge, select, request(select).toString()) } }
            runCurrent()
            assertFailure("api.busy", runCatching { bridge.serviceAction("service", "start") })
            assertFailure("api.busy", runCatching { bridge.management("""{"requestId":"management","operation":"logs_clear"}""") })
            bridge.pause(); bridge.resume(); runCurrent()
            assertTrue(pending.await().isFailure)
            assertEquals(3, transport.calls.size)
        }
    }

    @Test fun failureProjectionOnlyRetainsTypedConfirmationFacts() {
        for (item in listOf(JsonNull, JsonPrimitive("true"), JsonPrimitive(1), JsonObject(emptyMap()), JsonArray(emptyList()))) {
            val data = buildJsonObject {
                put("accepted", item); put("persisted", item); put("runtime_synced", item)
                put("cause", PRIVATE); put("path", PRIVATE)
            }
            assertEquals(JsonObject(emptyMap()), PolicyGroups.failure(data))
        }
        for (value in listOf(JsonPrimitive(-1), JsonPrimitive(0.5), JsonPrimitive("1"), JsonPrimitive(true), JsonNull)) {
            assertEquals(JsonObject(emptyMap()), PolicyGroups.failure(buildJsonObject { put("core_started_at_millis", value) }))
        }
        assertEquals(Json.parseToJsonElement("""{"accepted":false,"persisted":false,"runtime_synced":true,"core_started_at_millis":0}"""),
            PolicyGroups.failure(Json.parseToJsonElement("""{"accepted":false,"persisted":false,"runtime_synced":true,"core_started_at_millis":0,"cause":"$PRIVATE"}""")))
    }

    @Test fun serviceReadActionsAndStatusEventsKeepDefaultOutboundWithoutLegacySelectionOrPrivateFields() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        for (selected in listOf("", "known-leaf")) {
            val expected = Json.parseToJsonElement("""{"state":"ready","configured_backend":"tun","active_backend":"tun","pid":42,"default_outbound":"Proxy","runtime_selected":"$selected","outbound_mode":"Rule","configured_outbound_mode":"Rule","available_outbound_modes":["Rule","Direct"],"error":""}""")
            val value = JsonObject(expected.jsonObject + listOf("selector_mode", "active_group_id", "active_group_name", "active_group_runtime_tag", "active_group_node_count", "selected_node_ref", "secret")
                .associateWith { JsonPrimitive(PRIVATE) })
            transport.content = envelope(value)
            assertEquals(expected, bridge.read("status", "service").body["data"])
            for (action in listOf("start", "stop", "restart", "reload", "check")) {
                transport.content = envelope(buildJsonObject { put("action", action); put("status", value); put("secret", PRIVATE) })
                assertEquals(buildJsonObject { put("action", action); put("status", expected) }, bridge.serviceAction("action", action).body["data"])
            }
            bridge.openEvents("status", listOf("status"))
            transport.events.send(ModuleEvent("status", envelope(value)))
            runCurrent()
            assertEquals(expected, bridge.nextEvent("status").body["data"])
            bridge.closeEvents("status")
            runCurrent()
        }
        transport.content = envelope(Json.parseToJsonElement("""{"default_outbound":1}"""))
        assertFailure("api.invalid_response", runCatching { bridge.read("bad", "service") })
        transport.content = envelope(Json.parseToJsonElement("""{"error":"$PRIVATE"}"""))
        assertFalse(bridge.read("error", "service").body.toString().contains(PRIVATE))
    }

    @Test fun policyRequestsAcceptExactlyFourMiBAndRejectTheNextUtf8Byte() {
        val limit = 4 * 1024 * 1024
        for (select in listOf(false, true)) {
            val empty = buildJsonObject {
                put("requestId", "r"); put("groupTag", "")
                if (select) put("memberTag", "a")
            }
            val prefix = empty.toString().toByteArray(Charsets.UTF_8).size
            val exact = JsonObject(empty + ("groupTag" to JsonPrimitive("x".repeat(limit - prefix)))).toString()
            assertEquals(limit, exact.toByteArray(Charsets.UTF_8).size)
            if (select) PolicyGroups.select(exact) else PolicyGroups.delay(exact)
            val excessive = JsonObject(empty + ("groupTag" to JsonPrimitive("x".repeat(limit - prefix + 1)))).toString()
            assertFailure("api.body_too_large", runCatching {
                if (select) PolicyGroups.select(excessive) else PolicyGroups.delay(excessive)
            })
        }
    }

    private suspend fun rejected(bridge: ModuleApiBridge, select: Boolean, raw: String) =
        assertFailure("api.invalid_request", runCatching { execute(bridge, select, raw) })
    private suspend fun execute(bridge: ModuleApiBridge, select: Boolean, raw: String) =
        if (select) bridge.policySelect(raw) else bridge.policyDelay(raw)
    private fun assertFailure(code: String, result: Result<*>) {
        val failure = result.exceptionOrNull() as? BridgeFailure
        assertNotNull("Expected $code, got ${result.exceptionOrNull()}", failure)
        assertEquals(code, failure!!.code)
        assertNull(failure.cause)
    }
    private fun TestScope.bridge(transport: ModuleApiTransport) = ModuleApiBridge(
        transport, CoroutineScope(SupervisorJob() + StandardTestDispatcher(testScheduler)),
        workDispatcher = StandardTestDispatcher(testScheduler),
    ).also { bridges.add(it); it.resume() }
    private data class Call(val method: String, val path: String, val body: JsonObject?, val query: Map<String, String>, val mutation: Boolean)
    private class Transport : ModuleApiTransport {
        val calls = mutableListOf<Call>()
        val events = Channel<ModuleEvent>(1)
        var content = envelope(snapshot)
        var status = 200
        var fail = false
        var gate: CompletableDeferred<Unit>? = null
        override suspend fun <T> request(method: String, path: String, body: JsonObject?, query: Map<String, String>, mutation: Boolean, decode: (Int, InputStream) -> T): T {
            calls.add(Call(method, path, body, query, mutation))
            if (fail) throw IOException(PRIVATE)
            gate?.await()
            return decode(status, content.byteInputStream())
        }
        override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) {
            for (event in events) consume(event)
        }
        override suspend fun reconnect(address: String): Unit = error("No reconnect")
        override suspend fun image(path: String): ByteArray = error("No image")
        override suspend fun download(path: String, query: Map<String, String>, destination: File): Unit = error("No download")
    }
    companion object {
        private const val PRIVATE = "private-policy-marker"
        private val snapshot = Json.parseToJsonElement("""{"groups":[{"tag":"Proxy","type":"selector","selectable":true,"expanded":false,"selected":"a","items":[{"tag":"a","type":"vless","url_test_time":1234,"url_test_delay":42,"password":"$PRIVATE"}],"url":"$PRIVATE"}],"live":true,"core_started_at_millis":1234,"default_outbound":"Proxy","selection":{"secret":"$PRIVATE"},"runtime_groups":"$PRIVATE","secret":"$PRIVATE"}""")
        private fun request(select: Boolean, tag: String = "Proxy") = buildJsonObject {
            put("requestId", "r"); put("groupTag", tag)
            if (select) put("memberTag", tag)
        }
        private fun envelope(data: JsonElement, ok: Boolean = true) =
            """{"schema":1,"ok":$ok,"code":"policy.fixture","message":"$PRIVATE","data":$data}"""
    }
}
