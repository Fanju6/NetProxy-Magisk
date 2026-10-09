package com.fanjv.netproxy.feature.settings.data

import com.fanjv.netproxy.core.command.CommandFileStore
import com.fanjv.netproxy.core.command.ConfigurationWrites
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.command.NetProxyCtlOutput
import com.fanjv.netproxy.core.command.NetProxyCtlTransport
import com.fanjv.netproxy.feature.settings.model.WifiPolicySettings
import com.fanjv.netproxy.feature.settings.presentation.SettingsViewModel
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.yield
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class ConfigRepositoryTest {
    @get:Rule val temporaryFolder = TemporaryFolder()

    private val defaultWifi = """{"wifi":{"enabled":false,"mode":"blacklist","blacklist":[],"whitelist":[],"proxy_on_non_wifi":true}}"""

    private fun output(data: String) = NetProxyCtlOutput(true,
        listOf("""{"schema":1,"ok":true,"code":"test","message":"","data":$data}"""), emptyList())

    private fun snapshot(content: String, revision: String) = output(JsonObject(mapOf(
        "content" to JsonPrimitive(content), "revision" to JsonPrimitive(revision)
    )).toString())

    private suspend fun assertModuleReadRejected(target: String, content: String) {
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            assertEquals(listOf("config", "read", target), args)
            snapshot(content, "read")
        })
        val repository = ConfigRepository(client, CommandFileStore(temporaryFolder.root))
        try {
            if (target == "module/wifi") repository.readWifi() else repository.readAutoStart()
            fail("应拒绝无效配置: $content")
        } catch (_: IllegalArgumentException) { }
    }

    @Test fun savedWifiNamesUseReadonlyNetworkContract() = runBlocking {
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            assertEquals(listOf("network", "wifi-list"), args)
            output("""{"ssids":["Home, Wi-Fi","Office"]}""")
        })
        assertEquals(listOf("Home, Wi-Fi", "Office"), ConfigRepository(client, CommandFileStore(temporaryFolder.root)).savedWifiNetworks())
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }

    @Test fun modulePartitionsDecodeTypedValuesAndIndependentRevisions() = runBlocking {
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            when (args) {
                listOf("config", "read", "module/wifi") -> snapshot(defaultWifi, "wifi-read")
                listOf("config", "read", "module/auto_start") -> snapshot("""{"auto_start":false}""", "auto-read")
                else -> error("非分区读取: $args")
            }
        })
        val repository = ConfigRepository(client, CommandFileStore(temporaryFolder.root))
        val wifi = repository.readWifi()
        assertEquals(WifiPolicySettings(), wifi.wifi)
        assertEquals("wifi-read", wifi.revision)
        val auto = repository.readAutoStart()
        assertFalse(auto.enabled)
        assertEquals("auto-read", auto.revision)
    }

    @Test fun modulePartitionWritesContainOnlyOwnedFieldsIncludingDefaults() = runBlocking {
        val writes = ConfigurationWrites(this)
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            assertEquals(listOf("config", "apply", "--revision"), args.take(3))
            val target = args[4]
            assertTrue(target in setOf("module/wifi", "module/auto_start"))
            assertEquals(if (target == "module/wifi") "wifi-read" else "auto-read", args[3])
            assertEquals(Json.parseToJsonElement(if (target == "module/wifi") defaultWifi else """{"auto_start":false}"""),
                Json.parseToJsonElement(File(args.last()).readText()))
            assertTrue(args.last().endsWith(".json"))
            output("""{"revision":"$target-saved"}""")
        })
        val repository = ConfigRepository(client, CommandFileStore(temporaryFolder.root), writes::await)
        withTimeout(5_000) {
            writes.launch("module/wifi", write = {
                assertEquals("module/wifi-saved", repository.applyWifi(WifiPolicySettings(), "wifi-read"))
            }).join()
            writes.launch("module/auto_start", write = {
                assertEquals("module/auto_start-saved", repository.applyAutoStart(false, "auto-read"))
            }).join()
        }
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }

    @Test fun omittedModuleFieldsUseConstructorDefaultsWithoutWriting() = runBlocking {
        val wifiCases = listOf(
            "{}" to WifiPolicySettings(),
            """{"wifi":{}}""" to WifiPolicySettings(),
            """{"wifi":{"enabled":true}}""" to WifiPolicySettings(enabled = true),
            """{"wifi":{"mode":"whitelist","whitelist":[" Home,Wi-Fi "]}}""" to
                WifiPolicySettings(mode = "whitelist", whitelist = listOf(" Home,Wi-Fi ")),
            """{"wifi":{"proxy_on_non_wifi":false}}""" to WifiPolicySettings(proxyOnNonWifi = false)
        )
        for ((content, expected) in wifiCases) {
            val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
                assertEquals(listOf("config", "read", "module/wifi"), args)
                snapshot(content, "sparse-wifi")
            })
            val read = ConfigRepository(client, CommandFileStore(temporaryFolder.root)).readWifi()
            assertEquals(expected, read.wifi)
            assertEquals("sparse-wifi", read.revision)
        }
        for ((content, expected) in listOf("{}" to false, """{"auto_start":false}""" to false,
            """{"auto_start":true}""" to true)) {
            val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
                assertEquals(listOf("config", "read", "module/auto_start"), args)
                snapshot(content, "sparse-auto")
            })
            val read = ConfigRepository(client, CommandFileStore(temporaryFolder.root)).readAutoStart()
            assertEquals(expected, read.enabled)
            assertEquals("sparse-auto", read.revision)
        }
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }

    @Test fun nullUnknownAndMalformedModuleJsonAreRejected() = runBlocking {
        val invalidWifi = listOf("""{"wifi":null}""", """{"wifi":{"unknown":true}}""", """{"unknown":true}""",
            """{"wifi":{"enabled":null}}""", """{"wifi":{"mode":null}}""",
            """{"wifi":{"blacklist":null}}""", """{"wifi":{"whitelist":null}}""",
            """{"wifi":{"proxy_on_non_wifi":null}}""",
            defaultWifi.replace("\"enabled\":false", "\"enabled\":0"),
            defaultWifi.replace("\"mode\":\"blacklist\"", "\"mode\":\"off\""),
            defaultWifi.replace("\"blacklist\":[]", "\"blacklist\":[1]"))
        for ((target, invalid) in listOf("module/wifi" to invalidWifi,
            "module/auto_start" to listOf("""{"auto_start":null}""", """{"auto_start":1}""",
                """{"unknown":true}"""))) {
            for (content in invalid) assertModuleReadRejected(target, content)
        }
    }

    @Test fun quotedModuleBooleansAreRejected() = runBlocking {
        for (value in listOf("true", "false")) {
            assertModuleReadRejected("module/auto_start", """{"auto_start":"$value"}""")
            for (field in listOf("enabled", "proxy_on_non_wifi")) {
                assertModuleReadRejected("module/wifi", """{"wifi":{"$field":"$value"}}""")
            }
        }
    }

    @Test fun numericWifiListElementsAreRejected() = runBlocking {
        for (field in listOf("blacklist", "whitelist")) {
            for (values in listOf("[1]", "[\"Home\",2]", "[1.5]")) {
                assertModuleReadRejected("module/wifi", """{"wifi":{"$field":$values}}""")
            }
        }
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { _, _ ->
            snapshot("""{"wifi":{"blacklist":["1"],"whitelist":["1.5"]}}""", "read")
        })
        val wifi = ConfigRepository(client, CommandFileStore(temporaryFolder.root)).readWifi().wifi
        assertEquals(listOf("1"), wifi.blacklist)
        assertEquals(listOf("1.5"), wifi.whitelist)
    }

    @Test fun strictModuleCodecDoesNotChangeCliResponseUnknownFieldPolicy() = runBlocking {
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            assertEquals(listOf("config", "read", "module/auto_start"), args)
            output("""{"content":"{}","revision":"auto-read","extra":true}""")
        })
        assertTrue(client.json.configuration.ignoreUnknownKeys)
        val read = ConfigRepository(client, CommandFileStore(temporaryFolder.root)).readAutoStart()
        assertFalse(read.enabled)
        assertEquals("auto-read", read.revision)
        assertTrue(client.json.configuration.ignoreUnknownKeys)
    }

    @Test fun reopeningWaitsForDetachedCommitAndReadsItsNewRevision() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val saveScope = CoroutineScope(coroutineContext + Job())
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var content = defaultWifi
        var revision = 0
        var reads = 0
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            if (args[1] == "read") {
                reads++
                if (args[2] == "module/wifi") snapshot(content, "$revision")
                else snapshot("""{"auto_start":false}""", "auto")
            } else {
                entered.complete(Unit); withTimeout(5_000) { release.await() }
                assertEquals(listOf("config", "apply", "--revision", "$revision", "module/wifi"), args.dropLast(1))
                content = File(args.last()).readText()
                revision++
                output("""{"revision":"$revision"}""")
            }
        })
        val writes = ConfigurationWrites(saveScope)
        val repository = ConfigRepository(client, CommandFileStore(temporaryFolder.root), writes::await)
        val old = SettingsViewModel(repository, pageScope, writes)
        try {
            old.refresh()
            withTimeout(5_000) { old.state.first { it.hasLoaded } }
            old.setWifiSsidMode("blacklist"); old.requestWifiFlush()
            pageScope.cancel()
            withTimeout(5_000) { entered.await() }
            val reopened = SettingsViewModel(repository, this, writes)
            reopened.refresh(); yield()
            assertTrue(reopened.state.value.isLoading)
            assertEquals(2, reads)
            release.complete(Unit)
            withTimeout(5_000) { reopened.state.first { it.hasLoaded } }
            assertTrue(reopened.state.value.wifi.enabled)
            assertEquals(4, reads)
            assertEquals(1, revision)
            assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
        } finally { release.complete(Unit); pageScope.cancel(); saveScope.cancel() }
    }

    @Test fun unrelatedConfigurationReadDoesNotWaitForModuleWrite() = runBlocking {
        val release = CompletableDeferred<Unit>()
        val writes = ConfigurationWrites(this)
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            assertEquals(listOf("config", "read", "singbox/config.json"), args)
            snapshot("{}", "main")
        })
        val repository = ConfigRepository(client, CommandFileStore(temporaryFolder.root), writes::await)
        val writer = writes.launch("module/wifi", write = { release.await() })
        try {
            val snapshot = withTimeout(5_000) { repository.readSnapshot("singbox/config.json") }
            assertEquals("main", snapshot.revision)
            assertTrue(writer.isActive)
        } finally { release.complete(Unit); writer.join() }
    }

    @Test fun savesReadPartitionRevisionAndNeverRereadsOrRetriesConflict() = runBlocking {
        val calls = mutableListOf<List<String>>()
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            calls.add(args)
            if (args[1] == "read") {
                assertEquals(listOf("config", "read", "module/auto_start"), args)
                snapshot("""{"auto_start":false}""", "auto-read")
            } else {
                assertEquals(listOf("config", "apply", "--revision", "auto-read", "module/auto_start"), args.dropLast(1))
                assertEquals("""{"auto_start":true}""", File(args.last()).readText())
                NetProxyCtlOutput(false, listOf("""{"schema":1,"ok":false,"code":"config.conflict","message":"配置已被修改，请重新加载后再保存"}"""), emptyList())
            }
        })
        val repository = ConfigRepository(client, CommandFileStore(temporaryFolder.root))
        val read = repository.readAutoStart()
        try {
            repository.applyAutoStart(true, read.revision)
            fail("应返回配置冲突")
        } catch (error: NetProxyCtlException) {
            assertEquals("config.conflict", error.resultCode)
            assertTrue(error.message.contains("重新加载"))
        }
        assertEquals(2, calls.size)
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }

    @Test fun fullModuleJsonRemainsAvailableWithoutAndroidMerging() = runBlocking {
        val content = JsonObject(Json.parseToJsonElement(defaultWifi).jsonObject + mapOf(
            "auto_start" to JsonPrimitive(false),
            "selection" to JsonObject(mapOf("group_id" to JsonPrimitive("default"), "node_tag" to JsonPrimitive("")))
        )).toString()
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            if (args[1] == "read") {
                assertEquals(listOf("config", "read", "module"), args)
                snapshot(content, "full-read")
            } else {
                assertEquals(listOf("config", "apply", "--revision", "full-read", "module"), args.dropLast(1))
                assertEquals(content, File(args.last()).readText())
                output("""{"revision":"full-saved"}""")
            }
        })
        val repository = ConfigRepository(client, CommandFileStore(temporaryFolder.root))
        val read = repository.readSnapshot("module")
        assertEquals("full-saved", repository.apply("module", read.content, read.revision))
    }

    @Test fun nativeInboundValidationCarriesRevisionAndCleansCandidate() = runBlocking {
        val content = """{"tun":{"type":"tun","tag":"netproxy-in"}}"""
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            assertEquals(listOf("config", "validate", "--revision", "tun-revision", "inbound/tun"), args.dropLast(1))
            assertEquals(content, File(args.last()).readText())
            output("{}")
        })
        ConfigRepository(client, CommandFileStore(temporaryFolder.root)).validate("inbound/tun", content, "tun-revision")
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }
}
