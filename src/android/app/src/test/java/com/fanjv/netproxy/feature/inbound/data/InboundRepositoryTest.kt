package com.fanjv.netproxy.feature.inbound.data

import com.fanjv.netproxy.core.api.*

import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.core.module.ServiceStatusSnapshot
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class InboundRepositoryTest {
    @get:Rule val folder = TemporaryFolder()

    private fun output(data: String) = ModuleHttpResponse(200, """{"schema":1,"ok":true,"code":"test","message":"","data":$data}""")
    private fun snapshot(content: String, revision: String) = output(JsonObject(mapOf(
        "content" to JsonPrimitive(content), "revision" to JsonPrimitive(revision)
    )).toString())
    private fun status(state: String, configured: String, active: String?) =
        output("""{"state":"$state","configured_backend":"$configured","active_backend":${active?.let { "\"$it\"" } ?: "null"}}""")
    private fun repository(handler: suspend (ApiTestRequest) -> ModuleHttpResponse): InboundRepository {
        val client = ModuleApiClient(transport = TestApiTransport(handler))
        return InboundRepository(ConfigRepository(client), ServiceRepository(client))
    }

    @Test fun loadUsesOnlyNewPartitionsAndDistinctRevisions() = runBlocking {
        val calls = mutableListOf<ApiTestRequest>()
        val repo = repository { args ->
            calls += args
            if (args.path == "/service") status("stopped", "ebpf", null)
            else {
                val key = args.target.substringAfter('/')
                snapshot(when (key) {
                    "backend" -> """{"backend":"ebpf"}"""
                    "root_policy" -> """{"root_policy":"default"}"""
                    else -> """{"$key":{"type":"$key","tag":"netproxy-in"}}"""
                }, "$key-revision")
            }
        }
        val loaded = repo.load()
        assertEquals("ebpf", loaded.backend)
        assertEquals("default", loaded.rootPolicy)
        assertEquals(setOf("ebpf", "tun"), loaded.native.keys)
        assertNull(loaded.status!!.activeBackend)
        assertEquals("tun-revision", loaded.partitions.getValue("tun").revision)
        assertEquals(listOf("inbound/backend", "inbound/root_policy", "inbound/ebpf", "inbound/tun"),
            calls.filter { it.path == "/config" }.map { it.target })
        assertEquals("/service", calls.last().path)
    }

    @Test fun choicesExpandEveryListableRuleSetTagAndKeepTokensIntact() = runBlocking {
        val calls = mutableListOf<ApiTestRequest>()
        val repo = repository { args ->
            calls += args
            snapshot("""{"route":{"rule_set":[
                {"tag":"private"},
                {"tag":["corp,lan","two","private"]},
                {"tag":["three"," full width，tag "]}
            ]}}""", "main-revision")
        }
        assertEquals(listOf("private", "corp,lan", "two", "three", " full width，tag "), repo.choices().ruleSets)
        assertEquals(listOf("singbox/config.json"), calls.map { it.target })
    }

    @Test fun runningSwitchRequiresConfirmationBeforeAnyWrite() = runBlocking {
        var writes = 0
        val repo = repository { args ->
            if (args.path == "/service") status("ready", "ebpf", "ebpf")
            else { writes++; output("""{"revision":"saved"}""") }
        }
        try {
            repo.apply("inbound/backend", """{"backend":"tun"}""", "backend-revision")
            fail("运行中切换必须确认")
        } catch (_: InboundSwitchConfirmationRequired) {}
        assertEquals(0, writes)
    }

    @Test fun confirmedSwitchUsesPartitionRevisionAndWriteTransaction() = runBlocking {
        var applied = false
        val repo = repository { args ->
            if (args.path == "/service") {
                if (applied) status("ready", "tun", "tun") else status("ready", "ebpf", "ebpf")
            } else {
                args.assertConfig("PUT", "inbound/backend", "backend-revision")
                assertEquals("""{"backend":"tun"}""", args.text("content"))
                assertTrue(args.mutation)
                applied = true
                output("""{"revision":"new-revision"}""")
            }
        }
        assertEquals("new-revision", repo.apply("inbound/backend", """{"backend":"tun"}""", "backend-revision", true).revision)
        assertTrue(folder.root.listFiles()!!.isEmpty())
    }

    @Test fun stoppedSwitchDoesNotStartServiceOrWorker() = runBlocking {
        var applied = false
        val calls = mutableListOf<ApiTestRequest>()
        val repo = repository { args ->
            calls += args
            if (args.path == "/service") status("stopped", if (applied) "tun" else "ebpf", null)
            else { applied = true; output("""{"revision":"saved"}""") }
        }
        assertEquals("saved", repo.apply("inbound/backend", """{"backend":"tun"}""", "read").revision)
        assertTrue(calls.filter { it.path.startsWith("/service") }.all { it.path == "/service" })
    }

    @Test fun unconfirmedActiveBackendNeverReturnsSaveSuccess() = runBlocking {
        for (active in listOf(null, "ebpf")) {
            var applied = false
            val repo = repository { args ->
                if (args.path == "/service") {
                    if (applied) status("ready", "tun", active) else status("ready", "ebpf", "ebpf")
                } else { applied = true; output("""{"revision":"saved"}""") }
            }
            try {
                repo.apply("inbound/backend", """{"backend":"tun"}""", "read", true)
                fail("未确认实际后端不能显示成功")
            } catch (error: ModuleApiException) {
                assertEquals("inbound.not_confirmed", error.resultCode)
            }
        }
    }

    @Test fun unselectedPartitionCanSaveWithoutChangingActiveBackend() = runBlocking {
        val repo = repository { args ->
            if (args.path == "/service") status("ready", "ebpf", "ebpf")
            else {
                assertEquals("inbound/tun", args.target)
                output("""{"revision":"tun-saved"}""")
            }
        }
        assertEquals("tun-saved", repo.apply("inbound/tun", """{"tun":{"type":"tun","tag":"netproxy-in"}}""", "tun-read").revision)
    }

    @Test fun conflictsAndNativeFailuresAreNotRetried() = runBlocking {
        for (code in listOf("config.conflict", "tun.start_failed", "inbound.restart_required")) {
            var writes = 0
            val repo = repository { args ->
                if (args.path == "/service") status("ready", "ebpf", "ebpf")
                else {
                    writes++
                    ModuleHttpResponse(500, """{"schema":1,"ok":false,"code":"$code","message":"failed"}""")
                }
            }
            try {
                repo.apply("inbound/backend", """{"backend":"tun"}""", "stale", true)
                fail("应保留错误")
            } catch (error: ModuleApiException) { assertEquals(code, error.resultCode) }
            assertEquals(1, writes)
            assertTrue(folder.root.listFiles()!!.isEmpty())
        }
    }

    @Test fun configuredAndActiveBackendNeverFallBackToEachOther() {
        val json = kotlinx.serialization.json.Json { ignoreUnknownKeys = true }
        val value = json.decodeFromString<ServiceStatusSnapshot>("""{"state":"ready","configured_backend":"tun","active_backend":null}""")
        assertEquals("tun", value.configuredBackend)
        assertNull(value.activeBackend)
        val missing = json.decodeFromString<ServiceStatusSnapshot>("""{"state":"ready"}""")
        assertEquals("", missing.configuredBackend)
        assertNull(missing.activeBackend)
    }

    @Test fun manuallyChangedConfiguredBackendStillRequiresConfirmationAgainstActual() = runBlocking {
        for (active in listOf("ebpf", null)) {
            var writes = 0
            val repo = repository { args ->
                if (args.path == "/service") status("ready", "tun", active)
                else { writes++; output("""{"revision":"saved"}""") }
            }
            assertTrue(repo.requiresSwitchConfirmation("inbound/backend", """{"backend":"tun"}"""))
            assertTrue(repo.requiresSwitchConfirmation("inbound", """{"backend":"tun"}"""))
            try {
                repo.apply("inbound/backend", """{"backend":"tun"}""", "read")
                fail("必须以实际后端判断是否切换")
            } catch (_: InboundSwitchConfirmationRequired) {}
            assertEquals(0, writes)
        }
        val repo = repository { _ -> status("ready", "tun", "ebpf") }
        assertFalse(repo.requiresSwitchConfirmation("inbound/backend", """{"backend":"ebpf"}"""))
    }

    @Test fun stoppedInactivePartitionReadsStatusWithoutInventingRunningState() = runBlocking {
        val calls = mutableListOf<ApiTestRequest>()
        val repo = repository { args ->
            calls += args
            if (args.path == "/service") status("stopped", "ebpf", null)
            else {
                args.assertConfig("PUT", "inbound/tun", "tun-read")
                output("""{"revision":"tun-saved"}""")
            }
        }
        assertEquals("tun-saved", repo.apply("inbound/tun", """{"tun":{"type":"tun","tag":"netproxy-in"}}""", "tun-read").revision)
        assertEquals(3, calls.size)
        assertEquals("/service", calls.first().path)
        assertEquals("/service", calls.last().path)
        assertFalse(ServiceStatusSnapshot(state = "stopped", configuredBackend = "ebpf", activeBackend = null).mayBeRunning())
    }
}
