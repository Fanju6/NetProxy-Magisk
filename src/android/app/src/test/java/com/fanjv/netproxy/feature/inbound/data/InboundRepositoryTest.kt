package com.fanjv.netproxy.feature.inbound.data

import com.fanjv.netproxy.core.command.*
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

    private fun output(data: String) = NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"test","message":"","data":$data}"""), emptyList())
    private fun snapshot(content: String, revision: String) = output(JsonObject(mapOf(
        "content" to JsonPrimitive(content), "revision" to JsonPrimitive(revision)
    )).toString())
    private fun status(state: String, configured: String, active: String?) =
        output("""{"state":"$state","configured_backend":"$configured","active_backend":${active?.let { "\"$it\"" } ?: "null"}}""")
    private fun repository(transport: NetProxyCtlTransport): InboundRepository {
        val client = NetProxyCtlClient(transport = transport)
        return InboundRepository(ConfigRepository(client, CommandFileStore(folder.root)), ServiceRepository(client))
    }

    @Test fun loadUsesOnlyNewPartitionsAndDistinctRevisions() = runBlocking {
        val calls = mutableListOf<List<String>>()
        val repo = repository { args, _ ->
            calls += args
            if (args.first() == "service") status("stopped", "ebpf", null)
            else {
                val key = args.last().substringAfter('/')
                snapshot(if (key == "backend") """{"backend":"ebpf"}""" else """{"$key":{"type":"$key","tag":"netproxy-in"}}""", "$key-revision")
            }
        }
        val loaded = repo.load()
        assertEquals("ebpf", loaded.backend)
        assertNull(loaded.status!!.activeBackend)
        assertEquals("tun-revision", loaded.partitions.getValue("tun").revision)
        assertEquals(listOf(
            listOf("config", "read", "inbound/backend"), listOf("config", "read", "inbound/ebpf"),
            listOf("config", "read", "inbound/tun"), listOf("service", "status")
        ), calls)
    }

    @Test fun choicesExpandEveryListableRuleSetTagAndKeepTokensIntact() = runBlocking {
        val calls = mutableListOf<List<String>>()
        val repo = repository { args, _ ->
            calls += args
            snapshot("""{"route":{"rule_set":[
                {"tag":"private"},
                {"tag":["corp,lan","two","private"]},
                {"tag":["three"," full width，tag "]}
            ]}}""", "main-revision")
        }
        assertEquals(listOf("private", "corp,lan", "two", "three", " full width，tag "), repo.choices().ruleSets)
        assertEquals(listOf(listOf("config", "read", "singbox/config.json")), calls)
    }

    @Test fun runningSwitchRequiresConfirmationBeforeAnyWrite() = runBlocking {
        var writes = 0
        val repo = repository { args, _ ->
            if (args.first() == "service") status("ready", "ebpf", "ebpf")
            else { writes++; output("""{"revision":"saved"}""") }
        }
        try {
            repo.apply("inbound/backend", """{"backend":"tun"}""", "backend-revision")
            fail("运行中切换必须确认")
        } catch (_: InboundSwitchConfirmationRequired) {}
        assertEquals(0, writes)
    }

    @Test fun confirmedSwitchUsesPartitionRevisionAndLongTimeout() = runBlocking {
        var applied = false
        val repo = repository { args, timeout ->
            if (args.first() == "service") {
                if (applied) status("ready", "tun", "tun") else status("ready", "ebpf", "ebpf")
            } else {
                assertEquals(listOf("config", "apply", "--revision", "backend-revision", "inbound/backend"), args.dropLast(1))
                assertEquals("""{"backend":"tun"}""", File(args.last()).readText())
                assertEquals(120_000L, timeout)
                applied = true
                output("""{"revision":"new-revision"}""")
            }
        }
        assertEquals("new-revision", repo.apply("inbound/backend", """{"backend":"tun"}""", "backend-revision", true))
        assertTrue(folder.root.listFiles()!!.isEmpty())
    }

    @Test fun stoppedSwitchDoesNotStartServiceOrWorker() = runBlocking {
        var applied = false
        val calls = mutableListOf<List<String>>()
        val repo = repository { args, _ ->
            calls += args
            if (args.first() == "service") status("stopped", if (applied) "tun" else "ebpf", null)
            else { applied = true; output("""{"revision":"saved"}""") }
        }
        assertEquals("saved", repo.apply("inbound/backend", """{"backend":"tun"}""", "read"))
        assertTrue(calls.filter { it.first() == "service" }.all { it == listOf("service", "status") })
    }

    @Test fun unconfirmedActiveBackendNeverReturnsSaveSuccess() = runBlocking {
        for (active in listOf(null, "ebpf")) {
            var applied = false
            val repo = repository { args, _ ->
                if (args.first() == "service") {
                    if (applied) status("ready", "tun", active) else status("ready", "ebpf", "ebpf")
                } else { applied = true; output("""{"revision":"saved"}""") }
            }
            try {
                repo.apply("inbound/backend", """{"backend":"tun"}""", "read", true)
                fail("未确认实际后端不能显示成功")
            } catch (error: NetProxyCtlException) {
                assertEquals("inbound.not_confirmed", error.resultCode)
            }
        }
    }

    @Test fun unselectedPartitionCanSaveWithoutChangingActiveBackend() = runBlocking {
        val repo = repository { args, _ ->
            if (args.first() == "service") status("ready", "ebpf", "ebpf")
            else {
                assertEquals("inbound/tun", args[4])
                output("""{"revision":"tun-saved"}""")
            }
        }
        assertEquals("tun-saved", repo.apply("inbound/tun", """{"tun":{"type":"tun","tag":"netproxy-in"}}""", "tun-read"))
    }

    @Test fun conflictsAndNativeFailuresAreNotRetried() = runBlocking {
        for (code in listOf("config.conflict", "tun.start_failed", "inbound.restart_required")) {
            var writes = 0
            val repo = repository { args, _ ->
                if (args.first() == "service") status("ready", "ebpf", "ebpf")
                else {
                    writes++
                    NetProxyCtlOutput(false, listOf("""{"schema":1,"ok":false,"code":"$code","message":"failed"}"""), emptyList())
                }
            }
            try {
                repo.apply("inbound/backend", """{"backend":"tun"}""", "stale", true)
                fail("应保留错误")
            } catch (error: NetProxyCtlException) { assertEquals(code, error.resultCode) }
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
            val repo = repository { args, _ ->
                if (args.first() == "service") status("ready", "tun", active)
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
        val repo = repository { _, _ -> status("ready", "tun", "ebpf") }
        assertFalse(repo.requiresSwitchConfirmation("inbound/backend", """{"backend":"ebpf"}"""))
    }

    @Test fun stoppedInactivePartitionReadsStatusWithoutInventingRunningState() = runBlocking {
        val calls = mutableListOf<List<String>>()
        val repo = repository { args, _ ->
            calls += args
            if (args.first() == "service") status("stopped", "ebpf", null)
            else {
                assertEquals(listOf("config", "apply", "--revision", "tun-read", "inbound/tun"), args.dropLast(1))
                output("""{"revision":"tun-saved"}""")
            }
        }
        assertEquals("tun-saved", repo.apply("inbound/tun", """{"tun":{"type":"tun","tag":"netproxy-in"}}""", "tun-read"))
        assertEquals(3, calls.size)
        assertEquals(listOf("service", "status"), calls.first())
        assertEquals(listOf("service", "status"), calls.last())
        assertFalse(ServiceStatusSnapshot(state = "stopped", configuredBackend = "ebpf", activeBackend = null).mayBeRunning())
    }
}
