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
import java.io.InputStream
import java.util.Base64

@OptIn(ExperimentalCoroutinesApi::class)
class ManagementTest {
    private val bridges = mutableListOf<ModuleApiBridge>()
    @After fun close() = bridges.forEach { it.destroy() }

    @Test fun allOperationsHaveOnlyFixedMethodsPathsAndPerOperationFields() {
        for ((operation, input, method, path) in cases()) {
            val request = Management.parse(args(operation, input))
            val route = Management.route(request)
            assertEquals(method, route.method); assertEquals(path, route.path)
            for (extra in listOf("method", "path", "url", "query", "headers", "unknown")) {
                invalid(args(operation, JsonObject(input + (extra to JsonPrimitive("injected")))))
            }
            for (key in input.keys) invalid(args(operation, JsonObject(input + (key to JsonNull))))
        }
        for (operation in listOf("diagnostics", "apps", "wifi", "config_list")) {
            Management.parse(args(operation))
            invalid(args(operation).dropLast(1) + ",\"input\":null}")
        }
        for (raw in listOf(args("GET"), args("apps").dropLast(1) + ",\"method\":\"GET\"}", args("apps").replace("\"r\"", "null"))) invalid(raw)
        for (target in listOf("api", "telemetry", "api.json", "module/selection", "/module", "singbox/../api.json", "singbox/rules/local/a%2fb.json", "runtime/unknown.json")) {
            invalid(args("config_read", obj("""{"target":"$target"}""")))
        }
        for (target in listOf("module", "module/wifi", "module/auto_start", "inbound/root_policy", "singbox/dns", "singbox/rules/local/user-1.json", "runtime/inbound.json", "runtime/providers.json")) assertTrue(Management.target(target))
        for (target in listOf("runtime/inbound.json", "runtime/providers.json")) {
            assertFalse(Management.target(target, true))
            invalid(args("config_apply", buildJsonObject { put("target", target); put("revision", "a".repeat(64)); put("content", "{}") }))
        }
        for ((operation, input) in listOf("logs" to obj("""{"lines":0}"""), "logs" to obj("""{"lines":2001}"""), "logs" to obj("""{"lines":"10"}"""), "logs_clear" to obj("""{"kind":"all"}"""), "ebpf" to obj("""{"mode":"shell"}"""), "ebpf" to obj("""{"raw":"true"}"""), "node_read" to obj("""{"group_id":"a/b","node_tag":"x"}"""))) invalid(args(operation, input))
        assertEquals("api.body_too_large", runCatching { Management.parse(args("node_import", buildJsonObject { put("content", "x".repeat(4*1024*1024)) })) }.exceptionOrNull().let { (it as BridgeFailure).code })
    }

    @Test fun configurationReadsAreExplicitAndErrorsNeverEchoPrivateContent() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        transport.content = envelope(obj("""{"target":"module/wifi","revision":"${"a".repeat(64)}","content":"{private content}","url":"$PRIVATE"}"""))
        val result = bridge.management(args("config_read", obj("""{"target":"module/wifi"}""")))
        assertEquals("{private content}", result.body["data"]!!.jsonObject["content"]!!.jsonPrimitive.content)
        assertFalse(result.body.toString().contains(PRIVATE))
        assertEquals(mapOf("target" to "module/wifi"), transport.queries.single())
        transport.status = 409
        transport.content = envelope(obj("""{"persisted":true,"runtime_synced":false,"actual_revision":"${"b".repeat(64)}","cause":"$PRIVATE","content":"$PRIVATE","diagnostics":[{"code":"config.invalid","index":1,"source":"$PRIVATE","message":"$PRIVATE"}]}"""), false)
        val error = bridge.management(args("config_apply", obj("""{"target":"module/wifi","revision":"${"a".repeat(64)}","content":"{}"}""")))
        assertEquals(409, error.status)
        assertTrue(error.body["data"]!!.jsonObject["persisted"]!!.jsonPrimitive.boolean)
        assertFalse(error.body.toString().contains(PRIVATE))
    }

    @Test fun managementRejectsMalformedUnicodeBeforeTransportAndEnforcesTheUtf8BodyLimit() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        for (text in listOf("\uD800", "\uDC00")) {
            for ((operation, input) in listOf(
                "node_read" to buildJsonObject { put("group_id", "default"); put("node_tag", text) },
                "node_add" to buildJsonObject { put("content", text) },
                "catalog_create" to buildJsonObject { put("id", "local-1"); put("name", text) },
            )) {
                for (raw in listOf(args(operation, input), args(operation, input).replace(text, if (text == "\uD800") "\\ud800" else "\\udc00"))) {
                    failure("api.invalid_request", runCatching { bridge.management(raw) })
                }
            }
        }
        assertTrue(transport.paths.isEmpty())
        val limit = 4 * 1024 * 1024
        val overhead = args("node_import", buildJsonObject { put("content", "") }).encodeToByteArray().size
        for (extra in 0..1) {
            val raw = args("node_import", buildJsonObject { put("content", "x".repeat(limit - overhead + extra)) })
            assertEquals(limit + extra, raw.encodeToByteArray().size)
            if (extra == 0) Management.parse(raw) else {
                val error = runCatching { bridge.management(raw) }.exceptionOrNull() as? BridgeFailure
                assertNotNull(error)
                assertEquals("api.body_too_large", error!!.code)
                assertEquals(413, error.status)
            }
        }
        assertTrue(transport.paths.isEmpty())
    }

    @Test fun configListFiltersForbiddenFilesAndAppsAreQueriedOnceForAllUsers() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        transport.content = envelope(Json.parseToJsonElement("""[{"id":"module","filename":"module.json","category":"config","editable":true},{"id":"api","filename":"api.json","category":"config","editable":true}]"""))
        assertEquals(1, bridge.management(args("config_list")).body["data"]!!.jsonArray.size)
        transport.content = envelope(obj("""{"users":[{"id":0,"name":"Owner","revision":"r","secret":"$PRIVATE","apps":[{"package_name":"test.app","uid":10001,"label":"Test","system":false,"version_code":1,"last_update_time":1,"icon_key":"","url":"$PRIVATE"}]},{"id":10,"name":"Work","revision":"r","apps":[]}]}"""))
        val apps = bridge.management(args("apps"))
        assertEquals(2, apps.body["data"]!!.jsonObject["users"]!!.jsonArray.size)
        assertEquals(listOf("/configs", "/android/apps"), transport.paths)
        assertFalse(apps.body.toString().contains(PRIVATE))
    }

    @Test fun iconIsFixedPNGOnlyBoundedAndUsesTheSameCancellationRegistry() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        val input = buildJsonObject { put("icon_key", "a".repeat(64)) }
        val result = bridge.management(args("icon", input))
        assertEquals("/android/icons/" + "a".repeat(64), transport.paths.single())
        assertArrayEquals(transport.image, Base64.getDecoder().decode(result.body["data"]!!.jsonObject["content"]!!.jsonPrimitive.content))
        transport.image = ByteArray(256 * 1024 + 1)
        failure("api.body_too_large", runCatching { bridge.management(args("icon", input)) })
        transport.image = byteArrayOf(1, 2)
        failure("api.invalid_response", runCatching { bridge.management(args("icon", input)) })
        transport.hold = true
        val pending = async { runCatching { bridge.management(args("icon", input)) } }
        runCurrent(); bridge.pause(); runCurrent()
        assertTrue(pending.await().exceptionOrNull() is CancellationException)
    }

    @Test fun managementWritesCoordinateWithServiceRoutingAndSubscriptionWrites() = runTest {
        for (operation in listOf("config_apply", "config_validate", "logs_clear", "ebpf", "node_add", "node_edit", "node_remove", "node_import", "catalog_create", "catalog_rename", "catalog_remove")) {
            val transport = Transport().apply { hold = true }
            val bridge = bridge(transport)
            val input = cases().first { it.operation == operation }.input
            val pending = async { runCatching { bridge.management(args(operation, input)) } }
            runCurrent()
            failure("api.busy", runCatching { bridge.serviceAction("service", "restart") })
            failure("api.busy", runCatching { bridge.changeMode("mode", "Rule") })
            failure("api.busy", runCatching { bridge.policySelect("""{"requestId":"policy","groupTag":"Proxy","memberTag":"a"}""") })
            failure("api.busy", runCatching { bridge.policyDelay("""{"requestId":"delay","groupTag":"Proxy"}""") })
            failure("api.busy", runCatching { bridge.subscriptionAction("""{"requestId":"sub","groupId":"default","action":"update"}""") })
            bridge.pause(); runCurrent(); bridge.resume(); runCurrent()
            assertTrue(pending.await().exceptionOrNull() is CancellationException)
            assertEquals(1, transport.paths.size)
        }
    }

    @Test fun catalogOperationsValidateIdentityNameAndFixedRoutesWithoutAdditionalPermissions() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        for (operation in listOf("catalog_create", "catalog_rename", "catalog_remove")) {
            val input = buildJsonObject { put("id", "local-1"); if (operation != "catalog_remove") put("name", " Local \u9999\u6e2f ") }
            val request = Management.parse(args(operation, input))
            val route = Management.route(request)
            assertTrue(route.mutation)
            assertTrue(route.query.isEmpty())
            assertEquals(when (operation) { "catalog_create" -> input; "catalog_rename" -> JsonObject(input - "id"); else -> JsonObject(emptyMap()) }, route.body)
            transport.content = envelope(obj("""{"id":"local-1","name":"$PRIVATE","url":"$PRIVATE","headers":{"Authorization":"$PRIVATE"},"active":true}"""))
            assertEquals(obj("""{"id":"local-1"}"""), bridge.management(args(operation, input)).body["data"])
            for (id in listOf("", "staging", "a..b", "a/b", "a%2fb", "a?x=1", "_a", "\u4e2d\u6587")) invalid(args(operation, JsonObject(input + ("id" to JsonPrimitive(id)))))
            for (key in input.keys) invalid(args(operation, JsonObject(input - key)))
            for (extra in listOf("method", "path", "query", "url", "headers", "type", "provider_tag")) invalid(args(operation, JsonObject(input + (extra to JsonPrimitive(PRIVATE)))))
            for (bad in listOf(JsonNull, JsonPrimitive(1), JsonPrimitive(false), JsonArray(emptyList()), JsonObject(emptyMap()))) {
                invalid(args(operation, JsonObject(input + ("id" to bad))))
                if (operation != "catalog_remove") invalid(args(operation, JsonObject(input + ("name" to bad))))
            }
            transport.content = envelope(obj("""{"id":"other"}"""))
            failure("api.invalid_response", runCatching { bridge.management(args(operation, input)) })
            transport.status = 409
            transport.content = envelope(obj("""{"accepted":true,"persisted":true,"runtime_synced":false,"core_started_at_millis":1234,"url":"$PRIVATE","cause":"$PRIVATE","path":"$PRIVATE","id":"other"}"""), false)
            val result = bridge.management(args(operation, input))
            assertEquals(409, result.status)
            assertEquals(obj("""{"accepted":true,"persisted":true,"runtime_synced":false,"core_started_at_millis":1234}"""), result.body["data"])
            assertFalse(result.body.toString().contains(PRIVATE))
            transport.status = 200
        }
        invalid(args("catalog_remove", obj("""{"id":"default"}""")))
        invalid(args("catalog_remove", obj("""{"id":"local-1","name":"unexpected"}""")))
        for (operation in listOf("catalog_create", "catalog_rename")) {
            for (name in listOf("", " ", "a\n", "\u007f", "\u0085", "x".repeat(257), "\u9f99".repeat(86), "\uD800", "\uDC00")) {
                invalid(args(operation, buildJsonObject { put("id", "local-1"); put("name", name) }))
            }
            for (name in listOf("x".repeat(256), "\u9f99".repeat(85) + "a", " Local ")) {
                val parsed = Management.parse(args(operation, buildJsonObject { put("id", "local-1"); put("name", name) }))
                assertEquals(name, parsed.input!!.getValue("name").jsonPrimitive.content)
            }
        }
    }

    @Test fun diagnosticsSharingHasNoPathInputAndWaitIsCancelledWithoutReplay() = runTest {
        var shares = 0
        val bridge = bridge(Transport(), prepare = { DiagnosticShare.Prepared({ shares++ }, {}) })
        val result = bridge.management(args("diagnostics"))
        assertTrue(result.body["data"]!!.jsonObject["prepared"]!!.jsonPrimitive.boolean)
        assertEquals(0, shares)
        result.afterDelivery!!.deliver()
        assertEquals(1, shares)
        invalid(args("diagnostics", obj("""{"path":"/secret"}""")))
        val waiting = bridge(Transport(), prepare = { shares++; awaitCancellation() })
        val pending = async { runCatching { waiting.management(args("diagnostics")) } }
        runCurrent(); waiting.destroy(); runCurrent(); waiting.resume()
        assertTrue(pending.await().exceptionOrNull() is CancellationException)
        assertEquals(2, shares)
    }

    @Test fun probeSupportsRealHumanFeatureNamesAndDoesNotForwardProcessExceptions() = runTest {
        val transport = Transport()
        val bridge = bridge(transport)
        val report = obj("""{"platform":"android","kernel_release":"6.1.0+","architecture":"arm64","mode":"all","network":["tcp","udp"],"ipv6":true,"preflight":true,"exact_object_load":true,"findings":[{"status":"WARN","scope":"shared","importance":"required","feature":"shared interface","reason":"temporarily_unavailable","detail":"$PRIVATE"}],"summary":{"pass":8,"warn":1,"fail":0},"result":"preflight_passed"}""")
        for (raw in listOf(false, true)) {
            transport.status = 422
            transport.content = envelope(buildJsonObject { put("mode", "all"); put("raw", raw); put("content", PRIVATE); put("report", report) }, false)
            val result = bridge.management(args("ebpf", buildJsonObject { put("mode", "all"); put("raw", raw) }))
            assertEquals(422, result.status)
            assertFalse(result.body.toString().contains(PRIVATE))
            val data = result.body["data"]!!.jsonObject
            assertEquals("shared interface", data["report"]!!.jsonObject["findings"]!!.jsonArray[0].jsonObject["feature"]!!.jsonPrimitive.content)
            assertEquals("temporarily_unavailable", data["report"]!!.jsonObject["findings"]!!.jsonArray[0].jsonObject["reason"]!!.jsonPrimitive.content)
            if (raw) assertEquals(data["report"], Json.parseToJsonElement(data["content"]!!.jsonPrimitive.content))
        }
    }

    private fun TestScope.bridge(transport: ModuleApiTransport, prepare: (suspend () -> DiagnosticShare.Prepared)? = null) = ModuleApiBridge(transport, CoroutineScope(SupervisorJob() + StandardTestDispatcher(testScheduler)), workDispatcher = StandardTestDispatcher(testScheduler), prepareDiagnostics = prepare).also { bridges.add(it); it.resume() }
    private fun invalid(raw: String) = failure("api.invalid_request", runCatching { Management.parse(raw) })
    private fun failure(code: String, result: Result<*>) { assertEquals(code, (result.exceptionOrNull() as? BridgeFailure)?.code) }
    private class Transport : ModuleApiTransport {
        val paths = mutableListOf<String>()
        val queries = mutableListOf<Map<String,String>>()
        var content = envelope(JsonObject(emptyMap()))
        var status = 200
        var hold = false
        var image = byteArrayOf(0x89.toByte(), 0x50, 0x4e, 0x47, 13, 10, 26, 10)
        override suspend fun <T> request(method: String, path: String, body: JsonObject?, query: Map<String, String>, mutation: Boolean, decode: (Int, InputStream) -> T): T { paths.add(path); queries.add(query); if (hold) awaitCancellation(); return decode(status, content.byteInputStream()) }
        override suspend fun image(path: String): ByteArray { paths.add(path); if (hold) awaitCancellation(); return image }
        override suspend fun download(path: String, query: Map<String, String>, destination: File): Unit = error("Native share owns fixed destination")
        override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) = awaitCancellation()
        override suspend fun reconnect(address: String): Unit = error("No reconnect")
    }
    private data class Case(val operation: String, val input: JsonObject, val method: String, val path: String)
    companion object {
        private const val PRIVATE = "https://private.invalid/secret"
        private fun obj(raw: String) = Json.parseToJsonElement(raw).jsonObject
        private fun args(operation: String, input: JsonObject? = null) = buildJsonObject { put("requestId", "r"); put("operation", operation); if (input != null) put("input", input) }.toString()
        private fun envelope(data: JsonElement, ok: Boolean = true) = """{"schema":1,"ok":$ok,"code":"config.result","message":"$PRIVATE","data":$data}"""
        private fun cases() = listOf(
            Case("config_list", obj("{}"), "GET", "/configs"), Case("config_read", obj("""{"target":"module"}"""), "GET", "/config"),
            Case("config_apply", obj("""{"target":"module/wifi","revision":"${"a".repeat(64)}","content":"{}"}"""), "PUT", "/config"),
            Case("config_validate", obj("""{"target":"inbound/app","revision":"${"a".repeat(64)}","content":"{}"}"""), "POST", "/config/validate"),
            Case("apps", obj("{}"), "GET", "/android/apps"), Case("wifi", obj("{}"), "GET", "/network/wifi-list"),
            Case("logs", obj("""{"kind":"service","lines":200}"""), "GET", "/logs"), Case("logs_clear", obj("{}"), "DELETE", "/logs"), Case("ebpf", obj("{}"), "POST", "/diagnostics/ebpf"),
            Case("node_read", obj("""{"group_id":"default","node_tag":"node"}"""), "GET", "/nodes/get"), Case("node_export", obj("""{"group_id":"default","node_tag":"node"}"""), "GET", "/nodes/export"), Case("node_remove", obj("""{"group_id":"default","node_tag":"node"}"""), "DELETE", "/nodes"),
            Case("node_add", obj("""{"content":"{}"}"""), "POST", "/nodes"), Case("node_edit", obj("""{"group_id":"default","node_tag":"node","content":"{}"}"""), "PUT", "/nodes"), Case("node_import", obj("""{"content":"{}"}"""), "POST", "/nodes/import"),
            Case("catalog_create", obj("""{"id":"local-1","name":"Local"}"""), "POST", "/catalog"),
            Case("catalog_rename", obj("""{"id":"local-1","name":"Renamed"}"""), "PUT", "/catalog/local-1"),
            Case("catalog_remove", obj("""{"id":"local-1"}"""), "DELETE", "/catalog/local-1"))
    }
}
